---
layout: default
title: API 文档
nav_order: 2
permalink: /api-reference
---

# EasyServer API 参考文档

> 版本: 1.0  
> 基础路径: `http://localhost:<port>/api`  
> 认证方式: JWT Bearer Token (在 `Authorization` 头中传递)  
> WebSocket 认证: 通过 `Sec-WebSocket-Protocol` 头传递 JWT Token

---

## 目录

- [统一响应格式](#统一响应格式)
- [错误码说明](#错误码说明)
- [认证说明](#认证说明)
- [1. 通用](#1-通用)
- [2. 认证](#2-认证)
- [3. 系统监控](#3-系统监控)
- [4. 服务管理](#4-服务管理)
- [5. 终端](#5-终端)
- [6. 文件管理](#6-文件管理)
- [7. 审计日志](#7-审计日志)
- [8. 系统设置](#8-系统设置)
- [9. 系统信息](#9-系统信息)
- [10. 腾讯云](#10-腾讯云)
- [11. 部署管理](#11-部署管理)
- [12. 运行环境](#12-运行环境)
- [13. 环境配置](#13-环境配置)
- [14. Web服务器](#14-web服务器)
- [15. 数据库管理](#15-数据库管理)
- [16. 定时任务](#16-定时任务)
- [17. 防火墙](#17-防火墙)
- [18. SSH管理](#18-ssh管理)
- [19. 容器/Docker](#19-容器docker)
- [20. 模板](#20-模板)
- [21. 进程守护](#21-进程守护)
- [22. 系统进程](#22-系统进程)
- [23. 通知管理](#23-通知管理)

---

## 统一响应格式

### 成功响应

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... }
}
```

### 分页响应

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [...],
    "total": 100
  }
}
```

### 错误响应

```json
{
  "code": 40000,
  "message": "错误描述",
  "data": null
}
```

---

## 错误码说明

| 错误码 | HTTP状态码 | 说明 |
|--------|-----------|------|
| 0 | 200 | 成功 |
| 40000 | 400 | 请求参数错误 |
| 40100 | 401 | 未认证/认证失败 |
| 40300 | 403 | 禁止访问 |
| 40400 | 404 | 资源不存在 |
| 40900 | 409 | 资源冲突 |
| 50000 | 500 | 服务器内部错误 |

---

## 认证说明

### HTTP API 认证

大部分 API 需要在请求头中携带 JWT Token：

```
Authorization: Bearer <token>
```

### WebSocket 认证

WebSocket 连接通过 `Sec-WebSocket-Protocol` 子协议传递 Token：

```
Sec-WebSocket-Protocol: token, <jwt-token>
```

### 公开接口

以下接口不需要认证：
- `GET /health`
- `POST /api/auth/login`
- `POST /api/auth/verify-totp`
- `POST /api/auth/verify-backup`

---

## 1. 通用

### 1.1 `GET /health`

**描述**: 健康检查  
**认证**: 公开（无需认证）

**请求参数**: 无

**响应示例**:

```json
{
  "status": "ok"
}
```

> 注意：此接口直接返回 JSON，不使用统一响应格式包装。

---

## 2. 认证

### 2.1 `POST /api/auth/login`

**描述**: 用户登录。如果用户启用了 TOTP 二次验证，将返回临时令牌用于 TOTP 验证。  
**认证**: 公开

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

**响应示例 - 正常登录**:

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
    "must_change_pass": false
  }
}
```

**响应示例 - 需要 TOTP 验证**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "requires_totp": true,
    "temp_token": "eyJhbGciOiJIUzI1NiIs..."
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请求参数错误 |
| 40100 | 用户名或密码错误 |

---

### 2.2 `POST /api/auth/verify-totp`

**描述**: TOTP 二次验证（登录第二步）  
**认证**: 公开（使用临时令牌）

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| temp_token | string | 是 | 登录时返回的临时令牌 |
| code | string | 是 | TOTP 验证码（6位数字） |

**响应示例**:

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
    "must_change_pass": false
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请求参数错误 |
| 40100 | 临时令牌无效或已过期、TOTP 验证码无效 |

---

### 2.3 `POST /api/auth/verify-backup`

**描述**: 备用码验证（登录第二步，当 TOTP 不可用时使用）  
**认证**: 公开（使用临时令牌）

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| temp_token | string | 是 | 登录时返回的临时令牌 |
| backup_code | string | 是 | 备用验证码 |

**响应示例**:

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
    "must_change_pass": false
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请求参数错误 |
| 40100 | 临时令牌无效、备用码无效 |
| 50000 | 验证备用码失败 |

---

### 2.4 `POST /api/auth/logout`

**描述**: 退出登录，将当前令牌加入黑名单并删除会话  
**认证**: 需要 JWT

**请求参数**: 无（从 Authorization 头获取令牌）

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 2.5 `GET /api/auth/me`

**描述**: 获取当前登录用户的详细信息  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "must_change_pass": false
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40100 | 未登录 |
| 50000 | 获取用户信息失败 |

---

### 2.6 `POST /api/auth/change-password`

**描述**: 修改当前用户密码。修改成功后会使所有已存在的令牌失效。  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| old_password | string | 是 | 旧密码 |
| new_password | string | 是 | 新密码（最少 8 位） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请求参数错误或旧密码不正确 |

---

### 2.7 `POST /api/auth/totp/setup`

**描述**: 生成 TOTP 设置信息（二维码 URL 和密钥），用于后续启用 2FA  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "secret": "JBSWY3DPEHPK3PXP",
    "qr_url": "otpauth://totp/EasyServer:admin?secret=...",
    "issuer": "EasyServer"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | TOTP 已启用，无需重复设置 |
| 50000 | 生成 TOTP 设置失败 |

---

### 2.8 `POST /api/auth/totp/enable`

**描述**: 启用 2FA（需要先调用 setup 获取密钥，然后提供验证码确认）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | TOTP 验证码（用于确认密钥正确） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "backup_codes": [
      "a1b2c3d4",
      "e5f6g7h8",
      "i9j0k1l2",
      "m3n4o5p6",
      "q7r8s9t0"
    ]
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请先设置 TOTP 或验证码无效 |

---

### 2.9 `POST /api/auth/totp/disable`

**描述**: 禁用 2FA（需要提供当前密码确认）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| password | string | 是 | 当前用户密码 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 密码错误或禁用失败 |

---

### 2.10 `GET /api/auth/totp/status`

**描述**: 获取当前用户的 TOTP 启用状态  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "enabled": true
  }
}
```

---

### 2.11 `GET /api/auth/sessions`

**描述**: 获取当前用户的所有活跃会话列表  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "token": "eyJhbGciOi...",
      "user_id": 1,
      "username": "admin",
      "ip": "192.168.1.100",
      "user_agent": "Mozilla/5.0 ...",
      "created_at": "2024-01-15T10:00:00Z",
      "expires_at": "2024-01-15T22:00:00Z"
    }
  ]
}
```

---

### 2.12 `POST /api/auth/sessions/kick`

**描述**: 踢出指定会话（不能踢出自己的当前会话）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 要踢出的会话令牌 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "会话已踢出"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 不能踢出自己的会话 |
| 40400 | 会话不存在 |

---

### 2.13 `POST /api/auth/sessions/kick-all`

**描述**: 踢出当前用户的所有其他会话（保留当前会话）  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "已踢出所有其他会话"
  }
}
```

---

## 3. 系统监控

### 3.1 `GET /api/monitor/stats`

**描述**: 获取当前系统状态快照（CPU、内存、磁盘、网络等）  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "cpu_percent": 25.5,
    "memory_total": 8589934592,
    "memory_used": 4294967296,
    "memory_percent": 50.0,
    "disk_total": 107374182400,
    "disk_used": 53687091200,
    "disk_percent": 50.0,
    "net_rx_bytes": 1234567,
    "net_tx_bytes": 7654321,
    "uptime": 86400,
    "load1": 1.5,
    "load5": 1.2,
    "load15": 1.0,
    "timestamp": "2024-01-15T10:00:00Z"
  }
}
```

---

### 3.2 `GET /api/monitor/history`

**描述**: 获取历史监控数据（时间范围内的数据点）  
**认证**: 需要 JWT

**请求参数 (Query)**:

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| start | string | 否 | 1小时前 | 开始时间（RFC3339格式） |
| end | string | 否 | 当前时间 | 结束时间（RFC3339格式） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "points": [
      {
        "cpu_percent": 20.0,
        "memory_percent": 45.0,
        "disk_percent": 50.0,
        "timestamp": "2024-01-15T09:00:00Z"
      }
    ]
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的时间格式或时间范围 |

> 注意：最大查询范围为 7 天，超出范围会自动截断。

---

### 3.3 `WS /ws/monitor`

**描述**: WebSocket 实时监控数据流  
**认证**: 需要 JWT（通过 Sec-WebSocket-Protocol 头传递）

**连接方式**:

```
ws://localhost:port/ws/monitor
Sec-WebSocket-Protocol: token, <jwt-token>
```

**服务端推送消息格式**:

```json
{
  "type": "stats",
  "data": {
    "cpu_percent": 25.5,
    "memory_percent": 50.0,
    "timestamp": "2024-01-15T10:00:00Z"
  }
}
```

**客户端发送心跳**:

```json
{"type": "ping"}
```

**服务端回复**:

```json
{"type": "pong"}
```

---

## 4. 服务管理

### 4.1 `GET /api/services`

**描述**: 获取所有 systemd 服务列表  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "name": "nginx",
      "description": "nginx HTTP server",
      "load_state": "loaded",
      "active_state": "active",
      "sub_state": "running",
      "enabled": true
    }
  ]
}
```

---

### 4.2 `GET /api/services/:name`

**描述**: 获取指定服务的详细信息  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称（仅允许字母、数字、连字符、下划线） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "description": "nginx HTTP server",
    "load_state": "loaded",
    "active_state": "active",
    "sub_state": "running",
    "enabled": true
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的服务名称 |
| 40400 | 服务不存在 |

---

### 4.3 `GET /api/services/:name/logs`

**描述**: 获取指定服务的 systemd 日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| tail | int | 100 | 返回的日志行数 |
| since | string | - | 起始时间过滤 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "lines": [
      "Jan 15 10:00:00 server nginx[1234]: started",
      "Jan 15 10:00:01 server nginx[1234]: ready"
    ]
  }
}
```

---

### 4.4 `POST /api/services/:name/start`

**描述**: 启动指定服务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "state": "active"
  }
}
```

---

### 4.5 `POST /api/services/:name/stop`

**描述**: 停止指定服务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "state": "inactive"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无法停止面板自身的服务 |

---

### 4.6 `POST /api/services/:name/restart`

**描述**: 重启指定服务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "state": "active"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无法从此处重启面板自身服务，请使用 /api/settings/restart |

---

### 4.7 `POST /api/services/:name/enable`

**描述**: 启用服务（开机自启）  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "enabled": true
  }
}
```

---

### 4.8 `POST /api/services/:name/disable`

**描述**: 禁用服务（取消开机自启）  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "nginx",
    "enabled": false
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无法禁用面板自身的服务 |

---

### 4.9 `WS /ws/services/:name/logs`

**描述**: WebSocket 实时服务日志流  
**认证**: 需要 JWT（通过 Sec-WebSocket-Protocol 头传递）

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**服务端推送消息格式**:

```json
{
  "type": "log",
  "data": {
    "time": "2024-01-15 10:00:00",
    "message": "nginx started",
    "priority": "info"
  }
}
```

---

## 5. 终端

### 5.1 `GET /api/terminal/:id`

**描述**: 终端会话检查（验证终端会话存在性）  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 终端会话 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 5.2 `WS /ws/terminal/:id`

**描述**: WebSocket 终端交互  
**认证**: 需要 JWT（通过 Sec-WebSocket-Protocol 头传递）

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 终端会话 ID |

**交互协议**: 基于 xterm.js，支持终端输入输出和窗口大小调整。

---

## 6. 文件管理

### 6.1 `GET /api/files`

**描述**: 列出指定目录下的文件和子目录  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 否 | 目录路径（相对于基础路径，为空则列出根目录） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "path": "/home/user/files",
    "parent": "/home/user",
    "entries": [
      {
        "name": "file.txt",
        "path": "/home/user/files/file.txt",
        "is_dir": false,
        "size_bytes": 1024,
        "mode": "-rw-r--r--",
        "modified_at": "2024-01-15T10:00:00Z",
        "is_symlink": false
      }
    ]
  }
}
```

---

### 6.2 `GET /api/files/base-path`

**描述**: 获取文件管理器的基础路径  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "base_path": "/home/user"
  }
}
```

---

### 6.3 `GET /api/files/download`

**描述**: 下载文件  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径（相对于基础路径） |

**响应**: 直接返回文件流（Content-Type: application/octet-stream）

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | path 为空或路径是目录 |
| 40400 | 文件不存在 |

---

### 6.4 `GET /api/files/content`

**描述**: 读取文件内容（文本文件，最大 10MB）  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "path": "/home/user/config.txt",
    "content": "file content here...",
    "encoding": "utf-8"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | path 为空、是目录或文件过大 |

---

### 6.5 `GET /api/files/search`

**描述**: 按文件名搜索文件  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| path | string | 否 | 基础路径 | 搜索起始目录 |
| q | string | 是 | - | 搜索关键词 |
| limit | int | 否 | 100 | 最大返回结果数 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "path": "/home/user/config.txt",
      "name": "config.txt",
      "is_dir": false,
      "size": 1024
    }
  ]
}
```

---

### 6.6 `GET /api/files/search-content`

**描述**: 按文件内容搜索  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| path | string | 否 | 基础路径 | 搜索起始目录 |
| q | string | 是 | - | 搜索文本 |
| limit | int | 否 | 50 | 最大返回结果数 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "path": "/home/user/config.txt",
      "name": "config.txt",
      "is_dir": false,
      "size": 1024,
      "match": "matching line content..."
    }
  ]
}
```

---

### 6.7 `GET /api/files/details`

**描述**: 获取文件详细信息  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | path 为空 |
| 40400 | 文件不存在 |

---

### 6.8 `GET /api/files/mime-type`

**描述**: 获取文件的 MIME 类型  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |

---

### 6.9 `POST /api/files/mkdir`

**描述**: 创建目录  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 目录路径（相对于基础路径） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.10 `POST /api/files/upload`

**描述**: 上传文件  
**认证**: 需要 JWT  
**Content-Type**: `multipart/form-data`

**表单字段**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | file | 是 | 上传的文件 |
| path | string | 否 | 目标路径（默认为根目录下的同名文件） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "name": "uploaded.txt",
    "path": "/uploaded.txt",
    "size": 1024
  }
}
```

---

### 6.11 `PUT /api/files/rename`

**描述**: 重命名/移动文件  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| old_path | string | 是 | 原路径 |
| new_path | string | 是 | 新路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.12 `DELETE /api/files`

**描述**: 删除文件或目录  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件/目录路径 |
| recursive | string | 否 | 设为 "true" 递归删除目录 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.13 `POST /api/files/move`

**描述**: 移动文件到目标目录  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| paths | string[] | 是 | 要移动的文件路径列表 |
| dest | string | 是 | 目标目录路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.14 `POST /api/files/copy`

**描述**: 复制文件  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| source | string | 是 | 源文件路径 |
| dest | string | 是 | 目标文件路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.15 `PUT /api/files/content`

**描述**: 保存文件内容  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |
| content | string | 否 | 文件内容 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.16 `POST /api/files/compress`

**描述**: 压缩文件为 zip 归档  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| sources | string[] | 是 | 要压缩的源文件路径列表 |
| dest | string | 是 | 压缩文件目标路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.17 `POST /api/files/extract`

**描述**: 解压归档文件  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| source | string | 是 | 归档文件路径 |
| dest | string | 是 | 解压目标目录 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.18 `PUT /api/files/chmod`

**描述**: 修改文件权限  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |
| mode | string | 是 | 权限模式（八进制，如 "0755"、"644"） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 6.19 `PUT /api/files/chown`

**描述**: 修改文件所有者  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 文件路径 |
| uid | int | 否 | 用户 ID |
| gid | int | 否 | 组 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

## 7. 审计日志

### 7.1 `GET /api/audit-logs`

**描述**: 获取审计日志列表（分页 + 过滤）  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码 |
| page_size | int | 50 | 每页数量（最大 200） |
| username | string | - | 用户名过滤（模糊匹配） |
| action | string | - | 操作类型过滤（精确匹配） |
| resource | string | - | 资源过滤（模糊匹配） |
| ip | string | - | IP 过滤（模糊匹配） |
| start_date | string | - | 开始日期 |
| end_date | string | - | 结束日期 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total": 100,
    "items": [
      {
        "id": 1,
        "user_id": 1,
        "username": "admin",
        "action": "LOGIN_SUCCESS",
        "resource": "auth",
        "detail": "User logged in successfully",
        "ip": "192.168.1.100",
        "user_agent": "Mozilla/5.0 ...",
        "created_at": "2024-01-15 10:00:00"
      }
    ]
  }
}
```

---

### 7.2 `GET /api/audit-logs/actions`

**描述**: 获取所有不同的操作类型列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    "LOGIN_SUCCESS",
    "LOGIN_FAILED",
    "LOGOUT",
    "PASSWORD_CHANGED"
  ]
}
```

---

### 7.3 `GET /api/audit-logs/stats`

**描述**: 获取审计日志统计信息  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| days | int | 7 | 统计天数（最大 90） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user_stats": [
      {"username": "admin", "count": 50}
    ],
    "action_stats": [
      {"action": "LOGIN_SUCCESS", "count": 30}
    ],
    "day_stats": [
      {"day": "2024-01-15", "count": 15}
    ],
    "status_stats": [
      {"status": "2xx", "count": 100},
      {"status": "4xx", "count": 5}
    ],
    "alerts": [
      {
        "id": 1,
        "username": "admin",
        "action": "API_CALL",
        "resource": "/api/settings",
        "status": 400,
        "ip": "192.168.1.100",
        "created_at": "2024-01-15 10:00:00"
      }
    ]
  }
}
```

---

### 7.4 `GET /api/audit-logs/clean-policy`

**描述**: 获取审计日志清理策略  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "retention_days": 90,
    "total_records": 1500,
    "auto_clean": true
  }
}
```

---

### 7.5 `GET /api/audit-logs/export`

**描述**: 导出审计日志为 CSV 文件  
**认证**: 需要 JWT

**查询参数**: 同 `GET /api/audit-logs`（不含分页参数）

**响应**: 返回 CSV 文件流（Content-Type: text/csv），最多导出 10000 条记录。

---

### 7.6 `DELETE /api/audit-logs/clean`

**描述**: 清理旧的审计日志记录  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| days | int | 90 | 删除多少天前的日志 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "deleted": 500
  }
}
```

---

### 7.7 `GET /api/audit-logs/verify`

**描述**: 验证审计日志完整性  
**认证**: 需要 JWT

---

## 8. 系统设置

### 8.1 `GET /api/settings`

**描述**: 获取当前系统设置（敏感字段会被脱敏）  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "server": {
      "port": 8080,
      "host": "0.0.0.0",
      "serve_frontend": true,
      "tls_enabled": false
    },
    "auth": {
      "session_timeout": "12h0m0s",
      "idle_timeout": "30m0s",
      "max_login_attempts": 5,
      "lockout_duration": "15m0s",
      "rate_limit": 60,
      "rate_interval": "1m0s"
    },
    "monitor": {
      "history_retention": "168h0m0s",
      "collect_interval": "10s"
    },
    "database": {
      "path": "/***/easyserver.db"
    },
    "audit": {
      "enabled": true,
      "log_path": ""
    },
    "notify": {
      "enabled": false,
      "webhook_url": "https://example.com/***"
    },
    "tencentcloud": {
      "enabled": false,
      "region": "ap-guangzhou",
      "instance_id": "",
      "has_secret": false
    }
  }
}
```

---

### 8.2 `GET /api/settings/system`

**描述**: 获取系统信息（版本、操作系统、运行时间等）  
**认证**: 需要 JWT

---

### 8.3 `PUT /api/settings/server`

**描述**: 更新服务器配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| port | int | 否 | 监听端口（1-65535） |
| host | string | 否 | 监听地址 |
| serve_frontend | bool | 否 | 是否提供前端静态文件服务 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "服务器配置已更新",
    "requires_restart": true
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 端口超范围或主机名为空/过长 |

---

### 8.4 `PUT /api/settings/auth`

**描述**: 更新认证配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| session_timeout | string | 否 | 会话超时时间（Go duration 格式，最少 5m） |
| idle_timeout | string | 否 | 空闲超时时间（最少 1m） |
| max_login_attempts | int | 否 | 最大登录尝试次数（3-100） |
| lockout_duration | string | 否 | 锁定时间（1m-24h） |
| rate_limit | int | 否 | 速率限制请求数（最少 10） |
| rate_interval | string | 否 | 速率限制时间窗口（最少 1s） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "认证配置已更新"
  }
}
```

---

### 8.5 `PUT /api/settings/monitor`

**描述**: 更新监控配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| history_retention | string | 否 | 历史数据保留时间（1m-8760h） |
| collect_interval | string | 否 | 数据采集间隔（1s-1h） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "监控配置已更新"
  }
}
```

