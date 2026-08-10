package database

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateConfigFileFormats(t *testing.T) {
	// 只写传入的覆盖项；未传入的不写（服务端编译默认兜底）。
	mysql := generateConfigFile(DBTypeMySQL, map[string]string{"max_connections": "500"})
	if !strings.Contains(mysql, "[mysqld]\n") {
		t.Fatalf("mysql missing [mysqld] section:\n%s", mysql)
	}
	if !strings.Contains(mysql, "max_connections = 500\n") {
		t.Fatalf("mysql missing override:\n%s", mysql)
	}
	if strings.Contains(mysql, "innodb_buffer_pool_size") {
		t.Fatalf("unset param must be omitted:\n%s", mysql)
	}

	// port 不写进文件（端口由容器映射管理）。
	withPort := generateConfigFile(DBTypeMySQL, map[string]string{"port": "4000", "max_connections": "500"})
	if strings.Contains(withPort, "port") {
		t.Fatalf("port must not be written to config file:\n%s", withPort)
	}

	// Redis：save 多行展开成多条 save 指令；空值参数跳过。
	redis := generateConfigFile(DBTypeRedis, map[string]string{"save": "3600 1\n300 100\n60 10000", "logfile": ""})
	if !strings.Contains(redis, "save 3600 1\n") || !strings.Contains(redis, "save 60 10000\n") {
		t.Fatalf("redis save lines not expanded:\n%s", redis)
	}
	if strings.Contains(redis, "logfile") {
		t.Fatalf("redis empty logfile should be omitted:\n%s", redis)
	}

	// PostgreSQL：非关键字字符串值加单引号，数字不加。
	pg := generateConfigFile(DBTypePostgreSQL, map[string]string{"listen_addresses": "*", "max_connections": "100"})
	if !strings.Contains(pg, "listen_addresses = '*'\n") {
		t.Fatalf("pg string value should be quoted:\n%s", pg)
	}
	if !strings.Contains(pg, "max_connections = 100\n") {
		t.Fatalf("pg number default unquoted:\n%s", pg)
	}
}

func TestParseConfigFileRoundTrip(t *testing.T) {
	cases := []struct {
		dbType DBType
		params map[string]string
	}{
		{DBTypeMySQL, map[string]string{"max_connections": "500", "wait_timeout": "100"}},
		{DBTypePostgreSQL, map[string]string{"listen_addresses": "*", "shared_buffers": "256MB"}},
		{DBTypeRedis, map[string]string{"save": "3600 1\n300 100\n60 10000", "appendonly": "yes"}},
	}
	for _, c := range cases {
		content := generateConfigFile(c.dbType, c.params)
		got := parseConfigFile(c.dbType, content)
		if len(got) != len(c.params) {
			t.Fatalf("%s: round trip mismatch got=%+v want=%+v\ncontent:\n%s", c.dbType, got, c.params, content)
		}
		for key, want := range c.params {
			if got[key] != want {
				t.Fatalf("%s: key %q = %q, want %q\ncontent:\n%s", c.dbType, key, got[key], want, content)
			}
		}
	}
}

func TestStructuredConfigGetAndSave(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt)
	ctx := context.Background()

	id, err := repo.CreateInstance(ctx, &DBInstance{
		DBType: DBTypeMySQL, Version: "8.0", ContainerEngine: "docker", Image: "docker.io/mysql:8.0",
		ContainerName: "c1", VolumeName: "c1-data", ConfigDir: "/etc/mysql/conf.d",
		BindAddress: "127.0.0.1", Port: 3306, AdminPassword: "pw", Status: "running",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 从未配置 → 返回编译默认值，port 显示实例当前值。
	view, err := svc.GetInstanceConfig(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(view.Sections) != 1 || view.Sections[0].Name != "mysqld" {
		t.Fatalf("unexpected sections: %+v", view.Sections)
	}
	if view.Sections[0].Params["max_connections"] != "151" {
		t.Fatalf("expected compiled default, got %q", view.Sections[0].Params["max_connections"])
	}
	if view.Sections[0].Params["innodb_buffer_pool_size"] != "128M" {
		t.Fatalf("expected compiled default, got %q", view.Sections[0].Params["innodb_buffer_pool_size"])
	}
	if view.Sections[0].Params["port"] != "3306" {
		t.Fatalf("port should reflect instance port, got %q", view.Sections[0].Params["port"])
	}
	if len(view.Sections[0].Meta) == 0 {
		t.Fatal("meta must be embedded for the editor")
	}

	// 保存覆盖项 → 合并进配置卷文件（只写覆盖项，port 由 DB 列管理不落文件）。
	err = svc.SaveInstanceConfig(ctx, id, []ConfigSectionView{
		{Name: "mysqld", Params: map[string]string{"max_connections": "500", "wait_timeout": "100"}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(rt.seedCalls) != 1 || rt.seedCalls[0].volume != "c1-config" || rt.seedCalls[0].dest != "easyserver.cnf" {
		t.Fatalf("unexpected seed write: %+v", rt.seedCalls)
	}
	if !strings.Contains(rt.seedCalls[0].content, "max_connections = 500\n") {
		t.Fatalf("generated file missing override:\n%s", rt.seedCalls[0].content)
	}

	// 重新读取 → 覆盖项生效，未覆盖项仍为默认。
	view, err = svc.GetInstanceConfig(ctx, id)
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	if view.Sections[0].Params["max_connections"] != "500" {
		t.Fatalf("override lost: %+v", view.Sections[0].Params)
	}
	if view.Sections[0].Params["innodb_buffer_pool_size"] != "128M" {
		t.Fatalf("unset param must fall back to default: %+v", view.Sections[0].Params)
	}
}

func TestSaveConfigWithPortChangeRecreatesContainer(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt)
	ctx := context.Background()

	id, err := repo.CreateInstance(ctx, &DBInstance{
		DBType: DBTypeMySQL, Version: "8.0", ContainerEngine: "docker", Image: "docker.io/mysql:8.0",
		ContainerName: "c1", VolumeName: "c1-data", ConfigDir: "/etc/mysql/conf.d",
		BindAddress: "127.0.0.1", Port: 3306, AdminPassword: "pw", Status: "running",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// port 参数变化 → 重建容器更新映射；配置文件不写 port。
	err = svc.SaveInstanceConfig(ctx, id, []ConfigSectionView{
		{Name: "mysqld", Params: map[string]string{"port": "4000", "max_connections": "300"}},
	})
	if err != nil {
		t.Fatalf("save with port change: %v", err)
	}
	if repo.instances[id].Port != 4000 {
		t.Fatalf("port not persisted: %d", repo.instances[id].Port)
	}
	if len(rt.removed) != 1 || rt.removed[0] != "c1" {
		t.Fatalf("expected container removed for recreation, got %v", rt.removed)
	}
	if len(rt.createSpecs) != 1 || rt.createSpecs[0].HostPort != 4000 {
		t.Fatalf("expected recreate with port 4000, got %+v", rt.createSpecs)
	}
	if len(rt.seedCalls) != 1 || !strings.Contains(rt.seedCalls[0].content, "max_connections = 300\n") || strings.Contains(rt.seedCalls[0].content, "port") {
		t.Fatalf("config file wrong: %+v", rt.seedCalls)
	}
}
