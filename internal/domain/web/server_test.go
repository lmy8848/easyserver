package web

import "testing"

// web 服务器类型 registry：预定义 + 插件注册合并（功能补充扩展点）。

func TestServiceAllTypesWithPlugins(t *testing.T) {
	s := NewService(nil) // AllTypes/FindType/AllTypeNames 不碰 repo

	// 预定义类型
	if s.FindType("nginx") == nil {
		t.Fatal("预定义应含 nginx")
	}
	if len(s.AllTypes()) < 4 {
		t.Fatalf("预定义类型应 ≥4, got %d", len(s.AllTypes()))
	}

	// 注入插件类型（web_servers 贡献点）
	s.SetPluginWebServers(func() []WebServer {
		return []WebServer{{
			Name: "lighttpd", DisplayName: "Lighttpd", ServiceName: "lighttpd",
			InstallCmd: "apt-get install -y lighttpd", ConfigPath: "/etc/lighttpd",
			TestCmd: "lighttpd -t", VersionCmd: "lighttpd -v",
		}}
	})

	if s.FindType("lighttpd") == nil {
		t.Fatal("插件类型 lighttpd 应可查到")
	}
	names := s.AllTypeNames()
	found := false
	for _, n := range names {
		if n == "lighttpd" {
			found = true
		}
	}
	if !found {
		t.Error("AllTypeNames 应含 lighttpd")
	}
	if s.FindType("nonexistent") != nil {
		t.Error("未知类型应返回 nil")
	}
}
