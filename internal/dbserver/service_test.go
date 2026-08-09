package dbserver

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
	servers   map[int64]*DBServer
	instances map[int64]*DBInstance
	nextID    int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		servers: map[int64]*DBServer{
			1: {ID: 1, Name: "mysql", DisplayName: "MySQL", DefaultPort: 3306},
		},
		instances: map[int64]*DBInstance{},
		nextID:    100,
	}
}
func (r *fakeRepo) ListServers(context.Context) ([]DBServer, error) { return nil, nil }
func (r *fakeRepo) GetServer(_ context.Context, id int64) (*DBServer, error) {
	return r.servers[id], nil
}
func (r *fakeRepo) SeedServer(context.Context, string, string, string, int) error { return nil }
func (r *fakeRepo) ListVersions(context.Context, int64) ([]DBInstance, error) {
	var out []DBInstance
	for _, v := range r.instances {
		out = append(out, *v)
	}
	return out, nil
}
func (r *fakeRepo) GetVersion(_ context.Context, id int64) (*DBInstance, error) {
	return r.instances[id], nil
}
func (r *fakeRepo) CountVersionsByServerAndVersion(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (r *fakeRepo) CreateVersion(context.Context, int64, string, string, int, string) (int64, error) {
	return 0, nil
}
func (r *fakeRepo) CreateContainerVersion(_ context.Context, v *DBInstance) (int64, error) {
	r.nextID++
	v.ID = r.nextID
	r.instances[v.ID] = v
	return v.ID, nil
}
func (r *fakeRepo) DeleteVersion(_ context.Context, id int64) error {
	delete(r.instances, id)
	return nil
}
func (r *fakeRepo) CountDatabasesByVersion(context.Context, int64) (int, error) { return 0, nil }
func (r *fakeRepo) UpdateVersionStatus(_ context.Context, id int64, status string) error {
	if v := r.instances[id]; v != nil {
		v.Status = status
	}
	return nil
}
func (r *fakeRepo) UpdateVersionPort(_ context.Context, id int64, port int) error {
	if v := r.instances[id]; v != nil {
		v.Port = port
	}
	return nil
}
func (r *fakeRepo) UpdateVersionPassword(_ context.Context, id int64, pw string) error {
	if v := r.instances[id]; v != nil {
		v.AdminPassword = pw
	}
	return nil
}
func (r *fakeRepo) UpdateServerStatus(context.Context, int64, string, string) error { return nil }

func mustKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func TestInstallVersionHealthy(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt, string(mustKey(t)))

	v, err := svc.InstallVersion(context.Background(), 1, &CreateDBInstanceRequest{Version: "8.0"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if v == nil || v.Status != "running" {
		t.Fatalf("expected running instance, got %+v", v)
	}
	if v.BindAddress != "127.0.0.1" {
		t.Fatalf("expected loopback bind by default, got %q", v.BindAddress)
	}
	if len(rt.createSpecs) != 1 || rt.createSpecs[0].Name != v.ContainerName {
		t.Fatalf("unexpected create specs: %+v", rt.createSpecs)
	}
	if len(rt.removed) != 0 {
		t.Fatalf("healthy install must not remove the container, got %v", rt.removed)
	}
}

func TestInstallVersionHealthFailRollsBack(t *testing.T) {
	repo := newFakeRepo()
	// Container exits before becoming healthy → waitForHealthy fails fast and
	// InstallVersion must roll back the container.
	rt := &fakeDBRuntime{status: ContainerStatus{State: "exited"}}
	svc := NewServiceWithRuntime(repo, rt, string(mustKey(t)))

	if _, err := svc.InstallVersion(context.Background(), 1, &CreateDBInstanceRequest{Version: "8.0"}); err == nil {
		t.Fatal("expected install to fail when container never becomes healthy")
	}
	if len(rt.removed) != 1 {
		t.Fatalf("expected container rollback after health failure, removed=%v", rt.removed)
	}
}

func TestDestroyRemovesContainerAndVolume(t *testing.T) {
	repo := newFakeRepo()
	rt := &fakeDBRuntime{status: ContainerStatus{State: "running", Health: "healthy"}}
	svc := NewServiceWithRuntime(repo, rt, string(mustKey(t)))

	v, err := svc.InstallVersion(context.Background(), 1, &CreateDBInstanceRequest{Version: "8.0"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if err := svc.DestroyVersion(context.Background(), v.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(rt.removed) != 1 || rt.removed[0] != v.ContainerName {
		t.Fatalf("expected container removed, got %v", rt.removed)
	}
	if len(rt.removedVol) != 2 || rt.removedVol[0] != v.VolumeName {
		t.Fatalf("expected data + config volumes removed, got %v", rt.removedVol)
	}
	if _, ok := repo.instances[v.ID]; ok {
		t.Fatal("expected instance metadata deleted")
	}
}
