package systemd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 托管 unit 文件统一写到 /etc/systemd/system/，文件名前缀 easyserver-。
// 这一层前缀 + unit 内的 ManagedBy 注释，是 ListManaged 识别托管服务的依据。
const (
	managedUnitPrefix  = "easyserver-"
	managedUnitDir     = "/etc/systemd/system/"
	managedUnitSuffix  = ".service"
	managedMarkerKey   = "ManagedBy"
	managedMarkerValue = "easyserver"
)

// unitNameRegex 限制托管 unit 的 <name> 部分：小写字母/数字/连字符。
// 防止用户输入注入到文件名或 unit 指令里。
var unitNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// envKeyRegex 限制 env 变量名格式，防止 key 含换行注入 unit 指令。
// 标准 POSIX 环境变量名：字母/数字/下划线，不以数字开头。
var envKeyRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ManagedUnitSpec 是创建/更新托管服务的输入配置。
// 对应原 internal/process 的 CreateProcessRequest 字段子集（去掉了 group/log_file/startup_timeout）。
type ManagedUnitSpec struct {
	Name             string            `json:"name"`               // 不含前缀，如 "my-app"
	Description      string            `json:"description"`        // 显示名，写入 Description=
	ExecStart        string            `json:"exec_start"`         // 完整启动命令，如 "node /app/server.js --port 3000"
	Dir              string            `json:"dir"`                // WorkingDirectory
	Env              map[string]string `json:"env"`                // Environment=
	AutoRestart      bool              `json:"auto_restart"`       // Restart=on-failure
	MaxRestarts      int               `json:"max_restarts"`       // StartLimitBurst=
	RestartDelay     int               `json:"restart_delay"`      // RestartSec=
	StopTimeout      int               `json:"stop_timeout"`       // TimeoutStopSec=
	AutoStart        bool              `json:"auto_start"`         // [Install] + systemctl enable
	RuntimeVersionID int64             `json:"runtime_version_id"` // 写入注释，反查用
	RuntimeLang      string            `json:"runtime_lang"`       // mise 工具名，如 "node"
	RuntimeExact     string            `json:"runtime_exact"`      // mise 版本，如 "20.10.0"
}

// UnitFilePath 返回托管 unit 的绝对路径。
func UnitFilePath(name string) string {
	return filepath.Join(managedUnitDir, managedUnitPrefix+name+managedUnitSuffix)
}

// UnitName 从完整 unit 文件名提取 <name>（去前缀和后缀）。
// 输入 "easyserver-foo.service" -> "foo"；非托管 unit 返回空串。
func UnitName(unitFileName string) string {
	base := strings.TrimSuffix(unitFileName, managedUnitSuffix)
	if !strings.HasPrefix(base, managedUnitPrefix) {
		return ""
	}
	return strings.TrimPrefix(base, managedUnitPrefix)
}

// ValidateManagedName 校验 <name> 是否合法。
func ValidateManagedName(name string) error {
	if name == "" {
		return fmt.Errorf("name 不能为空")
	}
	if len(name) > 60 {
		return fmt.Errorf("name 过长（最多 60 字符）")
	}
	if strings.HasPrefix(name, managedUnitPrefix) {
		return fmt.Errorf("name 不能以 %s 前缀开头", managedUnitPrefix)
	}
	if !unitNameRegex.MatchString(name) {
		return fmt.Errorf("name 只能包含小写字母、数字、连字符，且不能以连字符开头/结尾")
	}
	return nil
}

