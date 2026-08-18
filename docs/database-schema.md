# EasyServer 数据库设计

> 数据库：SQLite 3 (modernc.org/sqlite)
> 存储位置：`./data/easyserver.db`
> 模式：WAL

## 总览

```
┌─────────────┐    ┌─────────────────┐
│    users    │────│   audit_logs    │
└─────────────┘    └─────────────────┘
       │
       │ 1:N        ┌─────────────────┐
       ├────────────│    sessions     │
       │            └─────────────────┘
       │
       │ 1:N        ┌─────────────────────────┐
       └────────────│  user_activities        │
                    └─────────────────────────┘

┌─────────────────┐    ┌─────────────────────┐
│  monitor_data   │    │ token_blacklist     │
└─────────────────┘    └─────────────────────┘

┌─────────────────┐    ┌─────────────────────┐
│  cron_tasks     │    │  file_shares        │
└─────────────────┘    └─────────────────────┘

┌─────────────────┐    ┌─────────────────────┐
│ firewall_rules  │    │  notifications      │
└─────────────────┘    └─────────────────────┘

┌─────────────────────────┐    ┌─────────────────────────┐
│ web_servers             │    │  websites               │
└─────────────────────────┘    └─────────────────────────┘

┌─────────────────────────┐    ┌─────────────────────────┐
│ db_servers              │    │  db_versions            │
└─────────────────────────┘    └─────────────────────────┘

┌─────────────────────────┐    ┌─────────────────────────┐
│ runtime_environments    │    │  packages               │
└─────────────────────────┘    └─────────────────────────┘
```

---

## 表结构

### users

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 用户 ID |
| username | TEXT | UNIQUE NOT NULL | 用户名 |
| password_hash | TEXT | NOT NULL | bcrypt 哈希 |
| role | TEXT | NOT NULL DEFAULT 'admin' | 角色（单管理员模式固定 admin） |
| must_change_pass | INTEGER | DEFAULT 0 | 首次登录需改密码 |
| last_login_ip | TEXT | DEFAULT '' | 最后登录 IP |
| login_attempts | INTEGER | DEFAULT 0 | 连续登录失败次数 |
| locked_until | DATETIME | | 锁定到期时间 |
| expires_at | DATETIME | | 账号过期时间 |
| ip_whitelist | TEXT | DEFAULT '' | IP 白名单（逗号分隔） |
| totp_secret | TEXT | DEFAULT '' | TOTP 密钥 |
| totp_enabled | INTEGER | DEFAULT 0 | 是否启用 2FA |
| totp_backup_codes | TEXT | DEFAULT '[]' | 备用恢复码（JSON） |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 更新时间 |

```sql
CREATE TABLE IF NOT EXISTS users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    username          TEXT    NOT NULL UNIQUE,
    password_hash     TEXT    NOT NULL,
    role              TEXT    NOT NULL DEFAULT 'admin',
    must_change_pass  INTEGER DEFAULT 0,
    last_login_ip     TEXT    DEFAULT '',
    login_attempts    INTEGER DEFAULT 0,
    locked_until      DATETIME,
    expires_at        DATETIME,
    ip_whitelist      TEXT    DEFAULT '',
    totp_secret       TEXT    DEFAULT '',
    totp_enabled      INTEGER DEFAULT 0,
    totp_backup_codes TEXT    DEFAULT '[]',
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### sessions

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 会话 ID |
| token | TEXT | UNIQUE NOT NULL | JWT Token（SHA-256 哈希） |
| user_id | INTEGER | NOT NULL | 用户 ID |
| username | TEXT | NOT NULL | 用户名 |
| role | TEXT | NOT NULL | 用户角色 |
| ip | TEXT | DEFAULT '' | 登录 IP |
| user_agent | TEXT | DEFAULT '' | 客户端信息 |
| client_type | TEXT | DEFAULT 'web' | 客户端类型（web/mobile） |
| device_id | TEXT | DEFAULT '' | 设备 ID |
| device_info | TEXT | DEFAULT '' | 设备信息 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| expires_at | DATETIME | NOT NULL | 过期时间 |

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    token        TEXT    NOT NULL UNIQUE,
    user_id      INTEGER NOT NULL,
    username     TEXT    NOT NULL,
    role         TEXT    NOT NULL,
    ip           TEXT    DEFAULT '',
    user_agent   TEXT    DEFAULT '',
    client_type  TEXT    DEFAULT 'web',
    device_id    TEXT    DEFAULT '',
    device_info  TEXT    DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at   DATETIME NOT NULL
);

CREATE INDEX idx_sessions_user  ON sessions(user_id);
CREATE INDEX idx_sessions_token ON sessions(token);
```

