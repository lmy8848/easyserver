package systemd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/mise"
)

// 定时任务（cron → systemd timer）的 unit 命名：每个任务一对 .timer + .service，
// 文件名前缀 easyserver-cron-，与进程守护的 easyserver- 前缀隔离（见 ADR-0004）。
// 这一层前缀 + unit 内的 ManagedBy=easyserver-cron 注释，是 ListCronTimers 识别依据。
const (
	CronTimerPrefix = "easyserver-cron-"
	cronTimerSuffix = ".timer"
)

// TimerSpec 是生成定时任务 .timer / .service 一对 unit 的输入配置。
// 与 ManagedUnitSpec 同构，但字段语义针对 systemd timer（OnCalendar/Persistent）。
type TimerSpec struct {
	Name         string            // 不含前缀，如 "daily-backup"
	Description  string            // 显示名，写入 Description=
	OnCalendar   string            // 触发表达式，如 "*-*-* 03:00:00"
	Persistent   bool              // [Timer] Persistent=<true|false>
	ExecStart    string            // 用户命令或脚本路径（mise exec 包裹）
	Dir          string            // WorkingDirectory
	Env          map[string]string // Environment=
	MaxRetry     int               // StartLimitBurst = MaxRetry + 1
	RestartDelay int               // RestartSec
	Timeout      int               // TimeoutStartSec
	Runtime      string            // lang@exact，"" = 不绑定（ADR-0009 绑定键）
}

// CronTimerFileName 返回 <name> 对应的 .timer 文件名。
func CronTimerFileName(name string) string {
	return CronTimerPrefix + name + cronTimerSuffix
}

// CronServiceFileName 返回 <name> 对应的 .service 文件名。
func CronServiceFileName(name string) string {
	return CronTimerPrefix + name + managedUnitSuffix
}

// CronTimerPath 返回 .timer 的绝对路径。
func CronTimerPath(name string) string {
	return filepath.Join(managedUnitDir, CronTimerFileName(name))
}

// CronServicePath 返回 .service 的绝对路径。
func CronServicePath(name string) string {
	return filepath.Join(managedUnitDir, CronServiceFileName(name))
}

// CronTimerName 从 unit 文件名提取 <name>。
// 输入 "easyserver-cron-foo.timer" -> "foo"；非 cron timer 文件返回空串。
func CronTimerName(unitFileName string) string {
	if !strings.HasSuffix(unitFileName, cronTimerSuffix) {
		return ""
	}
	base := strings.TrimSuffix(unitFileName, cronTimerSuffix)
	if !strings.HasPrefix(base, CronTimerPrefix) {
		return ""
	}
	return strings.TrimPrefix(base, CronTimerPrefix)
}

// UnitDir 返回托管 unit 目录（供扫描 cron timer 用）。
func UnitDir() string {
	return managedUnitDir
}

// ValidateCronName 校验定时任务 <name> 是否合法（复用进程守护的命名规则）。
func ValidateCronName(name string) error {
	if name == "" {
		return errors.New("name 不能为空")
	}
	if len(name) > 60 {
		return errors.New("name 过长（最多 60 字符）")
	}
	if strings.HasPrefix(name, CronTimerPrefix) {
		return fmt.Errorf("name 不能以 %s 前缀开头", CronTimerPrefix)
	}
	if !unitNameRegex.MatchString(name) {
		return errors.New("name 只能包含小写字母、数字、连字符，且不能以连字符开头/结尾")
	}
	return nil
}

