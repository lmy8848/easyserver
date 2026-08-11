package database

// Lifecycle tests drive the Service through the DatabaseRuntime seam with a
// fake runtime, per the PRD (verify create / health-fail / start-stop /
// destroy without a real container runtime). They assert observable behavior
// only — never CLI command concatenation.

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// --- SanitizeSQLError tests ---
// SQL errors are sanitized before reaching the UI so file paths never leak.

func TestSanitizeSQLError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips paths, keeps message",
			input: "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)\n/usr/bin/mysql",
			want:  "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)\n[...]",
		},
		{
			name:  "multiple paths on one line",
			input: "error at /var/lib/mysql/data and /etc/mysql/conf",
			want:  "error at [...] and [...]",
		},
		{
			name:  "empty after trimming",
			input: "   \n   \n   ",
			want:  "",
		},
		{
			name:  "no paths",
			input: "ERROR: syntax error at or near \"SELEC\"",
			want:  "ERROR: syntax error at or near \"SELEC\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSQLError(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSQLError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- isValidTableName tests ---

func TestIsValidTableName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"users", true},
		{"_internal", true},
		{"table_01", true},
		{"", false},
		{"a]b", false},
		{"table name", false},
		{"table-name", false},
		{"a", true},
		{strings.Repeat("a", 65), false}, // 65 chars, too long
		{strings.Repeat("a", 64), true},  // 64 chars, max
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			if got := isValidTableName(tt.name); got != tt.want {
				t.Errorf("isValidTableName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// fakeDBRuntime records container lifecycle calls and returns scripted status.
type fakeDBRuntime struct {
	createSpecs []ContainerSpec
	status      ContainerStatus
	statusErr   error
	removed     []string
	removedVol  []string
	started     []string
	stopped     []string
	exists      bool // 预检 Exists 的返回值（默认 false）
	seedCalls   []seedCall
}

func (f *fakeDBRuntime) Create(_ context.Context, spec ContainerSpec) error {
	f.createSpecs = append(f.createSpecs, spec)
	return nil
}
func (f *fakeDBRuntime) Start(_ context.Context, _, name string) error {
	f.started = append(f.started, name)
	return nil
}
func (f *fakeDBRuntime) Stop(_ context.Context, _, name string) error {
	f.stopped = append(f.stopped, name)
	return nil
}
func (f *fakeDBRuntime) Restart(context.Context, string, string) error { return nil }
func (f *fakeDBRuntime) Remove(_ context.Context, _, name string) error {
	f.removed = append(f.removed, name)
	return nil
}
func (f *fakeDBRuntime) Status(context.Context, string, string) (ContainerStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeDBRuntime) Logs(context.Context, string, string, int) (string, error) { return "", nil }
func (f *fakeDBRuntime) Exec(context.Context, string, string, ...string) (string, error) {
	return "", nil
}
func (f *fakeDBRuntime) CopyFrom(context.Context, string, string, string, string) error { return nil }
func (f *fakeDBRuntime) CopyTo(context.Context, string, string, string, string) error   { return nil }
func (f *fakeDBRuntime) RemoveVolume(_ context.Context, _, volume string) error {
	f.removedVol = append(f.removedVol, volume)
	return nil
}
func (f *fakeDBRuntime) Exists(context.Context, string, string) (bool, error) {
	return f.exists, nil
}
func (f *fakeDBRuntime) SeedVolumeFile(_ context.Context, _, _, volume, dest, content string) error {
	f.seedCalls = append(f.seedCalls, seedCall{volume: volume, dest: dest, content: content})
	return nil
}
func (f *fakeDBRuntime) ReadVolumeFile(_ context.Context, _, _, volume, path string) (string, error) {
	// 回放最后一次写入该卷+路径的文件内容；无则返回空（新实例/未配置）。
	for i := len(f.seedCalls) - 1; i >= 0; i-- {
		if f.seedCalls[i].volume == volume && f.seedCalls[i].dest == path {
			return f.seedCalls[i].content, nil
		}
	}
	return "", nil
}

// seedCall records one SeedVolumeFile call for the structured-config test.
type seedCall struct {
	volume  string
	dest    string
	content string
}

// fakeRepo is a minimal in-memory Repository for the lifecycle tests.
type fakeRepo struct {
	instances map[int64]*DBInstance
	nextID    int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		instances: map[int64]*DBInstance{},
		nextID:    100,
	}
}
func (r *fakeRepo) ListInstances(context.Context, DBType) ([]DBInstance, error) {
	var out []DBInstance
	for _, v := range r.instances {
		out = append(out, *v)
	}
	return out, nil
}
func (r *fakeRepo) GetInstance(_ context.Context, id int64) (*DBInstance, error) {
	return r.instances[id], nil
}
func (r *fakeRepo) CountInstancesByDBTypeAndVersion(context.Context, DBType, string) (int, error) {
	return 0, nil
}
func (r *fakeRepo) CreateInstance(_ context.Context, v *DBInstance) (int64, error) {
	r.nextID++
	v.ID = r.nextID
	r.instances[v.ID] = v
	return v.ID, nil
}
func (r *fakeRepo) DeleteInstance(_ context.Context, id int64) error {
	delete(r.instances, id)
	return nil
}
func (r *fakeRepo) UpdateInstanceStatus(_ context.Context, id int64, status string) error {
	if v := r.instances[id]; v != nil {
		v.Status = status
	}
	return nil
}
func (r *fakeRepo) UpdateInstancePort(_ context.Context, id int64, port int) error {
	if v := r.instances[id]; v != nil {
		v.Port = port
	}
	return nil
}
func (r *fakeRepo) UpdateInstancePassword(_ context.Context, id int64, pw string) error {
	if v := r.instances[id]; v != nil {
		v.AdminPassword = pw
	}
	return nil
}

// backup operations (unused by the lifecycle tests, but required by the
// Repository interface).
func (r *fakeRepo) CreateBackup(context.Context, *DBBackup) (int64, error) { return 0, nil }
func (r *fakeRepo) UpdateBackupStatus(context.Context, int64, string, int64, string) error {
	return nil
}
func (r *fakeRepo) ListBackups(context.Context, int64, string) ([]DBBackup, error) {
	return nil, nil
}
func (r *fakeRepo) GetBackup(context.Context, int64) (*DBBackup, error) { return nil, nil }
func (r *fakeRepo) DeleteBackup(context.Context, int64) error           { return nil }

func TestCreateInstanceHealthy(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt)

	res, err := svc.CreateInstance(context.Background(), DBTypeMySQL, &CreateDBInstanceRequest{Version: "8.0", Port: 3306, Image: "mysql:8.0"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	// The row exists from submit time (status "installing"); the install
	// goroutine flips it to "running". What matters here is no "stopped"
	// instance ever appears.
	if err := svc.WaitForInstall(res.InstallID); err != nil {
		t.Fatalf("install wait: %v", err)
	}
	got := findInstanceByStatus(repo, "running")
	if got == nil {
		t.Fatalf("expected running instance after install, got %+v", repo.instances)
	}
	if got.BindAddress != "127.0.0.1" {
		t.Fatalf("expected loopback bind by default, got %q", got.BindAddress)
	}
	if len(rt.createSpecs) != 1 || rt.createSpecs[0].Name != got.ContainerName {
		t.Fatalf("unexpected create specs: %+v", rt.createSpecs)
	}
	if len(rt.removed) != 0 {
		t.Fatalf("healthy install must not remove the container, got %v", rt.removed)
	}
}

func TestCreateInstanceHealthFailKeepsContainer(t *testing.T) {
	repo := newFakeRepo()
	// Container exits before becoming healthy → waitForHealthy fails fast. The
	// container is deliberately kept for troubleshooting (its logs are lost on
	// rm); reinstall runs "uninstall + install", and uninstall removes it.
	rt := &fakeDBRuntime{status: ContainerStatus{State: "exited"}}
	svc := NewServiceWithRuntime(repo, rt)

	res, err := svc.CreateInstance(context.Background(), DBTypeMySQL, &CreateDBInstanceRequest{Version: "8.0", Port: 3306, Image: "mysql:8.0"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.WaitForInstall(res.InstallID); err == nil {
		t.Fatal("expected install to fail when container never becomes healthy")
	}
	if len(rt.removed) != 0 {
		t.Fatalf("failed install must keep the container for inspection, removed=%v", rt.removed)
	}
	got := findInstanceByStatus(repo, "failed")
	if got == nil {
		t.Fatalf("expected failed instance row kept for inspection, got %+v", repo.instances)
	}
}

func TestUninstallPurgeRemovesContainerAndVolume(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt)

	res, err := svc.CreateInstance(context.Background(), DBTypeMySQL, &CreateDBInstanceRequest{Version: "8.0", Port: 3306, Image: "mysql:8.0"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.WaitForInstall(res.InstallID); err != nil {
		t.Fatalf("install wait: %v", err)
	}
	got := findInstanceByStatus(repo, "running")
	if got == nil {
		t.Fatalf("expected running instance after install, got %+v", repo.instances)
	}

	// Uninstall with purge=true — the container, both volumes and the metadata
	// row all go away (this is the former DestroyInstance behavior).
	if err := svc.UninstallInstance(context.Background(), got.ID, true); err != nil {
		t.Fatalf("uninstall(purge): %v", err)
	}
	if len(rt.removed) != 1 || rt.removed[0] != got.ContainerName {
		t.Fatalf("expected container removed, got %v", rt.removed)
	}
	if len(rt.removedVol) != 2 || rt.removedVol[0] != got.VolumeName {
		t.Fatalf("expected data + config volumes removed, got %v", rt.removedVol)
	}
	if _, ok := repo.instances[got.ID]; ok {
		t.Fatal("expected instance metadata deleted")
	}
}

func TestPostgres18MovesDataDir(t *testing.T) {
	// postgres:18+ moved PGDATA into a version subdir — the volume must mount the
	// parent (/var/lib/postgresql) and the auto.conf override lives under the
	// version dir. Older majors keep the classic /var/lib/postgresql/data layout.
	// Empty dataDir skips the pgDataDir assertion (that helper is postgres-only).
	cases := []struct {
		image    string
		dataDir  string
		confPath string
	}{
		{"docker.io/postgres:18", "/var/lib/postgresql", "/var/lib/postgresql/18/docker/postgresql.auto.conf"},
		{"docker.io/postgres:18-alpine", "/var/lib/postgresql", "/var/lib/postgresql/18/docker/postgresql.auto.conf"},
		{"docker.io/postgres:17", "/var/lib/postgresql/data", "/var/lib/postgresql/data/postgresql.auto.conf"},
		{"docker.io/postgres:16", "/var/lib/postgresql/data", "/var/lib/postgresql/data/postgresql.auto.conf"},
		// config paths must survive the fully-qualified image form used at runtime
		{"docker.io/mysql:9.7", "", "/etc/mysql/conf.d/easyserver.cnf"},
		{"docker.io/redis:8.0-alpine", "", "/usr/local/etc/redis/redis.conf"},
	}
	for _, c := range cases {
		if c.dataDir != "" {
			if got := pgDataDir(c.image); got != c.dataDir {
				t.Errorf("%s: data dir = %q, want %q", c.image, got, c.dataDir)
			}
		}
		if got := configPathForImage(c.image); got != c.confPath {
			t.Errorf("%s: config path = %q, want %q", c.image, got, c.confPath)
		}
	}
}

func TestValidateContainerName(t *testing.T) {
	valid := []string{"easyserver-db-mysql-8", "my-db", "MyDB_1", "a.b-c", "0x"}
	invalid := []string{"", "-lead", ".lead", "has space", "has/slash", "结尾-", "a:b", "Ünïcode"}
	for _, name := range valid {
		if err := validateContainerName(name); err != nil {
			t.Errorf("validateContainerName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range invalid {
		if err := validateContainerName(name); err == nil {
			t.Errorf("validateContainerName(%q) = nil, want error", name)
		}
	}
	tooLong := strings.Repeat("a", maxContainerNameLen+1)
	if err := validateContainerName(tooLong); err == nil {
		t.Error("expected over-length name rejected")
	}
}

func TestDefaultContainerName(t *testing.T) {
	if got := defaultContainerName(DBTypeMySQL, "8.0", ""); got != "easyserver-db-mysql-8-0" {
		t.Fatalf("default = %q, want easyserver-db-mysql-8-0", got)
	}
	if got := defaultContainerName(DBTypeMySQL, "8.0", "my-custom"); got != "my-custom" {
		t.Fatalf("custom = %q, want my-custom", got)
	}
}

func TestCreateInstanceRejectsTakenContainerName(t *testing.T) {
	// 预检：同名容器已存在 → CreateInstance 报错，且不写 row、不起任务。
	repo := newFakeRepo()
	rt := &fakeDBRuntime{exists: true}
	svc := NewServiceWithRuntime(repo, rt)

	_, err := svc.CreateInstance(context.Background(), DBTypeMySQL,
		&CreateDBInstanceRequest{Version: "8.0", Port: 3306, Image: "mysql:8.0", ContainerName: "taken"})
	if err == nil {
		t.Fatal("expected create to fail when container name is taken")
	}
	if len(repo.instances) != 0 {
		t.Fatalf("no row must be written when name is taken, got %+v", repo.instances)
	}
	if len(rt.createSpecs) != 0 {
		t.Fatalf("no task must start when name is taken, got %+v", rt.createSpecs)
	}
}

// findInstanceByStatus returns the (single) instance row for the database type with
// the given status, or nil. Installs write exactly one row on completion.
func findInstanceByStatus(repo *fakeRepo, status string) *DBInstance {
	rows, _ := repo.ListInstances(context.Background(), DBTypeMySQL)
	for i := range rows {
		if rows[i].Status == status {
			return &rows[i]
		}
	}
	return nil
}
