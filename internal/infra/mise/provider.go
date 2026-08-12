package mise

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Command 表示"在某 runtime（lang@exact）下执行 cmd"的结构化形式，不压平成字符串。
// Exec 是 argv（Exec[0] 为程序），Env 是需要注入的环境变量。调用方（cron/systemd）
// 负责把 Exec/Env 序列化成 crontab 行 / systemd ExecStart 段；换底层只改 Exec/Env 形状。
type Command struct {
	Exec []string
	Env  []string
}

// Provider 抽象"运行环境底层"的实现方式。当前唯一实现是 mise（本包）。
// runtimeenv / cron / systemd 只依赖此接口，不直接碰底层二进制；将来换底层
// （容器 / nix 等）只需提供新的实现。
//
// lang 为通用语言名（node/python/go/java/php），实现内部映射到底层标识
// （mise tool / 容器镜像名 / nix 包名）。
type Provider interface {
	// ListRemoteVersions 返回某语言当前可用的远端版本列表（newest first）。
	ListRemoteVersions(ctx context.Context, lang string) ([]string, error)
	// ResolveVersion 把"前缀版本"解析为精确版本（如 node@20 → 20.11.0）。
	// version 已是精确版本时原样返回。
	ResolveVersion(ctx context.Context, lang, version string) (string, error)
	// Install 安装 <lang>@<exact>，实时输出写入 out。
	Install(ctx context.Context, lang, exact string, out io.Writer) error
	// Uninstall 卸载 <lang>@<exact>，实时输出写入 out。
	Uninstall(ctx context.Context, lang, exact string, out io.Writer) error
	// Command 生成在 <lang>@<exact> 下执行 cmd 的命令表示（渲染方向）。
	Command(lang, exact, cmd string) (Command, error)
	// Unwrap 从 Command 序列化后的命令行字符串还原用户原始 cmd（解析方向）。
	// unit/crontab 文件固有字符串形态，还原逻辑归实现（mise 的 "-- cmd"、容器的
	// "sh -c" 位置不同），调用方不解析格式。
	Unwrap(lang, exact, execLine string) string
	// InstallPath 返回 (lang, exact) 的安装目录（供 UI 展示）。
	InstallPath(lang, exact string) string
	// WriteConfig 让实现按 env 持久化自身配置（如镜像源）。
	// envs 的 key 均为底层标识。
	WriteConfig(ctx context.Context, envs map[string]string) error
}

// NewProvider returns the mise-backed Provider.
func NewProvider() Provider { return miseProvider{} }

// miseProvider is the mise-backed implementation of Provider.
type miseProvider struct{}

// langToTool maps generic language names to mise tool identifiers.
// 与 catalog 的 MiseTool 字段一致，但收敛为底层自己持有（catalog 字段仅供前端展示）。
var langToTool = map[string]string{
	"node":   "node",
	"python": "python",
	"go":     "go",
	"java":   "vfox:version-fox/vfox-java",
	"php":    "php",
}

func toolFor(lang string) (string, error) {
	tool, ok := langToTool[strings.ToLower(lang)]
	if !ok {
		return "", fmt.Errorf("unsupported runtime: %s", lang)
	}
	return tool, nil
}

// miseCmdEnv 返回执行 mise 命令所需的环境变量（追加到进程环境之后）。
func miseCmdEnv() []string {
	return []string{
		"MISE_DATA_DIR=" + DataDir,
		"MISE_CONFIG_DIR=" + ConfigDir,
		"MISE_YES=1",
	}
}

func (miseProvider) ListRemoteVersions(ctx context.Context, lang string) ([]string, error) {
	tool, err := toolFor(lang)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, BinPath, "ls-remote", tool)
	cmd.Env = append(os.Environ(), miseCmdEnv()...)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("failed to fetch remote versions: %w, stderr: %s", err, stderr)
	}

	var versions []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && isValidVersion(line) {
			versions = append(versions, line)
		}
	}
	// Reverse to put newest first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}
	return versions, nil
}

