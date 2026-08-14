package runtimeenv

import (
	"testing"

	"easyserver/internal/infra/task"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool // a < b
	}{
		{"20.11.0", "21.0.1", true},
		{"9", "10", true},     // 语义版本序，非字典序
		{"8.2", "8.10", true}, // 段级数字比较
		{"20.11.0", "20.11.0", false},
		{"20.11.1", "20.11.0", false},
		{"21.0.1", "20.11.0", false},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMarkerPathShape(t *testing.T) {
	// node@20.11.0 → installs/node/20.11.0/.easyserver-ok（ADR-0009 完成标记）
	p := markerPath("node", "20.11.0")
	wantSuffix := "installs/node/20.11.0/.easyserver-ok"
	if len(p) < len(wantSuffix) || p[len(p)-len(wantSuffix):] != wantSuffix {
		t.Errorf("markerPath(node,20.11.0) = %q, want suffix %q", p, wantSuffix)
	}
	// java 的 tool 目录名做了 :/ 归一
	p = markerPath("java", "21")
	wantSuffix = "installs/vfox-version-fox-vfox-java/21/.easyserver-ok"
	if len(p) < len(wantSuffix) || p[len(p)-len(wantSuffix):] != wantSuffix {
		t.Errorf("markerPath(java,21) = %q, want suffix %q", p, wantSuffix)
	}
}

func TestTaskStatusToRuntime(t *testing.T) {
	cases := []struct {
		st        task.Status
		installed bool
		want      string
	}{
		{task.StatusRunning, false, "installing"},
		{task.StatusRunning, true, "uninstalling"},
		{task.StatusFailed, false, "failed"},
		{task.StatusFailed, true, "uninstall_failed"},
	}
	for _, c := range cases {
		if got := taskStatusToRuntime(c.st, c.installed); got != c.want {
			t.Errorf("taskStatusToRuntime(%v, %v) = %q, want %q", c.st, c.installed, got, c.want)
		}
	}
}

func TestParseRuntimeTaskKey(t *testing.T) {
	lang, exact, ok := parseRuntimeTaskKey("runtime:node@20.11.0")
	if !ok || lang != "node" || exact != "20.11.0" {
		t.Fatalf("parse = (%q, %q, %v), want (node, 20.11.0, true)", lang, exact, ok)
	}
	if _, _, ok := parseRuntimeTaskKey("deploy:node@20"); ok {
		t.Fatal("non-runtime prefix must be rejected")
	}
	if _, _, ok := parseRuntimeTaskKey("runtime:node"); ok {
		t.Fatal("missing exact version must be rejected")
	}
}