---

### 8.6 `PUT /api/settings/audit`

**描述**: 更新审计配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| enabled | bool | 否 | 是否启用审计日志 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "审计配置已更新"
  }
}
```

---

### 8.7 `PUT /api/settings/notify`

**描述**: 更新通知配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| enabled | bool | 否 | 是否启用通知 |
| webhook_url | string | 否 | Webhook URL（http/https） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "通知配置已更新"
  }
}
```

---

### 8.8 `POST /api/settings/notify/test`

**描述**: 测试 Webhook 通知  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "测试通知已发送"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请先配置 Webhook URL |
| 50000 | 测试通知失败 |

---

### 8.9 `GET /api/alerts/rules`

**描述**: 获取告警规则列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "rules": [
      {
        "name": "high_cpu",
        "metric": "cpu_percent",
        "threshold": 90,
        "duration": "5m",
        "enabled": true
      }
    ]
  }
}
```

---

### 8.10 `PUT /api/alerts/rules`

**描述**: 更新告警规则  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| rules | array | 是 | 告警规则数组 |

---

### 8.11 `PUT /api/settings/cloud`

**描述**: 更新腾讯云配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| enabled | bool | 否 | 是否启用腾讯云功能 |
| secret_id | string | 否 | 腾讯云 SecretID |
| secret_key | string | 否 | 腾讯云 SecretKey |
| region | string | 否 | 地域（如 ap-guangzhou、ap-shanghai 等） |
| instance_id | string | 否 | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "云配置已更新"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的地域 |

---

### 8.12 `POST /api/settings/cloud/test`

**描述**: 测试腾讯云连接  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "连接成功",
    "instance_count": 3
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 请先配置 SecretID 和 SecretKey |
| 50000 | 连接失败 |

---

### 8.13 `POST /api/settings/restart`

**描述**: 重启面板服务  
**认证**: 需要 JWT

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "面板正在重启..."
  }
}
```

