package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra/executor"
	"easyserver/internal/infra/mise"
	"easyserver/internal/systemd"
)

// TimerManager 是把定时任务承载在 systemd timer 上的编排器（ADR-0004）。
// 每个任务 = 一对 .timer（OnCalendar 触发）+ .service（mise exec 执行），
// 状态读 systemctl，日志走 journald，重试/超时交给 systemd 原生。
//
// 任务以 unit 名（不含前缀）为唯一标识，无 DB 记录；unit 文件注释
// （ManagedBy=easyserver-cron + Runtime*）是反查与编辑回显的依据。
type TimerManager struct {
	mu       sync.Mutex // 保护 unit CRUD 并发
	executor executor.CommandExecutor
	provider mise.Provider
	runtime  RuntimeLookup // 可 nil，绑定 runtime 时必填
}

// RuntimeLookup 查询 runtime_version 表补 lang/exact/status。
type RuntimeLookup interface {
	GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error)
}

// NewTimerManager 创建 TimerManager。
func NewTimerManager(exec executor.CommandExecutor, p mise.Provider, runtime RuntimeLookup) *TimerManager {
	return &TimerManager{executor: exec, provider: p, runtime: runtime}
}

// List 返回全部定时任务。扫描 /etc/systemd/system/ 下的 easyserver-cron-*.timer，
// 保证新建但未 enable 的任务也能列出。状态用 systemctl show 批量补全。
func (m *TimerManager) List(ctx context.Context) ([]CronTask, error) {
	names, err := m.listTimerNames()
	if err != nil {
		return nil, err
	}
	tasks := make([]CronTask, 0, len(names))
	for _, name := range names {
		t, err := m.loadTask(ctx, name)
		if err != nil {
			log.Printf("cron: fail to load task %s: %v", name, err)
			continue
		}
		tasks = append(tasks, *t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })
	return tasks, nil
}

