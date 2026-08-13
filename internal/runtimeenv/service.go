package runtimeenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"os/exec"
	"sync"
	"unicode/utf8"

	"easyserver/internal/envconfig"
	"easyserver/internal/infra"
	"easyserver/internal/infra/mise"
	"easyserver/internal/infra/task"
)

// EnvConfigProvider provides read access to environment configurations.
// Implemented by *envconfig.Service; defined here so runtimeenv can read env
// configs for GenerateMiseConfig without a hard circular dependency.
type EnvConfigProvider interface {
	ListEnvConfigs(ctx context.Context) ([]envconfig.EnvConfig, error)
}

// RuntimeLookup 提供"已安装运行环境"的只读查询（ADR-0009：installs/ 文件系统权威）。
// 由 cron/systemd 消费：绑定 lang@exact 时校验对应安装目录存在。
type RuntimeLookup interface {
	// Installed 判断 lang@exact 是否已安装（目录存在且带完成标记）。
	Installed(ctx context.Context, lang, exact string) bool
}

type Service struct {
	envConfigs EnvConfigProvider
	provider   mise.Provider
	taskMgr    *task.Manager // 后台 install/uninstall 执行器（key=runtime:<lang@exact> 互斥）
}

func NewService(envConfigs EnvConfigProvider, provider mise.Provider) *Service {
	return &Service{
		envConfigs: envConfigs,
		provider:   provider,
		taskMgr:    task.NewManager(8),
	}
}

// Init performs boot-time initialization: regenerates the panel-private mise
// config.toml from the current env config + defaults. 无 DB 状态可 heal（ADR-0009）。
func (s *Service) Init(ctx context.Context) error {
	if err := s.GenerateMiseConfig(ctx); err != nil {
		stdlog.Printf("runtime: failed to generate mise config on boot: %v", err)
		return err
	}
	return nil
}

// GenerateMiseConfig 让底层按当前 DB 中的环境变量持久化自身配置。
// 收集纯数据（env map），具体写入由 provider 负责。
func (s *Service) GenerateMiseConfig(ctx context.Context) error {
	envConfigs, err := s.envConfigs.ListEnvConfigs(ctx)
	if err != nil {
		return err
	}
	envs := make(map[string]string, len(envConfigs))
	for _, c := range envConfigs {
		if c.Enabled {
			envs[c.Name] = c.Value
		}
	}
	return s.provider.WriteConfig(ctx, envs)
}

// Installed 判断 lang@exact 是否已安装（目录存在且带完成标记）。
// 实现 RuntimeLookup，供 cron/systemd 绑定校验。
func (s *Service) Installed(ctx context.Context, lang, exact string) bool {
	return Installed(ctx, lang, exact)
}

// Installed 包级函数：磁盘判定，纯只读。cron/systemd 经 RuntimeLookup 用，
// runtimeenv 内部（install 去重、uninstall 存在性）直接调。
func Installed(ctx context.Context, lang, exact string) bool {
	_, err := os.Stat(markerPath(lang, exact))
	return err == nil
}

// ListAll returns all installed runtime environments（扫描 installs/ 目录）。
func (s *Service) ListAll(ctx context.Context) ([]RuntimeEnvironment, error) {
	return ScanInstalled(ctx)
}