### audit_logs

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 日志 ID |
| user_id | INTEGER | | 操作用户 |
| username | TEXT | | 操作时用户名 |
| action | TEXT | NOT NULL | 操作标识 |
| resource | TEXT | | 操作对象/资源路径 |
| detail | TEXT | | 附加信息（JSON） |
| status | INTEGER | DEFAULT 0 | 操作状态（0=成功,1=失败） |
| ip | TEXT | | 请求来源 IP |
| user_agent | TEXT | | 客户端 User-Agent |
| type | TEXT | DEFAULT 'security' | 日志类型（security/system） |
| signature | TEXT | DEFAULT '' | 数据签名 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 操作时间 |

```sql
CREATE TABLE IF NOT EXISTS audit_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER,
    username    TEXT,
    action      TEXT    NOT NULL,
    resource    TEXT,
    detail      TEXT,
    status      INTEGER DEFAULT 0,
    ip          TEXT,
    user_agent  TEXT,
    type        TEXT    DEFAULT 'security',
    signature   TEXT    DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_user       ON audit_logs(user_id);
CREATE INDEX idx_audit_action     ON audit_logs(action);
CREATE INDEX idx_audit_type       ON audit_logs(type);
```

### user_activities

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 活动 ID |
| user_id | INTEGER | NOT NULL | 用户 ID |
| username | TEXT | NOT NULL | 用户名 |
| action | TEXT | NOT NULL | 操作类型（LOGIN_SUCCESS 等） |
| ip | TEXT | DEFAULT '' | 来源 IP |
| user_agent | TEXT | DEFAULT '' | 客户端信息 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 时间 |

```sql
CREATE TABLE IF NOT EXISTS user_activities (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    username   TEXT    NOT NULL,
    action     TEXT    NOT NULL,
    ip         TEXT    DEFAULT '',
    user_agent TEXT    DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_user_activities_user    ON user_activities(user_id);
CREATE INDEX idx_user_activities_created ON user_activities(created_at);
```

### monitor_data

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| cpu | REAL | | CPU 使用率 |
| cpu_load_1m | REAL | DEFAULT 0 | 1 分钟负载 |
| cpu_load_5m | REAL | DEFAULT 0 | 5 分钟负载 |
| cpu_load_15m | REAL | DEFAULT 0 | 15 分钟负载 |
| mem_total | INTEGER | | 内存总量 (bytes) |
| mem_used | INTEGER | | 内存已用 (bytes) |
| mem_percent | REAL | | 内存使用率 |
| swap_total | INTEGER | | Swap 总量 |
| swap_used | INTEGER | | Swap 已用 |
| disk_total | INTEGER | | 磁盘总量 (bytes) |
| disk_used | INTEGER | | 磁盘已用 (bytes) |
| disk_percent | REAL | | 磁盘使用率 |
| net_bytes_sent | INTEGER | | 增量发送字节 |
| net_bytes_recv | INTEGER | | 增量接收字节 |
| timestamp | DATETIME | DEFAULT CURRENT_TIMESTAMP | 采集时间点 |