// cleanUnitValue 清理 unit 文件字段中的换行，替换为空格。
func cleanUnitValue(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

// RenderUnit 生成 unit 文件内容。纯函数，无副作用，便于测试。
func RenderUnit(spec *ManagedUnitSpec) (string, error) {
	if err := ValidateManagedName(spec.Name); err != nil {
		return "", err
	}
	if spec.ExecStart == "" {
		return "", fmt.Errorf("exec_start 不能为空")
	}
	if strings.ContainsAny(spec.ExecStart, "\n\r") {
		return "", fmt.Errorf("exec_start 不能包含换行")
	}
	if strings.ContainsAny(spec.Dir, "\n\r") {
		return "", fmt.Errorf("dir 不能包含换行")
	}
	// 防御纵深：runtime 字段也不能含换行
	if strings.ContainsAny(spec.RuntimeLang, "\n\r") {
		return "", fmt.Errorf("runtime_lang 不能包含换行")
	}
	if strings.ContainsAny(spec.RuntimeExact, "\n\r") {
		return "", fmt.Errorf("runtime_exact 不能包含换行")
	}
	// env key 校验：只允许合法的 shell 变量名，防注入。
	for k := range spec.Env {
		if !envKeyRegex.MatchString(k) {
			return "", fmt.Errorf("env key %q 非法（只允许字母数字下划线，不以数字开头）", k)
		}
	}

	spec.Description = cleanUnitValue(spec.Description)
	spec.ExecStart = cleanUnitValue(spec.ExecStart)
	spec.Dir = cleanUnitValue(spec.Dir)
	spec.RuntimeLang = cleanUnitValue(spec.RuntimeLang)
	spec.RuntimeExact = cleanUnitValue(spec.RuntimeExact)

	execStart := buildExecStart(spec)
	envLines := buildEnvLines(spec.Env)

	restartLine := ""
	if spec.AutoRestart {
		restartLine = "Restart=on-failure"
	}

	maxRestarts := spec.MaxRestarts
	if maxRestarts <= 0 {
		maxRestarts = 10
	}
	restartDelay := spec.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 5
	}
	stopTimeout := spec.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = 10
	}

	desc := spec.Description
	if desc == "" {
		desc = spec.Name
	}

	var b strings.Builder
	// [Unit] 段：注释放最前面，作为元数据标记 + 反查锚点。
	fmt.Fprintf(&b, "[Unit]\n")
	fmt.Fprintf(&b, "Description=easyserver-managed: %s\n", desc)
	fmt.Fprintf(&b, "# %s=%s\n", managedMarkerKey, managedMarkerValue)
	if spec.RuntimeVersionID > 0 {
		fmt.Fprintf(&b, "# RuntimeVersionID=%d\n", spec.RuntimeVersionID)
	}
	if spec.RuntimeLang != "" {
		fmt.Fprintf(&b, "# RuntimeLang=%s\n", spec.RuntimeLang)
	}
	if spec.RuntimeExact != "" {
		fmt.Fprintf(&b, "# RuntimeExact=%s\n", spec.RuntimeExact)
	}
	fmt.Fprintf(&b, "After=network.target\n\n")

	// [Service] 段
	fmt.Fprintf(&b, "[Service]\n")
	fmt.Fprintf(&b, "Type=simple\n")
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
	if restartLine != "" {
		fmt.Fprintf(&b, "%s\n", restartLine)
	}
	fmt.Fprintf(&b, "RestartSec=%d\n", restartDelay)
	fmt.Fprintf(&b, "TimeoutStopSec=%d\n", stopTimeout)
	fmt.Fprintf(&b, "StartLimitBurst=%d\n", maxRestarts)
	fmt.Fprintf(&b, "StartLimitIntervalSec=300\n\n")

	// [Install] 段：AutoStart=true 时 systemctl enable 才会生效。
	fmt.Fprintf(&b, "[Install]\n")
	fmt.Fprintf(&b, "WantedBy=multi-user.target\n")

	return b.String(), nil
}

// buildExecStart 拼接 ExecStart 值。
// 绑定 runtime 时前置 mise exec <lang>@<exact> -- 。
// spec.ExecStart 是用户填的完整命令（如 "node /app/server.js --port 3000"），
// 后端只在前面补 mise 包裹，不再拆分 command/args。
func buildExecStart(spec *ManagedUnitSpec) string {
	if spec.RuntimeVersionID > 0 && spec.RuntimeLang != "" && spec.RuntimeExact != "" {
		return "/usr/local/bin/mise exec " + spec.RuntimeLang + "@" + spec.RuntimeExact + " -- " + spec.ExecStart
	}
	return spec.ExecStart
}

// buildEnvLines 把 env map 转成 systemd Environment= 行。
// key 已在 RenderUnit 中用 envKeyRegex 校验；value 含换行跳过。
// 含空格/制表/引号/反斜杠时用双引号包裹并转义。
func buildEnvLines(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(env))
	for _, k := range keys {
		v := env[k]
		if strings.ContainsAny(v, "\n\r") {
			continue // 跳过非法值，不阻断整体生成
		}
		if strings.ContainsAny(v, " \t\"'\\") {
			// systemd Environment= 双引号语义：内部双引号和反斜杠转义。
			v = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
		}
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	return lines
}