// ListByName returns all versions of a specific runtime environment.
func (s *Service) ListByName(ctx context.Context, name string) ([]RuntimeEnvironment, error) {
	envs, err := ScanInstalled(ctx)
	if err != nil {
		return nil, err
	}
	var filtered []RuntimeEnvironment
	for _, e := range envs {
		if e.Name == name {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

// GetByLangExact returns a runtime environment by lang@exact.
func (s *Service) GetByLangExact(ctx context.Context, lang, exact string) (*RuntimeEnvironment, error) {
	if !Installed(ctx, lang, exact) {
		return nil, nil
	}
	envs, err := ScanInstalled(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		if e.Name == lang && e.Version == exact {
			return &e, nil
		}
	}
	return nil, nil
}

// Install installs a runtime environment.
func (s *Service) Install(ctx context.Context, name, version string) error {
	if !isValidVersion(version) {
		return fmt.Errorf("invalid version format: %s", version)
	}

	// Regenerate once per install so mirror env vars from env_configs are fresh
	// before resolve/install read the panel-private config.
	if err := s.GenerateMiseConfig(ctx); err != nil {
		return fmt.Errorf("failed to regenerate mise config before install: %w", err)
	}

	// 底层把"前缀版本"解析成精确版本（如 Node 选主版本 20 → 20.11.0）。
	// vfox 插件（PHP/Java）解析可能返回空，底层已兜底用传入的 version。
	exactVersion, err := s.provider.ResolveVersion(ctx, name, version)
	if err != nil {
		return err
	}

	// 已安装判定走磁盘（目录 + 完成标记），而非 DB 行。
	if Installed(ctx, name, exactVersion) {
		return fmt.Errorf("%s %s is already installed", name, exactVersion)
	}

	// 安装脱离请求生命周期：task 执行器内部用 WithoutCancel 剥离取消，请求断开
	// 任务照常执行。同 runtime 的安装与卸载共用同一 key 互斥（key=lang@exact），
	// 重复提交/并发超限同步返回错误。日志进 TaskLog 内存流，由 SSE 端点回放。
	// 安装失败不再有 DB 行可查（ADR-0009）：列表不会显示失败记录。
	if _, err := s.taskMgr.StartWithLog(ctx, runtimeTaskKey(name, exactVersion), task.Options{}, func(ctx context.Context, log *task.TaskLog) error {
		return s.installRuntime(ctx, name, exactVersion, log)
	}); err != nil {
		return err
	}
	return nil
}

// runtimeTaskKey 生成 install/uninstall 的 task key：同一 lang@exact 的安装与
// 卸载共用同一 key，task 执行器据此保证二者互斥。
func runtimeTaskKey(lang, exact string) string {
	return "runtime:" + lang + "@" + exact
}

// installRuntime performs the actual installation.
func (s *Service) installRuntime(ctx context.Context, name, exactVersion string, log *task.TaskLog) error {
	// PHP / Python 是源码编译，必须先把 autoconf / libxml2-dev 等系统级
	// 编译依赖装上，否则 mise install 会卡在 buildconf 阶段并报错。
	// node / go / java 走预编译二进制，ensureBuildDeps 对它们是 no-op。
	if err := s.ensureBuildDeps(ctx, name, log); err != nil {
		stdlog.Printf("runtime: failed to ensure build deps for %s: %v", name, err)
		return err
	}

	log.Append(fmt.Sprintf("正在安装 %s...", exactVersion))
	if err := s.provider.Install(ctx, name, exactVersion, installWriter{log: log}); err != nil {
		stdlog.Printf("runtime: failed to install %s %s: %v", name, exactVersion, err)
		return fmt.Errorf("安装失败: %w", err)
	}

	// 安装成功后写完成标记；卸载删除目录时一并消失。
	if err := os.WriteFile(markerPath(name, exactVersion), []byte("ok\n"), 0644); err != nil {
		stdlog.Printf("runtime: failed to write marker for %s %s: %v", name, exactVersion, err)
		return fmt.Errorf("安装完成但写入标记失败: %w", err)
	}

	log.Append("安装完成")
	stdlog.Printf("runtime: installed %s %s", name, exactVersion)
	return nil
}

// installWriter 把 provider.Install/Uninstall 的实时输出写进任务日志
// （TaskLog 内存流）。provider 接口要 io.Writer，TaskLog 是 Append(string)，
// 这里做最小桥接。
type installWriter struct {
	log *task.TaskLog
}

func (w installWriter) Write(p []byte) (int, error) {
	if w.log != nil {
		w.log.Append(string(p))
	}
	return len(p), nil
}

// runStreaming runs a command and streams its output to the task log
// (TaskLog memory ring buffer). 返回完整输出（供错误消息）、退出码与 wait 错误。
func (s *Service) runStreaming(ctx context.Context, initialMsg string, log *task.TaskLog, name string, args ...string) (string, int, error) {
	log.Append(initialMsg)

	cmd := exec.CommandContext(ctx, name, args...)
	// DEBIAN_FRONTEND=noninteractive 防止 ensureBuildDeps 里的 apt-get install
	// 撞上 tzdata 这类会忽略 -y 的交互式 postinst 直接挂住。
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, err
	}

	if err := cmd.Start(); err != nil {
		return "", -1, err
	}

	var outputBuf bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup

	writeFn := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				log.Append(string(buf[:n]))
				mu.Lock()
				outputBuf.Write(buf[:n])
				// truncate buffer to avoid OOM, leave roughly 100KB headroom
				if outputBuf.Len() > 500000 {
					b := outputBuf.Bytes()
					targetStart := len(b) - 400000
					// Find the first newline after targetStart to avoid breaking UTF-8 chars
					idx := bytes.IndexByte(b[targetStart:], '\n')
					if idx == -1 {
						idx = 0 // if no newline, find first valid UTF-8 boundary
						for idx < len(b)-targetStart && !utf8.RuneStart(b[targetStart+idx]) {
							idx++
						}
					}

					prefix := []byte("...")
					remain := b[targetStart+idx:]
					remainLen := len(remain)

					// Use copy to avoid allocation
					copy(b[len(prefix):], remain)
					copy(b[:len(prefix)], prefix)
					outputBuf.Truncate(len(prefix) + remainLen)
				}
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}

	wg.Add(2)
	go writeFn(stdout)
	go writeFn(stderr)

	errChan := make(chan error, 1)
	infra.Go(func() {
		wg.Wait()
		errChan <- cmd.Wait()
	})

	err = <-errChan
	mu.Lock()
	finalOutput := outputBuf.String()
	mu.Unlock()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return finalOutput, exitCode, err
}

// InstallTask 返回后台 install/uninstall 任务句柄（key=runtime:<lang@exact>），
// 供 SSE 端点回放日志。任务不存在（成功即清或面板重启后内存态已失）时 ok=false。
func (s *Service) InstallTask(lang, exact string) (*task.Task, bool) {
	return s.taskMgr.Get(runtimeTaskKey(lang, exact))
}

// isValidVersion validates version string to prevent command injection
// Only allows numbers, letters, dots, hyphens, plus, and underscores (e.g., 17.0.19, 20.10.0, 1.21.5-beta, 21.0.1+12-LTS, temurin-21.0.1)
func isValidVersion(version string) bool {
	if len(version) == 0 || len(version) > 50 {
		return false
	}
	for _, c := range version {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '.' && c != '-' && c != '+' && c != '_' {
			return false
		}
	}
	// Must start with a digit or a letter
	first := version[0]
	return (first >= '0' && first <= '9') || (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
}

// Uninstall uninstalls a runtime environment.
func (s *Service) Uninstall(ctx context.Context, name, version string) error {
	// 存在性判定走磁盘（目录 + 完成标记）。
	if !Installed(ctx, name, version) {
		return fmt.Errorf("%s %s not found", name, version)
	}

	// 正在安装/卸载同一版本 → 拒绝（task 互斥，但显式报错更友好）。
	if _, busy := s.taskMgr.Get(runtimeTaskKey(name, version)); busy {
		return fmt.Errorf("operation in progress: %s@%s", name, version)
	}

	// 卸载与安装共用同一 task key（lang@exact），task 执行器保证互斥。任务体
	// 脱离请求生命周期（WithoutCancel），成功删除目录 + 标记，失败仅内存态。
	if _, err := s.taskMgr.StartWithLog(ctx, runtimeTaskKey(name, version), task.Options{}, func(ctx context.Context, log *task.TaskLog) error {
		uninstallErr := s.uninstallRuntime(ctx, name, version, log)
		if uninstallErr != nil {
			stdlog.Printf("runtime: failed to uninstall %s %s: %v", name, version, uninstallErr)
			return uninstallErr
		}
		// 卸载后标记随目录删除；此处显式删标记防目录残留半截。
		_ = os.Remove(markerPath(name, version))
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// uninstallRuntime performs the actual uninstallation.
func (s *Service) uninstallRuntime(ctx context.Context, name, version string, log *task.TaskLog) error {
	log.Append(fmt.Sprintf("正在卸载 %s...", version))
	if err := s.provider.Uninstall(ctx, name, version, installWriter{log: log}); err != nil {
		return fmt.Errorf("卸载失败: %w", err)
	}
	stdlog.Printf("runtime: uninstalled %s %s", name, version)
	return nil
}

// GetRemoteVersions dynamically fetches available versions via the provider.
func (s *Service) GetRemoteVersions(ctx context.Context, lang string) ([]string, error) {
	return s.provider.ListRemoteVersions(ctx, lang)
}
