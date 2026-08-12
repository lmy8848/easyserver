package database

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

// redisTestInstance is a container-backed Redis instance with a valid mapping.
func redisTestInstance() *DBInstance {
	return &DBInstance{
		ID:            1,
		DBType:        DBTypeRedis,
		Port:          6379,
		AdminPassword: "secret",
	}
}

// runnerClientFor swaps the runner's pooled client for one backed by redismock
// and returns the mock so tests can script expected commands.
func runnerClientFor(t *testing.T, r *redisRunner, inst *DBInstance, db int) redismock.ClientMock {
	t.Helper()
	client, mock := redismock.NewClientMock()
	r.mu.Lock()
	r.conns[redisPoolKey{instanceID: inst.ID, db: db}] = client
	r.mu.Unlock()
	return mock
}

func TestRedisGetValueByType(t *testing.T) {
	inst := redisTestInstance()
	runner := newRedisRunner()
	defer runner.Close(inst.ID)
	ctx := context.Background()

	tests := []struct {
		name     string
		typ      string
		value    interface{}
		wantType string
	}{
		{"string", "string", "hello", "string"},
		{"hash", "hash", map[string]string{"a": "1"}, "hash"},
		{"list", "list", []string{"x", "y"}, "list"},
		{"set", "set", []string{"b", "a"}, "set"}, // sorted by GetValue
		{"zset", "zset", []redis.Z{{Score: 1.5, Member: "m"}}, "zset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := runnerClientFor(t, runner, inst, 0)
			mock.ExpectType("k").SetVal(tt.typ)
			switch tt.typ {
			case "string":
				mock.ExpectGet("k").SetVal(tt.value.(string))
			case "hash":
				mock.ExpectHGetAll("k").SetVal(tt.value.(map[string]string))
			case "list":
				mock.ExpectLRange("k", 0, -1).SetVal(tt.value.([]string))
			case "set":
				mock.ExpectSMembers("k").SetVal(tt.value.([]string))
			case "zset":
				mock.ExpectZRangeWithScores("k", 0, -1).SetVal(tt.value.([]redis.Z))
			}

			got, err := runner.GetValue(ctx, inst, 0, "k")
			if err != nil {
				t.Fatalf("GetValue: %v", err)
			}
			if got.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", got.Type, tt.wantType)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRedisSetExpirePersistFlush(t *testing.T) {
	inst := redisTestInstance()
	runner := newRedisRunner()
	defer runner.Close(inst.ID)
	ctx := context.Background()

	mock := runnerClientFor(t, runner, inst, 0)
	mock.ExpectSet("k", "v", 0).SetVal("OK")
	if err := runner.SetValue(ctx, inst, 0, "", "k", "v", 0); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	mock.ExpectExpire("k", time.Minute).SetVal(true)
	if err := runner.Expire(ctx, inst, 0, "k", time.Minute); err != nil {
		t.Fatalf("Expire: %v", err)
	}

	mock.ExpectPersist("k").SetVal(true)
	if err := runner.Persist(ctx, inst, 0, "k"); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	mock.ExpectDel("k", "k2").SetVal(2)
	n, err := runner.DelKeys(ctx, inst, 0, "k", "k2")
	if err != nil || n != 2 {
		t.Fatalf("DelKeys = %d, %v; want 2, nil", n, err)
	}

	mock.ExpectFlushDB().SetVal("OK")
	if err := runner.FlushDB(ctx, inst, 0); err != nil {
		t.Fatalf("FlushDB: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRedisScanKeysFillsMeta(t *testing.T) {
	inst := redisTestInstance()
	runner := newRedisRunner()
	defer runner.Close(inst.ID)
	ctx := context.Background()

	mock := runnerClientFor(t, runner, inst, 0)
	mock.ExpectScan(0, "*", int64(50)).SetVal([]string{"a", "b"}, 100)
	mock.ExpectType("a").SetVal("string")
	mock.ExpectTTL("a").SetVal(-1 * time.Second) // permanent
	mock.ExpectMemoryUsage("a", 0).SetVal(42)
	mock.ExpectType("b").SetVal("hash")
	mock.ExpectTTL("b").SetVal(3600 * time.Second)
	mock.ExpectMemoryUsage("b", 0).SetVal(128)

	keys, next, err := runner.ScanKeys(ctx, inst, 0, 0, "*", 50)
	if err != nil {
		t.Fatalf("ScanKeys: %v", err)
	}
	if next != 100 {
		t.Fatalf("next cursor = %d, want 100", next)
	}
	if len(keys) != 2 || keys[0].Name != "a" || keys[0].TTL != -1 || keys[0].Size != 42 {
		t.Fatalf("unexpected keys: %+v", keys)
	}
	if keys[1].TTL != 3600 || keys[1].Type != "hash" {
		t.Fatalf("unexpected second key: %+v", keys[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRedisSetTypedValue(t *testing.T) {
	inst := redisTestInstance()
	runner := newRedisRunner()
	defer runner.Close(inst.ID)
	ctx := context.Background()
	mock := runnerClientFor(t, runner, inst, 0)

	// hash
	mock.ExpectHSet("h", map[string]string{"f": "v"}).SetVal(1)
	if err := runner.SetValue(ctx, inst, 0, "hash", "h", map[string]string{"f": "v"}, 0); err != nil {
		t.Fatalf("hash SetValue: %v", err)
	}
	// list
	mock.ExpectRPush("l", "a", "b").SetVal(2)
	if err := runner.SetValue(ctx, inst, 0, "list", "l", []any{"a", "b"}, 0); err != nil {
		t.Fatalf("list SetValue: %v", err)
	}
	// set
	mock.ExpectSAdd("s", "x").SetVal(1)
	if err := runner.SetValue(ctx, inst, 0, "set", "s", []string{"x"}, 0); err != nil {
		t.Fatalf("set SetValue: %v", err)
	}
	// zset with TTL → ZAdd then Expire
	mock.ExpectZAdd("z", redis.Z{Score: 1.5, Member: "m"}).SetVal(1)
	mock.ExpectExpire("z", 10*time.Second).SetVal(true)
	if err := runner.SetValue(ctx, inst, 0, "zset", "z", []any{map[string]any{"member": "m", "score": 1.5}}, 10*time.Second); err != nil {
		t.Fatalf("zset SetValue: %v", err)
	}
	// 空 hash 拒绝
	if err := runner.SetValue(ctx, inst, 0, "hash", "e", map[string]string{}, 0); err == nil {
		t.Fatal("empty hash should be rejected")
	}
	// 未知类型拒绝
	if err := runner.SetValue(ctx, inst, 0, "stream", "s", "v", 0); err == nil {
		t.Fatal("unsupported type should be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRedisDialFailureSurfacesConnectionError(t *testing.T) {
	// A dead container / wrong mapping surfaces as a connection error from the
	// first command (clientFor pings on first use) — the guard that used to sit
	// on container_port is gone, the dial error is the signal now.
	inst := &DBInstance{ID: 9, DBType: DBTypeRedis, Port: 1}
	runner := newRedisRunner()
	_, _, err := runner.ScanKeys(context.Background(), inst, 0, 0, "*", 10)
	if err == nil {
		t.Fatal("expected dial error for unreachable instance")
	}
}