// listTimerNames 扫描 unit 目录，返回全部 cron timer 的 <name>。
func (m *TimerManager) listTimerNames() ([]string, error) {
	entries, err := os.ReadDir(systemd.UnitDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 unit 目录失败: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name := systemd.CronTimerName(e.Name()); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// Get 返回单个定时任务。
func (m *TimerManager) Get(ctx context.Context, name string) (*CronTask, error) {
	return m.loadTask(ctx, name)
}

// Create 创建定时任务：写 .timer + .service、daemon-reload、按需 enable。
func (m *TimerManager) Create(ctx context.Context, spec *CronTask) error {
	if err := systemd.ValidateCronName(spec.Name); err != nil {
		return err
	}
	if err := m.fillRuntime(ctx, spec); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.timerUnitExists(spec.Name) {
		return fmt.Errorf("定时任务 %s 已存在", spec.Name)
	}

	// 命令先落盘为脚本，unit 的 ExecStart 指其路径。
	if err := writeTaskCommand(spec.Name, spec.Command); err != nil {
		return fmt.Errorf("写入命令脚本失败: %w", err)
	}

	timerContent, err := m.renderTimer(spec)
	if err != nil {
		return err
	}
	svcContent, err := m.renderService(spec)
	if err != nil {
		return err
	}

	if err := systemd.WriteCronUnitFile(systemd.CronTimerFileName(spec.Name), timerContent); err != nil {
		return err
	}
	if err := systemd.WriteCronUnitFile(systemd.CronServiceFileName(spec.Name), svcContent); err != nil {
		_ = systemd.RemoveCronUnitFile(systemd.CronTimerFileName(spec.Name))
		return err
	}
	// daemon-reload 失败：回滚两个 unit 文件。
	if err := m.daemonReload(ctx); err != nil {
		_ = systemd.RemoveCronUnitFile(systemd.CronTimerFileName(spec.Name))
		_ = systemd.RemoveCronUnitFile(systemd.CronServiceFileName(spec.Name))
		_ = m.daemonReload(ctx)
		return fmt.Errorf("daemon-reload 失败（已回滚）: %w", err)
	}
	if spec.Enabled {
		if err := m.enableTimer(ctx, spec.Name); err != nil {
			m.rollbackCreate(ctx, spec.Name)
			return fmt.Errorf("enable 失败（已回滚）: %w", err)
		}
	}
	return nil
}

// Update 更新定时任务：重写 unit 文件 + daemon-reload，运行中则 restart。
func (m *TimerManager) Update(ctx context.Context, spec *CronTask) error {
	if err := systemd.ValidateCronName(spec.Name); err != nil {
		return err
	}
	if err := m.fillRuntime(ctx, spec); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.timerUnitExists(spec.Name) {
		return fmt.Errorf("定时任务 %s 不存在", spec.Name)
	}

	oldTimer, _ := systemd.ReadCronUnitFile(systemd.CronTimerFileName(spec.Name))
	oldSvc, _ := systemd.ReadCronUnitFile(systemd.CronServiceFileName(spec.Name))
	wasEnabled := m.timerEnabled(ctx, spec.Name)
	wasTimerActive := m.timerActive(ctx, spec.Name)

	rollback := func() {
		if oldTimer != "" {
			_ = systemd.WriteCronUnitFile(systemd.CronTimerFileName(spec.Name), oldTimer)
		}
		if oldSvc != "" {
			_ = systemd.WriteCronUnitFile(systemd.CronServiceFileName(spec.Name), oldSvc)
		}
		_ = m.daemonReload(ctx)
		if wasEnabled && !m.timerEnabled(ctx, spec.Name) {
			_ = m.enableTimer(ctx, spec.Name)
		} else if !wasEnabled && m.timerEnabled(ctx, spec.Name) {
			_ = m.disableTimer(ctx, spec.Name)
		}
		if wasTimerActive {
			_ = m.restartTimer(ctx, spec.Name)
		}
	}

	// 命令落盘为脚本（覆盖旧内容），unit 的 ExecStart 指其路径。
	if err := writeTaskCommand(spec.Name, spec.Command); err != nil {
		return fmt.Errorf("写入命令脚本失败: %w", err)
	}

	timerContent, err := m.renderTimer(spec)
	if err != nil {
		return err
	}
	svcContent, err := m.renderService(spec)
	if err != nil {
		return err
	}

	if err := systemd.WriteCronUnitFile(systemd.CronTimerFileName(spec.Name), timerContent); err != nil {
		return err
	}
	if err := systemd.WriteCronUnitFile(systemd.CronServiceFileName(spec.Name), svcContent); err != nil {
		rollback()
		return err
	}
	if err := m.daemonReload(ctx); err != nil {
		rollback()
		return fmt.Errorf("daemon-reload 失败（已回滚）: %w", err)
	}

	// enable 状态切换
	nowEnabled := m.timerEnabled(ctx, spec.Name)
	if spec.Enabled && !nowEnabled {
		if err := m.enableTimer(ctx, spec.Name); err != nil {
			rollback()
			return fmt.Errorf("enable 失败（已回滚）: %w", err)
		}
	} else if !spec.Enabled && nowEnabled {
		if err := m.disableTimer(ctx, spec.Name); err != nil {
			rollback()
			return fmt.Errorf("disable 失败（已回滚）: %w", err)
		}
	}

	// 运行中则 restart 使新调度/命令生效
	if m.timerActive(ctx, spec.Name) {
		if err := m.restartTimer(ctx, spec.Name); err != nil {
			rollback()
			return fmt.Errorf("unit 已更新但重启失败（已回滚）: %w", err)
		}
	}
	return nil
}

// Delete 删除定时任务：stop + disable + 删两个 unit 文件 + daemon-reload。
func (m *TimerManager) Delete(ctx context.Context, name string) error {
	if err := systemd.ValidateCronName(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.timerUnitExists(name) {
		return fmt.Errorf("定时任务 %s 不存在", name)
	}

	timerFull := systemd.CronTimerFileName(name)
	svcFull := systemd.CronServiceFileName(name)

	// 先 disable + stop（best-effort，可能本来就未启用）
	m.executor.RunCombined(ctx, "systemctl", "disable", timerFull)
	m.executor.RunCombined(ctx, "systemctl", "stop", timerFull)
	m.executor.RunCombined(ctx, "systemctl", "stop", svcFull)

	if err := systemd.RemoveCronUnitFile(timerFull); err != nil {
		return fmt.Errorf("删除 timer unit 失败: %w", err)
	}
	if err := systemd.RemoveCronUnitFile(svcFull); err != nil {
		return fmt.Errorf("删除 service unit 失败: %w", err)
	}
	_ = removeTaskCommand(name) // 命令脚本一并清理（best-effort）
	return m.daemonReload(ctx)
}

// Enable 启用定时任务（systemctl enable timer）。
func (m *TimerManager) Enable(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.timerUnitExists(name) {
		return fmt.Errorf("定时任务 %s 不存在", name)
	}
	return m.enableTimer(ctx, name)
}

// Disable 禁用定时任务（disable + stop timer）。
func (m *TimerManager) Disable(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.timerUnitExists(name) {
		return fmt.Errorf("定时任务 %s 不存在", name)
	}
	if err := m.disableTimer(ctx, name); err != nil {
		return err
	}
	// disable 后立刻 stop，避免已排定的下一次触发继续跑
	return m.stopTimer(ctx, name)
}

// RunNow 立即手动执行任务：systemctl start <name>.service，与 timer 触发同路径。
func (m *TimerManager) RunNow(ctx context.Context, name string) error {
	if !m.timerUnitExists(name) {
		return fmt.Errorf("定时任务 %s 不存在", name)
	}
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "start", systemd.CronServiceFileName(name))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("立即执行失败: %s", output)
	}
	return nil
}

// GetLogs 返回任务的 journald 日志。
func (m *TimerManager) GetLogs(ctx context.Context, name string, tail int) ([]LogLine, error) {
	args := []string{"-u", systemd.CronServiceFileName(name), "--no-pager", "--output=json"}
	if tail > 0 {
		args = append(args, "-n", strconv.Itoa(tail))
	}
	stdout, _, exitCode, err := m.executor.Run(ctx, "journalctl", args...)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("读取日志失败: %w", err)
	}
	return parseJournalLines(stdout), nil
}

