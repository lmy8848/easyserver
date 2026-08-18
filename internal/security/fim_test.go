package security

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestDefaultFIMPaths_UsesActualConfigPath(t *testing.T) {
	paths := defaultFIMPaths("config.toml")
	if !slices.Contains(paths, "/etc/ssh/sshd_config") {
		t.Fatalf("missing sshd_config in %v", paths)
	}
	if !slices.Contains(paths, "/etc/nginx/nginx.conf") {
		t.Fatalf("missing nginx.conf in %v", paths)
	}
	if !slices.Contains(paths, "/root/.ssh/authorized_keys") {
		t.Fatalf("missing authorized_keys in %v", paths)
	}

	// 面板配置文件取实际路径（相对路径归一化为绝对），不再硬编码 /opt/easyserver。
	for _, p := range paths {
		if p == "/opt/easyserver/config.toml" {
			t.Fatalf("hardcoded panel config path still present: %v", paths)
		}
	}
	abs, err := filepath.Abs("config.toml")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if !slices.Contains(paths, abs) {
		t.Fatalf("actual config path %q not monitored, got %v", abs, paths)
	}
}

func TestDefaultFIMPaths_EmptyConfigPathSkipped(t *testing.T) {
	paths := defaultFIMPaths("")
	for _, p := range paths {
		if p == "" || p == "/opt/easyserver/config.toml" {
			t.Fatalf("unexpected path %q in %v", p, paths)
		}
	}
}
