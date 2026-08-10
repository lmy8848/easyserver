package database

// Lifecycle tests drive the Service through the DatabaseRuntime seam with a
// fake runtime, per the PRD (verify create / health-fail / start-stop /
// destroy without a real container runtime). They assert observable behavior
// only — never CLI command concatenation.

import (
	"context"
	"testing"
)

// fakeDBRuntime records container lifecycle calls and returns scripted status.
type fakeDBRuntime struct {
	createSpecs []ContainerSpec
	status      ContainerStatus
	statusErr   error
	removed     []string
	removedVol  []string
	started     []string
	stopped     []string
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
	if len(rt.createSpecs) != 1 || rt.createSpecs[0].Name != got.ContainerID {
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

func TestDestroyRemovesContainerAndVolume(t *testing.T) {
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

	if err := svc.DestroyInstance(context.Background(), got.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(rt.removed) != 1 || rt.removed[0] != got.ContainerID {
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
	// parent (/var/lib/postgresql) and config lives under the version dir. Older
	// majors keep the classic /var/lib/postgresql/data layout. Empty dataDir skips
	// the pgDataDir assertion (that helper is postgres-only).
	cases := []struct {
		image    string
		dataDir  string
		confPath string
	}{
		{"docker.io/postgres:18", "/var/lib/postgresql", "/var/lib/postgresql/18/docker/postgresql.conf"},
		{"docker.io/postgres:18-alpine", "/var/lib/postgresql", "/var/lib/postgresql/18/docker/postgresql.conf"},
		{"docker.io/postgres:17", "/var/lib/postgresql/data", "/var/lib/postgresql/data/postgresql.conf"},
		{"docker.io/postgres:16", "/var/lib/postgresql/data", "/var/lib/postgresql/data/postgresql.conf"},
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