---

## 9. 系统信息

### 9.1 `GET /api/system/ssh-logins`

**描述**: 获取 SSH 登录历史记录  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| limit | int | 50 | 返回记录数 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "records": [
      {
        "user": "root",
        "ip": "192.168.1.100",
        "time": "2024-01-15 10:00:00",
        "status": "accepted"
      }
    ]
  }
}
```

---

### 9.2 `GET /api/system/ssh-config`

**描述**: 获取系统 SSH 配置信息  
**认证**: 需要 JWT

---

### 9.3 `GET /api/system/check-port`

**描述**: 检查单个端口是否可用  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| port | int | 是 | 端口号 |

---

### 9.4 `GET /api/system/check-ports`

**描述**: 批量检查端口可用性  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ports | string | 是 | 端口列表（逗号分隔，如 "80,443,8080"） |

---

## 10. 腾讯云

### 10.1 `GET /api/cloud/instances`

**描述**: 获取腾讯云实例列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "instances": [
      {
        "instance_id": "ins-xxxxx",
        "instance_name": "MyServer",
        "status": "RUNNING",
        "public_ip": "1.2.3.4",
        "private_ip": "10.0.0.1",
        "cpu": 2,
        "memory": 4,
        "os_name": "Ubuntu 22.04"
      }
    ],
    "total_count": 1
  }
}
```

---

### 10.2 `GET /api/cloud/instances/:id`

**描述**: 获取指定实例详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | instance id 为空 |
| 40400 | 实例不存在或云服务未启用 |

---

### 10.3 `GET /api/cloud/monitor/:id`

**描述**: 获取实例监控数据  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| metric | string | - | 监控指标（CPUUsage、MemoryUsage 等） |
| start | string | 1小时前 | 开始时间（RFC3339） |
| end | string | 当前时间 | 结束时间（RFC3339） |

---

### 10.4 `GET /api/cloud/firewall/:id`

**描述**: 获取实例的防火墙规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "rules": [
      {
        "policy": "ACCEPT",
        "port": "80",
        "protocol": "tcp",
        "cidr_block": "0.0.0.0/0"
      }
    ]
  }
}
```

---

### 10.5 `GET /api/cloud/snapshots`

**描述**: 获取快照列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| instance_id | string | 是 | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "snapshots": [
      {
        "snapshot_id": "snap-xxxxx",
        "snapshot_name": "backup-20240115",
        "status": "NORMAL",
        "created_at": "2024-01-15T10:00:00Z"
      }
    ]
  }
}
```

---

### 10.6 `GET /api/cloud/traffic`

**描述**: 获取实例流量信息  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| instance_id | string | 否 | 实例 ID |

---

### 10.7 `POST /api/cloud/instances/:id/start`

**描述**: 启动腾讯云实例  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "instance_id": "ins-xxxxx",
    "state": "STARTING"
  }
}
```

---

### 10.8 `POST /api/cloud/instances/:id/stop`

**描述**: 停止腾讯云实例  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "instance_id": "ins-xxxxx",
    "state": "STOPPING"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无法停止运行面板的当前实例 |

---

### 10.9 `POST /api/cloud/instances/:id/restart`

**描述**: 重启腾讯云实例  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "instance_id": "ins-xxxxx",
    "state": "REBOOTING"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无法重启运行面板的当前实例，请使用 /api/settings/restart |

---

### 10.10 `POST /api/cloud/firewall/:id`

**描述**: 添加实例防火墙规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| port | string | 是 | 端口（如 "80"、"443"、"8000-9000"、"ALL"） |
| protocol | string | 否 | 协议（tcp、udp） |
| action | string | 否 | 策略（ACCEPT、DROP） |
| cidr_block | string | 否 | 源 IP/CIDR |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 端口为空或不能在当前实例上封锁面板端口 |

---

### 10.11 `DELETE /api/cloud/firewall/:id/:ruleId`

**描述**: 删除实例防火墙规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 实例 ID |
| ruleId | string | 规则 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 10.12 `POST /api/cloud/snapshots`

**描述**: 创建快照  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| instance_id | string | 是 | 实例 ID |
| name | string | 是 | 快照名称 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 10.13 `POST /api/cloud/snapshots/:id/apply`

**描述**: 回滚快照  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 快照 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "snapshot_id": "snap-xxxxx",
    "status": "APPLYING"
  }
}
```

---

## 11. 部署管理

### 11.1 `GET /api/deploy/servers`

