package database

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDB describes one logical Redis database (0-15) that holds data.
type RedisDB struct {
	Index int   `json:"index"`
	Size  int64 `json:"size"` // DBSIZE: number of keys
}

// RedisKey is one key in a logical database, with the display metadata the
// front-end key browser shows per row (type / TTL / size).
type RedisKey struct {
	Name string `json:"name"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`  // seconds; -1 = no expiry, -2 = key gone
	Size int64  `json:"size"` // bytes (MEMORY USAGE)
}

// RedisValue is a key's decoded value, shaped by its type. Value is a string
// for string keys, map[string]string for hash, []string for list/set, and
// []RedisZMember for sorted sets.
type RedisValue struct {
	Type  string      `json:"type"` // string | hash | list | set | zset
	Value interface{} `json:"value"`
}

// RedisZMember is one sorted-set entry (score kept separate from member so the
// front-end can render both).
type RedisZMember struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}

// RedisOps is the direct-connection seam for Redis instances. It mirrors
// SQLRunner for the SQL engines: Service talks to this interface,
// redisRunner implements it over go-redis. Redis has no SQL, so the
// operations are key-browser primitives instead of Query/Exec.
type RedisOps interface {
	ListDBs(ctx context.Context, inst *DBInstance) ([]RedisDB, error)
	ScanKeys(ctx context.Context, inst *DBInstance, db int, cursor uint64, pattern string, count int64) ([]RedisKey, uint64, error)
	GetValue(ctx context.Context, inst *DBInstance, db int, key string) (*RedisValue, error)
	SetValue(ctx context.Context, inst *DBInstance, db int, key, value string, ttl time.Duration) error
	DelKeys(ctx context.Context, inst *DBInstance, db int, keys ...string) (int64, error)
	Expire(ctx context.Context, inst *DBInstance, db int, key string, ttl time.Duration) error
	Persist(ctx context.Context, inst *DBInstance, db int, key string) error
	FlushDB(ctx context.Context, inst *DBInstance, db int) error
	BgSave(ctx context.Context, inst *DBInstance) error
	Close(instanceID int64)
}

// redisRunner implements RedisOps over a direct go-redis connection. The
// client pool is keyed by (instance, logical db) — go-redis fixes the DB on
// client construction, and Redis instances only have 0-15, so one client per
// used db is cheap.
type redisRunner struct {
	mu    sync.Mutex
	conns map[redisPoolKey]*redis.Client
}

type redisPoolKey struct {
	instanceID int64
	db         int
}

func newRedisRunner() *redisRunner {
	return &redisRunner{conns: make(map[redisPoolKey]*redis.Client)}
}

// Close closes every client belonging to an instance (called when the instance
// is uninstalled/destroyed).
func (r *redisRunner) Close(instanceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, c := range r.conns {
		if key.instanceID == instanceID {
			c.Close()
			delete(r.conns, key)
		}
	}
}

func (r *redisRunner) clientFor(ctx context.Context, inst *DBInstance, db int) (*redis.Client, error) {
	if inst.ContainerPort <= 0 {
		return nil, fmt.Errorf("实例端口映射异常（container_port=%d），无法直连，请改端口重建", inst.ContainerPort)
	}
	key := redisPoolKey{instanceID: inst.ID, db: db}

	r.mu.Lock()
	if c, ok := r.conns[key]; ok {
		r.mu.Unlock()
		return c, nil
	}
	c := redis.NewClient(&redis.Options{
		Addr:        fmt.Sprintf("127.0.0.1:%d", inst.Port),
		Password:    inst.AdminPassword,
		DB:          db,
		DialTimeout: 5 * time.Second,
	})
	r.conns[key] = c
	r.mu.Unlock()

	// Ping establishes the first connection so a dead container / wrong mapping
	// surfaces here as a clear error, not on the first command.
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("无法连接 Redis 实例: %w", err)
	}
	return c, nil
}