// --- rendering + fill ---

func (m *TimerManager) renderTimer(spec *CronTask) (string, error) {
	return systemd.RenderTimer(&systemd.TimerSpec{
		Name:        spec.Name,
		Description: spec.Description,
		OnCalendar:  spec.Schedule,
		Persistent:  spec.Persistent,
	})
}

func (m *TimerManager) renderService(spec *CronTask) (string, error) {
	return systemd.RenderCronService(&systemd.TimerSpec{
		Name:             spec.Name,
		Description:      spec.Description,
		ExecStart:        taskCommandPath(spec.Name), // 命令已统一落盘为脚本，ExecStart 指向路径（单行）
		Dir:              spec.WorkDir,
		Env:              parseEnvMap(spec.EnvVars),
		MaxRetry:         spec.MaxRetry,
		RestartDelay:     5,
		Timeout:          spec.Timeout,
		RuntimeVersionID: spec.RuntimeVersionID,
		RuntimeLang:      spec.RuntimeLang,
		RuntimeExact:     spec.RuntimeExact,
	}, m.provider)
}

// fillRuntime 当绑定 runtime 时查 DB 补 lang/exact，并校验已安装。
func (m *TimerManager) fillRuntime(ctx context.Context, spec *CronTask) error {
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

// --- load/parse ---

// loadTask 从 unit 文件 + systemctl show 组装 CronTask。
func (m *TimerManager) loadTask(ctx context.Context, name string) (*CronTask, error) {
	timerContent, err := systemd.ReadCronUnitFile(systemd.CronTimerFileName(name))
	if err != nil || timerContent == "" {
		return nil, fmt.Errorf("任务 %s 的 timer unit 缺失", name)
	}
	svcContent, _ := systemd.ReadCronUnitFile(systemd.CronServiceFileName(name))

	t := &CronTask{Name: name}
	parseTimerUnit(timerContent, t)
	if svcContent != "" {
		parseServiceUnit(m.provider, svcContent, t)
	}

	m.fillStatus(ctx, t)
	return t, nil
}

// fillStatus 用 systemctl show 补全状态字段。
func (m *TimerManager) fillStatus(ctx context.Context, t *CronTask) {
	// timer：enabled + active + next elapse
	if out, _, _, _ := m.executor.Run(ctx, "systemctl", "is-enabled", systemd.CronTimerFileName(t.Name)); strings.TrimSpace(out) == "enabled" {
		t.Enabled = true
	}
	showTimer := m.show(ctx, systemd.CronTimerFileName(t.Name),
		"--property=ActiveState,NextElapseRealtime")
	// systemd 对永不触发的 timer 用远未来哨兵 "39766-..." 表示；此时不填下次执行。
	if v, ok := showTimer["NextElapseRealtime"]; ok && v != "" && !strings.HasPrefix(v, "39766") {
		t.NextRun = v
	}
	showSvc := m.show(ctx, systemd.CronServiceFileName(t.Name),
		"--property=ActiveState,Result,ExecMainExitTimestamp")

	t.Status = "inactive"
	if v, ok := showSvc["ActiveState"]; ok {
		t.Status = v
	}
	if v, ok := showSvc["Result"]; ok {
		t.LastResult = v
	}
	if v, ok := showSvc["ExecMainExitTimestamp"]; ok {
		t.LastRun = v
	}
}

// show 执行 systemctl show 并把 KEY=VAL 解析成 map。
func (m *TimerManager) show(ctx context.Context, unit string, props ...string) map[string]string {
	args := append([]string{"show", unit}, props...)
	out, _, _, _ := m.executor.Run(ctx, "systemctl", args...)
	res := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			res[k] = v
		}
	}
	return res
}

