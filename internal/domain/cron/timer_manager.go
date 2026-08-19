package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"easyserver/internal/domain/systemd"
	"easyserver/internal/infra/errx"
	"easyserver/internal/infra/mise"
	infrasystemd "easyserver/internal/infra/systemd"
	"easyserver/internal/util"
)

// TimerManager 是把定时任务承载在 systemd timer 上的编排器（ADR-0004）。
// 每个任务 = 一对 .timer（OnCalendar 触发）+ .service（mise exec 执行），
// 状态读 D-Bus 客户端，日志走 journald，重试/超时交给 systemd 原生。
//
// 任务以 unit 名（不含前缀）为唯一标识，无 DB 记录；unit 文件注释
// （ManagedBy=easyserver-cron + Runtime*）是反查与编辑回显的依据。
type TimerManager struct {
	mu       sync.Mutex // 保护 unit CRUD 并发
	provider mise.Provider
	runtime  RuntimeLookup // 可 nil，绑定 runtime 时必填
	client   infrasystemd.SystemdClient
}

// RuntimeLookup 校验运行时绑定：lang@exact 是否已安装（ADR-0009 目录权威）。
type RuntimeLookup interface {
	Installed(ctx context.Context, lang, exact string) bool
}

// NewTimerManager 创建 TimerManager。
func NewTimerManager(p mise.Provider, runtime RuntimeLookup) *TimerManager {
	return &TimerManager{provider: p, runtime: runtime, client: infrasystemd.DefaultClient()}
}

// SetClient overrides the systemd client (useful for unit tests).
func (m *TimerManager) SetClient(c infrasystemd.SystemdClient) {
	m.client = c
}

func (m *TimerManager) getClient() infrasystemd.SystemdClient {
	if m.client != nil {
		return m.client
	}
	return infrasystemd.DefaultClient()
}

// List 返回全部定时任务。扫描 /usr/local/lib/systemd/system/ 下的 easyserver-cron-*.timer，
// 保证新建但未 enable 的任务也能列出。状态用 D-Bus 批量补全。
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
		return errx.Conflict("定时任务 %s 已存在", spec.Name)
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
		return errx.NotFound("定时任务 %s 不存在", spec.Name)
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
		return errx.NotFound("定时任务 %s 不存在", name)
	}

	timerFull := systemd.CronTimerFileName(name)
	svcFull := systemd.CronServiceFileName(name)

	// 先 disable + stop，仅允许 "not loaded" / "NoSuchUnit" / "inactive" 等预期错误
	client := m.getClient()
	if _, err := client.DisableUnitFilesContext(ctx, []string{timerFull}, false); err != nil {
		if !strings.Contains(err.Error(), "not loaded") && !strings.Contains(err.Error(), "NoSuchUnit") && !strings.Contains(err.Error(), "does not exist") {
			log.Printf("cron: disable timer %s failed: %v", timerFull, err)
			return fmt.Errorf("disable timer 失败: %w", err)
		}
	}
	if _, err := client.StopUnitContext(ctx, timerFull, "replace"); err != nil {
		if !strings.Contains(err.Error(), "not loaded") && !strings.Contains(err.Error(), "NoSuchUnit") && !strings.Contains(err.Error(), "inactive") {
			log.Printf("cron: stop timer %s failed: %v", timerFull, err)
			return fmt.Errorf("stop timer 失败: %w", err)
		}
	}
	if _, err := client.StopUnitContext(ctx, svcFull, "replace"); err != nil {
		if !strings.Contains(err.Error(), "not loaded") && !strings.Contains(err.Error(), "NoSuchUnit") && !strings.Contains(err.Error(), "inactive") {
			log.Printf("cron: stop service %s failed: %v", svcFull, err)
			return fmt.Errorf("stop service 失败: %w", err)
		}
	}

	if err := systemd.RemoveCronUnitFile(timerFull); err != nil {
		return fmt.Errorf("删除 timer unit 失败: %w", err)
	}
	if err := systemd.RemoveCronUnitFile(svcFull); err != nil {
		return fmt.Errorf("删除 service unit 失败: %w", err)
	}
	_ = removeTaskCommand(name) // 命令脚本一并清理（best-effort）
	return m.daemonReload(ctx)
}

// Enable 启用定时任务（enable timer）。
func (m *TimerManager) Enable(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.timerUnitExists(name) {
		return errx.NotFound("定时任务 %s 不存在", name)
	}
	return m.enableTimer(ctx, name)
}

