package plugin

import "testing"

// panel.api.proxy 只读白名单（panelAPIAllowed）。

func TestPanelAPIAllowed(t *testing.T) {
	allowed := []string{
		"/api/monitor/stats",
		"/api/monitor/history?start=now&end=1h",
		"/api/monitor/processes",
		"/api/audit-logs",
		"/api/audit-logs/actions",
		"/api/alerts/rules",
		"/api/security/fim/changes",
		"/api/security/cve/kernel",
	}
	for _, p := range allowed {
		if !panelAPIAllowed(p) {
			t.Errorf("%s 应在白名单内", p)
		}
	}
	rejected := []string{
		"/api/monitor", // 无尾斜杠，非 /api/monitor/ 前缀
		"/api/container/instances",
		"/api/web-servers",
		"/api/firewall/rules",
		"/api/firewall/port-forwards",
		"/api/auth/me",
		"/api/plugins/styles",
		"/api/db/instances",
		"/api/cron/tasks",
	}
	for _, p := range rejected {
		if panelAPIAllowed(p) {
			t.Errorf("%s 不应在白名单内", p)
		}
	}
}