**描述**: 获取部署服务器列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "Production",
      "host": "192.168.1.100",
      "port": 22,
      "username": "deploy",
      "auth_type": "key",
      "status": "online",
      "last_ping": "2024-01-15 10:00:00",
      "created_at": "2024-01-01 00:00:00"
    }
  ]
}
```

> 注意：`auth_data` 字段不会返回给客户端。

---

### 11.2 `POST /api/deploy/servers`

**描述**: 创建部署服务器  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 服务器名称 |
| host | string | 是 | 主机地址 |
| port | int | 是 | SSH 端口（1-65535） |
| username | string | 是 | SSH 用户名 |
| auth_type | string | 是 | 认证类型（"password" 或 "key"） |
| auth_data | string | 是 | 密码或密钥路径 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "name": "Production",
    "host": "192.168.1.100",
    "port": 22,
    "username": "deploy",
    "auth_type": "key",
    "auth_data": "",
    "status": "unknown"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 参数校验失败（名称/主机/端口/用户名/认证类型/认证数据） |

---

### 11.3 `GET /api/deploy/servers/:id`

**描述**: 获取部署服务器详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 服务器 ID |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的服务器 ID |
| 40400 | 服务器不存在 |

---

### 11.4 `PUT /api/deploy/servers/:id`

**描述**: 更新部署服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 服务器 ID |

**请求参数 (Request Body)**: 同创建接口

---

### 11.5 `DELETE /api/deploy/servers/:id`

**描述**: 删除部署服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 服务器 ID |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40900 | 存在关联的任务或版本记录 |

---

### 11.6 `POST /api/deploy/servers/:id/test`

**描述**: 测试部署服务器 SSH 连接  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "online"
  }
}
```

---

### 11.7 `GET /api/deploy/tasks`

**描述**: 获取部署任务列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "server_id": 1,
      "server_name": "Production",
      "name": "deploy-frontend",
      "type": "sync",
      "source_path": "/build/dist",
      "dest_path": "/var/www/html",
      "command": "",
      "status": "success",
      "result": "",
      "created_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 11.8 `POST /api/deploy/tasks`

**描述**: 创建部署任务  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 任务名称 |
| server_id | int | 是 | 目标服务器 ID |
| type | string | 是 | 任务类型（"sync"、"command"、"rollback"） |
| source_path | string | 否 | 源路径（sync 类型时需要） |
| dest_path | string | 否 | 目标路径 |
| command | string | 否 | 命令（command 类型时需要） |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 参数校验失败 |
| 40400 | 服务器不存在 |

---

### 11.9 `GET /api/deploy/tasks/:id`

**描述**: 获取部署任务详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 11.10 `DELETE /api/deploy/tasks/:id`

**描述**: 删除部署任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 11.11 `POST /api/deploy/tasks/:id/exec`

**描述**: 执行部署任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "executing"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40900 | 任务已在执行中 |

---

### 11.12 `GET /api/deploy/versions`

**描述**: 获取部署版本列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| server_id | int | 是 | 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "server_id": 1,
      "server_name": "Production",
      "task_id": 1,
      "version": "v1.0.0",
      "files": "[\"index.html\",\"app.js\"]",
      "backup_path": "/backups/v1.0.0",
      "created_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 11.13 `POST /api/deploy/versions/:id/rollback`

**描述**: 回滚到指定版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 版本 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "rolling_back"
  }
}
```

---

## 12. 运行环境

### 12.1 `GET /api/runtime`

**描述**: 获取已安装的运行环境列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "node",
      "version": "18.17.0",
      "path": "/usr/local/node-18.17.0",
      "is_default": true,
      "installed_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 12.2 `GET /api/runtime/:name`

**描述**: 获取指定运行环境的已安装版本列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称（如 node、python、go、java） |

---

### 12.3 `POST /api/runtime/install`

**描述**: 安装运行环境  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 运行环境名称 |
| version | string | 是 | 版本号 |

---

### 12.4 `POST /api/runtime/uninstall`

**描述**: 卸载运行环境  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 运行环境名称 |
| version | string | 是 | 版本号 |

---

### 12.5 `POST /api/runtime/set-default`

**描述**: 设置运行环境默认版本  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 运行环境名称 |
| version | string | 是 | 版本号 |

---

### 12.6 `GET /api/runtime/detect`

**描述**: 检测系统中已安装的运行环境  
**认证**: 需要 JWT

---

### 12.7 `POST /api/runtime/import-detected`

**描述**: 导入检测到的运行环境到管理列表  
**认证**: 需要 JWT

---

### 12.8 `GET /api/runtime/progress/:id`

**描述**: 获取安装/卸载进度  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 操作 ID |

---

### 12.9 `GET /api/runtime/check-deps/:name`

**描述**: 检查运行环境依赖  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称 |

---

### 12.10 `GET /api/runtime/logs/:id`

**描述**: 获取安装日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 操作 ID |

---

### 12.11 `GET /api/runtime/cleanup/:id`

**描述**: 获取卸载清理信息  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 运行环境 ID |

---

### 12.12 `GET /api/runtime-versions/:name`

**描述**: 获取指定运行环境的可用版本列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称 |

---

### 12.13 `POST /api/runtime-versions/:name/fetch`

**描述**: 从远程获取最新版本列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称 |

---

### 12.14 `GET /api/runtime-versions/:name/resolve/:alias`

**描述**: 解析版本别名（如 lts -> 18.17.0）  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称 |
| alias | string | 版本别名 |

---

### 12.15 `GET /api/runtime-versions/:name/suggestions`

**描述**: 获取版本建议  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 运行环境名称 |

---

### 12.16 `GET /api/packages`

**描述**: 获取已安装的包列表  
**认证**: 需要 JWT

---

### 12.17 `GET /api/packages/scan/:id`

**描述**: 扫描指定运行环境的已安装包  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 运行环境 ID |

---

### 12.18 `GET /api/packages/search`

**描述**: 搜索包  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 包管理器名称（npm、pip、go） |
| q | string | 是 | 搜索关键词 |

---

### 12.19 `GET /api/packages/versions/:name`

**描述**: 获取包的可用版本列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 包名称 |

---

### 12.20 `POST /api/packages/install`

**描述**: 安装包  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 包管理器名称 |
| package | string | 是 | 包名 |
| version | string | 否 | 版本号 |

---

### 12.21 `POST /api/packages/uninstall`

**描述**: 卸载包  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 包管理器名称 |
| package | string | 是 | 包名 |

---

### 12.22 `POST /api/packages/update`

**描述**: 更新包  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 包管理器名称 |
| package | string | 是 | 包名 |

---

## 13. 环境配置

### 13.1 `GET /api/env-config`

**描述**: 获取环境配置列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| runtime_id | int | 否 | 按运行环境过滤 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "configs": [
      {
        "id": 1,
        "name": "NODE_ENV",
        "value": "production",
        "runtime_id": 0,
        "is_global": true
      }
    ]
  }
}
```

---

### 13.2 `GET /api/env-config/:id`

**描述**: 获取环境配置详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

---

### 13.3 `POST /api/env-config`

**描述**: 创建环境配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 变量名 |
| value | string | 是 | 变量值 |
| runtime_id | int | 否 | 运行环境 ID（0 表示全局） |
| is_global | bool | 否 | 是否全局 |

---

### 13.4 `PUT /api/env-config/:id`

**描述**: 更新环境配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 变量名 |
| value | string | 是 | 变量值 |

---

### 13.5 `DELETE /api/env-config/:id`

**描述**: 删除环境配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "删除成功"
  }
}
```

---

### 13.6 `GET /api/env-config/path`

**描述**: 获取 PATH 条目列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| runtime_id | int | 否 | 按运行环境过滤 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "entries": [
      {
        "id": 1,
        "path": "/usr/local/node-18.17.0/bin",
        "runtime_id": 1,
        "is_global": false
      }
    ]
  }
}
```

---

### 13.7 `POST /api/env-config/path`

**描述**: 创建 PATH 条目  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 路径 |
| runtime_id | int | 否 | 运行环境 ID |
| is_global | bool | 否 | 是否全局 |

---

### 13.8 `DELETE /api/env-config/path/:id`

**描述**: 删除 PATH 条目  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | PATH 条目 ID |

---

### 13.9 `GET /api/env-config/script`

**描述**: 生成环境变量设置脚本  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| runtime_id | int | 否 | 按运行环境过滤 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "script": "export NODE_ENV=production\nexport PATH=/usr/local/node-18.17.0/bin:$PATH\n"
  }
}
```

---

### 13.10 `GET /api/global-config`

**描述**: 获取全局配置列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| category | string | 否 | 按分类过滤 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "configs": [
      {
        "id": 1,
        "category": "general",
        "key": "timezone",
        "value": "Asia/Shanghai",
        "description": "系统时区"
      }
    ]
  }
}
```

---

### 13.11 `GET /api/global-config/:id`

**描述**: 获取全局配置详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

---

### 13.12 `POST /api/global-config`

**描述**: 创建全局配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| category | string | 是 | 分类 |
| key | string | 是 | 键名 |
| value | string | 是 | 值 |
| description | string | 否 | 描述 |

---

### 13.13 `PUT /api/global-config/:id`

**描述**: 更新全局配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| value | string | 是 | 值 |
| description | string | 否 | 描述 |

---

### 13.14 `DELETE /api/global-config/:id`

**描述**: 删除全局配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 配置 ID |

---

## 14. Web服务器

### 14.1 `GET /api/web-servers/project-types`

**描述**: 获取支持的项目类型列表  
**认证**: 需要 JWT

---

### 14.2 `GET /api/web-servers/browse`

**描述**: 浏览服务器目录  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 否 | 浏览路径 |

---

### 14.3 `GET /api/web-servers/validate-path`

**描述**: 验证路径是否有效  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 要验证的路径 |

