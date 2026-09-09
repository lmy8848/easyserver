package systemd

import "testing"

func TestNormalizeUnit(t *testing.T) {
	cases := []struct{ in, want string }{
		// 裸服务名 -> 补 .service
		{"nginx", "nginx.service"},
		{"apache2", "apache2.service"},
		{"tomcat9", "tomcat9.service"},
		{"caddy", "caddy.service"},
		{"easyserver-app", "easyserver-app.service"},
		// 已带类型后缀的不动
		{"nginx.service", "nginx.service"},
		{"docker.service", "docker.service"},
		{"podman.socket", "podman.socket"},
		{"cron.timer", "cron.timer"},
		{"easyserver-app.service", "easyserver-app.service"},
		// 通配符 pattern 不动（供 ListUnitsByPatterns 使用）
		{"nginx*", "nginx*"},
		{"*", "*"},
		// 空值不动
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeUnit(c.in); got != c.want {
			t.Errorf("normalizeUnit(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