// Disable 禁用定时任务（disable + stop timer）。
func (m *TimerManager) Disable(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.timerUnitExists(name) {
		return errx.NotFound("定时任务 %s 不存在", name)
	}
	if err := m.disableTimer(ctx, name); err != nil {
		return err
	}
	// disable 后立刻 stop，避免已排定的下一次触发继续跑
	return m.stopTimer(ctx, name)
}

// RunNow 立即手动执行任务：start <name>.service，与 timer 触发同路径。
func (m *TimerManager) RunNow(ctx context.Context, name string) error {
	if !m.timerUnitExists(name) {
		return errx.NotFound("定时任务 %s 不存在", name)
	}
	svcName := systemd.CronServiceFileName(name)
	if _, err := m.getClient().StartUnitContext(ctx, svcName, "replace"); err != nil {
		return fmt.Errorf("立即执行失败: %w", err)
	}
	return nil
}

// GetRuns 返回任务的 journald 日志，按 invocation ID 分组为每次执行。
// 支持 since / until 时间窗口过滤（格式如 "2006-01-02" 或 "2006-01-02 15:04:05"）。
func (m *TimerManager) GetRuns(ctx context.Context, name string, limit int, since, until string) ([]CronRun, error) {
	args := []string{"-u", systemd.CronServiceFileName(name), "--no-pager", "--output=json"}
	if since != "" {
		args = append(args, "--since", since)
	}
	if until != "" {
		args = append(args, "--until", until)
	}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit*50))
	}
	stdout, err := exec.CommandContext(ctx, "journalctl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("读取日志失败: %w", err)
	}
	runs := parseJournalRuns(string(stdout))
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
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
		Name:         spec.Name,
		Description:  spec.Description,
		ExecStart:    taskCommandPath(spec.Name), // 命令已统一落盘为脚本，ExecStart 指向路径（单行）
		Dir:          spec.WorkDir,
		Env:          parseEnvMap(spec.EnvVars),
		MaxRetry:     spec.MaxRetry,
		RestartDelay: 5,
		Timeout:      spec.Timeout,
		Runtime:      spec.Runtime,
	}, m.provider)
}

// fillRuntime 当绑定 runtime 时校验 lang@exact 已安装。绑定键即字符串（ADR-0009）。
func (m *TimerManager) fillRuntime(ctx context.Context, spec *CronTask) error {
	if spec.Runtime == "" {
		return nil
	}
	lang, exact, _ := strings.Cut(spec.Runtime, "@")
	if lang == "" || exact == "" {
		return fmt.Errorf("无效的运行时绑定: %s", spec.Runtime)
	}
	if m.runtime == nil {
		return fmt.Errorf("runtime 查询未配置，无法绑定运行时 %s", spec.Runtime)
	}
	if !m.runtime.Installed(ctx, lang, exact) {
		return errx.NotFound("运行时 %s 未安装（需先到「运行环境管理」安装）", spec.Runtime)
	}
	return nil
}

// --- load/parse ---

// loadTask 从 unit 文件 + D-Bus 组装 CronTask。
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

	// 从 command.sh 读回原始用户脚本
	if script, err := readTaskCommand(name); err == nil && script != "" {
		t.Command = script
	}

	if err := m.fillStatus(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// fillStatus 用 D-Bus 补全状态字段。
func (m *TimerManager) fillStatus(ctx context.Context, t *CronTask) error {
	client := m.getClient()
	timerName := systemd.CronTimerFileName(t.Name)
	svcName := systemd.CronServiceFileName(t.Name)

	// Read UnitFileState from generic Unit interface
	if prop, err := client.GetUnitPropertyContext(ctx, timerName, "UnitFileState"); err != nil {
		if errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
			return err
		}
	} else if prop != nil {
		if str, ok := prop.Value.Value().(string); ok && str == "enabled" {
			t.Enabled = true
		}
	}

	// Query Timer-specific properties
	timerProps, err := client.GetUnitTypePropertiesContext(ctx, timerName, "org.freedesktop.systemd1.Timer")
	if err != nil {
		if errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
			return err
		}
	} else {
		// NextElapseUSecRealtime is uint64 microseconds since epoch
		if nextUsec, ok := timerProps["NextElapseUSecRealtime"].(uint64); ok && nextUsec > 0 && nextUsec != ^uint64(0) {
			t.NextRun = util.UnixMicros(int64(nextUsec)).Format(util.TimeLayout)
		}
	}

	// Query Service-specific properties
	svcProps, err := client.GetUnitTypePropertiesContext(ctx, svcName, "org.freedesktop.systemd1.Service")
	if err != nil {
		if errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
			return err
		}
	} else {
		t.Status = "inactive"
		if v, ok := svcProps["ActiveState"].(string); ok && v != "" {
			t.Status = v
		}
		if v, ok := svcProps["Result"].(string); ok && v != "" {
			t.LastResult = v
		}
		// ExecMainExitTimestamp is uint64 microseconds since epoch
		if exitUsec, ok := svcProps["ExecMainExitTimestamp"].(uint64); ok && exitUsec > 0 && exitUsec != ^uint64(0) {
			t.LastRun = util.UnixMicros(int64(exitUsec)).Format(util.TimeLayout)
		}
	}
	return nil
}