// --- parse unit helpers ---

// parseTimerUnit 从 .timer 文件解析 OnCalendar / Persistent / Description。
func parseTimerUnit(content string, t *CronTask) {
	section := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}
		switch section {
		case "[Unit]":
			if strings.HasPrefix(line, "Description=") {
				t.Description = strings.TrimPrefix(line, "Description=")
			}
		case "[Timer]":
			if strings.HasPrefix(line, "OnCalendar=") {
				t.Schedule = strings.TrimPrefix(line, "OnCalendar=")
			}
			if strings.HasPrefix(line, "Persistent=") {
				t.Persistent = strings.TrimPrefix(line, "Persistent=") == "yes"
			}
		}
	}
}

// parseServiceUnit 从 .service 文件解析命令/工作目录/超时/重试/runtime 注释。
func parseServiceUnit(p mise.Provider, content string, t *CronTask) {
	section := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}
		if section == "[Unit]" && strings.HasPrefix(line, "# ") {
			kv := strings.SplitN(strings.TrimPrefix(line, "# "), "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch strings.TrimSpace(kv[0]) {
			case "RuntimeVersionID":
				fmt.Sscanf(strings.TrimSpace(kv[1]), "%d", &t.RuntimeVersionID)
			case "RuntimeLang":
				t.RuntimeLang = strings.TrimSpace(kv[1])
			case "RuntimeExact":
				t.RuntimeExact = strings.TrimSpace(kv[1])
			}
			continue
		}
		if section != "[Service]" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "ExecStart="):
			execStart := strings.TrimPrefix(line, "ExecStart=")
			if t.RuntimeVersionID > 0 {
				execStart = p.Unwrap(t.RuntimeLang, t.RuntimeExact, execStart)
			}
			// ExecStart 指向任务命令脚本时，读回脚本内容作为命令（支持多行回显）。
			if execStart == taskCommandPath(t.Name) {
				if cmd, err := readTaskCommand(t.Name); err == nil {
					t.Command = cmd
				}
			} else {
				t.Command = execStart
			}
		case strings.HasPrefix(line, "WorkingDirectory="):
			dir := strings.TrimPrefix(line, "WorkingDirectory=")
			if len(dir) >= 2 && dir[0] == '"' && dir[len(dir)-1] == '"' {
				dir = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(dir[1 : len(dir)-1])
			}
			t.WorkDir = dir
		case strings.HasPrefix(line, "Environment="):
			k, v := parseEnvLine(line)
			if k != "" {
				t.EnvVars += k + "=" + v + "\n"
			}
		case strings.HasPrefix(line, "StartLimitBurst="):
			var burst int
			fmt.Sscanf(strings.TrimPrefix(line, "StartLimitBurst="), "%d", &burst)
			t.MaxRetry = burst - 1
		case strings.HasPrefix(line, "TimeoutStartSec="):
			fmt.Sscanf(strings.TrimPrefix(line, "TimeoutStartSec="), "%d", &t.Timeout)
		}
	}
	t.EnvVars = strings.TrimSpace(t.EnvVars)
}

// parseEnvLine 解析 "KEY=VALUE" 或 KEY="quoted value"。
func parseEnvLine(line string) (key, val string) {
	ll := strings.TrimPrefix(line, "Environment=")
	eq := strings.Index(ll, "=")
	if eq < 0 {
		return "", ""
	}
	key = ll[:eq]
	val = ll[eq+1:]
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(val[1 : len(val)-1])
	}
	return key, val
}

// parseEnvMap 把 "KEY=VALUE\n..." 转成 map（供渲染）。
func parseEnvMap(envStr string) map[string]string {
	if strings.TrimSpace(envStr) == "" {
		return nil
	}
	out := make(map[string]string)
	for _, line := range strings.Split(envStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			out[k] = v
		}
	}
	return out
}

// --- 任务命令落盘 ---
// 所有命令统一写成可执行脚本落盘，ExecStart 指向脚本路径（保持单行），
// 从而支持多行命令且规避 systemd unit 换行注入。脚本目录与脚本库分离。

const taskCommandDir = "/opt/easyserver/scripts/tasks"

func taskCommandPath(name string) string {
	return filepath.Join(taskCommandDir, name)
}

