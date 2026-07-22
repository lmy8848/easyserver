package systemd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// ManagedUnitSpec 是创建/更新托管服务的输入配置。
// 对应原 internal/process 的 CreateProcessRequest 字段子集（去掉了 group/log_file/startup_timeout）。
type ManagedUnitSpec struct {
	Name             string            `json:"name"`               // 不含前缀，如 "my-app"
	Description      string            `json:"description"`        // 显示名，写入 Description=
	Command          string            `json:"command"`            // 启动命令，如 "node /app/server.js"
	Args             string            `json:"args"`               // 命令参数（空格分隔）
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

// ManagedUnitMeta 是从已存在的 unit 文件解析出来的元数据。
// ListManaged 返回它，前端用于还原表单。读不到的注释字段为零值，不阻断显示。
type ManagedUnitMeta struct {
	Name             string `json:"name"` // 不含前缀
	Description      string `json:"description"`
	RuntimeVersionID int64  `json:"runtime_version_id"`
	RuntimeLang      string `json:"runtime_lang"`
	RuntimeExact     string `json:"runtime_exact"`
	HasMarker        bool   `json:"has_marker"` // ManagedBy=easyserver 注释是否存在
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
	if !unitNameRegex.MatchString(name) {
		return fmt.Errorf("name 只能包含小写字母、数字、连字符，且不能以连字符开头/结尾")
	}
	return nil
}

// RenderUnit 生成 unit 文件内容。纯函数，无副作用，便于测试。
//
// 安全性：command/args/dir/env 的值由调用方保证可信（前端表单 + 后端再校验），
// 这里不做 shell 转义——systemd 的 ExecStart/Environment 本身是声明式解析，
// 不经过 shell，不存在注入。但会拒绝包含换行的值，防止破坏 unit 文件结构。
func RenderUnit(spec *ManagedUnitSpec) (string, error) {
	if err := ValidateManagedName(spec.Name); err != nil {
		return "", err
	}
	if spec.Command == "" {
		return "", fmt.Errorf("command 不能为空")
	}
	if strings.ContainsAny(spec.Command, "\n\r") {
		return "", fmt.Errorf("command 不能包含换行")
	}
	if strings.ContainsAny(spec.Args, "\n\r") {
		return "", fmt.Errorf("args 不能包含换行")
	}
	if strings.ContainsAny(spec.Dir, "\n\r") {
		return "", fmt.Errorf("dir 不能包含换行")
	}

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
	fmt.Fprintf(&b, "Description=easyserver-managed: %s\n", escapeUnitValue(desc))
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
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", spec.Dir)
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
func buildExecStart(spec *ManagedUnitSpec) string {
	parts := []string{}
	if spec.RuntimeVersionID > 0 && spec.RuntimeLang != "" && spec.RuntimeExact != "" {
		parts = append(parts, "/usr/local/bin/mise", "exec",
			spec.RuntimeLang+"@"+spec.RuntimeExact, "--")
	}
	parts = append(parts, spec.Command)
	if spec.Args != "" {
		parts = append(parts, parseArgs(spec.Args)...)
	}
	return strings.Join(parts, " ")
}

// buildEnvLines 把 env map 转成 systemd Environment= 行。
// 值含空格或特殊字符时用引号包裹；拒绝换行。
func buildEnvLines(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	lines := make([]string, 0, len(env))
	for k, v := range env {
		if strings.ContainsAny(v, "\n\r") {
			continue // 跳过非法值，不阻断整体生成
		}
		if strings.ContainsAny(v, " \t\"'") {
			v = fmt.Sprintf("%q", v)
		}
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	return lines
}

// ParseUnitMeta 从 unit 文件内容解析元数据注释。
// 只读 [Unit] 段前的注释，不依赖文件存在。
func ParseUnitMeta(content string) ManagedUnitMeta {
	meta := ManagedUnitMeta{}
	scanner := strings.Split(content, "\n")
	inUnitSection := false
	for _, line := range scanner {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inUnitSection = trimmed == "[Unit]"
			continue
		}
		if !inUnitSection {
			continue
		}
		// 注释行：# Key=Value
		if strings.HasPrefix(trimmed, "# ") {
			kv := strings.SplitN(strings.TrimPrefix(trimmed, "# "), "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			switch key {
			case managedMarkerKey:
				meta.HasMarker = val == managedMarkerValue
			case "RuntimeVersionID":
				fmt.Sscanf(val, "%d", &meta.RuntimeVersionID)
			case "RuntimeLang":
				meta.RuntimeLang = val
			case "RuntimeExact":
				meta.RuntimeExact = val
			}
			continue
		}
		// Description= 行
		if strings.HasPrefix(trimmed, "Description=") {
			desc := strings.TrimPrefix(trimmed, "Description=")
			// 去掉 "easyserver-managed: " 前缀还原显示名
			desc = strings.TrimPrefix(desc, "easyserver-managed: ")
			meta.Description = desc
		}
	}
	return meta
}

// escapeUnitValue 转义 unit 文件值里的换行（防御性，正常已在 RenderUnit 前拦截）。
func escapeUnitValue(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

// parseArgs 简单参数切分（支持双引号）。
// 从 internal/process/service.go 搬来，保持行为一致。
func parseArgs(args string) []string {
	if args == "" {
		return nil
	}
	var result []string
	current := ""
	inQuote := false
	for _, c := range args {
		switch c {
		case '"':
			inQuote = !inQuote
		case ' ':
			if inQuote {
				current += string(c)
			} else if current != "" {
				result = append(result, current)
				current = ""
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// writeUnitFile 写 unit 文件到磁盘（0644 权限，systemd 标准）。
// 调用方需保证已 root 或对目标目录有写权限。
func writeUnitFile(name, content string) error {
	path := UnitFilePath(name)
	return os.WriteFile(path, []byte(content), 0644)
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
