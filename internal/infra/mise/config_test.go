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

	got := BuildConfigContent(envs)

	mustContain(t, got, `"MISE_NODE_MIRROR_URL" = "https://npmmirror.com/mirrors/node/"`)
	mustContain(t, got, `"MISE_GO_DOWNLOAD_MIRROR" = "https://mirrors.aliyun.com/golang/"`)
	if !strings.HasPrefix(got, "[env]\n") {
		t.Fatalf("[env] header missing, got: %q", got)
	}
	// 不再有 [tools] 段（默认版本功能已移除）。
	if strings.Contains(got, "[tools]") {
		t.Fatalf("[tools] section should not be emitted, got:\n%s", got)
	}
}

func TestBuildConfigContent_Empty(t *testing.T) {
	got := BuildConfigContent(map[string]string{})
	if got != "[env]\n" {
		t.Fatalf("expected bare [env] header, got: %q", got)
	}
}

// TOML injection regression: even if a malformed env var name sneaks in
// (defense-in-depth), the %q rendering must NOT let it forge a new section.
func TestBuildConfigContent_EnvKeyInjectionEscaped(t *testing.T) {
	got := BuildConfigContent(map[string]string{"FOO\n[tools]\nnode": "x"})

	for line := range strings.SplitSeq(got, "\n") {
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
