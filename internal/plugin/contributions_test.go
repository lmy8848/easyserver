package plugin

import "testing"

// 新增贡献点（styles / web_servers / commands）的 manifest 校验。

func TestContributionStylesValidation(t *testing.T) {
	m := validManifest()
	m.Contributions.Styles = []StyleContribution{{ID: "brand", Path: "styles/brand.css"}}
	if err := m.Validate(); err != nil {
		t.Fatalf("合法 styles 应通过: %v", err)
	}
	cases := []StyleContribution{
		{ID: "s", Path: ""},
		{ID: "s", Path: "/absolute/path.css"},
		{ID: "s", Path: "../escape.css"},
	}
	for _, c := range cases {
		m2 := validManifest()
		m2.Contributions.Styles = []StyleContribution{c}
		if err := m2.Validate(); err == nil {
			t.Errorf("styles path %q 应被拒绝", c.Path)
		}
	}
}

func TestContributionWebServersValidation(t *testing.T) {
	m := validManifest()
	m.Contributions.WebServers = []WebServerContribution{
		{Name: "lighttpd", DisplayName: "Lighttpd", InstallCmd: "apt-get install -y lighttpd", ServiceName: "lighttpd"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("合法 web_servers 应通过: %v", err)
	}
	cases := []WebServerContribution{
		{Name: "Bad_Name", DisplayName: "X", InstallCmd: "apt-get", ServiceName: "x"}, // 非法类型名
		{Name: "ok", DisplayName: "", InstallCmd: "apt-get", ServiceName: "x"},        // 缺 display_name
		{Name: "ok", DisplayName: "X", InstallCmd: "", ServiceName: "x"},              // 缺 install_cmd
		{Name: "ok", DisplayName: "X", InstallCmd: "apt-get", ServiceName: ""},        // 缺 service_name
	}
	for _, c := range cases {
		m2 := validManifest()
		m2.Contributions.WebServers = []WebServerContribution{c}
		if err := m2.Validate(); err == nil {
			t.Errorf("web_servers %+v 应被拒绝", c)
		}
	}
}

func TestContributionCommandsValidation(t *testing.T) {
	m := validManifest()
	m.Contributions.Commands = []CommandContribution{{ID: "restart", Title: "重启"}}
	if err := m.Validate(); err != nil {
		t.Fatalf("合法 commands 应通过: %v", err)
	}
	m2 := validManifest()
	m2.Contributions.Commands = []CommandContribution{{ID: "Bad ID", Title: "x"}}
	if err := m2.Validate(); err == nil {
		t.Error("非法命令 id 应被拒绝")
	}
}

func TestContributionDatasourcesValidation(t *testing.T) {
	m := validManifest()
	m.Contributions.Datasources = []DatasourceContribution{{ID: "status", Title: "隧道状态", Kind: "json"}}
	if err := m.Validate(); err != nil {
		t.Fatalf("合法 datasources 应通过: %v", err)
	}
	cases := []DatasourceContribution{
		{ID: "Bad ID", Title: "x", Kind: "json"}, // 非法 id
		{ID: "ok", Title: "", Kind: "json"},      // 缺 title
		{ID: "ok", Title: "x", Kind: "xml"},      // 非法 kind
	}
	for _, c := range cases {
		m2 := validManifest()
		m2.Contributions.Datasources = []DatasourceContribution{c}
		if err := m2.Validate(); err == nil {
			t.Errorf("datasources %+v 应被拒绝", c)
		}
	}
}