---

### 14.4 `GET /api/web-servers`

**描述**: 获取 Web 服务器列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "nginx",
      "display_name": "Nginx",
      "status": "running",
      "version": "1.24.0"
    }
  ]
}
```

---

### 14.5 `GET /api/web-servers/:id`

**描述**: 获取 Web 服务器详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.6 `POST /api/web-servers`

**描述**: 创建 Web 服务器（基于预定义模板）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 服务器类型名称（如 nginx、apache、caddy） |
| display_name | string | 否 | 显示名称 |
| description | string | 否 | 描述 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 名称格式无效或未知的服务器类型 |

---

### 14.7 `DELETE /api/web-servers/:id`

**描述**: 删除 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.8 `POST /api/web-servers/:id/install`

**描述**: 安装 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "已安装"
  }
}
```

---

### 14.9 `POST /api/web-servers/:id/uninstall`

**描述**: 卸载 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.10 `POST /api/web-servers/:id/start`

**描述**: 启动 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "running"
  }
}
```

---

### 14.11 `POST /api/web-servers/:id/stop`

**描述**: 停止 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "stopped"
  }
}
```

---

### 14.12 `POST /api/web-servers/:id/restart`

**描述**: 重启 Web 服务器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.13 `GET /api/web-servers/:id/status`

**描述**: 获取 Web 服务器状态  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "running",
    "version": "1.24.0"
  }
}
```

---

### 14.14 `POST /api/web-servers/:id/reload`

**描述**: 重载 Web 服务器配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.15 `GET /api/web-servers/:id/test-config`

**描述**: 测试 Web 服务器配置文件  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "valid": true,
    "message": "配置文件语法正确"
  }
}
```

---

### 14.16 `GET /api/web-servers/:id/config`

**描述**: 获取 Web 服务器配置文件内容  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "content": "server {\n    listen 80;\n    ...\n}"
  }
}
```

---

### 14.17 `PUT /api/web-servers/:id/config`

**描述**: 保存 Web 服务器配置文件  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| content | string | 是 | 配置文件内容 |

---

### 14.18 `GET /api/web-servers/:id/logs`

**描述**: 获取 Web 服务器服务日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| lines | int | 100 | 返回行数 |

---

### 14.19 `POST /api/web-servers/:id/auto-start`

**描述**: 设置 Web 服务器开机自启  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| enabled | bool | 是 | 是否启用自启 |

---

### 14.20 `GET /api/web-servers/:id/process`

**描述**: 获取 Web 服务器进程信息  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "pid": 1234,
    "memory_bytes": 10485760,
    "uptime": "2d 3h 15m"
  }
}
```

---

### 14.21 `GET /api/web-servers/:id/websites`

**描述**: 获取 Web 服务器下的网站列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

---

### 14.22 `GET /api/web-servers/:id/websites/:wid`

**描述**: 获取网站详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

---

### 14.23 `POST /api/web-servers/:id/websites`

**描述**: 创建网站  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| domain | string | 是 | 域名 |
| root_path | string | 否 | 网站根目录 |
| ... | ... | ... | 其他配置字段 |

---

### 14.24 `PUT /api/web-servers/:id/websites/:wid`

**描述**: 更新网站配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

---

### 14.25 `DELETE /api/web-servers/:id/websites/:wid`

**描述**: 删除网站  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

---

### 14.26 `POST /api/web-servers/:id/websites/:wid/enable`

**描述**: 启用网站  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "active"
  }
}
```

---

### 14.27 `POST /api/web-servers/:id/websites/:wid/disable`

**描述**: 禁用网站  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "disabled"
  }
}
```

---

### 14.28 `GET /api/web-servers/:id/websites/:wid/logs`

**描述**: 获取网站日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| type | string | access | 日志类型（access/error） |
| lines | int | 200 | 返回行数 |

---

### 14.29 `POST /api/web-servers/:id/websites/:wid/ssl`

**描述**: 申请/配置网站 SSL 证书  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | Web 服务器 ID |
| wid | int | 网站 ID |

---

## 15. 数据库管理

### 15.1 `GET /api/db-servers`

**描述**: 获取数据库服务器列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "mysql",
      "type": "mysql",
      "status": "running"
    }
  ]
}
```

---

### 15.2 `GET /api/db-servers/:id`

**描述**: 获取数据库服务器详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.3 `GET /api/db-servers/:id/version-templates`

**描述**: 获取数据库版本安装模板  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.4 `GET /api/db-servers/:id/versions`

**描述**: 获取数据库服务器的已安装版本列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.5 `POST /api/db-servers/:id/versions`

**描述**: 安装数据库版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.6 `DELETE /api/db-servers/versions/:vid`

**描述**: 卸载数据库版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

---

### 15.7 `POST /api/db-servers/versions/:vid/start`

**描述**: 启动数据库版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

---

### 15.8 `POST /api/db-servers/versions/:vid/stop`

**描述**: 停止数据库版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

---

### 15.9 `POST /api/db-servers/versions/:vid/restart`

**描述**: 重启数据库版本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

---

### 15.10 `PUT /api/db-servers/versions/:vid/port`

**描述**: 更新数据库版本监听端口  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| port | int | 是 | 新端口号 |

---

### 15.11 `GET /api/db-servers/versions/:vid/logs`

**描述**: 获取数据库版本日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| vid | int | 版本 ID |

---

### 15.12 `GET /api/db-servers/:id/databases`

**描述**: 获取数据库列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.13 `POST /api/db-servers/:id/databases`

**描述**: 创建数据库  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 数据库名称 |
| charset | string | 否 | 字符集 |
| collation | string | 否 | 排序规则 |

---

### 15.14 `DELETE /api/db-servers/:id/databases/:did`

**描述**: 删除数据库  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |
| did | int | 数据库 ID |

---

### 15.15 `GET /api/db-servers/:id/users`

**描述**: 获取数据库用户列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

---

### 15.16 `POST /api/db-servers/:id/users`

**描述**: 创建数据库用户  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

---

### 15.17 `DELETE /api/db-servers/:id/users/:uid`

**描述**: 删除数据库用户  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |
| uid | int | 用户 ID |

---

### 15.18 `POST /api/db-servers/:id/users/:uid/grant`

**描述**: 授予数据库用户权限  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 数据库服务器 ID |
| uid | int | 用户 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| database | string | 是 | 数据库名 |
| privileges | string[] | 是 | 权限列表 |

---

### 15.19 `GET /api/db-servers/databases/:did/tables`

**描述**: 获取数据库表列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

---

### 15.20 `GET /api/db-servers/databases/:did/describe`

**描述**: 获取表结构  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table | string | 是 | 表名 |

---

### 15.21 `GET /api/db-servers/databases/:did/query`

**描述**: 查询表数据（分页）  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| table | string | - | 表名（必填） |
| page | int | 1 | 页码 |
| page_size | int | 50 | 每页数量 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "headers": ["id", "name", "email"],
    "rows": [
      [1, "张三", "zhang@example.com"]
    ],
    "total": 100,
    "page": 1,
    "page_size": 50
  }
}
```

---

### 15.22 `POST /api/db-servers/databases/:did/execute`

**描述**: 执行 SQL 语句  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| sql | string | 是 | SQL 语句 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "success": true,
    "output": "Query OK, 1 row affected"
  }
}
```

---

### 15.23 `POST /api/db-servers/databases/:did/insert`

**描述**: 插入记录  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| dry_run | string | 设为 "true" 仅生成 SQL 不执行 |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table | string | 是 | 表名 |
| data | object | 是 | 字段名值对 |

---

### 15.24 `POST /api/db-servers/databases/:did/update`

**描述**: 更新记录  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| dry_run | string | 设为 "true" 仅生成 SQL 不执行 |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table | string | 是 | 表名 |
| data | object | 是 | 要更新的字段名值对 |
| primary_key | string | 是 | 主键字段名 |
| primary_val | any | 是 | 主键值 |

---

### 15.25 `POST /api/db-servers/databases/:did/delete`

**描述**: 删除记录  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| dry_run | string | 设为 "true" 仅生成 SQL 不执行 |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table | string | 是 | 表名 |
| primary_key | string | 是 | 主键字段名 |
| primary_val | any | 是 | 主键值 |

---

### 15.26 `POST /api/db-servers/databases/:did/tables`

**描述**: 创建表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 表名 |
| columns | array | 是 | 列定义数组 |

**columns 数组元素**:

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 列名 |
| type | string | 数据类型 |
| nullable | bool | 是否可空 |
| is_primary | bool | 是否主键 |
| auto_incr | bool | 是否自增 |

---

### 15.27 `DELETE /api/db-servers/databases/:did/tables`

**描述**: 删除表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table | string | 是 | 表名 |

---

### 15.28 `POST /api/db-servers/databases/:did/backup`

**描述**: 创建数据库备份  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

---

### 15.29 `GET /api/db-servers/databases/:did/backups`

**描述**: 获取数据库备份列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| did | int | 数据库 ID |

---

### 15.30 `GET /api/db-servers/backups/:bid/download`

**描述**: 下载数据库备份文件  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| bid | int | 备份 ID |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 备份未完成 |
| 40400 | 备份不存在或备份文件不存在 |

---

### 15.31 `POST /api/db-servers/backups/:bid/restore`

**描述**: 恢复数据库备份  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| bid | int | 备份 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| confirm | bool | 是 | 必须为 true 确认恢复操作 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 未确认（需设置 confirm: true） |
| 40400 | 备份不存在 |

---

### 15.32 `DELETE /api/db-servers/backups/:bid`

**描述**: 删除数据库备份  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| bid | int | 备份 ID |

---

### 15.33 `GET /api/db-servers/mysql/config`

**描述**: 获取 MySQL 配置文件内容  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "found": true,
    "config": {
      "file_path": "/etc/mysql/my.cnf",
      "sections": [
        {
          "name": "mysqld",
          "params": {
            "max_connections": "151",
            "innodb_buffer_pool_size": "128M"
          }
        }
      ]
    },
    "sections": {
      "mysqld": {
        "params": {"max_connections": "151"},
        "meta": [{"key": "max_connections", "description": "最大连接数"}]
      }
    }
  }
}
```

