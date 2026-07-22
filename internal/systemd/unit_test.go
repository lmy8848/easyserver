package systemd

import (
	"strings"
	"testing"
)

func TestValidateManagedName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"my-app", true},
		{"website-foo-com", true},
		{"a", true},
		{"123", true},
		{"", false},
		{"My-App", false},                // 大写
		{"-foo", false},                  // 连字符开头
		{"foo-", false},                  // 连字符结尾
		{"foo bar", false},               // 空格
		{"foo_bar", false},               // 下划线
		{"foo.service", false},           // 点
		{strings.Repeat("a", 61), false}, // 过长
	}
	for _, c := range cases {
		err := ValidateManagedName(c.name)
		if c.ok && err != nil {
			t.Errorf("ValidateManagedName(%q) 期望通过，实际错误: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateManagedName(%q) 期望拒绝，实际通过", c.name)
		}
	}
}

func TestRenderUnit_Minimal(t *testing.T) {
	spec := &ManagedUnitSpec{
		Name:    "my-app",
		Command: "node /app/server.js",
	}
	content, err := RenderUnit(spec)
	if err != nil {
		t.Fatalf("RenderUnit 失败: %v", err)
	}

	mustContain(t, content, "[Unit]")
	mustContain(t, content, "Description=easyserver-managed: my-app")
	mustContain(t, content, "# ManagedBy=easyserver")
	mustContain(t, content, "ExecStart=node /app/server.js")
	mustContain(t, content, "Type=simple")
	mustContain(t, content, "RestartSec=5")       // 默认值
	mustContain(t, content, "TimeoutStopSec=10")  // 默认值
	mustContain(t, content, "StartLimitBurst=10") // 默认值
	mustContain(t, content, "[Install]")
	mustContain(t, content, "WantedBy=multi-user.target")

	// AutoRestart=false 时不应有 Restart= 行
	mustNotContain(t, content, "Restart=on-failure")
	// 无 runtime 绑定时不应有 mise
	mustNotContain(t, content, "mise")
}

func TestRenderUnit_FullWithRuntime(t *testing.T) {
	spec := &ManagedUnitSpec{
		Name:             "my-app",
		Description:      "我的应用",
		Command:          "node",
		Args:             `/app/server.js --port 3000`,
		Dir:              "/app",
		Env:              map[string]string{"NODE_ENV": "production", "PORT": "3000"},
		AutoRestart:      true,
		MaxRestarts:      5,
		RestartDelay:     3,
		StopTimeout:      15,
		AutoStart:        true,
		RuntimeVersionID: 7,
		RuntimeLang:      "node",
		RuntimeExact:     "20.10.0",
	}
	content, err := RenderUnit(spec)
	if err != nil {
		t.Fatalf("RenderUnit 失败: %v", err)
	}

	mustContain(t, content, "Description=easyserver-managed: 我的应用")
	mustContain(t, content, "# RuntimeVersionID=7")
	mustContain(t, content, "# RuntimeLang=node")
	mustContain(t, content, "# RuntimeExact=20.10.0")
	// mise 前置包裹
	mustContain(t, content, "ExecStart=/usr/local/bin/mise exec node@20.10.0 -- node /app/server.js --port 3000")
	mustContain(t, content, "WorkingDirectory=/app")
	mustContain(t, content, "Environment=NODE_ENV=production")
	mustContain(t, content, "Environment=PORT=3000")
	mustContain(t, content, "Restart=on-failure")
	mustContain(t, content, "RestartSec=3")
	mustContain(t, content, "TimeoutStopSec=15")
	mustContain(t, content, "StartLimitBurst=5")
}

func TestRenderUnit_RejectsNewline(t *testing.T) {
	cases := []struct {
		field string
		spec  *ManagedUnitSpec
	}{
		{"command", &ManagedUnitSpec{Name: "foo", Command: "node\nrm -rf /"}},
		{"args", &ManagedUnitSpec{Name: "foo", Command: "x", Args: "a\nb"}},
		{"dir", &ManagedUnitSpec{Name: "foo", Command: "x", Dir: "/app\n/etc"}},
	}
	for _, c := range cases {
		_, err := RenderUnit(c.spec)
		if err == nil {
			t.Errorf("RenderUnit %s 含换行应被拒绝", c.field)
		}
	}
}

func TestParseUnitMeta_RoundTrip(t *testing.T) {
	spec := &ManagedUnitSpec{
		Name:             "my-app",
		Description:      "测试应用",
		Command:          "node",
		RuntimeVersionID: 42,
		RuntimeLang:      "node",
		RuntimeExact:     "20.11.0",
	}
	content, err := RenderUnit(spec)
	if err != nil {
		t.Fatalf("RenderUnit 失败: %v", err)
	}

	meta := ParseUnitMeta(content)
	if !meta.HasMarker {
		t.Error("HasMarker 应为 true")
	}
	if meta.Description != "测试应用" {
		t.Errorf("Description 期望「测试应用」，实际 %q", meta.Description)
	}
	if meta.RuntimeVersionID != 42 {
		t.Errorf("RuntimeVersionID 期望 42，实际 %d", meta.RuntimeVersionID)
	}
	if meta.RuntimeLang != "node" {
		t.Errorf("RuntimeLang 期望 node，实际 %q", meta.RuntimeLang)
	}
	if meta.RuntimeExact != "20.11.0" {
		t.Errorf("RuntimeExact 期望 20.11.0，实际 %q", meta.RuntimeExact)
	}
}

func TestParseUnitMeta_NoMarker(t *testing.T) {
	// 模拟系统自带 unit（无 ManagedBy 注释）
	content := `[Unit]
Description=nginx web server
After=network.target

[Service]
Type=forking
ExecStart=/usr/sbin/nginx

[Install]
WantedBy=multi-user.target
`
	meta := ParseUnitMeta(content)
	if meta.HasMarker {
		t.Error("无 ManagedBy 注释时 HasMarker 应为 false")
	}
}

func TestUnitName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"easyserver-foo.service", "foo"},
		{"easyserver-website-bar.service", "website-bar"},
		{"nginx.service", ""},     // 非托管
		{"easyserver-foo", "foo"}, // 无后缀也能解析（TrimSuffix 对无后缀串是 no-op）
		{"easyserver-", ""},       // 空名
		{"", ""},
	}
	for _, c := range cases {
		got := UnitName(c.input)
		if got != c.want {
			t.Errorf("UnitName(%q) 期望 %q，实际 %q", c.input, c.want, got)
		}
	}
}

func TestBuildEnvLines_SpecialChars(t *testing.T) {
	env := map[string]string{
		"SIMPLE":  "value",
		"SPACED":  "hello world",
		"QUOTE":   `it's "fine"`,
		"NEWLINE": "a\nb", // 应被跳过
	}
	lines := buildEnvLines(env)
	// NEWLINE 被跳过，剩 3 条
	if len(lines) != 3 {
		t.Fatalf("期望 3 条 Environment 行，实际 %d: %v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	mustContain(t, joined, "SIMPLE=value")
	// 含空格的值应被引号包裹
	mustContain(t, joined, `"hello world"`)
}

// helpers

func mustContain(t *testing.T, content, substr string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Errorf("期望内容包含 %q，实际:\n%s", substr, content)
	}
}

func mustNotContain(t *testing.T, content, substr string) {
	t.Helper()
	if strings.Contains(content, substr) {
		t.Errorf("期望内容不包含 %q，实际:\n%s", substr, content)
	}
}
