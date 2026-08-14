package mise

import (
	"context"
	"crypto/sha256"
	"easyserver/internal/infra/config"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	targetMiseVersion   = "v2026.6.14"
	fallbackMiseVersion = "v2026.6.13"
	targetMiseSha256    = "96ae1ef7b00a6ebbbec23ba1016d6e722f5e904966272f621d15326429e90d53"
	fallbackMiseSha256  = "d80fa6b4be4d926e7a09fadab1ba777c9ce52f9c22de6d8f6e3cce8d66d281d8"
)

const DataDir = config.DataRoot + "/mise"

// BootstrapMise ensures mise is installed and configured correctly
func BootstrapMise() error {
	// 下载前确保二进制所在目录存在（DataDir 根，二进制即 <DataDir>/mise）。
	// downloadMise 的临时文件放这里做同文件系统 rename，目录缺失会导致
	// CreateTemp 报 "no such file or directory"。
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return err
	}

	// 1. Check if already installed and version matches
	if err := checkMiseVersion(); err == nil {
		return setupMiseEnv()
	}

	// 2. Download binary directly
	if err := downloadMise(targetMiseVersion, targetMiseSha256); err != nil {
		log.Printf("Failed to download target version %s: %v, falling back to %s", targetMiseVersion, err, fallbackMiseVersion)
		if errFallback := downloadMise(fallbackMiseVersion, fallbackMiseSha256); errFallback != nil {
			return fmt.Errorf("failed to download both target and fallback: %w", errFallback)
		}
	}

	// 3. Setup environment
	return setupMiseEnv()
}

func checkMiseVersion() error {
	cmd := exec.CommandContext(context.Background(), filepath.Join(DataDir, "mise"), "--version")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	verStr := strings.TrimSpace(string(out))

	// mise --version output evolved over releases. Modern builds emit
	// "2026.6.13 linux-x64 (...)" with no leading "mise " — the old
	// " <ver> " space-padded substring check missed those because the
	// version sits at byte 0. Tokenize and compare exact fields instead.
	//
	// If the installed mise isn't on our pinned list we still accept it —
	// the user put it there; redownloading every boot is hostile. Pinning
	// only exists to keep the auto-download SHA verifiable.
	target := strings.TrimPrefix(targetMiseVersion, "v")
	fallback := strings.TrimPrefix(fallbackMiseVersion, "v")
	for f := range strings.FieldsSeq(verStr) {
		if f == target || f == fallback {
			return nil
		}
	}
	log.Printf("mise: existing binary at %s reports %q (expected %s or %s); using it as-is",
		filepath.Join(DataDir, "mise"), verStr, target, fallback)
	return nil
}

func downloadMise(version, expectedSha256 string) error {
	urls := []string{
		fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64", version, version),
		fmt.Sprintf("https://mirror.ghproxy.com/https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64", version, version),
	}

	var lastErr error
	for _, dlUrl := range urls {
		log.Printf("Downloading mise from %s", dlUrl)

		// Create tmp file in the SAME directory as the final target so the
		// atomic rename below stays within one filesystem. Defaulting to
		// /tmp blows up with "invalid cross-device link" on hosts where
		// /tmp is tmpfs and /opt is on the root fs.
		tmpFile, err := os.CreateTemp(DataDir, "mise-download-*.tmp")
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		defer os.Remove(tmpPath)

		err = downloadFile(tmpPath, dlUrl, expectedSha256)
		if err == nil {
			if err := os.Chmod(tmpPath, 0755); err != nil {
				return err
			}
			return os.Rename(tmpPath, filepath.Join(DataDir, "mise"))
		}
		log.Printf("Download failed from %s: %v", dlUrl, err)
		lastErr = err
	}
	return lastErr
}

func downloadFile(filepath string, url string, expectedSha256 string) error {
	// 总超时 3 分钟：~100MB 的 mise 二进制在慢速链路下 body 读取可能远超
	// 拨号时间，只约束连接建立会误杀下载；3 分钟对正常下载绰绰有余。
	client := &http.Client{Timeout: 3 * time.Minute}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	hasher := sha256.New()
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := io.MultiWriter(out, hasher)
	if _, err = io.Copy(writer, resp.Body); err != nil {
		return err
	}

	if hex.EncodeToString(hasher.Sum(nil)) != expectedSha256 {
		return errors.New("checksum mismatch")
	}
	return nil
}

func setupMiseEnv() error {
	// 数据目录与 config 目录同根（<DataDir>，全自包含在面板根 mise/ 子目录）。
	// 二进制 <DataDir>/mise 直接放根下，无需额外 bin/ 子目录。
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return err
	}

	// Create the shims directory ahead of time if it doesn't exist to prevent PATH warnings
	shimsDir := filepath.Join(DataDir, "shims")
	if err := os.MkdirAll(shimsDir, 0755); err != nil {
		return err
	}

	// 进程级 env 注入：只影响面板自身进程及其子进程，不侵入用户 shell。
	if !strings.Contains(os.Getenv("PATH"), shimsDir) {
		os.Setenv("PATH", shimsDir+":"+os.Getenv("PATH"))
	}
	if os.Getenv("MISE_DATA_DIR") == "" {
		os.Setenv("MISE_DATA_DIR", DataDir)
	}
	if os.Getenv("MISE_CONFIG_DIR") == "" {
		os.Setenv("MISE_CONFIG_DIR", DataDir)
	}

	return nil
}
