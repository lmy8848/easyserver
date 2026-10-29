package mobiledirect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"easyserver/internal/infra/config"
	"easyserver/internal/infra/logger"
)

// MintFunc 为通过签名的设备铸造短期会话令牌（由 api.Setup 注入）。
type MintFunc func(ctx context.Context, userID int64) (string, time.Time, error)

// Manager 持有直连通道的生命周期，支持面板运行时修改配置后热重启监听。
type Manager struct {
	store    *Store
	mint     MintFunc
	upstream string

	mu      sync.Mutex
	srv     *Server
	lastErr string
}

func NewManager(store *Store, upstream string, mint MintFunc) *Manager {
	return &Manager{store: store, upstream: upstream, mint: mint}
}

// Apply 按配置启动/重启/停止直连监听（配置变更后调用）。
func (m *Manager) Apply(cfg config.MobileDirectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		m.srv.Shutdown(ctx)
		cancel()
		m.srv = nil
	}
	m.lastErr = ""
	if !cfg.Enabled {
		return nil
	}
	if err := EnsureCert(&cfg); err != nil {
		m.lastErr = err.Error()
		return err
	}
	srv, err := NewServer(cfg, m.store, m.upstream, m.mint)
	if err != nil {
		m.lastErr = err.Error()
		return err
	}
	if err := srv.Start(); err != nil {
		m.lastErr = err.Error()
		return err
	}
	m.srv = srv
	logger.Info("mobile-direct", "started", "addr", cfg.Listen)
	return nil
}

// RegenerateCert 删除现有证书并重新生成自签证书，返回新指纹。
func (m *Manager) RegenerateCert(cfg config.MobileDirectConfig) (string, error) {
	_ = os.Remove(cfg.CertFile)
	_ = os.Remove(cfg.KeyFile)
	if err := EnsureCert(&cfg); err != nil {
		return "", err
	}
	return CertFingerprint(cfg.CertFile), nil
}

// Stop 关闭监听（进程退出时调用）。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		m.srv.Shutdown(ctx)
		cancel()
		m.srv = nil
	}
}

// Status 返回运行状态、证书指纹与最近一次启动错误。
func (m *Manager) Status() (running bool, fingerprint, lastErr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.srv != nil, certFingerprint(m.srvCertPath()), m.lastErr
}

func (m *Manager) srvCertPath() string {
	if m.srv == nil {
		return ""
	}
	return m.srv.cfg.CertFile
}

// EnsureCert 在证书文件缺失时自动生成自签证书（IP 直连场景不依赖域名，
// 移动端通过证书指纹 pinning 校验；面板设置页展示指纹供核对）。
func EnsureCert(cfg *config.MobileDirectConfig) error {
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return errors.New("cert_file/key_file 未配置")
	}
	if fileExists(cfg.CertFile) && fileExists(cfg.KeyFile) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CertFile), 0o700); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}
	hosts := []string{}
	if cfg.Hostname != "" {
		hosts = append(hosts, cfg.Hostname)
	}
	hosts = append(hosts, "easyserver-direct")

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hosts[0], Organization: []string{"EasyServer Mobile Direct"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              hosts,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(cfg.CertFile, certPEM, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.KeyFile, keyPEM, 0o600); err != nil {
		return err
	}
	logger.Info("mobile-direct", "self-signed certificate generated", "cert", cfg.CertFile)
	return nil
}

// CertFingerprint 返回证书 DER 的 SHA-256 指纹（冒号分隔的十六进制），
// 移动端首连时展示给用户核对并固化（TOFU pinning）。
func CertFingerprint(path string) string { return certFingerprint(path) }

func certFingerprint(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	return formatFingerprint(sum[:])
}

func formatFingerprint(b []byte) string {
	out := make([]byte, 0, len(b)*3)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex.EncodeToString([]byte{c})...)
	}
	return string(out)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
