package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra/executor"
)

// ServiceInfo represents a systemd service.
// 托管服务（easyserver-* 前缀）会额外填充 Managed/Runtime* 字段；
// 系统服务这些字段为零值。
type ServiceInfo struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	State         string  `json:"state"`
	SubState      string  `json:"sub_state"`
	Enabled       bool    `json:"enabled"`
	UnitFileState string  `json:"unit_file_state"`
	PID           int     `json:"pid"`
	MemoryBytes   uint64  `json:"memory_bytes"`
	CPUPercent    float64 `json:"cpu_percent"`
	UptimeSeconds int64   `json:"uptime_seconds"`

	// 托管服务元数据（解析 unit 文件注释得到；系统服务为零值）
	Managed          bool   `json:"managed"`
	RuntimeVersionID int64  `json:"runtime_version_id"`
	RuntimeLang      string `json:"runtime_lang"`
	RuntimeExact     string `json:"runtime_exact"`

	// 托管服务配置回显（解析 [Service] 段得到；编辑表单用）
	Command     string            `json:"command"`
	Args        string            `json:"args"`
	Dir         string            `json:"dir"`
	Env         map[string]string `json:"env"`
	AutoRestart bool              `json:"auto_restart"`
}

// LogLine represents a log line from journalctl.
type LogLine struct {
	Time     string `json:"time"`
	Message  string `json:"message"`
	Priority string `json:"priority"`
}