---

### 15.34 `POST /api/db-servers/mysql/config`

**描述**: 保存 MySQL 配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| sections | array | 是 | 配置段数组 |

**sections 元素**:

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 段名称（如 mysqld） |
| params | object | 键值对参数 |

---

### 15.35 `GET /api/db-servers/mysql/common-params`

**描述**: 获取 MySQL 常用参数说明  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| section | string | mysqld | 配置段名称 |

---

### 15.36 `GET /api/db-servers/postgresql/config`

**描述**: 获取 PostgreSQL 配置文件内容  
**认证**: 需要 JWT

---

### 15.37 `POST /api/db-servers/postgresql/config`

**描述**: 保存 PostgreSQL 配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**: 同 MySQL 配置保存接口

---

### 15.38 `GET /api/db-servers/postgresql/common-params`

**描述**: 获取 PostgreSQL 常用参数说明  
**认证**: 需要 JWT

---

### 15.39 `GET /api/db-servers/redis/config`

**描述**: 获取 Redis 配置文件内容  
**认证**: 需要 JWT

---

### 15.40 `POST /api/db-servers/redis/config`

**描述**: 保存 Redis 配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**: 同 MySQL 配置保存接口

---

### 15.41 `GET /api/db-servers/redis/common-params`

**描述**: 获取 Redis 常用参数说明  
**认证**: 需要 JWT

---

## 16. 定时任务

### 16.1 `GET /api/cron/presets`

**描述**: 获取预设的 cron 表达式模板  
**认证**: 需要 JWT

---

### 16.2 `GET /api/cron/describe`

**描述**: 解析 cron 表达式为人类可读描述  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| schedule | string | 是 | cron 表达式 |

---

### 16.3 `GET /api/cron/next-runs`

**描述**: 获取 cron 表达式的下次执行时间  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| schedule | string | 是 | cron 表达式 |

---

### 16.4 `GET /api/cron/tasks`

**描述**: 获取所有定时任务  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "backup-daily",
      "command": "/scripts/backup.sh",
      "schedule": "0 2 * * *",
      "description": "每日备份",
      "enabled": true,
      "status": "idle",
      "script_id": 0,
      "timeout": 3600,
      "max_retry": 3,
      "env_vars": "",
      "work_dir": "/tmp",
      "last_run": "2024-01-15 02:00:00",
      "next_run": "2024-01-16 02:00:00",
      "created_at": "2024-01-01 00:00:00"
    }
  ]
}
```

---

### 16.5 `POST /api/cron/tasks`

**描述**: 创建定时任务  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 任务名称（唯一） |
| schedule | string | 是 | cron 表达式（5 个字段：分 时 日 月 周） |
| command | string | 条件 | 命令（与 script_id 二选一） |
| script_id | int | 条件 | 脚本 ID（与 command 二选一） |
| description | string | 否 | 描述 |
| timeout | int | 否 | 超时时间（0-86400 秒） |
| max_retry | int | 否 | 最大重试次数（0-10） |
| env_vars | string | 否 | 环境变量（JSON） |
| work_dir | string | 否 | 工作目录 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 参数校验失败（表达式格式/名称重复/超时范围/缺少命令或脚本） |

---

### 16.6 `GET /api/cron/tasks/:id`

**描述**: 获取定时任务详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 16.7 `PUT /api/cron/tasks/:id`

**描述**: 更新定时任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

**请求参数 (Request Body)**: 所有字段均可选（部分更新）

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 任务名称 |
| schedule | string | cron 表达式 |
| command | string | 命令 |
| description | string | 描述 |
| script_id | int | 脚本 ID |
| timeout | int | 超时时间 |
| max_retry | int | 最大重试次数 |
| env_vars | string | 环境变量 |
| work_dir | string | 工作目录 |

---

### 16.8 `DELETE /api/cron/tasks/:id`

**描述**: 删除定时任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 16.9 `POST /api/cron/tasks/:id/enable`

**描述**: 启用定时任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 16.10 `POST /api/cron/tasks/:id/disable`

**描述**: 禁用定时任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

---

### 16.11 `POST /api/cron/tasks/:id/run`

**描述**: 立即执行定时任务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "任务已执行"
  }
}
```

---

### 16.12 `GET /api/cron/tasks/:id/logs`

**描述**: 获取定时任务执行日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 任务 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| limit | int | 50 | 返回条数（最大 200） |

---

### 16.13 `GET /api/cron/scripts`

**描述**: 获取脚本列表  
**认证**: 需要 JWT

---

### 16.14 `POST /api/cron/scripts`

**描述**: 创建脚本  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 脚本名称（唯一） |
| content | string | 是 | 脚本内容 |
| language | string | 否 | 语言（sh、bash、python、python3，默认 sh） |
| description | string | 否 | 描述 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 名称重复/内容为空/不支持的语言/解释器未安装 |

---

### 16.15 `GET /api/cron/scripts/:id`

**描述**: 获取脚本详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 脚本 ID |

---

### 16.16 `PUT /api/cron/scripts/:id`

**描述**: 更新脚本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 脚本 ID |

**请求参数 (Request Body)**: 所有字段均可选

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 脚本名称 |
| content | string | 脚本内容 |
| language | string | 语言 |
| description | string | 描述 |

---

### 16.17 `DELETE /api/cron/scripts/:id`

**描述**: 删除脚本  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 脚本 ID |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40900 | 存在依赖此脚本的定时任务 |

---

### 16.18 `GET /api/cron/docs`

**描述**: 获取定时任务文档列表  
**认证**: 需要 JWT

---

### 16.19 `POST /api/cron/docs`

**描述**: 创建定时任务文档  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 是 | 文档标题 |
| content | string | 是 | 文档内容 |

---

### 16.20 `GET /api/cron/docs/:id`

**描述**: 获取文档详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 文档 ID |

---

### 16.21 `PUT /api/cron/docs/:id`

**描述**: 更新文档  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 文档 ID |

---

### 16.22 `DELETE /api/cron/docs/:id`

**描述**: 删除文档  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 文档 ID |

---

## 17. 防火墙

### 17.1 `GET /api/firewall/status`

**描述**: 获取防火墙状态  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "enabled": true,
    "tool": "iptables",
    "version": "1.8.7",
    "rule_count": 15,
    "custom_rule_count": 5,
    "default_in": "ACCEPT",
    "default_out": "ACCEPT"
  }
}
```

---

### 17.2 `POST /api/firewall/enable`

**描述**: 启用防火墙  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "防火墙已启用"
  }
}
```

---

### 17.3 `POST /api/firewall/disable`

**描述**: 禁用防火墙（需要确认）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| confirm | bool | 是 | 必须为 true |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 未确认（需设置 confirm: true） |

---

### 17.4 `POST /api/firewall/default-policy`

**描述**: 设置默认链策略  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| chain | string | 是 | 链名（INPUT 或 OUTPUT） |
| policy | string | 是 | 策略（ACCEPT 或 DROP） |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的链或策略 |

---

### 17.5 `GET /api/firewall/logs`