// ListDBs reports the non-empty logical databases (DBSIZE > 0) in index order.
func (r *redisRunner) ListDBs(ctx context.Context, inst *DBInstance) ([]RedisDB, error) {
	var dbs []RedisDB
	for i := 0; i < 16; i++ {
		c, err := r.clientFor(ctx, inst, i)
		if err != nil {
			return nil, err
		}
		n, err := c.DBSize(ctx).Result()
		if err != nil {
			return nil, err
		}
		if n > 0 {
			dbs = append(dbs, RedisDB{Index: i, Size: n})
		}
	}
	return dbs, nil
}

// ScanKeys pages through keys with SCAN (type/TTL/size filled per key).
func (r *redisRunner) ScanKeys(ctx context.Context, inst *DBInstance, db int, cursor uint64, pattern string, count int64) ([]RedisKey, uint64, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return nil, 0, err
	}
	keys, next, err := c.Scan(ctx, cursor, pattern, count).Result()
	if err != nil {
		return nil, 0, err
	}
	out := make([]RedisKey, 0, len(keys))
	for _, k := range keys {
		rk := RedisKey{Name: k}
		if t, err := c.Type(ctx, k).Result(); err == nil {
			rk.Type = t
		}
		if d, err := c.TTL(ctx, k).Result(); err == nil {
			rk.TTL = int64(d / time.Second) // -1 (no expiry) and -2 (gone) survive division
		}
		if sz, err := c.MemoryUsage(ctx, k, 0).Result(); err == nil {
			rk.Size = sz
		}
		out = append(out, rk)
	}
	return out, next, nil
}

// GetValue reads and decodes a key's value by its type.
func (r *redisRunner) GetValue(ctx context.Context, inst *DBInstance, db int, key string) (*RedisValue, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return nil, err
	}
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	switch typ {
	case "string":
		v, err := c.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "string", Value: v}, nil
	case "hash":
		v, err := c.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "hash", Value: v}, nil
	case "list":
		v, err := c.LRange(ctx, key, 0, -1).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "list", Value: v}, nil
	case "set":
		v, err := c.SMembers(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		sort.Strings(v)
		return &RedisValue{Type: "set", Value: v}, nil
	case "zset":
		zs, err := c.ZRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			return nil, err
		}
		out := make([]RedisZMember, 0, len(zs))
		for _, z := range zs {
			out = append(out, RedisZMember{Member: fmt.Sprintf("%v", z.Member), Score: z.Score})
		}
		return &RedisValue{Type: "zset", Value: out}, nil
	default:
		// stream / rejson / module types: no structured decode, report the type
		// only so the UI can show it's unsupported for editing.
		return &RedisValue{Type: typ, Value: nil}, nil
	}
}

// SetValue writes a string key (the UI only edits string values inline).
func (r *redisRunner) SetValue(ctx context.Context, inst *DBInstance, db int, key, value string, ttl time.Duration) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, value, ttl).Err()
}

func (r *redisRunner) DelKeys(ctx context.Context, inst *DBInstance, db int, keys ...string) (int64, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return 0, err
	}
	return c.Del(ctx, keys...).Result()
}

func (r *redisRunner) Expire(ctx context.Context, inst *DBInstance, db int, key string, ttl time.Duration) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Expire(ctx, key, ttl).Err()
}

func (r *redisRunner) Persist(ctx context.Context, inst *DBInstance, db int, key string) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Persist(ctx, key).Err()
}

func (r *redisRunner) FlushDB(ctx context.Context, inst *DBInstance, db int) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.FlushDB(ctx).Err()
}

// BgSave triggers an asynchronous persistence; the caller still copies
// dump.rdb out of the container (a container file operation the driver cannot
// do). Used by backupRedis.
func (r *redisRunner) BgSave(ctx context.Context, inst *DBInstance) error {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return err
	}
	return c.BgSave(ctx).Err()
}