// journalEntry represents a journalctl JSON output line.
type journalEntry struct {
	Message           string `json:"MESSAGE"`
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Priority          string `json:"PRIORITY"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
	Transport         string `json:"_TRANSPORT"`
}

// RuntimeLookup 查询 runtime_version 表，补 lang/exact/status。
// 由 cron 包实现（systemd 包不反向依赖 cron），在 app.go 注入。
type RuntimeLookup interface {
	// GetRuntime 返回 runtime_version 行的 lang/exact/status。
	// 不存在返回错误。
	GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error)
}

// ServiceManager manages systemd services.
type ServiceManager struct {
	mu       sync.Mutex // 保护 managed CRUD 并发（创建/更新/删除互斥）
	executor executor.CommandExecutor
	runtime  RuntimeLookup // 可选，nil 时跳过 runtime 补全
}

// NewServiceManager creates a new ServiceManager.
func NewServiceManager(exec executor.CommandExecutor) *ServiceManager {
	return &ServiceManager{executor: exec}
}

// SetRuntimeLookup 注入 runtime 查询依赖（app.go 装配时调用）。
func (m *ServiceManager) SetRuntimeLookup(r RuntimeLookup) {
	m.runtime = r
}

// List returns all systemd services with basic info (name, state, description).
// 对 easyserver-* 前缀的托管服务，额外读 unit 文件填充 managed/runtime_* 元数据。
// 批量补全 PID/enabled/memory/uptime，前端无需再调 GetDetails。
func (m *ServiceManager) List(ctx context.Context) ([]ServiceInfo, error) {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain", "--full")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var services []ServiceInfo
	lines := strings.Split(stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "UNIT") || strings.HasPrefix(line, "LOAD") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// 跳过不存在的服务（LOAD 列为 not-found）
		if fields[1] == "not-found" {
			continue
		}

		name := strings.TrimSuffix(fields[0], ".service")
		svc := ServiceInfo{
			Name:     name,
			State:    fields[2],
			SubState: fields[3],
		}

		if len(fields) > 4 {
			svc.Description = strings.Join(fields[4:], " ")
		}

		// 托管服务：读 unit 文件补元数据 + 配置回显（本地 IO，不调 systemctl）
		if shortName := UnitName(fields[0]); shortName != "" {
			if content, _ := readUnitFile(shortName); content != "" {
				ParseUnitMeta(content, &svc)
			}
		}

		services = append(services, svc)
	}

	// 批量补全 PID/enabled/memory/uptime（避免前端 N+1 调 GetDetails）
	m.batchGetDetailedInfo(ctx, services)

	return services, nil
}

// GetDetails fetches PID, memory, and enabled status for specific services.
func (m *ServiceManager) GetDetails(ctx context.Context, names []string) ([]ServiceInfo, error) {
	if len(names) == 0 {
		return nil, nil
	}

	services := make([]ServiceInfo, len(names))
	for i, name := range names {
		services[i] = ServiceInfo{Name: name}
	}

	m.batchGetDetailedInfo(ctx, services)

	return services, nil
}

// batchGetDetailedInfo gets PID, memory, and enabled status for multiple services efficiently.
func (m *ServiceManager) batchGetDetailedInfo(ctx context.Context, services []ServiceInfo) {
	if len(services) == 0 {
		return
	}

	// Batch into groups to avoid systemd "Unknown object" errors with too many units
	const batchSize = 50
	for start := 0; start < len(services); start += batchSize {
		end := start + batchSize
		if end > len(services) {
			end = len(services)
		}
		m.batchGetDetailedInfoChunk(ctx, services[start:end])
	}
}

func (m *ServiceManager) batchGetDetailedInfoChunk(ctx context.Context, services []ServiceInfo) {
	args := []string{"show"}
	for _, svc := range services {
		args = append(args, svc.Name+".service")
	}
	args = append(args, "--property=Id,MainPID,MemoryCurrent,ActiveState,UnitFileState,Description,SubState")

	stdout, _, _, _ := m.executor.Run(ctx, "systemctl", args...)
	if stdout == "" {
		return
	}

	currentName := ""
	props := make(map[string]string)

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentName != "" {
				m.applyServiceProps(services, currentName, props)
			}
			currentName = ""
			props = make(map[string]string)
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			if parts[0] == "Id" {
				currentName = parts[1]
			}
			props[parts[0]] = parts[1]
		}
	}

	if currentName != "" {
		m.applyServiceProps(services, currentName, props)
	}
}

// parseMemoryCurrent parses systemd MemoryCurrent value.
// systemd returns "[not set]" or uint64 max when memory accounting is
// unavailable; both should be treated as 0.
func parseMemoryCurrent(v string) uint64 {
	if v == "" || v == "[not set]" {
		return 0
	}
	var n uint64
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0
	}
	if n == ^uint64(0) {
		return 0
	}
	return n
}

func (m *ServiceManager) applyServiceProps(services []ServiceInfo, id string, props map[string]string) {
	for i := range services {
		if services[i].Name+".service" == id || services[i].Name == id {
			if v, ok := props["MainPID"]; ok {
				fmt.Sscanf(v, "%d", &services[i].PID)
			}
			if v, ok := props["MemoryCurrent"]; ok {
				services[i].MemoryBytes = parseMemoryCurrent(v)
			}
			if v, ok := props["UnitFileState"]; ok {
				services[i].UnitFileState = v
				services[i].Enabled = v == "enabled"
			}
			if v, ok := props["ActiveState"]; ok {
				services[i].State = v
			}
			if v, ok := props["SubState"]; ok {
				services[i].SubState = v
			}
			if v, ok := props["Description"]; ok {
				services[i].Description = v
			}
			break
		}
	}
}

// Get returns info for a specific service.
func (m *ServiceManager) Get(ctx context.Context, name string) (*ServiceInfo, error) {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "show", name+".service",
		"--property=ActiveState,SubState,MainPID,MemoryCurrent,Description,UnitFileState")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}

	svc := &ServiceInfo{
		Name: name,
	}

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "ActiveState":
			svc.State = value
		case "SubState":
			svc.SubState = value
		case "MainPID":
			fmt.Sscanf(value, "%d", &svc.PID)
		case "MemoryCurrent":
			svc.MemoryBytes = parseMemoryCurrent(value)
		case "Description":
			svc.Description = value
		case "UnitFileState":
			svc.UnitFileState = value
			svc.Enabled = value == "enabled"
		}
	}

	// 托管服务：读 unit 文件补元数据
	if shortName := UnitName(name + ".service"); shortName != "" {
		if content, _ := readUnitFile(shortName); content != "" {
			ParseUnitMeta(content, svc)
		}
	}

	return svc, nil
}

// Start starts a service.
func (m *ServiceManager) Start(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	info, err := m.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get service state: %w", err)
	}
	if info.State == "active" {
		return fmt.Errorf("service %s is already running", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "start", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start service: %s", output)
	}
	log.Printf("service: started %s", name)
	return nil
}

// Stop stops a service.
func (m *ServiceManager) Stop(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	info, err := m.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get service state: %w", err)
	}
	if info.State == "inactive" || info.State == "failed" {
		return fmt.Errorf("service %s is already stopped", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "stop", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop service: %s", output)
	}
	log.Printf("service: stopped %s", name)
	return nil
}

// Restart restarts a service.
func (m *ServiceManager) Restart(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "restart", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart service: %s", output)
	}
	log.Printf("service: restarted %s", name)
	return nil
}

// Enable enables a service for auto-start.
func (m *ServiceManager) Enable(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	if m.isEnabled(ctx, name) {
		return fmt.Errorf("service %s is already enabled", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "enable", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to enable service: %s", output)
	}
	log.Printf("service: enabled %s", name)
	return nil
}

// Disable disables a service from auto-start.
func (m *ServiceManager) Disable(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	if !m.isEnabled(ctx, name) {
		return fmt.Errorf("service %s is already disabled", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "disable", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to disable service: %s", output)
	}
	log.Printf("service: disabled %s", name)
	return nil
}

// GetLogs returns recent logs for a service.
func (m *ServiceManager) GetLogs(ctx context.Context, name string, tail int, since string) ([]LogLine, error) {
	args := []string{
		"-u", name + ".service",
		"--no-pager",
		"--output=json",
	}

	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}

	if since != "" {
		args = append(args, "--since", since)
	}

	stdout, _, exitCode, err := m.executor.Run(ctx, "journalctl", args...)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	var logs []LogLine
	lines := strings.Split(stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry journalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			logs = append(logs, LogLine{
				Message: line,
				Time:    time.Now().Format("2006-01-02 15:04:05"),
			})
			continue
		}

		logTime := ""
		if entry.RealtimeTimestamp != "" {
			var usec int64
			fmt.Sscanf(entry.RealtimeTimestamp, "%d", &usec)
			t := time.Unix(usec/1000000, (usec%1000000)*1000)
			logTime = t.Format("2006-01-02 15:04:05")
		}

		priority := "info"
		switch entry.Priority {
		case "0":
			priority = "emerg"
		case "1":
			priority = "alert"
		case "2":
			priority = "crit"
		case "3":
			priority = "err"
		case "4":
			priority = "warn"
		case "5":
			priority = "notice"
		case "6":
			priority = "info"
		case "7":
			priority = "debug"
		}

		logs = append(logs, LogLine{
			Time:     logTime,
			Message:  entry.Message,
			Priority: priority,
		})
	}

	return logs, nil
}

// isEnabled checks if a service is enabled.
func (m *ServiceManager) isEnabled(ctx context.Context, name string) bool {
	stdout, _, _, _ := m.executor.Run(ctx, "systemctl", "is-enabled", name+".service")
	// systemctl is-enabled returns exit code 1 for disabled services,
	// so we check the output string instead of relying on exit code.
	return strings.TrimSpace(stdout) == "enabled"
}

// serviceExists checks if a service unit exists on the system.
func (m *ServiceManager) serviceExists(ctx context.Context, name string) bool {
	_, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "cat", name+".service")
	return err == nil && exitCode == 0
}

// requireServiceExists returns an error if the service does not exist.
func (m *ServiceManager) requireServiceExists(ctx context.Context, name string) error {
	if !m.serviceExists(ctx, name) {
		return fmt.Errorf("service %s does not exist", name)
	}
	return nil
}

// ============================================================
// Managed unit CRUD（面板托管服务）
// ============================================================

// CreateManaged 生成 unit 文件、daemon-reload、按需 enable/start。
// 已存在同名 unit 返回错误。全程持 m.mu 防并发创建同名。
func (m *ServiceManager) CreateManaged(ctx context.Context, spec *ManagedUnitSpec) error {
	if err := ValidateManagedName(spec.Name); err != nil {
		return err
	}

	// runtime 补全：前端只传 runtime_version_id，后端查 DB 补 lang/exact。
	if err := m.fillRuntime(ctx, spec); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.managedUnitExists(spec.Name) {
		return fmt.Errorf("托管服务 %s 已存在", spec.Name)
	}

	content, err := RenderUnit(spec)
	if err != nil {
		return fmt.Errorf("生成 unit 文件失败: %w", err)
	}
	if err := writeUnitFile(spec.Name, content); err != nil {
		return fmt.Errorf("写 unit 文件失败: %w", err)
	}
	// daemon-reload 失败：回滚（删 unit 文件），避免孤儿文件阻塞重试。
	if err := m.daemonReload(ctx); err != nil {
		_ = removeUnitFile(spec.Name)
		return fmt.Errorf("daemon-reload 失败（已回滚）: %w", err)
	}
	if spec.AutoStart {
		if err := m.enableManaged(ctx, spec.Name); err != nil {
			// enable 失败：回滚 unit 文件 + reload。
			_ = removeUnitFile(spec.Name)
			_ = m.daemonReload(ctx)
			return fmt.Errorf("enable 失败（已回滚）: %w", err)
		}
		if err := m.startManaged(ctx, spec.Name); err != nil {
			// start 失败：disable + 回滚 unit 文件 + reload。
			_, _, _ = m.executor.RunCombined(ctx, "systemctl", "disable", managedUnitPrefix+spec.Name+managedUnitSuffix)
			_ = removeUnitFile(spec.Name)
			_ = m.daemonReload(ctx)
			return fmt.Errorf("start 失败（已回滚）: %w", err)
		}
	}
	return nil
}

// UpdateManaged 重写 unit 文件 + daemon-reload。
// 运行中则 restart 使新配置生效；enabled 状态按 AutoStart 切换。
// 全程持 m.mu 防并发更新/删除。
func (m *ServiceManager) UpdateManaged(ctx context.Context, spec *ManagedUnitSpec) error {
	if err := ValidateManagedName(spec.Name); err != nil {
		return err
	}

	// runtime 补全
	if err := m.fillRuntime(ctx, spec); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.managedUnitExists(spec.Name) {
		return fmt.Errorf("托管服务 %s 不存在", spec.Name)
	}

	content, err := RenderUnit(spec)
	if err != nil {
		return fmt.Errorf("生成 unit 文件失败: %w", err)
	}
	if err := writeUnitFile(spec.Name, content); err != nil {
		return fmt.Errorf("写 unit 文件失败: %w", err)
	}
	if err := m.daemonReload(ctx); err != nil {
		return err
	}

	// AutoStart 状态切换
	wasEnabled := m.isEnabled(ctx, managedUnitPrefix+spec.Name)
	if spec.AutoStart && !wasEnabled {
		if err := m.enableManaged(ctx, spec.Name); err != nil {
			return err
		}
	} else if !spec.AutoStart && wasEnabled {
		if err := m.disableManaged(ctx, spec.Name); err != nil {
			return err
		}
	}

	// 运行中则 restart 使新配置生效（而非 startManaged 的幂等 no-op）
	info, err := m.Get(ctx, managedUnitPrefix+spec.Name)
	if err == nil && info.State == "active" {
		fullName := managedUnitPrefix + spec.Name + managedUnitSuffix
		output, exitCode, rerr := m.executor.RunCombined(ctx, "systemctl", "restart", fullName)
		if rerr != nil || exitCode != 0 {
			return fmt.Errorf("unit 已更新但重启失败: %s", output)
		}
	}
	return nil
}

// DeleteManaged 停止 + disable + 删 unit 文件 + daemon-reload。
// 全程持 m.mu 防并发。stop 失败返回错误（避免孤儿进程）。
func (m *ServiceManager) DeleteManaged(ctx context.Context, name string) error {
	if err := ValidateManagedName(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.managedUnitExists(name) {
		return fmt.Errorf("托管服务 %s 不存在", name)
	}

	fullName := managedUnitPrefix + name + managedUnitSuffix

	// 先 disable（best-effort，可能本来就未 enable）
	if _, _, err := m.executor.RunCombined(ctx, "systemctl", "disable", fullName); err != nil {
		log.Printf("systemd: disable %s during delete: %v", fullName, err)
	}

	// 停止：失败则返回错误，避免删 unit 后留孤儿进程。
	if _, _, err := m.executor.RunCombined(ctx, "systemctl", "stop", fullName); err != nil {
		return fmt.Errorf("停止 %s 失败，未删除 unit（避免孤儿进程）: %w", fullName, err)
	}

	if err := removeUnitFile(name); err != nil {
		return fmt.Errorf("删除 unit 文件失败: %w", err)
	}
	return m.daemonReload(ctx)
}

// fillRuntime 当 spec.RuntimeVersionID > 0 时查 DB 补 RuntimeLang/RuntimeExact，
// 并校验 runtime 状态为 installed。前端只传 ID，lang/exact 由后端补全，
// 避免前端传错或不一致。
func (m *ServiceManager) fillRuntime(ctx context.Context, spec *ManagedUnitSpec) error {
	if spec.RuntimeVersionID <= 0 {
		return nil
	}
	if m.runtime == nil {
		return fmt.Errorf("runtime 查询未配置，无法绑定运行时版本 %d", spec.RuntimeVersionID)
	}
	lang, exact, status, err := m.runtime.GetRuntime(ctx, spec.RuntimeVersionID)
	if err != nil {
		return fmt.Errorf("查询运行时版本 %d 失败: %w", spec.RuntimeVersionID, err)
	}
	if status != "installed" {
		return fmt.Errorf("运行时版本 %d 状态为 %s，无法绑定（需先安装）", spec.RuntimeVersionID, status)
	}
	spec.RuntimeLang = lang
	spec.RuntimeExact = exact
	return nil
}

// --- managed helpers ---

func (m *ServiceManager) managedUnitExists(name string) bool {
	path := UnitFilePath(name)
	_, err := os.Stat(path)
	return err == nil
}

func (m *ServiceManager) daemonReload(ctx context.Context) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "daemon-reload")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("daemon-reload 失败: %s", output)
	}
	return nil
}

func (m *ServiceManager) enableManaged(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "enable",
		managedUnitPrefix+name+managedUnitSuffix)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("enable 失败: %s", output)
	}
	return nil
}

func (m *ServiceManager) disableManaged(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "disable",
		managedUnitPrefix+name+managedUnitSuffix)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("disable 失败: %s", output)
	}
	return nil
}

// startManaged 启动托管服务。已 active 视为成功（幂等）。
func (m *ServiceManager) startManaged(ctx context.Context, name string) error {
	fullName := managedUnitPrefix + name + managedUnitSuffix
	info, err := m.Get(ctx, managedUnitPrefix+name)
	if err == nil && info.State == "active" {
		return nil
	}
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "start", fullName)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("start 失败: %s", output)
	}
	return nil
}