**描述**: 获取防火墙日志  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| lines | int | 100 | 返回行数（最大 1000） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "timestamp": "Jan 15 10:00:00",
      "action": "DROP",
      "protocol": "tcp",
      "src_ip": "1.2.3.4",
      "dst_ip": "5.6.7.8",
      "src_port": 12345,
      "dst_port": 22,
      "interface": "eth0",
      "raw": "Jan 15 10:00:00 server kernel: ..."
    }
  ]
}
```

---

### 17.6 `GET /api/firewall/rules`

**描述**: 获取防火墙规则列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "chain": "INPUT",
      "protocol": "tcp",
      "port": "80",
      "action": "ACCEPT",
      "source": "0.0.0.0/0",
      "target": "",
      "enabled": true,
      "priority": 100,
      "ip_version": "ipv4",
      "remark": "允许 HTTP",
      "created_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 17.7 `POST /api/firewall/rules`

**描述**: 创建防火墙规则  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| chain | string | 是 | 链（INPUT、OUTPUT、FORWARD） |
| protocol | string | 否 | 协议（tcp、udp、all） |
| port | string | 否 | 端口（如 "80"、"8000-9000"） |
| action | string | 是 | 动作（ACCEPT、DROP、REJECT） |
| source | string | 否 | 源 IP/CIDR |
| ip_version | string | 否 | IP 版本（ipv4、ipv6、both） |
| remark | string | 否 | 备注 |

---

### 17.8 `GET /api/firewall/rules/export`

**描述**: 导出防火墙规则  
**认证**: 需要 JWT

---

### 17.9 `POST /api/firewall/rules/import`

**描述**: 导入防火墙规则  
**认证**: 需要 JWT

---

### 17.10 `POST /api/firewall/rules/bulk-enable`

**描述**: 批量启用规则  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 规则 ID 列表 |

---

### 17.11 `POST /api/firewall/rules/bulk-disable`

**描述**: 批量禁用规则  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 规则 ID 列表 |

---

### 17.12 `POST /api/firewall/rules/bulk-delete`

**描述**: 批量删除规则  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 规则 ID 列表 |

---

### 17.13 `GET /api/firewall/rules/:id`

**描述**: 获取单条规则详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.14 `PUT /api/firewall/rules/:id`

**描述**: 更新防火墙规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

**请求参数 (Request Body)**: 所有字段均可选

| 字段 | 类型 | 说明 |
|------|------|------|
| chain | string | 链 |
| protocol | string | 协议 |
| port | string | 端口 |
| action | string | 动作 |
| source | string | 源 IP/CIDR |
| ip_version | string | IP 版本 |
| remark | string | 备注 |

---

### 17.15 `DELETE /api/firewall/rules/:id`

**描述**: 删除防火墙规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.16 `POST /api/firewall/rules/:id/enable`

**描述**: 启用单条规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.17 `POST /api/firewall/rules/:id/disable`

**描述**: 禁用单条规则  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.18 `POST /api/firewall/rules/:id/move-up`

**描述**: 上移规则优先级  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.19 `POST /api/firewall/rules/:id/move-down`

**描述**: 下移规则优先级  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 规则 ID |

---

### 17.20 `GET /api/firewall/system-rules`

**描述**: 获取系统级防火墙规则（由系统/其他工具管理的规则）  
**认证**: 需要 JWT

---

### 17.21 `POST /api/firewall/system-rules/delete`

**描述**: 删除系统级防火墙规则  
**认证**: 需要 JWT

---

### 17.22 `GET /api/firewall/templates`

**描述**: 获取防火墙规则模板  
**认证**: 需要 JWT

---

### 17.23 `POST /api/firewall/templates/apply`

**描述**: 应用防火墙规则模板  
**认证**: 需要 JWT

---

## 18. SSH管理

### 18.1 `GET /api/ssh/config`

**描述**: 获取 SSH 服务器配置  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "port": 22,
    "permit_root_login": "prohibit-password",
    "password_auth": "yes",
    "pubkey_auth": "yes",
    "max_auth_tries": 6,
    "login_grace_time": 120,
    "client_alive_interval": 300,
    "client_alive_count_max": 3,
    "allow_users": "",
    "deny_users": ""
  }
}
```

---

### 18.2 `PUT /api/ssh/config`

**描述**: 保存 SSH 配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| port | int | 否 | SSH 端口（1-65535） |
| permit_root_login | string | 否 | Root 登录策略（yes/no/prohibit-password） |
| password_auth | string | 否 | 密码认证（yes/no） |
| pubkey_auth | string | 否 | 公钥认证（yes/no） |
| max_auth_tries | int | 否 | 最大认证尝试次数（0-100） |
| login_grace_time | int | 否 | 登录超时时间（0-3600 秒） |
| client_alive_interval | int | 否 | 客户端心跳间隔（0-86400 秒） |
| client_alive_count_max | int | 否 | 心跳最大计数（0-100） |
| allow_users | string | 否 | 允许的用户列表 |
| deny_users | string | 否 | 拒绝的用户列表 |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 参数校验失败 |

---

### 18.3 `POST /api/ssh/config/test`

**描述**: 测试 SSH 配置文件  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "配置文件语法正确"
  }
}
```

---

### 18.4 `POST /api/ssh/config/reload`

**描述**: 重载 SSH 服务  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "SSH 服务已重载"
  }
}
```

---

### 18.5 `GET /api/ssh/sessions`

**描述**: 获取活跃 SSH 会话列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "sessions": [
      {
        "pid": 12345,
        "user": "root",
        "ip": "192.168.1.100",
        "started": "2024-01-15 10:00:00"
      }
    ]
  }
}
```

---

### 18.6 `POST /api/ssh/sessions/:pid/kill`

**描述**: 终止指定 SSH 会话  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| pid | int | 会话进程 PID（必须大于 1） |

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40000 | 无效的 PID |

---

### 18.7 `GET /api/ssh/logins`

**描述**: 获取 SSH 登录历史  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| limit | int | 50 | 返回记录数 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "records": [
      {
        "user": "root",
        "ip": "192.168.1.100",
        "time": "2024-01-15 10:00:00",
        "status": "accepted"
      }
    ]
  }
}
```

---

## 19. 容器/Docker

### 19.1 `GET /api/docker/status`

**描述**: 检测 Docker 安装状态  
**认证**: 需要 JWT

---

### 19.2 `POST /api/docker/install`

**描述**: 安装 Docker  
**认证**: 需要 JWT

---

### 19.3 `POST /api/docker/start`

**描述**: 启动 Docker 服务  
**认证**: 需要 JWT

---

### 19.4 `POST /api/docker/stop`

**描述**: 停止 Docker 服务  
**认证**: 需要 JWT

---

### 19.5 `POST /api/docker/restart`

**描述**: 重启 Docker 服务  
**认证**: 需要 JWT

---

### 19.6 `GET /api/docker/info`

**描述**: 获取 Docker 系统信息  
**认证**: 需要 JWT

---

### 19.7 `POST /api/docker/mirror`

**描述**: 配置 Docker 镜像加速器  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| mirrors | string[] | 是 | 镜像加速器 URL 列表 |

---

### 19.8 `GET /api/containers`

**描述**: 获取容器列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| all | string | 设为 "true" 包含已停止容器 |

---

### 19.9 `GET /api/containers/:id`

**描述**: 获取容器详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID 或名称 |

---

### 19.10 `POST /api/containers`

**描述**: 创建容器  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 容器名称 |
| image | string | 是 | 镜像名称 |
| ports | object | 否 | 端口映射 |
| volumes | array | 否 | 卷挂载 |
| env | array | 否 | 环境变量 |
| cmd | string[] | 否 | 启动命令 |

---

### 19.11 `POST /api/containers/:id/start`

**描述**: 启动容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.12 `POST /api/containers/:id/stop`

**描述**: 停止容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.13 `POST /api/containers/:id/restart`

**描述**: 重启容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.14 `POST /api/containers/:id/pause`

**描述**: 暂停容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.15 `POST /api/containers/:id/unpause`

**描述**: 恢复容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.16 `DELETE /api/containers/:id`

**描述**: 删除容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| force | string | 设为 "true" 强制删除运行中的容器 |

---

### 19.17 `GET /api/containers/:id/logs`

**描述**: 获取容器日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| tail | int | 100 | 返回行数 |
| since | string | - | 起始时间 |

---

### 19.18 `POST /api/containers/:id/exec`

**描述**: 在容器中执行命令  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| cmd | string[] | 是 | 命令及参数 |

---

### 19.19 `GET /api/containers/:id/stats`

**描述**: 获取容器资源统计  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.20 `GET /api/containers/:id/top`

**描述**: 获取容器进程列表  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.21 `POST /api/containers/:id/copy-to`

**描述**: 复制文件到容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**Content-Type**: `multipart/form-data`

---

### 19.22 `POST /api/containers/:id/copy-from`

**描述**: 从容器复制文件  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 容器内文件路径 |

---

### 19.23 `POST /api/containers/:id/rename`

**描述**: 重命名容器  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 新名称 |

---

### 19.24 `PUT /api/containers/:id/update`

**描述**: 更新容器配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 容器 ID |

---

### 19.25 `GET /api/images`

**描述**: 获取镜像列表  
**认证**: 需要 JWT

---

### 19.26 `POST /api/images/pull`

**描述**: 拉取镜像  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| image | string | 是 | 镜像名称（如 nginx:latest） |

---

### 19.27 `DELETE /api/images/:id`

**描述**: 删除镜像  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 镜像 ID |

---

### 19.28 `GET /api/compose/projects`

**描述**: 获取 Docker Compose 项目列表  
**认证**: 需要 JWT

---

### 19.29 `POST /api/compose/up`

**描述**: 启动 Compose 项目  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |

---

### 19.30 `POST /api/compose/down`

**描述**: 停止并删除 Compose 项目  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |

---

### 19.31 `POST /api/compose/restart`

**描述**: 重启 Compose 项目  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |

---

### 19.32 `GET /api/compose/logs`

**描述**: 获取 Compose 项目日志  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |

---

### 19.33 `GET /api/compose/config`

**描述**: 获取 Compose 配置文件内容  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |

---

### 19.34 `PUT /api/compose/config`

**描述**: 保存 Compose 配置文件  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | compose.yml 文件路径 |
| content | string | 是 | YAML 内容 |

---

### 19.35 `GET /api/volumes`

**描述**: 获取 Docker 卷列表  
**认证**: 需要 JWT

---

### 19.36 `POST /api/volumes`

**描述**: 创建 Docker 卷  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 卷名称 |
| driver | string | 否 | 驱动（默认 local） |

---

### 19.37 `DELETE /api/volumes/:name`

**描述**: 删除 Docker 卷  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 卷名称 |

---

### 19.38 `GET /api/networks`

**描述**: 获取 Docker 网络列表  
**认证**: 需要 JWT

---

### 19.39 `POST /api/networks`

