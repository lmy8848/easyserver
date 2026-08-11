package database

// 配置测试走驱动通道（分支迁移后的形态）：fakeConfigDriver 模拟 MySQL/PG 的
// 变量查询与持久化命令，Redis 用 redismock。断言的是驱动命令（SET PERSIST /
// ALTER SYSTEM / CONFIG SET）与运行时值返回，不再有配置文件读写。

import (
	"context"
	"strings"
	"testing"

	"easyserver/internal/infra/task"
)

// fakeConfigDriver is a scripted SQLRunner for config tests.
type fakeConfigDriver struct {
	mysqlVars    map[string]string // SHOW VARIABLES 返回值
	mysqlVersion string            // SELECT VERSION()
	pgValues     map[string]string // name → setting（pg_settings 读配置）
	pgContext    map[string]string // name → context（postmaster 判断）
	execs        []string          // 记录的 Exec SQL
}

func (f *fakeConfigDriver) Query(_ context.Context, _ *DBInstance, _, sql string, args ...any) (*QueryResult, error) {
	switch {
	case strings.HasPrefix(sql, "SHOW VARIABLES"):
		rows := make([][]any, 0, len(f.mysqlVars))
		for k, v := range f.mysqlVars {
			rows = append(rows, []any{k, v})
		}
		return &QueryResult{Columns: []ColumnMeta{{Name: "Variable_name", Type: "string"}, {Name: "Value", Type: "string"}}, Rows: rows}, nil
	case strings.Contains(sql, "VERSION()"):
		return &QueryResult{Rows: [][]any{{f.mysqlVersion}}}, nil
	case strings.Contains(sql, "pg_settings"):
		if strings.Contains(sql, "name, setting") {
			rows := make([][]any, 0, len(f.pgValues))
			for name, setting := range f.pgValues {
				rows = append(rows, []any{name, setting})
			}
			return &QueryResult{Rows: rows}, nil
		}
		// postmaster 判断：args[0] 是本次涉及参数（names），只返回其中 postmaster 级。
		names, _ := args[0].([]string)
		rows := [][]any{}
		for _, n := range names {
			if f.pgContext[n] == "postmaster" {
				rows = append(rows, []any{n})
			}
		}
		return &QueryResult{Rows: rows}, nil
	}
	return &QueryResult{}, nil
}

func (f *fakeConfigDriver) Exec(_ context.Context, _ *DBInstance, _, sql string, _ ...any) (*ExecResult, error) {
	f.execs = append(f.execs, sql)
	return &ExecResult{}, nil
}

func (f *fakeConfigDriver) Close(int64) {}

// configSvc builds a Service for config tests: fakeRepo + fake runtime (running)
// + fake driver (or redismock runner for Redis).
func configSvc(repo *fakeRepo, rt DatabaseRuntime, driver SQLRunner) *Service {
	return &Service{repo: repo, runtime: rt, driver: driver, redisOps: newRedisRunner(), taskMgr: task.NewManager(8)}
}

func configInstance(dbType DBType, name string, port int) *DBInstance {
	return &DBInstance{
		DBType: dbType, Version: "8.0", ContainerEngine: "docker", Image: "docker.io/mysql:8.0",
		ContainerName: name, VolumeName: name + "-data", BindAddress: "127.0.0.1",
		Port: port, AdminPassword: "pw", Status: "running",
	}
}