```sql
CREATE TABLE IF NOT EXISTS monitor_data (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    cpu             REAL,
    cpu_load_1m     REAL DEFAULT 0,
    cpu_load_5m     REAL DEFAULT 0,
    cpu_load_15m    REAL DEFAULT 0,
    mem_total       INTEGER,
    mem_used        INTEGER,
    mem_percent     REAL,
    swap_total      INTEGER,
    swap_used       INTEGER,
    disk_total      INTEGER,
    disk_used       INTEGER,
    disk_percent    REAL,
    net_bytes_sent  INTEGER,
    net_bytes_recv  INTEGER,
    timestamp       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_monitor_timestamp ON monitor_data(timestamp);
```

### token_blacklist

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| user_id | INTEGER | NOT NULL | 用户 ID |
| token | TEXT | NOT NULL | Token SHA-256 哈希 |
| expires_at | DATETIME | NOT NULL | 过期时间 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

```sql
CREATE TABLE IF NOT EXISTS token_blacklist (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    token      TEXT    NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_token_blacklist_user   ON token_blacklist(user_id);
CREATE INDEX idx_token_blacklist_expires ON token_blacklist(expires_at);
```

### cron_tasks

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL | 任务名称 |
| command | TEXT | | 执行命令 |
| schedule | TEXT | NOT NULL | Cron 表达式 |
| description | TEXT | | 任务描述 |
| script_id | INTEGER | | 关联脚本 ID |
| timeout | INTEGER | DEFAULT 60 | 超时（秒） |
| max_retry | INTEGER | DEFAULT 0 | 最大重试次数 |
| env_vars | TEXT | | 环境变量（JSON） |
| work_dir | TEXT | | 工作目录 |
| enabled | INTEGER | DEFAULT 1 | 是否启用 |
| last_run | DATETIME | | 上次运行 |
| last_status | TEXT | | 上次状态 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 更新时间 |