// RenderTimer 生成 .timer unit 文件内容。纯函数，无副作用，便于测试。
func RenderTimer(spec *TimerSpec) (string, error) {
	if err := ValidateCronName(spec.Name); err != nil {
		return "", err
	}
	if spec.OnCalendar == "" {
		return "", errors.New("on_calendar 不能为空")
	}
	if strings.ContainsAny(spec.OnCalendar, "\n\r") {
		return "", errors.New("on_calendar 不能包含换行")
	}

	persistent := "no"
	if spec.Persistent {
		persistent = "yes"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", cleanUnitValue(spec.Description))
	fmt.Fprintf(&b, "# %s=%s\n", managedMarkerKey, "easyserver-cron")
	fmt.Fprintf(&b, "\n[Timer]\n")
	fmt.Fprintf(&b, "OnCalendar=%s\n", strings.TrimSpace(spec.OnCalendar))
	fmt.Fprintf(&b, "Persistent=%s\n", persistent)
	fmt.Fprintf(&b, "Unit=%s\n", CronServiceFileName(spec.Name))
	fmt.Fprintf(&b, "\n[Install]\n")
	fmt.Fprintf(&b, "WantedBy=timers.target\n")
	return b.String(), nil
}

// RenderCronService 生成定时任务的 .service unit 文件内容。纯函数。
// Type=oneshot：任务跑到完成，systemd 跟踪退出状态；Restart=on-failure +
// StartLimitBurst 把重试/超时交给 systemd 原生（ADR-0004）。无 [Install] 段——
// service 由 timer 激活，不需要单独 enable。
func RenderCronService(spec *TimerSpec, p mise.Provider) (string, error) {
	if err := ValidateCronName(spec.Name); err != nil {
		return "", err
	}
	if spec.ExecStart == "" {
		return "", errors.New("exec_start 不能为空")
	}
	if strings.ContainsAny(spec.ExecStart, "\n\r") {
		return "", errors.New("exec_start 不能包含换行")
	}
	if strings.ContainsAny(spec.Dir, "\n\r") {
		return "", errors.New("dir 不能包含换行")
	}
	if strings.ContainsAny(spec.Runtime, "\n\r") {
		return "", errors.New("runtime 不能包含换行")
	}
	for k := range spec.Env {
		if !envKeyRegex.MatchString(k) {
			return "", fmt.Errorf("env key %q 非法（只允许字母数字下划线，不以数字开头）", k)
		}
	}

	lang, exact := splitBinding(spec.Runtime)
	execStart, runtimeEnv := buildCronExecStart(lang, exact, spec.ExecStart, p)
	envLines := buildEnvLines(mergeCommandEnv(spec.Env, runtimeEnv))

	// StartLimitBurst = MaxRetry + 1：首次执行算一次，重试 MaxRetry 次。
	burst := max(spec.MaxRetry+1, 1)
	restartDelay := spec.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 5
	}
	// 0 = 不超时（infinity）；否则按秒限制单次执行时长。
	timeoutLine := "TimeoutStartSec=infinity"
	if spec.Timeout > 0 {
		timeoutLine = fmt.Sprintf("TimeoutStartSec=%d", spec.Timeout)
	}

	desc := spec.Description
	if desc == "" {
		desc = spec.Name
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", cleanUnitValue(desc))
	fmt.Fprintf(&b, "# %s=%s\n", managedMarkerKey, "easyserver-cron")
	if lang != "" {
		fmt.Fprintf(&b, "# RuntimeLang=%s\n", lang)
	}
	if exact != "" {
		fmt.Fprintf(&b, "# RuntimeExact=%s\n", exact)
	}
	fmt.Fprintf(&b, "StartLimitBurst=%d\n", burst)
	fmt.Fprintf(&b, "StartLimitIntervalSec=300\n")
	fmt.Fprintf(&b, "After=network.target\n\n")

	fmt.Fprintf(&b, "[Service]\n")
	fmt.Fprintf(&b, "Type=oneshot\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", execStart)
	if spec.Dir != "" {
		dir := spec.Dir
		if strings.ContainsAny(dir, " \t\"'\\") {
			dir = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(dir) + `"`
		}
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", dir)
	}
	for _, line := range envLines {
		fmt.Fprintf(&b, "Environment=%s\n", line)
	}
	fmt.Fprintf(&b, "Restart=on-failure\n")
	fmt.Fprintf(&b, "RestartSec=%d\n", restartDelay)
	fmt.Fprintf(&b, "%s\n", timeoutLine)
	return b.String(), nil
}

// buildCronExecStart 拼接 ExecStart 值，并返回底层命令所需的额外环境变量。
// 绑定 runtime 时前置 mise 包裹（与进程守护的 buildExecStart 同逻辑）。
func buildCronExecStart(lang, exact, execStart string, p mise.Provider) (string, []string) {
	if lang != "" && exact != "" {
		if c, err := p.Command(lang, exact, execStart); err == nil {
			return strings.Join(c.Exec, " "), c.Env
		}
	}
	return execStart, nil
}

// WriteCronUnitFile 原子写入 cron unit 文件到 managedUnitDir。
func WriteCronUnitFile(fileName, content string) error {
	path := filepath.Join(managedUnitDir, fileName)
	dir := filepath.Dir(path)
	if err := ensureManagedUnitDir(); err != nil {
		return fmt.Errorf("创建 unit 目录失败: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时 unit 文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入临时 unit 文件失败: %w", err)
	}
	if err := tmpFile.Chmod(0644); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("设置 unit 文件权限失败: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync 临时 unit 文件失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭临时 unit 文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("重命名 unit 文件失败: %w", err)
	}
	return nil
}

// ReadCronUnitFile 读 cron unit 文件内容。文件不存在返回空串 + nil error。
func ReadCronUnitFile(fileName string) (string, error) {
	path := filepath.Join(managedUnitDir, fileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RemoveCronUnitFile 删除 cron unit 文件。文件不存在视为成功。
func RemoveCronUnitFile(fileName string) error {
	path := filepath.Join(managedUnitDir, fileName)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
