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
		{"easyserver-foo", false}, // 包含托管前缀
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
		Name:      "my-app",
		ExecStart: "node /app/server.js",
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
		ExecStart:        "node /app/server.js --port 3000",
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
		{"exec_start", &ManagedUnitSpec{Name: "foo", ExecStart: "node\nrm -rf /"}},
		{"dir", &ManagedUnitSpec{Name: "foo", ExecStart: "x", Dir: "/app\n/etc"}},
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
		ExecStart:        "node",
		RuntimeVersionID: 42,
		RuntimeLang:      "node",
		RuntimeExact:     "20.11.0",
	}
	content, err := RenderUnit(spec)
	if err != nil {
		t.Fatalf("RenderUnit 失败: %v", err)
	}

	info := &ServiceInfo{}
	ParseUnitMeta(content, info)
	if !info.Managed {
		t.Error("Managed 应为 true")
	}
	if info.Description != "测试应用" {
		t.Errorf("Description 期望「测试应用」，实际 %q", info.Description)
	}
	if info.RuntimeVersionID != 42 {
		t.Errorf("RuntimeVersionID 期望 42，实际 %d", info.RuntimeVersionID)
	}
	if info.RuntimeLang != "node" {
		t.Errorf("RuntimeLang 期望 node，实际 %q", info.RuntimeLang)
	}
	if info.RuntimeExact != "20.11.0" {
		t.Errorf("RuntimeExact 期望 20.11.0，实际 %q", info.RuntimeExact)
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
	info := &ServiceInfo{}
	ParseUnitMeta(content, info)
	if info.Managed {
		t.Error("无 ManagedBy 注释时 Managed 应为 false")
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
		"SIMPLE":    "value",
		"SPACED":    "hello world",
		"QUOTE":     `it's "fine"`,
		"BACKSLASH": `C:\app`,
		"NEWLINE":   "a\nb", // 应被跳过
	}
	lines := buildEnvLines(env)
	// NEWLINE 被跳过，剩 4 条
	if len(lines) != 4 {
		t.Fatalf("期望 4 条 Environment 行，实际 %d: %v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	mustContain(t, joined, "SIMPLE=value")
	// 含空格的值应被引号包裹
	mustContain(t, joined, `"hello world"`)
	mustContain(t, joined, `"C:\\app"`)
}

// 防御 env key 注入：key 含换行应被 RenderUnit 拒绝，
// 否则可注入任意 systemd 指令（如 "FOO\nExecStart=evil"）。
func TestRenderUnit_RejectsEnvKeyInjection(t *testing.T) {
	spec := &ManagedUnitSpec{
		Name:      "foo",
		ExecStart: "node",
		Env:       map[string]string{"FOO\nExecStart=/bin/evil": "bar"},
	}
	_, err := RenderUnit(spec)
	if err == nil {
		t.Error("env key 含换行应被拒绝（防 systemd 指令注入）")
	}
}

// env key 不符合 POSIX 变量名格式应被拒绝。
func TestRenderUnit_RejectsInvalidEnvKey(t *testing.T) {
	cases := []string{
		"1FOO",    // 数字开头
		"FOO-BAR", // 连字符
		"",        // 空串（map key 一般不会是空串，但防御）
	}
	for _, k := range cases {
		spec := &ManagedUnitSpec{
			Name:      "foo",
			ExecStart: "node",
			Env:       map[string]string{k: "v"},
		}
		_, err := RenderUnit(spec)
		if err == nil {
			t.Errorf("env key %q 应被拒绝", k)
		}
	}
}

// 防御 runtime 字段注入：RuntimeLang/RuntimeExact 含换行应被拒绝。
func TestRenderUnit_RejectsRuntimeNewline(t *testing.T) {
	cases := []struct {
		field string
		spec  *ManagedUnitSpec
	}{
		{"runtime_lang", &ManagedUnitSpec{Name: "foo", ExecStart: "x", RuntimeLang: "node\nExecStart=evil"}},
		{"runtime_exact", &ManagedUnitSpec{Name: "foo", ExecStart: "x", RuntimeExact: "20.0.0\nUser=root"}},
	}
	for _, c := range cases {
		_, err := RenderUnit(c.spec)
		if err == nil {
			t.Errorf("RenderUnit %s 含换行应被拒绝", c.field)
		}
	}
}

// ParseUnitMeta 应从 [Service] 段原生指令回填 ExecStart/Dir/Env/AutoRestart，
// 供编辑表单回显。ExecStart 去掉 mise 前缀还原用户原始命令。
func TestParseUnitMeta_ConfigRoundTrip(t *testing.T) {
	spec := &ManagedUnitSpec{
		Name:             "my-app",
		Description:      "测试",
		ExecStart:        `node /app/server.js --port 3000 --name "hello world"`,
		Dir:              "/app",
		Env:              map[string]string{"NODE_ENV": "production", "PORT": "3000", "GREETING": "hello, world"},
		AutoRestart:      true,
		RuntimeVersionID: 7,
		RuntimeLang:      "node",
		RuntimeExact:     "20.10.0",
	}
	content, err := RenderUnit(spec)
	if err != nil {
		t.Fatalf("RenderUnit 失败: %v", err)
	}

	info := &ServiceInfo{}
	ParseUnitMeta(content, info)

	// ExecStart 应去掉 mise 前缀，还原用户原始命令（含引号无损）
	if info.ExecStart != spec.ExecStart {
		t.Errorf("ExecStart 期望 %q，实际 %q", spec.ExecStart, info.ExecStart)
	}
	if info.Dir != "/app" {
		t.Errorf("Dir 期望 /app，实际 %q", info.Dir)
	}
	if !info.AutoRestart {
		t.Error("AutoRestart 期望 true")
	}
	// env 含逗号应无损还原（从 Environment= 行读取，引号语义由 parseEnvLine 处理）
	if info.Env["NODE_ENV"] != "production" {
		t.Errorf("Env[NODE_ENV] 期望 production，实际 %q", info.Env["NODE_ENV"])
	}
	if info.Env["PORT"] != "3000" {
		t.Errorf("Env[PORT] 期望 3000，实际 %q", info.Env["PORT"])
	}
	if info.Env["GREETING"] != "hello, world" {
		t.Errorf("Env[GREETING] 期望 'hello, world'，实际 %q", info.Env["GREETING"])
	}
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
