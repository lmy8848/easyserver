# 数据库差异知识库

> 开发参考，避免多数据库兼容性问题

## SQL 语法差异

### 字符串引号

| 操作 | MySQL | PostgreSQL |
|------|-------|-----------|
| 字符串值 | `'text'` | `'text'` |
| 转义单引号 | `\'` 或 `''` | `''`（只用这个） |
| 标识符（表名/列名） | `` `name` `` | `"name"` |
| 反引号在 PG | 不支持 | 不支持 |

### 布尔值

| | MySQL | PostgreSQL |
|--|-------|-----------|
| 类型 | `TINYINT(1)` | `BOOLEAN` |
| 真值 | `1` | `true` |
| 假值 | `0` | `false` |

### 自增

| | MySQL | PostgreSQL |
|--|-------|-----------|
| 语法 | `AUTO_INCREMENT` | `SERIAL` 或 `GENERATED ALWAYS AS IDENTITY` |
| 获取最后 ID | `LAST_INSERT_ID()` | `RETURNING id` |
| 重置序列 | `ALTER TABLE t AUTO_INCREMENT = 1` | `ALTER SEQUENCE t_id_seq RESTART WITH 1` |

### 分页

| | MySQL | PostgreSQL |
|--|-------|-----------|
| 语法 | `LIMIT x OFFSET y` | `LIMIT x OFFSET y`（相同） |
| 简写 | `LIMIT y, x` | 不支持 |

### 数据类型映射

| 通用类型 | MySQL | PostgreSQL |
|----------|-------|-----------|
| 整数 | `INT` | `INTEGER` |
| 大整数 | `BIGINT` | `BIGINT` |
| 字符串 | `VARCHAR(n)` | `VARCHAR(n)` |
| 长文本 | `TEXT` | `TEXT` |
| 布尔 | `TINYINT(1)` | `BOOLEAN` |
| 日期时间 | `DATETIME` | `TIMESTAMP` |
| 浮点数 | `DOUBLE` | `DOUBLE PRECISION` |
| 小数 | `DECIMAL(p,s)` | `NUMERIC(p,s)` |
| JSON | `JSON` | `JSONB` |
| 二进制 | `BLOB` | `BYTEA` |

### SHOW/DESCRIBE 命令

| 操作 | MySQL | PostgreSQL |
|------|-------|-----------|
| 列出数据库 | `SHOW DATABASES` | `\l` 或 `SELECT datname FROM pg_database` |
| 列出表 | `SHOW TABLES` | `\dt` 或 `SELECT tablename FROM pg_tables WHERE schemaname='public'` |
| 表结构 | `DESCRIBE table` | `\d table` 或 `SELECT ... FROM information_schema.columns` |
| 列出用户 | `SELECT User, Host FROM mysql.user` | `\du` 或 `SELECT usename FROM pg_user` |

### 用户管理

| 操作 | MySQL | PostgreSQL |
|------|-------|-----------|
| 创建用户 | `CREATE USER 'u'@'h' IDENTIFIED BY 'p'` | `CREATE USER u WITH PASSWORD 'p'` |
| 删除用户 | `DROP USER 'u'@'h'` | `DROP USER u` |
| 授权 | `GRANT privs ON db.* TO 'u'@'h'` | `GRANT privs ON DATABASE db TO u` |
| 刷新权限 | `FLUSH PRIVILEGES` | 不需要 |

### 数据库管理

| 操作 | MySQL | PostgreSQL |
|------|-------|-----------|
| 创建数据库 | `CREATE DATABASE name CHARACTER SET utf8mb4` | `CREATE DATABASE name ENCODING 'UTF8'` |
| 删除数据库 | `DROP DATABASE name` | `DROP DATABASE name` |

## 转义规则

### MySQL 字符串转义

```
'  → \'
\  → \\
"  → \"
\n → \n
\r → \r
\t → \t
\0 → \0
```

### PostgreSQL 字符串转义

```
'  → ''（两个单引号）
\  → \\（标准模式下）
```

## 安全注意事项

1. **SQL 注入** — 所有用户输入必须转义或使用参数化查询
2. **标识符注入** — 表名/列名必须用白名单校验，不能直接拼接
3. **权限最小化** — 数据库用户只授予必要权限
4. **DROP DATABASE** — 必须在应用层拦截，不能通过 SQL 控制台执行