// ParseUnitMeta 从 unit 文件内容解析元数据，填入 info 的托管字段。
// [Unit] 段：注释（ManagedBy/Runtime*）+ Description
// [Service] 段：WorkingDirectory/Environment/Restart（systemd 原生指令，直接读）
// 调用方负责设置 info.Name（不含前缀）。
func ParseUnitMeta(content string, info *ServiceInfo) {
	scanner := strings.Split(content, "\n")
	section := ""
	for _, line := range scanner {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = trimmed
			continue
		}

		// [Unit] 段：注释元数据 + Description
		if section == "[Unit]" {
			if strings.HasPrefix(trimmed, "# ") {
				kv := strings.SplitN(strings.TrimPrefix(trimmed, "# "), "=", 2)
				if len(kv) != 2 {
					continue
				}
				key := strings.TrimSpace(kv[0])
				val := strings.TrimSpace(kv[1])
				switch key {
				case managedMarkerKey:
					info.Managed = val == managedMarkerValue
				case "RuntimeVersionID":
					fmt.Sscanf(val, "%d", &info.RuntimeVersionID)
				case "RuntimeLang":
					info.RuntimeLang = val
				case "RuntimeExact":
					info.RuntimeExact = val
				}
				continue
			}
			if strings.HasPrefix(trimmed, "Description=") {
				desc := strings.TrimPrefix(trimmed, "Description=")
				desc = strings.TrimPrefix(desc, "easyserver-managed: ")
				info.Description = desc
			}
		}

		// [Service] 段：从原生指令读回 ExecStart/Dir/Env/AutoRestart
		if section == "[Service]" {
			switch {
			case strings.HasPrefix(trimmed, "ExecStart="):
				execStart := strings.TrimPrefix(trimmed, "ExecStart=")
				// 若绑定了 runtime，去掉 mise 包裹前缀，还原用户原始命令。
				if info.RuntimeVersionID > 0 {
					execStart = stripMisePrefix(execStart)
				}
				info.ExecStart = execStart
			case strings.HasPrefix(trimmed, "WorkingDirectory="):
				dir := strings.TrimPrefix(trimmed, "WorkingDirectory=")
				if len(dir) >= 2 && dir[0] == '"' && dir[len(dir)-1] == '"' {
					dir = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(dir[1 : len(dir)-1])
				}
				info.Dir = dir
			case strings.HasPrefix(trimmed, "Environment="):
				k, v := parseEnvLine(strings.TrimPrefix(trimmed, "Environment="))
				if k != "" {
					if info.Env == nil {
						info.Env = make(map[string]string)
					}
					info.Env[k] = v
				}
			case trimmed == "Restart=on-failure":
				info.AutoRestart = true
			}
		}
	}
}

// stripMisePrefix 去掉 mise 包裹前缀，还原用户原始命令。
// 输入 "/usr/local/bin/mise exec node@20.10.0 -- node /app/server.js"
// 返回 "node /app/server.js"。非 mise 前缀原样返回。
func stripMisePrefix(execStart string) string {
	prefix := "/usr/local/bin/mise exec "
	if !strings.HasPrefix(execStart, prefix) {
		return execStart
	}
	rest := strings.TrimPrefix(execStart, prefix)
	// 跳过 "node@20.10.0 -- " 这段
	dashIdx := strings.Index(rest, " -- ")
	if dashIdx < 0 {
		return execStart
	}
	return rest[dashIdx+4:]
}

// parseEnvLine 解析 Environment= 行的 "KEY=VALUE" 或 KEY="quoted value"。
func parseEnvLine(line string) (key, val string) {
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", ""
	}
	key = line[:eq]
	val = line[eq+1:]
	// 去掉 systemd 双引号包裹
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(val[1 : len(val)-1])
	}
	// 还原 % 转义
	val = strings.ReplaceAll(val, "%%", "%")
	return key, val
}

// writeUnitFile 原子写入 unit 文件到磁盘（0644 权限，原子 rename 模式，防并发读到空文件）。
func writeUnitFile(name, content string) error {
	path := UnitFilePath(name)
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, managedUnitPrefix+name+".*.tmp")
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

// readUnitFile 读 unit 文件内容。文件不存在返回空串 + nil error（视为无元数据）。
func readUnitFile(name string) (string, error) {
	path := UnitFilePath(name)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// removeUnitFile 删除 unit 文件。文件不存在视为成功。
func removeUnitFile(name string) error {
	path := UnitFilePath(name)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
