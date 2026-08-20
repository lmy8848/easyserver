package http

import (
	"strconv"

	"easyserver/internal/domain/database"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// RedisHandler handles the Redis key-browser endpoints. Redis has no SQL, so
// these are key-browser primitives (logical DBs, SCAN, value read/write, TTL,
// flush) instead of the database/user/table CRUD of the SQL engines.
type RedisHandler struct {
	svc *database.Service
}

func NewRedisHandler(svc *database.Service) *RedisHandler {
	return &RedisHandler{svc: svc}
}

func (h *RedisHandler) parseDB(c *gin.Context) (int, error) {
	db, err := strconv.Atoi(c.DefaultQuery("db", "0"))
	// 不设固定上限：databases 数量由 redis.conf 的 databases 指令决定（面板配置页
	// 暴露为启动期参数），默认 16 但可改。越界索引 redis 自己回 "ERR DB index is
	// out of range"，比硬编码 15 诚实。
	if err != nil || db < 0 {
		return 0, errx.BadRequest("无效的 Redis DB 索引")
	}
	return db, nil
}

// DBCount returns the configured logical database slot count (CONFIG GET
// databases, default 16). The key-browser dropdown renders one option per slot,
// so it must follow the instance's own config rather than a hardcoded 16.
func (h *RedisHandler) DBCount(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	n, err := h.svc.RedisDBCount(c.Request.Context(), iid)
	if err != nil {
		return nil, err
	}
	return gin.H{"databases": n}, nil
}

// ScanKeys pages through keys (SCAN cursor) with type/TTL/size per key.
func (h *RedisHandler) ScanKeys(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	db, err := h.parseDB(c)
	if err != nil {
		return nil, err
	}
	cursor, _ := strconv.ParseUint(c.DefaultQuery("cursor", "0"), 10, 64)
	pattern := c.DefaultQuery("pattern", "*")
	count, _ := strconv.ParseInt(c.DefaultQuery("count", "50"), 10, 64)
	if count <= 0 {
		count = 50
	}
	keys, next, err := h.svc.ScanRedisKeys(c.Request.Context(), iid, db, cursor, pattern, count)
	if err != nil {
		return nil, err
	}
	return gin.H{"keys": keys, "next_cursor": next, "db": db}, nil
}

// GetValue reads one key's decoded value.
func (h *RedisHandler) GetValue(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	db, err := h.parseDB(c)
	if err != nil {
		return nil, err
	}
	key := c.Query("key")
	if key == "" {
		return nil, errx.BadRequest("key 不能为空")
	}
	val, err := h.svc.GetRedisValue(c.Request.Context(), iid, db, key)
	if err != nil {
		return nil, err
	}
	return val, nil
}

type setRedisValueRequest struct {
	DB          int                      `json:"db"`
	Type        string                   `json:"type"` // string | hash | list | set | zset; 空 = string
	Key         string                   `json:"key" binding:"required"`
	Value       string                   `json:"value"`        // string 类型的值
	TTL         *int64                   `json:"ttl"`          // nil = 不更新（编辑保持原 TTL）；0 = 永久；>0 = 设置过期 N 秒
	HashFields  []database.RedisHashPair `json:"hash_fields"`  // hash：字段-值对
	Values      []string                 `json:"values"`       // list / set：元素
	ZSetMembers []database.RedisZMember  `json:"zset_members"` // zset：分值-成员对
}

// SetValue writes a string key (SET, 覆盖 — 编辑也走这里) or creates a
// collection key (HSET/RPUSH/SADD/ZADD). 集合类型 = 添加新键：已有同名键由
// 后端拒绝，前端弹窗对集合只开放添加、不开放编辑。
func (h *RedisHandler) SetValue(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	req, err := httpx.BindJSON[setRedisValueRequest](c)
	if err != nil {
		return nil, err
	}
	typ := req.Type
	if typ == "" {
		typ = "string"
	}
	// 0 非法：原生 EXPIRE 0 会删键、SET EX 0 报错。永久统一用 -1。
	if req.TTL != nil && *req.TTL == 0 {
		return nil, errx.BadRequest("TTL 不能为 0；永久请用 -1")
	}
	if typ == "string" {
		middleware.AuditSummary(c, "写入 Redis key "+req.Key)
		if err := h.svc.SetRedisValue(c.Request.Context(), iid, req.DB, req.Key, req.Value, req.TTL); err != nil {
			return nil, err
		}
		return nil, nil
	}
	switch typ {
	case "hash":
		if len(req.HashFields) == 0 {
			return nil, errx.BadRequest("hash 至少需要一个字段")
		}
	case "list", "set":
		if len(req.Values) == 0 {
			return nil, errx.BadRequest("%s 至少需要一个元素", typ)
		}
	case "zset":
		if len(req.ZSetMembers) == 0 {
			return nil, errx.BadRequest("zset 至少需要一个成员")
		}
	default:
		return nil, errx.BadRequest("不支持的 Redis 类型: %s", typ)
	}
	middleware.AuditSummary(c, "添加 Redis key "+req.Key)
	// 集合类型添加 = 新建键：ttl 缺省或 -1 视为永久（0），>0 才补 EXPIRE。
	// 0 已在上方统一拦截。
	ttl := int64(0)
	if req.TTL != nil && *req.TTL > 0 {
		ttl = *req.TTL
	}
	if err := h.svc.AddRedisKey(c.Request.Context(), iid, req.DB, typ, req.Key, ttl, req.HashFields, req.Values, req.ZSetMembers); err != nil {
		return nil, err
	}
	return nil, nil
}

type delRedisKeysRequest struct {
	DB   int      `json:"db"`
	Keys []string `json:"keys" binding:"required"`
}

// DelKeys deletes one or more keys.
func (h *RedisHandler) DelKeys(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	req, err := httpx.BindJSON[delRedisKeysRequest](c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "删除 Redis key "+strconv.Itoa(len(req.Keys))+" 个")
	n, err := h.svc.DeleteRedisKeys(c.Request.Context(), iid, req.DB, req.Keys)
	if err != nil {
		return nil, err
	}
	return gin.H{"deleted": n}, nil
}

type expireRedisKeyRequest struct {
	DB  int    `json:"db"`
	Key string `json:"key" binding:"required"`
	TTL int64  `json:"ttl" binding:"required"` // seconds; 必须 >0（0/-1 在此 handler 拒绝）
}

// Expire sets a key's TTL.
func (h *RedisHandler) Expire(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	req, err := httpx.BindJSON[expireRedisKeyRequest](c)
	if err != nil {
		return nil, err
	}
	// EXPIRE with ttl 0 deletes the key — refuse; -1 (永久) goes through persist.
	if req.TTL <= 0 {
		return nil, errx.BadRequest("TTL 必须大于 0；永久请用 -1（PERSIST）")
	}
	middleware.AuditSummary(c, "设置 Redis key 过期 "+req.Key)
	if err := h.svc.ExpireRedisKey(c.Request.Context(), iid, req.DB, req.Key, req.TTL); err != nil {
		return nil, err
	}
	return nil, nil
}

type persistRedisKeyRequest struct {
	DB  int    `json:"db"`
	Key string `json:"key" binding:"required"`
}

// Persist removes a key's TTL.
func (h *RedisHandler) Persist(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	req, err := httpx.BindJSON[persistRedisKeyRequest](c)
	if err != nil {
		return nil, err
	}
	if err := h.svc.PersistRedisKey(c.Request.Context(), iid, req.DB, req.Key); err != nil {
		return nil, err
	}
	return nil, nil
}

type flushRedisDBRequest struct {
	DB int `json:"db"`
}

// FlushDB removes all keys from a logical DB.
func (h *RedisHandler) FlushDB(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	req, err := httpx.BindJSON[flushRedisDBRequest](c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "清空 Redis DB "+strconv.Itoa(req.DB))
	if err := h.svc.FlushRedisDB(c.Request.Context(), iid, req.DB); err != nil {
		return nil, err
	}
	return nil, nil
}
