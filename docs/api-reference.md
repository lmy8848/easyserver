# EasyServer API 参考

> 基础路径：`/api`  
> 认证方式：`Authorization: Bearer <token>`  
> 响应格式：`{ "code": 0, "message": "ok", "data": {...} }`

## 通用约定

| 规范 | 说明 |
|------|------|
| 成功 | `code: 0`, `message: "ok"` |
| 客户端错误 | `code: 4xxxx`, `message: 错误描述` |
| 服务端错误 | `code: 5xxxx`, `message: 错误描述` |
| 分页 | 统一参数 `?page=1&page_size=20`，响应 `{ total, items }` |
| 时间格式 | ISO 8601: `2026-06-14T12:00:00Z` |

### 错误码

| 错误码 | 含义 |
|--------|------|
| 0 | 成功 |
| 40000 | 请求参数错误 |
| 40100 | 未认证 |
| 40101 | Token 过期 |
| 40300 | 无权限 |
| 40400 | 资源不存在 |
| 42900 | 请求频率过高 |
| 50000 | 服务器内部错误 |

---

## 1. 认证

### POST /api/auth/login

登录获取 Token。

**请求：**

```json
{
  "username": "admin",
  "password": "your-password"
}
```

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin"
    },
    "must_change_pass": true
  }
}
```

### POST /api/auth/logout

登出（客户端清除 Token）。

**响应：**

```json
{ "code": 0, "message": "ok", "data": null }
```

### GET /api/auth/me

获取当前用户信息（需要认证）。

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "created_at": "2026-01-01T00:00:00Z",
    "last_login": "2026-06-14T10:00:00Z"
  }
}
```

### POST /api/auth/change-password

修改密码（需要认证）。

**请求：**

```json
{
  "old_password": "current-password",
  "new_password": "new-password"
}
```

**响应：**

```json
{ "code": 0, "message": "ok", "data": null }
```

---

## 2. 系统监控

### GET /api/monitor/stats

获取当前系统状态快照（需要认证）。

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "cpu": {
      "usage_percent": 45.2,
      "load_1m": 2.1,
      "load_5m": 1.8,
      "load_15m": 1.5
    },
    "memory": {
      "total_bytes": 17179869184,
      "used_bytes": 8589934592,
      "usage_percent": 50.0
    },
    "disk": [
      {
        "mount_point": "/",
        "total_bytes": 107374182400,
        "used_bytes": 64424509440,
        "usage_percent": 60.0
      }
    ],
    "network": {
      "bytes_sent": 1024000,
      "bytes_recv": 2048000
    },
    "uptime_seconds": 360000,
    "timestamp": "2026-06-14T12:00:00Z"
  }
}
```

### GET /api/monitor/history

获取历史监控数据（需要认证）。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| start | string | 否 | 开始时间（ISO 8601），默认 1 小时前 |
| end | string | 否 | 结束时间（ISO 8601），默认当前时间 |

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "points": [
      {
        "timestamp": "2026-06-14T11:00:00Z",
        "cpu": 45.2,
        "memory": 50.0,
        "disk": 60.0,
        "net_sent": 1024,
        "net_recv": 2048
      }
    ]
  }
}
```

### WS /ws/monitor

实时监控数据 WebSocket。

**认证：** 通过 URL 参数传递 Token：`/ws/monitor?token=<jwt_token>`

**服务端推送消息格式：**

```json
{
  "type": "stats",
  "data": {
    "cpu": { "usage_percent": 45.2 },
    "memory": { "usage_percent": 50.0 },
    "disk": [{ "usage_percent": 60.0 }],
    "network": { "bytes_sent": 1024, "bytes_recv": 2048 },
    "timestamp": "2026-06-14T12:00:00Z"
  }
}
```

**心跳：**

| 方向 | 消息 | 间隔 |
|------|------|------|
| 服务端 → 客户端 | ping | 30s |
| 客户端 → 服务端 | `{"type":"pong"}` | 收到 ping 后 |

---

## 3. 用户管理

### GET /api/users

获取用户列表（需要 admin 权限）。

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "username": "admin",
      "role": "admin",
      "is_locked": false,
      "last_login_at": "2026-06-14T10:00:00Z",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

### POST /api/users

创建用户（需要 admin 权限）。

**请求：**

```json
{
  "username": "newuser",
  "password": "initial-password",
  "role": "operator"
}
```

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 2,
    "username": "newuser",
    "role": "operator"
  }
}
```

### PUT /api/users/:id

修改用户信息（需要 admin 权限）。

**请求：**

```json
{
  "role": "viewer"
}
```

### DELETE /api/users/:id

删除用户（需要 admin 权限）。

### POST /api/users/:id/unlock

解锁被锁定用户（需要 admin 权限）。

---

## 4. WebSocket 通用规范

### 连接建立

所有 WebSocket 端点均在连接 URL 中携带 Token 进行认证：

```
ws://server:8080/ws/monitor?token=<jwt_token>
ws://server:8080/ws/terminal/1?token=<jwt_token>
ws://server:8080/ws/services/nginx/logs?token=<jwt_token>
```

### 心跳

| 方向 | 消息 | 间隔 |
|------|------|------|
| 服务端 → 客户端 | ping | 30s |
| 客户端 → 服务端 | pong | 收到 ping 后 |

服务端 60 秒未收到 pong 则断开连接。

### 错误消息

```json
{ "type": "error", "code": 40000, "message": "参数错误" }
```
