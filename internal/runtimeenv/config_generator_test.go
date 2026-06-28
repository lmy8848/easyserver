package runtimeenv

import (
	"strings"
	"testing"
)

// minimal runnable check for the pure builder. Real I/O (mkdir + atomic rename)
// is covered indirectly by the integration verification in issue 07's AC.
func TestBuildMiseConfigContent(t *testing.T) {
	mirrors := []RuntimeMirror{
		{Lang: "node", EnvKey: "MISE_NODE_MIRROR_URL", EnvValue: "https://npmmirror.com/mirrors/node/", Enabled: 1},
		{Lang: "node", EnvKey: "MISE_NODE_MIRROR_URL", EnvValue: "https://nodejs.org/dist/", Enabled: 0}, // disabled → must not appear
		{Lang: "go", EnvKey: "MISE_GO_DOWNLOAD_MIRROR", EnvValue: "https://mirrors.aliyun.com/golang/", Enabled: 1},
	}
	defaults := []GlobalDefaultEntry{
		{Lang: "node", Exact: "20.11.0"},
		{Lang: "java", Exact: "21.0.0"}, // mise tool name contains ':' and '/' → must be quoted
	}

	got := buildMiseConfigContent(mirrors, defaults)

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

// TOML injection regression: even if a malformed env_key slips past
// CreateMirror's regex (defense-in-depth), the %q rendering must NOT let it
// forge a new section. The hostile bytes survive as escapes inside a quoted
// key — that's the goal: TOML parser sees one weird key, not a new section.
func TestBuildMiseConfigContent_EnvKeyInjectionEscaped(t *testing.T) {
	got := buildMiseConfigContent([]RuntimeMirror{
		{EnvKey: "FOO\n[tools]\nnode", EnvValue: "x", Enabled: 1},
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
