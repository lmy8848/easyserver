package plugin

import (
	"testing"
	"time"
)

func validManifest() Manifest {
	return Manifest{
		APIVersion: 1, Name: "demo", Version: "0.1.0",
		Runtime:     Runtime{Exec: "demo-bin", Socket: "demo.sock"},
		Permissions: []string{PermAuditWrite, PermExecRestricted},
	}
}

func TestManifestValidateOK(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("合法 manifest 不应报错: %v", err)
	}
	m.ManifestDefaults()
	if m.Runtime.StartupTimeout != 30*time.Second || m.Health.FailThreshold != 3 {
		t.Fatal("默认值未填充")
	}
}

func TestManifestValidateRejections(t *testing.T) {
	cases := map[string]func(m *Manifest){
		"非法名称":           func(m *Manifest) { m.Name = "Bad_Name" },
		"超长名称":           func(m *Manifest) { m.Name = string(make([]byte, 70)) },
		"api_version 越界": func(m *Manifest) { m.APIVersion = 99 },
		"空版本":            func(m *Manifest) { m.Version = "" },
		"空 exec":         func(m *Manifest) { m.Runtime.Exec = "" },
		"绝对路径 exec":      func(m *Manifest) { m.Runtime.Exec = "/usr/bin/evil" },
		"穿越 exec":        func(m *Manifest) { m.Runtime.Exec = "../evil" },
		"未知权限":           func(m *Manifest) { m.Permissions = []string{"root.everything"} },
		"菜单路径越界":         func(m *Manifest) { m.Contributions.Menus = []MenuContribution{{ID: "m", Path: "/plugins/other"}} },
		"路由前缀越界":         func(m *Manifest) { m.Contributions.Routes = []RouteContribution{{ID: "r", Prefix: "/api/other"}} },
		"事件名非反向域名":       func(m *Manifest) { m.Contributions.EventPubs = []EventPubContribution{{Event: "notdomain"}} },
	}
	for label, mutate := range cases {
		m := validManifest()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s 应被拒绝", label)
		}
	}
}

func TestManifestHasPermission(t *testing.T) {
	m := validManifest()
	if !m.HasPermission(PermAuditWrite) {
		t.Error("声明过的权限应可查到")
	}
	if m.HasPermission(PermNotifySend) {
		t.Error("未声明权限不应查到")
	}
}