```sql
CREATE TABLE IF NOT EXISTS cron_tasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    command     TEXT,
    schedule    TEXT NOT NULL,
    description TEXT,
    script_id   INTEGER,
    timeout     INTEGER DEFAULT 60,
    max_retry   INTEGER DEFAULT 0,
    env_vars    TEXT,
    work_dir    TEXT,
    enabled     INTEGER DEFAULT 1,
    last_run    DATETIME,
    last_status TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### file_shares

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| token | TEXT | UNIQUE NOT NULL | 分享 Token |
| file_path | TEXT | NOT NULL | 文件路径 |
| password_hash | TEXT | | 访问密码哈希 |
| expires_at | DATETIME | | 过期时间 |
| max_downloads | INTEGER | DEFAULT 0 | 最大下载次数（0=无限） |
| download_count | INTEGER | DEFAULT 0 | 已下载次数 |
| created_by | INTEGER | NOT NULL | 创建者用户 ID |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

```sql
CREATE TABLE IF NOT EXISTS file_shares (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    token          TEXT    NOT NULL UNIQUE,
    file_path      TEXT    NOT NULL,
    password_hash  TEXT,
    expires_at     DATETIME,
    max_downloads  INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    created_by     INTEGER NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### firewall_rules

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| chain | TEXT | NOT NULL | 链（INPUT/OUTPUT/FORWARD） |
| protocol | TEXT | | tcp/udp/icmp |
| port | TEXT | | 端口 |
| action | TEXT | NOT NULL | ACCEPT/DROP/REJECT |
| source | TEXT | | 源 IP/CIDR |
| ip_version | TEXT | DEFAULT 'ipv4' | IP 版本 |
| remark | TEXT | | 备注 |
| enabled | INTEGER | DEFAULT 1 | 是否启用 |
| sort_order | INTEGER | DEFAULT 0 | 排序 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 更新时间 |

```sql
CREATE TABLE IF NOT EXISTS firewall_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    chain      TEXT NOT NULL,
    protocol   TEXT,
    port       TEXT,
    action     TEXT NOT NULL,
    source     TEXT,
    ip_version TEXT DEFAULT 'ipv4',
    remark     TEXT,
    enabled    INTEGER DEFAULT 1,
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### notifications

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| type | TEXT | NOT NULL | 类型 |
| title | TEXT | NOT NULL | 标题 |
| message | TEXT | NOT NULL | 内容 |
| level | TEXT | DEFAULT 'info' | 级别（info/warning/error） |
| is_read | INTEGER | DEFAULT 0 | 是否已读 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

```sql
CREATE TABLE IF NOT EXISTS notifications (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT NOT NULL,
    title      TEXT NOT NULL,
    message    TEXT NOT NULL,
    level      TEXT DEFAULT 'info',
    is_read    INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notifications_read ON notifications(is_read);
CREATE INDEX idx_notifications_created ON notifications(created_at);
```

### web_servers

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL UNIQUE | 服务器名称 |
| type | TEXT | NOT NULL | nginx/apache |
| version | TEXT | | 版本 |
| status | TEXT | DEFAULT 'stopped' | 状态 |
| config_path | TEXT | | 配置文件路径 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 更新时间 |

### websites

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| server_id | INTEGER | NOT NULL | 关联 Web 服务器 |
| name | TEXT | NOT NULL | 站点名称 |
| domain | TEXT | | 域名 |
| port | INTEGER | DEFAULT 80 | 端口 |
| root_path | TEXT | | 根目录 |
| ssl_enabled | INTEGER | DEFAULT 0 | 是否启用 SSL |
| ssl_cert | TEXT | | 证书路径 |
| ssl_key | TEXT | | 密钥路径 |
| status | TEXT | DEFAULT 'stopped' | 状态 |
| process_id | INTEGER | DEFAULT 0 | 进程 ID |
| config_options | TEXT | DEFAULT '' | 配置选项（JSON） |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 更新时间 |

### db_servers

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| type | TEXT | NOT NULL | mysql/postgresql/redis |
| name | TEXT | NOT NULL | 名称 |
| status | TEXT | DEFAULT 'stopped' | 状态 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

### db_versions

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| server_id | INTEGER | NOT NULL | 关联数据库服务器 |
| version | TEXT | NOT NULL | 版本号 |
| port | INTEGER | | 端口 |
| status | TEXT | DEFAULT 'installed' | 状态 |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

### runtime_environments

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL | 运行时名称（node, python, go, java 等） |
| version | TEXT | NOT NULL | 版本号 |
| path | TEXT | NOT NULL | 安装路径 |
| is_default | INTEGER | DEFAULT 0 | 是否为默认版本 |
| status | TEXT | DEFAULT 'installed' | 状态 |
| installed_at | DATETIME | DEFAULT CURRENT_TIMESTAMP | 安装时间 |

```sql
CREATE TABLE IF NOT EXISTS runtime_environments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    path         TEXT NOT NULL,
    is_default   INTEGER DEFAULT 0,
    status       TEXT DEFAULT 'installed',
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version)
);
```

---

## 数据保留策略

| 表 | 保留时长 | 清理时机 |
|------|----------|----------|
| monitor_data | 168 小时（7天） | 后台定时任务每 10 分钟检查 |
| audit_logs | 90 天 | 用户可手动清理或配置自动清理 |
| sessions | 过期后清理 | 后台定时任务每 5 分钟清理 |
| token_blacklist | 过期后清理 | 后台定时任务每小时清理 |
| user_activities | 无自动清理 | — |
| notifications | 无自动清理 | 用户手动管理 |

---

## 迁移指南

使用纯 SQL 文件管理数据库版本：

```
migrations/
└── 000001_init_schema.up.sql
└── ...
└── 0000XX_xxx.up.sql
```

迁移表 `schema_migrations` 跟踪已应用的版本：

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

迁移工具在 `internal/infra/database/migrate.go` 中实现，支持：
- 幂等迁移（版本已存在则跳过）
- preTxHook 用于复杂迁移前置操作（如 ALTER TABLE ADD COLUMN 前检查列是否存在）