**描述**: 创建 Docker 网络  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 网络名称 |
| driver | string | 否 | 驱动（默认 bridge） |

---

### 19.40 `DELETE /api/networks/:id`

**描述**: 删除 Docker 网络  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 网络 ID |

---

## 20. 模板

### 20.1 `GET /api/templates/docker-images`

**描述**: 获取 Docker 镜像模板列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "version": 1,
    "categories": [
      {
        "name": "Web 服务器",
        "images": [
          {
            "name": "Nginx",
            "image": "nginx:latest",
            "description": "高性能 HTTP 服务器",
            "ports": ["80:80", "443:443"]
          }
        ]
      }
    ]
  }
}
```

---

### 20.2 `GET /api/templates/categories`

**描述**: 获取模板分类名称列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": ["Web 服务器", "数据库"]
}
```

---

### 20.3 `GET /api/templates/scripts`

**描述**: 获取脚本模板列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "categories": [
      {
        "name": "常用脚本",
        "scripts": [...]
      }
    ]
  }
}
```

---

## 21. 进程守护

### 21.1 `GET /api/processes`

**描述**: 获取所有托管进程列表（包含运行状态）  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "my-app",
      "command": "/usr/bin/node",
      "args": "server.js",
      "dir": "/app",
      "env": "{}",
      "auto_restart": true,
      "max_restarts": 5,
      "restart_delay": 5,
      "stop_timeout": 10,
      "startup_timeout": 30,
      "auto_start": true,
      "log_file": "/var/log/my-app.log",
      "group_id": 0,
      "status": {
        "id": 1,
        "process_id": 1,
        "status": "running",
        "pid": 12345,
        "uptime": 3600,
        "restarts": 0,
        "cpu_percent": 5.2,
        "memory_mb": 128.5,
        "exit_code": 0,
        "last_start": "2024-01-15 10:00:00"
      }
    }
  ]
}
```

---

### 21.2 `POST /api/processes`

**描述**: 创建进程配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 进程名称 |
| command | string | 是 | 启动命令 |
| args | string | 否 | 命令参数 |
| dir | string | 否 | 工作目录 |
| env | string | 否 | 环境变量（JSON） |
| auto_restart | bool | 否 | 是否自动重启 |
| max_restarts | int | 否 | 最大重启次数 |
| restart_delay | int | 否 | 重启延迟（秒） |
| stop_timeout | int | 否 | 停止超时（秒） |
| startup_timeout | int | 否 | 启动超时（秒） |
| auto_start | bool | 否 | 是否开机自启 |
| log_file | string | 否 | 日志文件路径 |
| group_id | int | 否 | 分组 ID |

---

### 21.3 `GET /api/processes/:id`

**描述**: 获取进程详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

---

### 21.4 `PUT /api/processes/:id`

**描述**: 更新进程配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

**请求参数 (Request Body)**: 所有字段均可选（部分更新）

---

### 21.5 `DELETE /api/processes/:id`

**描述**: 删除进程配置  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

---

### 21.6 `POST /api/processes/:id/start`

**描述**: 启动进程  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

---

### 21.7 `POST /api/processes/:id/stop`

**描述**: 停止进程  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

---

### 21.8 `POST /api/processes/:id/restart`

**描述**: 重启进程  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

---

### 21.9 `GET /api/processes/:id/logs`

**描述**: 获取进程日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| limit | int | 50 | 返回条数 |
| offset | int | 0 | 偏移量 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 1,
        "process_id": 1,
        "type": "stdout",
        "content": "Server started on port 3000",
        "created_at": "2024-01-15 10:00:00"
      }
    ],
    "total": 500
  }
}
```

---

### 21.10 `GET /api/processes/:id/stats`

**描述**: 获取进程运行时资源统计  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 进程 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "cpu_percent": 5.2,
    "memory_mb": 128.5,
    "pid": 12345,
    "uptime": 3600,
    "restarts": 0
  }
}
```

---

### 21.11 `POST /api/processes/batch/start`

**描述**: 批量启动进程  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 进程 ID 列表 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "started": 3,
    "failed": 1
  }
}
```

---

### 21.12 `POST /api/processes/batch/stop`

**描述**: 批量停止进程  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 进程 ID 列表 |

---

### 21.13 `POST /api/processes/batch/restart`

**描述**: 批量重启进程  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | int[] | 是 | 进程 ID 列表 |

---

### 21.14 `GET /api/process-groups`

**描述**: 获取进程分组列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "Web Apps",
      "description": "Web 应用进程组",
      "created_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 21.15 `POST /api/process-groups`

**描述**: 创建进程分组  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 分组名称 |
| description | string | 否 | 描述 |

---

### 21.16 `GET /api/process-groups/:id`

**描述**: 获取进程分组详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 分组 ID |

---

### 21.17 `PUT /api/process-groups/:id`

**描述**: 更新进程分组  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 分组 ID |

**请求参数 (Request Body)**: 所有字段均可选

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 分组名称 |
| description | string | 描述 |

---

### 21.18 `DELETE /api/process-groups/:id`

**描述**: 删除进程分组  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 分组 ID |

---

### 21.19 `GET /api/processes/export`

**描述**: 导出所有进程配置为 JSON  
**认证**: 需要 JWT

---

### 21.20 `POST /api/processes/import`

**描述**: 从 JSON 导入进程配置  
**认证**: 需要 JWT

**请求参数 (Request Body)**: 进程配置数组

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "imported": 5
  }
}
```

---

## 22. 系统进程

### 22.1 `GET /api/system/overview`

**描述**: 获取系统资源概览  
**认证**: 需要 JWT

---

### 22.2 `GET /api/system/processes`

**描述**: 获取系统进程列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| sort_by | string | memory | 排序字段（memory/cpu） |
| order | string | desc | 排序方向（asc/desc） |
| search | string | - | 搜索关键词 |
| limit | int | 100 | 返回数量 |

---

### 22.3 `GET /api/system/processes/:pid`

**描述**: 获取系统进程详情  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| pid | int | 进程 PID |

---

### 22.4 `GET /api/system/services`

**描述**: 获取系统 systemd 服务列表  
**认证**: 需要 JWT

---

### 22.5 `POST /api/system/services/:name/action`

**描述**: 对 systemd 服务执行操作  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| action | string | 是 | 操作（start/stop/restart/reload/enable/disable） |
| force | bool | 否 | 是否强制操作（对受保护服务） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "服务 nginx start 成功"
  }
}
```

**错误码**:

| 错误码 | 说明 |
|--------|------|
| 40300 | 受保护的服务，需设置 force: true |

---

### 22.6 `GET /api/system/services/:name/logs`

**描述**: 获取系统服务日志  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| lines | int | 100 | 返回行数 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "logs": ["log line 1", "log line 2"],
    "service": "nginx"
  }
}
```

---

### 22.7 `GET /api/system/services/protected`

**描述**: 获取受保护的服务列表  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "name": "sshd",
      "reason": "SSH 守护进程，停止后将无法远程访问"
    }
  ]
}
```

---

### 22.8 `GET /api/system/services/whitelist`

**描述**: 获取服务白名单  
**认证**: 需要 JWT

---

### 22.9 `POST /api/system/services/whitelist`

**描述**: 添加服务到白名单  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 服务名称 |

---

### 22.10 `DELETE /api/system/services/whitelist/:name`

**描述**: 从白名单移除服务  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 服务名称 |

---

## 23. 通知管理

### 23.1 `GET /api/notifications`

**描述**: 获取通知列表  
**认证**: 需要 JWT

**查询参数**:

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| unread | string | - | 设为 "true" 仅返回未读 |
| limit | int | 50 | 返回数量 |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "title": "CPU 使用率过高",
      "content": "CPU 使用率已超过 90%",
      "level": "warning",
      "read": false,
      "created_at": "2024-01-15 10:00:00"
    }
  ]
}
```

---

### 23.2 `GET /api/notifications/unread-count`

**描述**: 获取未读通知数量  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "count": 5
  }
}
```

---

### 23.3 `POST /api/notifications`

**描述**: 创建通知（管理员操作）  
**认证**: 需要 JWT

**请求参数 (Request Body)**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 是 | 通知标题 |
| content | string | 是 | 通知内容 |
| level | string | 否 | 级别（info/warning/error） |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "通知已创建"
  }
}
```

---

### 23.4 `PUT /api/notifications/:id/read`

**描述**: 标记通知为已读  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 通知 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "已标记为已读"
  }
}
```

---

### 23.5 `PUT /api/notifications/read-all`

**描述**: 标记所有通知为已读  
**认证**: 需要 JWT

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "全部已标记为已读"
  }
}
```

---

### 23.6 `DELETE /api/notifications/:id`

**描述**: 删除通知  
**认证**: 需要 JWT

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 通知 ID |

**响应示例**:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "message": "已删除"
  }
}
```

---

## 附录

### A. 请求频率限制

API 使用滑动窗口限流机制，默认配置：
- 60 次请求 / 1 分钟

超出限制后返回 HTTP 429 状态码。

### B. CORS 跨域

CORS 通过配置文件中的 `allowed_origins` 控制。在开发模式下，自动允许 `localhost` 来源。

### C. 审计日志

所有受保护的 API 调用（除了健康检查和认证接口）都会自动记录到审计日志中，包括：
- 操作用户
- 操作类型
- 请求资源
- 客户端 IP
- User-Agent
- 请求状态码

### D. 安全头部

所有响应自动附加以下安全头部：
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
