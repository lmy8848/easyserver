# EasyServer 数据库设计

> 数据库：SQLite 3  
> 存储位置：`./data/easyserver.db`

## 总览

```
┌─────────────┐    ┌─────────────────┐
│    users    │────│   audit_logs    │
└─────────────┘    └─────────────────┘
       │
       │ 1:N        ┌─────────────────┐
       ├────────────│  user_sessions  │
       │            └─────────────────┘
       │
       │ 1:N        ┌─────────────────────────┐
       └────────────│  monitor_history        │
                    └─────────────────────────┘

┌─────────────────┐
│   settings      │  (键值对，存放系统配置)
└─────────────────┘
```

## 表结构

### users

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 用户 ID |
| username | TEXT | UNIQUE NOT NULL | 用户名 |
| password_hash | TEXT | NOT NULL | bcrypt 哈希 |
| role | TEXT | NOT NULL DEFAULT 'viewer' | admin / operator / viewer |
| is_locked | INTEGER | NOT NULL DEFAULT 0 | 是否锁定（登录失败超限） |
| lockout_until | TEXT | | 锁定到期时间 (ISO 8601) |
| login_attempts | INTEGER | NOT NULL DEFAULT 0 | 连续登录失败次数 |
| last_login_at | TEXT | | 最后登录时间 |
| last_login_ip | TEXT | | 最后登录 IP |
| must_change_password | INTEGER | NOT NULL DEFAULT 0 | 首次登录需改密码 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

```sql
CREATE TABLE IF NOT EXISTS users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    username          TEXT    NOT NULL UNIQUE,
    password_hash     TEXT    NOT NULL,
    role              TEXT    NOT NULL DEFAULT 'viewer'
                              CHECK(role IN ('admin','operator','viewer')),
    is_locked         INTEGER NOT NULL DEFAULT 0,
    lockout_until     TEXT,
    login_attempts    INTEGER NOT NULL DEFAULT 0,
    last_login_at     TEXT,
    last_login_ip     TEXT,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_users_role ON users(role);
```

### audit_logs

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 日志 ID |
| user_id | INTEGER | FK -> users.id, NULLABLE | 操作用户（未认证操作可为 NULL） |
| username | TEXT | | 操作时用户名（冗余，防止用户删除后丢数据） |
| action | TEXT | NOT NULL | 操作标识，如 `user.login`, `service.start` |
| target | TEXT | | 操作对象，如 `service:nginx` |
| ip | TEXT | | 请求来源 IP |
| user_agent | TEXT | | 客户端 User-Agent |
| detail | TEXT | | 附加信息（JSON） |
| status | TEXT | NOT NULL | success / failure |
| duration_ms | INTEGER | | 操作耗时 |
| created_at | TEXT | NOT NULL | 操作时间 |

```sql
CREATE TABLE IF NOT EXISTS audit_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER REFERENCES users(id),
    username    TEXT,
    action      TEXT    NOT NULL,
    target      TEXT,
    ip          TEXT,
    user_agent  TEXT,
    detail      TEXT,
    status      TEXT    NOT NULL DEFAULT 'success'
                        CHECK(status IN ('success','failure')),
    duration_ms INTEGER,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_audit_user   ON audit_logs(user_id);
CREATE INDEX idx_audit_action ON audit_logs(action);
CREATE INDEX idx_audit_time   ON audit_logs(created_at);
```

### user_sessions

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 会话 ID |
| user_id | INTEGER | FK -> users.id NOT NULL | 用户 ID |
| token_hash | TEXT | UNIQUE NOT NULL | Token SHA-256 哈希 |
| ip | TEXT | | 登录 IP |
| user_agent | TEXT | | 客户端信息 |
| expires_at | TEXT | NOT NULL | 过期时间 |
| last_active_at | TEXT | NOT NULL | 最后活跃时间 |
| created_at | TEXT | NOT NULL | 创建时间 |

```sql
CREATE TABLE IF NOT EXISTS user_sessions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    token_hash      TEXT    NOT NULL UNIQUE,
    ip              TEXT,
    user_agent      TEXT,
    expires_at      TEXT    NOT NULL,
    last_active_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_sessions_user   ON user_sessions(user_id);
CREATE INDEX idx_sessions_expire ON user_sessions(expires_at);
```

### monitor_history

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| timestamp | TEXT | NOT NULL | 采集时间点 |
| cpu_percent | REAL | | CPU 使用率 (0-100) |
| cpu_load_1m | REAL | | 1 分钟负载 |
| cpu_load_5m | REAL | | 5 分钟负载 |
| cpu_load_15m | REAL | | 15 分钟负载 |
| mem_total | INTEGER | | 内存总量 (bytes) |
| mem_used | INTEGER | | 内存已用 (bytes) |
| mem_percent | REAL | | 内存使用率 (0-100) |
| disk_total | INTEGER | | 磁盘总量 (bytes) |
| disk_used | INTEGER | | 磁盘已用 (bytes) |
| disk_percent | REAL | | 磁盘使用率 (0-100) |
| net_bytes_sent | INTEGER | | 累计发送字节 |
| net_bytes_recv | INTEGER | | 累计接收字节 |
| uptime_seconds | INTEGER | | 系统运行时间 |

```sql
CREATE TABLE IF NOT EXISTS monitor_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp       TEXT    NOT NULL,
    cpu_percent     REAL,
    cpu_load_1m     REAL,
    cpu_load_5m     REAL,
    cpu_load_15m    REAL,
    mem_total       INTEGER,
    mem_used        INTEGER,
    mem_percent     REAL,
    disk_total      INTEGER,
    disk_used       INTEGER,
    disk_percent    REAL,
    net_bytes_sent  INTEGER,
    net_bytes_recv  INTEGER,
    uptime_seconds  INTEGER
);

CREATE INDEX idx_monitor_time ON monitor_history(timestamp);

-- 自动清理 24 小时前的数据（由后端定时任务触发）
```

### settings

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| key | TEXT | PK | 配置键 |
| value | TEXT | | 配置值（JSON 编码） |
| updated_at | TEXT | NOT NULL | 更新时间 |

```sql
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### blocked_ips

IP 白名单/黑名单。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| cidr | TEXT | UNIQUE NOT NULL | IP/CIDR |
| type | TEXT | NOT NULL | allow / deny |
| remark | TEXT | | 备注 |
| created_at | TEXT | NOT NULL | |

```sql
CREATE TABLE IF NOT EXISTS blocked_ips (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    cidr       TEXT    NOT NULL UNIQUE,
    type       TEXT    NOT NULL CHECK(type IN ('allow','deny')),
    remark     TEXT,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
```

## 数据保留策略

| 表 | 保留时长 | 清理时机 |
|------|----------|----------|
| monitor_history | 24 小时 | 每次写入时删除过期数据，或后台定时任务每 10 分钟 |
| audit_logs | 90 天 | 后台定时任务每天执行一次 |
| user_sessions | 过期 + 7 天 | 后台定时任务每小时清理 |
| blocked_ips | 永久 | 手动管理 |

## 迁移指南

使用纯 SQL 文件管理数据库版本：

```
migrations/
├── 001_create_users.sql
├── 002_create_audit_logs.sql
├── 003_create_monitor_history.sql
├── 004_create_settings.sql
└── 005_create_blocked_ips.sql
```

迁移表 `schema_migrations` 跟踪已应用的版本：

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

应用迁移：

```go
// internal/model/migrate.go
func Migrate(db *sql.DB) error {
    // 读取 migrations/ 目录下的 .sql 文件
    // 按顺序执行未应用的版本
    // 记录到 schema_migrations
}
```