// writeTaskCommand 把命令内容落盘为可执行脚本（shebang + chmod 0755）。
func writeTaskCommand(name, command string) error {
	if err := os.MkdirAll(taskCommandDir, 0o755); err != nil {
		return err
	}
	content := "#!/bin/bash\n" + command
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	path := taskCommandPath(name)
	tmp, err := os.CreateTemp(taskCommandDir, name+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时命令文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入命令文件失败: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("设置命令文件权限失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭命令文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("落盘命令文件失败: %w", err)
	}
	return nil
}

func removeTaskCommand(name string) error {
	return os.Remove(taskCommandPath(name)) // 文件不存在时返回 os.ErrNotExist，调用方按需忽略
}

// readTaskCommand 读回命令内容，剥离 shebang 首行，供编辑表单回显。
func readTaskCommand(name string) (string, error) {
	data, err := os.ReadFile(taskCommandPath(name))
	if err != nil {
		return "", err
	}
	if i := strings.IndexByte(string(data), '\n'); i >= 0 {
		return strings.TrimSuffix(string(data[i+1:]), "\n"), nil
	}
	return "", nil
}

// --- systemctl helpers ---

func (m *TimerManager) daemonReload(ctx context.Context) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "daemon-reload")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("daemon-reload 失败: %s", output)
	}
	return nil
}

func (m *TimerManager) enableTimer(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "enable", systemd.CronTimerFileName(name))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("enable 失败: %s", output)
	}
	return nil
}

func (m *TimerManager) disableTimer(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "disable", systemd.CronTimerFileName(name))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("disable 失败: %s", output)
	}
	return nil
}

func (m *TimerManager) stopTimer(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "stop", systemd.CronTimerFileName(name))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("stop 失败: %s", output)
	}
	return nil
}

func (m *TimerManager) restartTimer(ctx context.Context, name string) error {
	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "restart", systemd.CronTimerFileName(name))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("restart 失败: %s", output)
	}
	return nil
}

func (m *TimerManager) timerUnitExists(name string) bool {
	_, err := os.Stat(systemd.CronTimerPath(name))
	return err == nil
}

func (m *TimerManager) timerEnabled(ctx context.Context, name string) bool {
	out, _, _, _ := m.executor.Run(ctx, "systemctl", "is-enabled", systemd.CronTimerFileName(name))
	return strings.TrimSpace(out) == "enabled"
}

func (m *TimerManager) timerActive(ctx context.Context, name string) bool {
	props := m.show(ctx, systemd.CronTimerFileName(name), "--property=ActiveState")
	return props["ActiveState"] == "active"
}

// rollbackCreate 回滚 Create 失败时已写入的 unit 与命令脚本文件。
func (m *TimerManager) rollbackCreate(ctx context.Context, name string) {
	_ = systemd.RemoveCronUnitFile(systemd.CronTimerFileName(name))
	_ = systemd.RemoveCronUnitFile(systemd.CronServiceFileName(name))
	_ = removeTaskCommand(name)
	_ = m.daemonReload(ctx)
}

// parseJournalLines 解析 journalctl --output=json 输出为日志行。
func parseJournalLines(stdout string) []LogLine {
	var logs []LogLine
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Message           string `json:"MESSAGE"`
			RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
			Priority          string `json:"PRIORITY"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			logs = append(logs, LogLine{Message: line})
			continue
		}
		logs = append(logs, LogLine{
			Time:     formatJournalTime(entry.RealtimeTimestamp),
			Message:  entry.Message,
			Priority: priorityName(entry.Priority),
		})
	}
	return logs
}

// formatJournalTime 把微秒时间戳格式化为 "2006-01-02 15:04:05"。空值返回空串。
func formatJournalTime(usec string) string {
	if usec == "" {
		return ""
	}
	var n int64
	if _, err := fmt.Sscanf(usec, "%d", &n); err != nil {
		return ""
	}
	return time.Unix(n/1e6, (n%1e6)*1000).Format("2006-01-02 15:04:05")
}

// priorityName 把 journald 优先级数字映射为可读名。
func priorityName(p string) string {
	names := map[string]string{
		"0": "emerg", "1": "alert", "2": "crit", "3": "err",
		"4": "warn", "5": "notice", "6": "info", "7": "debug",
	}
	if n, ok := names[p]; ok {
		return n
	}
	return "info"
}

// LogLine 表示一条 journald 日志。
type LogLine struct {
	Time     string `json:"time"`
	Message  string `json:"message"`
	Priority string `json:"priority"`
}
