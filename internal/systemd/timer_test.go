package systemd

import (
	"strings"
	"testing"

	"easyserver/internal/infra/mise"
)

func TestValidateCronName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"daily-backup", true},
		{"a", true},
		{"easyserver-cron-foo", false}, // 含 cron 前缀
		{"", false},
		{"My_Task", false},
		{"foo bar", false},
	}
	for _, c := range cases {
		err := ValidateCronName(c.name)
		if c.ok && err != nil {
			t.Errorf("ValidateCronName(%q) 期望通过，实际错误: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateCronName(%q) 期望拒绝，实际通过", c.name)
		}
	}
}

func TestRenderTimer(t *testing.T) {
	content, err := RenderTimer(&TimerSpec{
		Name:        "daily-backup",
		Description: "每日备份",
		OnCalendar:  "*-*-* 03:00:00",
		Persistent:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"[Unit]",
		"Description=每日备份",
		"# ManagedBy=easyserver-cron",
		"[Timer]",
		"OnCalendar=*-*-* 03:00:00",
		"Persistent=yes",
		"Unit=easyserver-cron-daily-backup.service",
		"[Install]",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("RenderTimer 缺少 %q", want)
		}
	}
	// Persistent 默认 no
	c2, _ := RenderTimer(&TimerSpec{Name: "x", OnCalendar: "*:00/5"})
	if !strings.Contains(c2, "Persistent=no") {
		t.Errorf("期望 Persistent=no，实际:\n%s", c2)
	}
}

func TestRenderTimer_Invalid(t *testing.T) {
	if _, err := RenderTimer(&TimerSpec{Name: "x", OnCalendar: ""}); err == nil {
		t.Fatal("期望缺 OnCalendar 报错")
	}
	if _, err := RenderTimer(&TimerSpec{Name: "x", OnCalendar: "*-*-*\nEvil"}); err == nil {
		t.Fatal("期望 OnCalendar 含换行报错")
	}
}

func TestRenderCronService(t *testing.T) {
	content, err := RenderCronService(&TimerSpec{
		Name:             "daily-backup",
		Description:      "每日备份",
		ExecStart:        "backup.sh --all",
		MaxRetry:         3,
		Timeout:          120,
		RuntimeVersionID: 7,
		RuntimeLang:      "node",
		RuntimeExact:     "20.11.0",
		Env:              map[string]string{"FOO": "bar baz"},
	}, mise.NewProvider())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"Type=oneshot",
		"# RuntimeVersionID=7",
		"# RuntimeLang=node",
		"# RuntimeExact=20.11.0",
		"StartLimitBurst=4", // MaxRetry+1
		"Restart=on-failure",
		"TimeoutStartSec=120",
		"Environment=FOO=\"bar baz\"",
		"easyserver/mise/mise exec node@20.11.0 -- backup.sh --all",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("RenderCronService 缺少 %q\n%s", want, content)
		}
	}
	// 无 [Install] 段（service 由 timer 激活）
	if strings.Contains(content, "[Install]") {
		t.Errorf("cron service 不应有 [Install] 段:\n%s", content)
	}
}

func TestRenderCronService_TimeoutZero(t *testing.T) {
	// Timeout=0 表示不超时（infinity），而非默认 3600。
	content, err := RenderCronService(&TimerSpec{
		Name:      "x",
		ExecStart: "echo hi",
		Timeout:   0,
	}, mise.NewProvider())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "TimeoutStartSec=infinity") {
		t.Errorf("Timeout=0 应输出不超时，实际:\n%s", content)
	}
	if strings.Contains(content, "TimeoutStartSec=3600") {
		t.Errorf("Timeout=0 不应输出 3600:\n%s", content)
	}
}

func TestCronTimerName(t *testing.T) {
	if got := CronTimerName("easyserver-cron-daily-backup.timer"); got != "daily-backup" {
		t.Errorf("期望 daily-backup，实际 %q", got)
	}
	if got := CronTimerName("easyserver-foo.timer"); got != "" {
		t.Errorf("非 cron timer 应返回空，实际 %q", got)
	}
	if got := CronTimerName("easyserver-cron-foo.service"); got != "" {
		t.Errorf("非 .timer 文件应返回空，实际 %q", got)
	}
}
