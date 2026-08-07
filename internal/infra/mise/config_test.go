package mise

import (
	"strings"
	"testing"
)

// minimal runnable checks for the pure builder. Real I/O (mkdir + atomic rename)
// is covered indirectly by integration verification.
func TestBuildConfigContent(t *testing.T) {
	envs := map[string]string{
		"MISE_NODE_MIRROR_URL":    "https://npmmirror.com/mirrors/node/",
		"MISE_GO_DOWNLOAD_MIRROR": "https://mirrors.aliyun.com/golang/",
	}
	defaults := map[string]string{
		"node":                       "20.11.0",
		"vfox:version-fox/vfox-java": "21.0.0", // tool 名含 ':' 和 '/' → 必须加引号
	}

	got := BuildConfigContent(envs, defaults)

	mustContain(t, got, `"MISE_NODE_MIRROR_URL" = "https://npmmirror.com/mirrors/node/"`)
	mustContain(t, got, `"MISE_GO_DOWNLOAD_MIRROR" = "https://mirrors.aliyun.com/golang/"`)
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

func TestBuildConfigContent_NoDefaults(t *testing.T) {
	// When no defaults are set, the [tools] section is omitted entirely
	// rather than emitted as an empty block.
	got := BuildConfigContent(map[string]string{"FOO": "bar"}, nil)
	if strings.Contains(got, "[tools]") {
		t.Fatalf("[tools] section should be omitted when no defaults are set, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "[env]\n") {
		t.Fatalf("[env] header missing, got: %q", got)
	}
}

// TOML injection regression: even if a malformed env var name sneaks in
// (defense-in-depth), the %q rendering must NOT let it forge a new section.
func TestBuildConfigContent_EnvKeyInjectionEscaped(t *testing.T) {
	got := BuildConfigContent(map[string]string{"FOO\n[tools]\nnode": "x"}, nil)

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "[") && line != "[env]" {
			t.Fatalf("forged section header leaked at line start: %q in:\n%s", line, got)
		}
	}
	mustContain(t, got, `"FOO\n[tools]\nnode"`)
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}