func (miseProvider) ResolveVersion(ctx context.Context, lang, version string) (string, error) {
	tool, err := toolFor(lang)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, BinPath, "latest", fmt.Sprintf("%s@%s", tool, version))
	cmd.Env = append(os.Environ(), miseCmdEnv()...)
	out, _ := cmd.Output()

	// `mise latest` 对 vfox 插件（PHP/Java）经常返回空 stdout（vfox 的 latest 行为
	// 不统一），此时兜底用前端传入的 version。若它确实不是合法版本，后续 Install
	// 会有明确报错。
	var exact string
	if lines := strings.Split(strings.TrimSpace(string(out)), "\n"); len(lines) > 0 {
		exact = strings.TrimSpace(lines[len(lines)-1])
	}
	if exact == "" {
		exact = version
	}
	return exact, nil
}

// runInstallLike 执行 mise install/uninstall 并把 stdout+stderr 合并实时写入 out。
func (miseProvider) runInstallLike(ctx context.Context, op string, lang, exact string, out io.Writer) error {
	tool, err := toolFor(lang)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, BinPath, op, "-y", fmt.Sprintf("%s@%s", tool, exact))
	cmd.Env = append(os.Environ(), miseCmdEnv()...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// 合并两路输出到 out（顺序读 stdout 后 stderr，mise 输出以 stdout 为主）。
	// 实时写，调用方（runtimeenv）负责写 DB 与日志截断。
	// 复制失败（out 写入失败）不影响进程退出状态，真实错误由下方 cmd.Wait 决定。
	_, _ = io.Copy(out, io.MultiReader(stdout, stderr))

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s failed with exit code %d", op, exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func (miseProvider) Install(ctx context.Context, lang, exact string, out io.Writer) error {
	return (miseProvider{}).runInstallLike(ctx, "install", lang, exact, out)
}

func (miseProvider) Uninstall(ctx context.Context, lang, exact string, out io.Writer) error {
	return (miseProvider{}).runInstallLike(ctx, "uninstall", lang, exact, out)
}

func (miseProvider) Command(lang, exact, cmd string) (Command, error) {
	tool, err := toolFor(lang)
	if err != nil {
		return Command{}, err
	}
	return Command{
		Exec: []string{BinPath, "exec", tool + "@" + exact, "--", cmd},
		Env:  []string{"MISE_DATA_DIR=" + DataDir, "MISE_CONFIG_DIR=" + ConfigDir},
	}, nil
}

// Unwrap 从 ExecStart / crontab 行还原用户原始 cmd。mise 的包裹格式是
// `{bin} exec <tool>@<exact> -- <cmd>`：先匹配 bin 前缀（避免误剥非 runtime
// 命令），再取包裹处第一个 "-- " 之后（用户 cmd 自身含 "--" 也不会误剥）。
func (miseProvider) Unwrap(lang, exact, execLine string) string {
	prefix := BinPath + " exec "
	if !strings.HasPrefix(execLine, prefix) {
		return execLine
	}
	rest := strings.TrimPrefix(execLine, prefix)
	_, after, ok := strings.Cut(rest, " -- ")
	if !ok {
		return execLine
	}
	return after
}

// miseToolDirName 把底层 tool 标识归一为 installs/ 下的目录名。
// mise 用 '-' 替换 ':' 和 '/'（如 "vfox:version-fox/vfox-java" →
// "vfox-version-fox-vfox-java"）。核心插件（node/go）原样通过。
func miseToolDirName(tool string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(tool)
}

// InstallPath 返回 (lang, exact) 在 installs/ 下的预期目录。纯计算，不 stat 磁盘。
func (miseProvider) InstallPath(lang, exact string) string {
	tool, err := toolFor(lang)
	if err != nil || exact == "" {
		return ""
	}
	return filepath.Join(DataDir, "installs", miseToolDirName(tool), exact)
}

func (miseProvider) WriteConfig(ctx context.Context, envs map[string]string) error {
	content := BuildConfigContent(envs)

	dir := filepath.Dir(ConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "config.toml.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}
	return os.Rename(tmpName, ConfigPath)
}

// isValidVersion validates a version string to prevent command injection.
func isValidVersion(version string) bool {
	if len(version) == 0 || len(version) > 50 {
		return false
	}
	for _, c := range version {
		if !isVersionAlphaNum(c) && c != '.' && c != '-' && c != '+' && c != '_' {
			return false
		}
	}
	return isVersionAlphaNum(rune(version[0]))
}

// isVersionAlphaNum 判断字符是否为字母或数字（版本号的首字符与整体校验共用）。
func isVersionAlphaNum(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
