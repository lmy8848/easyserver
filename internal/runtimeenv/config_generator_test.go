package runtimeenv

import (
	"strings"
	"testing"
	"time"

	"easyserver/internal/envconfig"
)

// minimal runnable check for the pure builder. Real I/O (mkdir + atomic rename)
// is covered indirectly by the integration verification in issue 07's AC.
func TestBuildMiseConfigContent(t *testing.T) {
	envConfigs := []envconfig.EnvConfig{
		{Name: "MISE_NODE_MIRROR_URL", Value: "https://npmmirror.com/mirrors/node/", Enabled: true},
		{Name: "MISE_NODE_MIRROR_URL", Value: "https://nodejs.org/dist/", Enabled: false}, // disabled → must not appear
		{Name: "MISE_GO_DOWNLOAD_MIRROR", Value: "https://mirrors.aliyun.com/golang/", Enabled: true},
	}
	defaults := []GlobalDefaultEntry{
		{Lang: "node", Exact: "20.11.0"},
		{Lang: "java", Exact: "21.0.0"}, // mise tool name contains ':' and '/' → must be quoted
	}

	got := buildMiseConfigContent(envConfigs, defaults)

	mustContain(t, got, `"MISE_NODE_MIRROR_URL" = "https://npmmirror.com/mirrors/node/"`)
	mustContain(t, got, `"MISE_GO_DOWNLOAD_MIRROR" = "https://mirrors.aliyun.com/golang/"`)
	mustNotContain(t, got, `https://nodejs.org/dist/`)
	mustContain(t, got, "\n[tools]\n")
	mustContain(t, got, `node = "20.11.0"`)
	mustContain(t, got, `"vfox:version-fox/vfox-java" = "21.0.0"`)

	// Section order: [env] before [tools] — otherwise mise may interpret tool
	// keys as env values.
	envIdx := strings.Index(got, "[env]")
	toolsIdx := strings.Index(got, "[tools]")
	if envIdx == -1 || toolsIdx == -1 || envIdx > toolsIdx {
		t.Fatalf("section ordering wrong:\n%s", got)
	}
}

func TestBuildMiseConfigContent_NoDefaults(t *testing.T) {
	// When no global defaults are set, the [tools] section is omitted entirely
	// rather than emitted as an empty block.
	got := buildMiseConfigContent(nil, nil)
	if strings.Contains(got, "[tools]") {
		t.Fatalf("[tools] section should be omitted when no defaults are set, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "[env]\n") {
		t.Fatalf("[env] header missing, got: %q", got)
	}
}

func TestBuildMiseConfigContent_UnknownLangIsSkipped(t *testing.T) {
	// Defensive: a lang that's not in the catalog (shouldn't happen due to FK +
	// CHECK constraints) is silently skipped, not panicked on.
	got := buildMiseConfigContent(nil, []GlobalDefaultEntry{
		{Lang: "rust", Exact: "1.80.0"},
		{Lang: "node", Exact: "20.11.0"},
	})
	mustNotContain(t, got, "rust")
	mustContain(t, got, `node = "20.11.0"`)
}

// TOML injection regression: even if a malformed env var name slips past
// isValidEnvName (defense-in-depth), the %q rendering must NOT let it forge a
// new section. The hostile bytes survive as escapes inside a quoted key —
// that's the goal: TOML parser sees one weird key, not a new section.
func TestBuildMiseConfigContent_EnvKeyInjectionEscaped(t *testing.T) {
	got := buildMiseConfigContent([]envconfig.EnvConfig{
		{Name: "FOO\n[tools]\nnode", Value: "x", Enabled: true},
	}, nil)

	// No section header other than the legitimate [env] should appear at the
	// start of any line. If escaping fails, an attacker-supplied "[tools]"
	// would show up as a line-start bracket here.
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "[") && line != "[env]" {
			t.Fatalf("forged section header leaked at line start: %q in:\n%s", line, got)
		}
	}

	// The hostile bytes should survive as escapes inside the quoted key —
	// proof the rendering did NOT pass them through raw.
	mustContain(t, got, `"FOO\n[tools]\nnode"`)
}

// Ensure the time field doesn't affect output — it's metadata, not rendered.
func TestBuildMiseConfigContent_TimeFieldIgnored(t *testing.T) {
	now := time.Now()
	got := buildMiseConfigContent([]envconfig.EnvConfig{
		{Name: "FOO", Value: "bar", Enabled: true, CreatedAt: now, UpdatedAt: now},
	}, nil)
	mustContain(t, got, `"FOO" = "bar"`)
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}

func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected output NOT to contain %q, got:\n%s", needle, haystack)
	}
}