func TestGetConfigReadsRuntimeValues(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	f := &fakeConfigDriver{mysqlVersion: "8.0.36", mysqlVars: map[string]string{
		"max_connections": "500", "innodb_buffer_pool_size": "1G",
	}}
	id, _ := repo.CreateInstance(context.Background(), configInstance(DBTypeMySQL, "c1", 3306))
	svc := configSvc(repo, rt, f)

	view, err := svc.GetInstanceConfig(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// 读到啥返回啥：运行时值原样返回，未读到的参数不返回（无编译默认值合成）。
	if view.Params["max_connections"] != "500" {
		t.Fatalf("expected runtime value, got %q", view.Params["max_connections"])
	}
	if view.Params["innodb_buffer_pool_size"] != "1G" {
		t.Fatalf("expected runtime value, got %q", view.Params["innodb_buffer_pool_size"])
	}
	if _, ok := view.Params["wait_timeout"]; ok {
		t.Fatalf("unread param must be absent, got %+v", view.Params)
	}
	if view.Params["port"] != "3306" {
		t.Fatalf("port should reflect instance port, got %q", view.Params["port"])
	}
	if len(view.Meta) == 0 {
		t.Fatal("meta must be embedded for the editor")
	}
}

func TestSaveConfigAppliesViaDriver(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	f := &fakeConfigDriver{mysqlVersion: "8.0.36"}
	id, _ := repo.CreateInstance(context.Background(), configInstance(DBTypeMySQL, "c1", 3306))
	svc := configSvc(repo, rt, f)
	ctx := context.Background()

	if err := svc.SaveInstanceConfig(ctx, id, map[string]string{"max_connections": "500", "wait_timeout": "100"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 参数遍历走 map（无序），断言集合而非顺序。
	found := map[string]bool{}
	for _, e := range f.execs {
		found[e] = true
	}
	for _, want := range []string{
		"SET PERSIST `max_connections` = 500",
		"SET PERSIST `wait_timeout` = 100",
	} {
		if !found[want] {
			t.Fatalf("missing exec %q, got %v", want, f.execs)
		}
	}
	if len(f.execs) != 2 {
		t.Fatalf("expected exactly 2 execs, got %v", f.execs)
	}
	if len(rt.removed) != 0 {
		t.Fatalf("non-port save must not recreate the container, got %v", rt.removed)
	}

	// 空值不提交（前端已过滤）→ 后端兜底也不设置：空值参数不在 Exec 里出现。
	f.execs = nil
	if err := svc.SaveInstanceConfig(ctx, id, map[string]string{"max_connections": "500", "wait_timeout": ""}); err != nil {
		t.Fatalf("save with empty value: %v", err)
	}
	if len(f.execs) != 1 || f.execs[0] != "SET PERSIST `max_connections` = 500" {
		t.Fatalf("empty value must be skipped, got %v", f.execs)
	}

	// 保存生效后 SHOW VARIABLES 反映新值 → GET 返回运行时值。
	f.mysqlVars = map[string]string{"max_connections": "500", "wait_timeout": "100"}
	view, err := svc.GetInstanceConfig(ctx, id)
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	if view.Params["max_connections"] != "500" {
		t.Fatalf("override lost: %+v", view.Params)
	}
	if _, ok := view.Params["innodb_buffer_pool_size"]; ok {
		t.Fatalf("unread param must be absent, got %+v", view.Params)
	}
}

func TestSaveConfigMySQLBelow8Rejected(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	f := &fakeConfigDriver{mysqlVersion: "5.7.44"}
	id, _ := repo.CreateInstance(context.Background(), configInstance(DBTypeMySQL, "c1", 3306))
	svc := configSvc(repo, rt, f)

	err := svc.SaveInstanceConfig(context.Background(), id, map[string]string{"max_connections": "500"})
	if err == nil {
		t.Fatal("expected MySQL <8.0 to be rejected for SET PERSIST")
	}
	if !strings.Contains(err.Error(), "8.0") {
		t.Fatalf("error should mention MySQL 8.0+, got %v", err)
	}
}

func TestSaveConfigWithPortChangeRecreatesContainer(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	f := &fakeConfigDriver{mysqlVersion: "8.0.36"}
	id, _ := repo.CreateInstance(context.Background(), configInstance(DBTypeMySQL, "c1", 3306))
	svc := configSvc(repo, rt, f)

	err := svc.SaveInstanceConfig(context.Background(), id, map[string]string{"port": "4000", "max_connections": "300"})
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
	// port 不走驱动写；其余参数照常 SET PERSIST。
	if len(f.execs) != 1 || f.execs[0] != "SET PERSIST `max_connections` = 300" {
		t.Fatalf("expected only max_connections persisted, got %v", f.execs)
	}
}

func TestPostgresConfigReloadVsPostmasterRestart(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	f := &fakeConfigDriver{
		pgValues:  map[string]string{"shared_buffers": "128MB", "work_mem": "4MB"},
		pgContext: map[string]string{"shared_buffers": "postmaster", "work_mem": "user"},
	}
	inst := configInstance(DBTypePostgreSQL, "pg1", 5432)
	inst.Image = "docker.io/postgres:16"
	id, _ := repo.CreateInstance(context.Background(), inst)
	svc := configSvc(repo, rt, f)
	ctx := context.Background()

	// 只改 reload 级参数（work_mem）→ ALTER SYSTEM + reload，不重启容器。
	if err := svc.SaveInstanceConfig(ctx, id, map[string]string{"work_mem": "8MB"}); err != nil {
		t.Fatalf("save reload-level: %v", err)
	}
	if len(rt.restarted) != 0 {
		t.Fatalf("reload-level save must not restart, got %v", rt.restarted)
	}
	found := map[string]bool{}
	for _, e := range f.execs {
		found[e] = true
	}
	if !found["ALTER SYSTEM SET \"work_mem\" = '8MB'"] || !found["SELECT pg_reload_conf()"] {
		t.Fatalf("missing ALTER SYSTEM/reload, got %v", f.execs)
	}

	// 改 postmaster 级参数（shared_buffers）→ 重启容器。
	f.execs = nil
	if err := svc.SaveInstanceConfig(ctx, id, map[string]string{"shared_buffers": "256MB"}); err != nil {
		t.Fatalf("save postmaster-level: %v", err)
	}
	if len(rt.restarted) != 1 || rt.restarted[0] != "pg1" {
		t.Fatalf("postmaster-level save must restart, got %v", rt.restarted)
	}
}

func TestRedisConfigGetAndSave(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	inst := configInstance(DBTypeRedis, "r1", 6379)
	id, _ := repo.CreateInstance(context.Background(), inst)

	runner := newRedisRunner()
	defer runner.Close(id)
	mock := runnerClientFor(t, runner, inst, 0)
	svc := configSvc(repo, rt, nil)
	svc.redisOps = runner
	ctx := context.Background()

	// GET：CONFIG GET * → 面板参数取运行时值，未提供的参数不返回。
	mock.ExpectConfigGet("*").SetVal(map[string]string{
		"maxmemory": "100mb", "maxmemory-policy": "allkeys-lru", "appendonly": "no",
	})
	view, err := svc.GetInstanceConfig(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if view.Params["maxmemory"] != "100mb" {
		t.Fatalf("runtime value lost: %+v", view.Params)
	}
	if _, ok := view.Params["databases"]; ok {
		t.Fatalf("unread param must be absent, got %+v", view.Params)
	}

	// SAVE：CONFIG SET + CONFIG REWRITE。
	mock.ExpectConfigSet("maxmemory", "200mb").SetVal("OK")
	mock.ExpectConfigRewrite().SetVal("OK")
	if err := svc.SaveInstanceConfig(ctx, id, map[string]string{"maxmemory": "200mb"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