// --- parse unit helpers ---

// parseTimerUnit 从 .timer 文件解析 OnCalendar / Persistent / Description。
func parseTimerUnit(content string, t *CronTask) {
	section := ""
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}
		switch section {
		case "[Unit]":
			if after, ok := strings.CutPrefix(line, "Description="); ok {
				t.Description = after
			}
		case "[Timer]":
			if after, ok := strings.CutPrefix(line, "OnCalendar="); ok {
				t.Schedule = after
			}
			if after, ok := strings.CutPrefix(line, "Persistent="); ok {
				t.Persistent = after == "yes"
			}
		}
	}
}

// parseServiceUnit 从 .service 文件解析命令/工作目录/超时/重试/runtime 注释。
func parseServiceUnit(p mise.Provider, content string, t *CronTask) {
	section := ""
	var lang, exact string
	for line := range strings.SplitSeq(content, "\n") {
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
			case "RuntimeLang":
				lang = strings.TrimSpace(kv[1])
			case "RuntimeExact":
				exact = strings.TrimSpace(kv[1])
			}
			continue
		}
		if section != "[Service]" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "ExecStart="):
			execStart := strings.TrimPrefix(line, "ExecStart=")
			if lang != "" && exact != "" {
				execStart = p.Unwrap(lang, exact, execStart)
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
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, "StartLimitBurst="), "%d", &burst)
			t.MaxRetry = burst - 1
		case strings.HasPrefix(line, "TimeoutStartSec="):
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, "TimeoutStartSec="), "%d", &t.Timeout)
		}
	}
	t.EnvVars = strings.TrimSpace(t.EnvVars)
	// 由 Lang/Exact 组装绑定键（ADR-0009 绑定键即字符串）。
	if lang != "" && exact != "" {
		t.Runtime = lang + "@" + exact
	}
}

