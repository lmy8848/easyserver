package http

import (
	"strconv"

	"easyserver/internal/database"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"

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

func (h *RedisHandler) parseDB(c *gin.Context) (int, bool) {
	db, err := strconv.Atoi(c.DefaultQuery("db", "0"))
	// 不设固定上限：databases 数量由 redis.conf 的 databases 指令决定（面板配置页
	// 暴露为启动期参数），默认 16 但可改。越界索引 redis 自己回 "ERR DB index is
	// out of range"，比硬编码 15 诚实。
	if err != nil || db < 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 Redis DB 索引"))
		return 0, false
	}
	return db, true
}

// ScanKeys pages through keys (SCAN cursor) with type/TTL/size per key.

// DBCount returns the configured logical database slot count (CONFIG GET
// databases, default 16). The key-browser dropdown renders one option per slot,
// so it must follow the instance's own config rather than a hardcoded 16.
func (h *RedisHandler) DBCount(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	n, err := h.svc.RedisDBCount(c.Request.Context(), iid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"databases": n})
}
func (h *RedisHandler) ScanKeys(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	db, ok := h.parseDB(c)
	if !ok {
		return
	}
	cursor, _ := strconv.ParseUint(c.DefaultQuery("cursor", "0"), 10, 64)
	pattern := c.DefaultQuery("pattern", "*")
	count, _ := strconv.ParseInt(c.DefaultQuery("count", "50"), 10, 64)
	if count <= 0 {
		count = 50
	}
	keys, next, err := h.svc.ScanRedisKeys(c.Request.Context(), iid, db, cursor, pattern, count)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"keys": keys, "next_cursor": next, "db": db})
}

// GetValue reads one key's decoded value.
func (h *RedisHandler) GetValue(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	db, ok := h.parseDB(c)
	if !ok {
		return
	}
	key := c.Query("key")
	if key == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("key 不能为空"))
		return
	}
	val, err := h.svc.GetRedisValue(c.Request.Context(), iid, db, key)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, val)
}

type setRedisValueRequest struct {
	DB    int    `json:"db"`
	Type  string `json:"type"` // string | hash | list | set | zset（空 = string）
	Key   string `json:"key" binding:"required"`
	Value any    `json:"value"` // string / {field:value} / []string / []{member,score}
	TTL   int64  `json:"ttl"`   // seconds; <=0 = no expiry
}

// SetValue creates a key of the requested type.
func (h *RedisHandler) SetValue(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req setRedisValueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "写入 Redis key "+req.Key)
	if err := h.svc.SetRedisValue(c.Request.Context(), iid, req.DB, req.Type, req.Key, req.Value, req.TTL); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

type delRedisKeysRequest struct {
	DB   int      `json:"db"`
	Keys []string `json:"keys" binding:"required"`
}

// DelKeys deletes one or more keys.
func (h *RedisHandler) DelKeys(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req delRedisKeysRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "删除 Redis key "+strconv.Itoa(len(req.Keys))+" 个")
	n, err := h.svc.DeleteRedisKeys(c.Request.Context(), iid, req.DB, req.Keys)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"deleted": n})
}

type expireRedisKeyRequest struct {
	DB  int    `json:"db"`
	Key string `json:"key" binding:"required"`
	TTL int64  `json:"ttl" binding:"required"` // seconds; <=0 = no expiry
}

// Expire sets a key's TTL.
func (h *RedisHandler) Expire(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req expireRedisKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	// EXPIRE with ttl 0 deletes the key — refuse, and point at persist instead.
	if req.TTL <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("TTL 必须大于 0；移除过期请用持久化"))
		return
	}
	middleware.AuditSummary(c, "设置 Redis key 过期 "+req.Key)
	if err := h.svc.ExpireRedisKey(c.Request.Context(), iid, req.DB, req.Key, req.TTL); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

type persistRedisKeyRequest struct {
	DB  int    `json:"db"`
	Key string `json:"key" binding:"required"`
}

// Persist removes a key's TTL.
func (h *RedisHandler) Persist(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req persistRedisKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if err := h.svc.PersistRedisKey(c.Request.Context(), iid, req.DB, req.Key); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

type flushRedisDBRequest struct {
	DB int `json:"db"`
}

// FlushDB removes all keys from a logical DB.
func (h *RedisHandler) FlushDB(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req flushRedisDBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "清空 Redis DB "+strconv.Itoa(req.DB))
	if err := h.svc.FlushRedisDB(c.Request.Context(), iid, req.DB); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}