// parseEnvLine 解析 "KEY=VALUE" 或 KEY="quoted value"。
func parseEnvLine(line string) (key, val string) {
	ll := strings.TrimPrefix(line, "Environment=")
	before, after, ok := strings.Cut(ll, "=")
	if !ok {
		return "", ""
	}
	key = before
	val = after
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
	for line := range strings.SplitSeq(envStr, "\n") {
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

func taskCommandPath(name string) string {
	if err := systemd.ValidateCronName(name); err != nil {
		return ""
	}
	p := filepath.Join(taskCommandDir, name)
	rel, err := filepath.Rel(taskCommandDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return p
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
	if path == "" {
		return errors.New("invalid task name")
	}
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
	p := taskCommandPath(name)
	if p == "" {
		return errors.New("invalid task name")
	}
	return os.Remove(p) // 文件不存在时返回 os.ErrNotExist，调用方按需忽略
}

// readTaskCommand 读回命令内容，剥离 shebang 首行，供编辑表单回显。
func readTaskCommand(name string) (string, error) {
	p := taskCommandPath(name)
	if p == "" {
		return "", errors.New("invalid task name")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if i := strings.IndexByte(string(data), '\n'); i >= 0 {
		return strings.TrimSuffix(string(data[i+1:]), "\n"), nil
	}
	return "", nil
}

// --- systemd unit helpers ---

func (m *TimerManager) daemonReload(ctx context.Context) error {
	if err := m.getClient().ReloadContext(ctx); err != nil {
		return fmt.Errorf("daemon-reload 失败: %w", err)
	}
	return nil
}

func (m *TimerManager) enableTimer(ctx context.Context, name string) error {
	timerFile := systemd.CronTimerFileName(name)
	if _, _, err := m.getClient().EnableUnitFilesContext(ctx, []string{timerFile}, false, false); err != nil {
		return fmt.Errorf("enable 失败: %w", err)
	}
	return nil
}

func (m *TimerManager) disableTimer(ctx context.Context, name string) error {
	timerFile := systemd.CronTimerFileName(name)
	if _, err := m.getClient().DisableUnitFilesContext(ctx, []string{timerFile}, false); err != nil {
		return fmt.Errorf("disable 失败: %w", err)
	}
	return nil
}

func (m *TimerManager) stopTimer(ctx context.Context, name string) error {
	timerFile := systemd.CronTimerFileName(name)
	if _, err := m.getClient().StopUnitContext(ctx, timerFile, "replace"); err != nil {
		return fmt.Errorf("stop 失败: %w", err)
	}
	return nil
}

func (m *TimerManager) restartTimer(ctx context.Context, name string) error {
	timerFile := systemd.CronTimerFileName(name)
	if _, err := m.getClient().RestartUnitContext(ctx, timerFile, "replace"); err != nil {
		return fmt.Errorf("restart 失败: %w", err)
	}
	return nil
}

func (m *TimerManager) timerUnitExists(name string) bool {
	p := systemd.CronTimerPath(name)
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func (m *TimerManager) timerEnabled(ctx context.Context, name string) bool {
	props, err := m.getClient().GetUnitPropertiesContext(ctx, systemd.CronTimerFileName(name))
	if err == nil && props != nil {
		if ufs, ok := props["UnitFileState"].(string); ok && ufs != "" {
			return ufs == "enabled"
		}
	}
	return false
}

func (m *TimerManager) timerActive(ctx context.Context, name string) bool {
	timerFile := systemd.CronTimerFileName(name)
	props, err := m.getClient().GetUnitPropertiesContext(ctx, timerFile)
	if err != nil {
		return false
	}
	if state, ok := props["ActiveState"].(string); ok {
		return state == "active" || state == "activating" || state == "reloading"
	}
	return false
}

// rollbackCreate 回滚 Create 失败时已写入的 unit 与命令脚本文件。
func (m *TimerManager) rollbackCreate(ctx context.Context, name string) {
	_ = systemd.RemoveCronUnitFile(systemd.CronTimerFileName(name))
	_ = systemd.RemoveCronUnitFile(systemd.CronServiceFileName(name))
	_ = removeTaskCommand(name)
	_ = m.daemonReload(ctx)
}

// parseJournalRuns 解析 journalctl --output=json 输出，按 invocation ID 分组为每次执行。
// 子进程日志用 _SYSTEMD_INVOCATION_ID，systemd 自身（pid=1）用 INVOCATION_ID。
func parseJournalRuns(stdout string) []CronRun {
	type entry struct {
		Message           string `json:"MESSAGE"`
		RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
		Priority          string `json:"PRIORITY"`
		Invocation        string `json:"_SYSTEMD_INVOCATION_ID"`
		InvocationAlt     string `json:"INVOCATION_ID"`
	}
	idx := map[string]int{}
	var runs []CronRun
	for line := range strings.SplitSeq(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		inv := e.Invocation
		if inv == "" {
			inv = e.InvocationAlt
		}
		if inv == "" {
			continue
		}
		i, ok := idx[inv]
		if !ok {
			i = len(runs)
			idx[inv] = i
			runs = append(runs, CronRun{InvocationID: inv})
		}
		msg := e.Message
		// 状态标记：失败优先，其次成功。
		if strings.Contains(msg, "Failed to start") ||
			strings.Contains(msg, "Failed with result") ||
			strings.Contains(msg, "status=") && strings.Contains(msg, "FAILURE") {
			runs[i].Status = "failed"
		} else if runs[i].Status != "failed" &&
			(strings.Contains(msg, "Deactivated successfully") || strings.Contains(msg, "Finished")) {
			runs[i].Status = "success"
		}
		if runs[i].StartedAt == "" {
			runs[i].StartedAt = formatJournalTime(e.RealtimeTimestamp)
		}
		runs[i].Logs = append(runs[i].Logs, LogLine{
			Time:     formatJournalTime(e.RealtimeTimestamp),
			Message:  msg,
			Priority: priorityName(e.Priority),
		})
	}
	// 只有启动无结束标记的为 running。
	for i := range runs {
		if runs[i].Status == "" {
			runs[i].Status = "running"
		}
	}
	// journalctl 为正序输出（旧 → 新），单次日志已按时间正序，无需处理。
	// runs 反转为 newest-first（执行列表按最新在前）。
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
	return runs
}

// formatJournalTime 把微秒时间戳格式化为 util.TimeLayout。空值返回空串。
func formatJournalTime(usec string) string {
	if usec == "" {
		return ""
	}
	var n int64
	if _, err := fmt.Sscanf(usec, "%d", &n); err != nil {
		return ""
	}
	return util.UnixMicros(n).Format(util.TimeLayout)
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
