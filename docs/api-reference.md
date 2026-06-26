# EasyServer API 接口文档

> 最后更新：2026-06-26
> 总计：337 个接口（334 HTTP + 3 WebSocket）

---

## 目录

- [健康检查](#健康检查)
- [认证](#认证)
- [会话管理](#会话管理)
- [系统监控](#系统监控)
- [服务管理](#服务管理)
- [终端](#终端)
- [系统信息](#系统信息)
- [系统进程管理](#系统进程管理)
- [通知管理](#通知管理)
- [文件管理](#文件管理)
- [Web 服务器管理](#web-服务器管理)
- [数据库管理](#数据库管理)
- [部署管理](#部署管理)
- [运行环境管理](#运行环境管理)
- [版本管理](#版本管理)
- [包管理](#包管理)
- [环境配置管理](#环境配置管理)
- [全局配置](#全局配置)
- [云服务管理](#云服务管理)
- [审计日志](#审计日志)
- [计划任务](#计划任务)
- [防火墙管理](#防火墙管理)
- [面板设置](#面板设置)
- [SSH 管理](#ssh-管理)
- [容器管理](#容器管理)
- [模板管理](#模板管理)
- [进程守护管理](#进程守护管理)
- [WebSocket](#websocket)
- [通用响应格式](#通用响应格式)
- [认证方式](#认证方式)
- [角色权限](#角色权限)
- [接口统计](#接口统计)

---

## 健康检查

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/health` | 健康检查 | 公开 |

---

## 认证

### 公开接口

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/auth/login` | 用户登录 | 公开 |
| POST | `/api/auth/logout` | 用户登出 | 公开 |
| POST | `/api/auth/verify-totp` | 验证 TOTP 验证码（2FA 第二步） | 公开 |
| POST | `/api/auth/verify-backup` | 验证备用码（2FA 第二步） | 公开 |

### 已认证接口

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/auth/me` | 获取当前用户信息 | 已认证 |
| POST | `/api/auth/change-password` | 修改密码 | 已认证 |
| POST | `/api/auth/totp/setup` | 生成 TOTP 设置信息（二维码、密钥） | 已认证 |
| POST | `/api/auth/totp/enable` | 启用 2FA | 已认证 |
| POST | `/api/auth/totp/disable` | 禁用 2FA | 已认证 |
| GET | `/api/auth/totp/status` | 获取 TOTP 启用状态 | 已认证 |

---

## 会话管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/auth/sessions` | 获取当前用户会话列表 | 已认证 |
| POST | `/api/auth/sessions/kick` | 踢出指定会话 | 已认证 |
| POST | `/api/auth/sessions/kick-all` | 踢出其他所有会话 | 已认证 |

---

## 系统监控

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/monitor/stats` | 实时监控数据 | admin |
| GET | `/api/monitor/history` | 历史监控数据 | admin |

---

## 服务管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/services` | 服务列表 | admin |
| GET | `/api/services/:name` | 服务详情 | admin |
| GET | `/api/services/:name/logs` | 服务日志 | admin |
| POST | `/api/services/:name/start` | 启动服务 | admin |
| POST | `/api/services/:name/stop` | 停止服务 | admin |
| POST | `/api/services/:name/restart` | 重启服务 | admin |
| POST | `/api/services/:name/enable` | 开机自启 | admin |
| POST | `/api/services/:name/disable` | 取消自启 | admin |

---

## 终端

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/terminal/:id` | 终端会话（HTTP） | admin |

---

## 系统信息

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/ssh-logins` | SSH 登录历史 | admin |
| GET | `/api/system/ssh-config` | SSH 配置 | admin |
| GET | `/api/system/check-port` | 检查端口占用 | admin |
| GET | `/api/system/check-ports` | 批量检查端口 | admin |

---

## 系统进程管理

### 系统概览

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/overview` | 系统概览（CPU/内存/磁盘/网络/负载） | admin |

### 系统进程

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/processes` | 系统进程列表（支持排序/搜索/限制） | admin |
| GET | `/api/system/processes/:pid` | 进程详情 | admin |

### 系统服务（systemd）

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/services` | 系统服务列表 | admin |
| POST | `/api/system/services/:name/action` | 服务操作（start/stop/restart/enable/disable） | admin |
| GET | `/api/system/services/:name/logs` | 服务日志（journalctl） | admin |

### 受保护服务

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/services/protected` | 受保护服务列表（不允许操作的服务） | admin |

### 服务白名单

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/system/services/whitelist` | 白名单列表 | admin |
| POST | `/api/system/services/whitelist` | 添加白名单 | admin |
| DELETE | `/api/system/services/whitelist/:name` | 移除白名单 | admin |

---

## 通知管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/notifications` | 通知列表（支持 ?unread=true&limit=50） | admin |
| GET | `/api/notifications/unread-count` | 未读通知计数 | admin |
| POST | `/api/notifications` | 创建通知 | admin |
| PUT | `/api/notifications/:id/read` | 标记单条已读 | admin |
| PUT | `/api/notifications/read-all` | 标记全部已读 | admin |
| DELETE | `/api/notifications/:id` | 删除通知 | admin |

---

## 文件管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/files` | 文件列表 | admin |
| GET | `/api/files/base-path` | 获取基础路径 | admin |
| GET | `/api/files/download` | 下载文件 | admin |
| GET | `/api/files/content` | 读取文件内容 | admin |
| GET | `/api/files/search` | 搜索文件名 | admin |
| GET | `/api/files/search-content` | 搜索文件内容 | admin |
| GET | `/api/files/details` | 文件详情 | admin |
| GET | `/api/files/mime-type` | 获取 MIME 类型 | admin |
| POST | `/api/files/mkdir` | 创建目录 | admin |
| POST | `/api/files/upload` | 上传文件 | admin |
| PUT | `/api/files/rename` | 重命名 | admin |
| DELETE | `/api/files` | 删除文件 | admin |
| POST | `/api/files/move` | 移动文件 | admin |
| POST | `/api/files/copy` | 复制文件 | admin |
| PUT | `/api/files/content` | 保存文件内容 | admin |
| POST | `/api/files/compress` | 压缩文件 | admin |
| POST | `/api/files/extract` | 解压文件 | admin |
| PUT | `/api/files/chmod` | 修改权限 | admin |
| PUT | `/api/files/chown` | 修改所有者 | admin |

---

## Web 服务器管理

### 工具接口

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/web-servers/project-types` | 项目类型列表 | admin |
| GET | `/api/web-servers/browse` | 目录浏览 | admin |
| GET | `/api/web-servers/validate-path` | 验证路径 | admin |

### Web 服务器

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/web-servers` | 服务器列表 | admin |
| GET | `/api/web-servers/:id` | 服务器详情 | admin |
| POST | `/api/web-servers` | 创建服务器 | admin |
| DELETE | `/api/web-servers/:id` | 删除服务器 | admin |
| POST | `/api/web-servers/:id/install` | 安装 | admin |
| POST | `/api/web-servers/:id/uninstall` | 卸载 | admin |
| POST | `/api/web-servers/:id/start` | 启动 | admin |
| POST | `/api/web-servers/:id/stop` | 停止 | admin |
| POST | `/api/web-servers/:id/restart` | 重启 | admin |
| GET | `/api/web-servers/:id/status` | 服务状态 | admin |
| POST | `/api/web-servers/:id/reload` | 重载配置 | admin |
| GET | `/api/web-servers/:id/test-config` | 测试配置 | admin |
| GET | `/api/web-servers/:id/config` | 获取配置 | admin |
| PUT | `/api/web-servers/:id/config` | 保存配置 | admin |
| GET | `/api/web-servers/:id/logs` | 服务日志 | admin |
| POST | `/api/web-servers/:id/auto-start` | 设置开机自启 | admin |
| GET | `/api/web-servers/:id/process` | 进程信息 | admin |

### 网站管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/web-servers/:id/websites` | 网站列表 | admin |
| GET | `/api/web-servers/:id/websites/:wid` | 网站详情 | admin |
| POST | `/api/web-servers/:id/websites` | 创建网站 | admin |
| PUT | `/api/web-servers/:id/websites/:wid` | 更新网站 | admin |
| DELETE | `/api/web-servers/:id/websites/:wid` | 删除网站 | admin |
| POST | `/api/web-servers/:id/websites/:wid/enable` | 启用网站 | admin |
| POST | `/api/web-servers/:id/websites/:wid/disable` | 禁用网站 | admin |
| GET | `/api/web-servers/:id/websites/:wid/logs` | 网站日志 | admin |
| POST | `/api/web-servers/:id/websites/:wid/ssl` | 申请 SSL | admin |

---

## 数据库管理

### DB Server

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/db-servers` | 数据库服务器列表 | admin |
| GET | `/api/db-servers/:id` | 服务器详情 | admin |
| GET | `/api/db-servers/:id/version-templates` | 版本模板 | admin |
| GET | `/api/db-servers/:id/versions` | 版本列表 | admin |
| POST | `/api/db-servers/:id/versions` | 安装版本 | admin |
| DELETE | `/api/db-servers/versions/:vid` | 卸载版本 | admin |
| POST | `/api/db-servers/versions/:vid/start` | 启动版本 | admin |
| POST | `/api/db-servers/versions/:vid/stop` | 停止版本 | admin |
| POST | `/api/db-servers/versions/:vid/restart` | 重启版本 | admin |
| PUT | `/api/db-servers/versions/:vid/port` | 修改端口 | admin |
| GET | `/api/db-servers/versions/:vid/logs` | 版本日志 | admin |

### 数据库操作

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/db-servers/:id/databases` | 数据库列表 | admin |
| POST | `/api/db-servers/:id/databases` | 创建数据库 | admin |
| DELETE | `/api/db-servers/:id/databases/:did` | 删除数据库 | admin |
| GET | `/api/db-servers/databases/:did/tables` | 表列表 | admin |
| GET | `/api/db-servers/databases/:did/describe` | 表结构 | admin |
| GET | `/api/db-servers/databases/:did/query` | 查询数据 | admin |
| POST | `/api/db-servers/databases/:did/execute` | 执行 SQL | admin |
| POST | `/api/db-servers/databases/:did/insert` | 插入记录 | admin |
| POST | `/api/db-servers/databases/:did/update` | 更新记录 | admin |
| POST | `/api/db-servers/databases/:did/delete` | 删除记录 | admin |
| POST | `/api/db-servers/databases/:did/tables` | 创建表 | admin |
| DELETE | `/api/db-servers/databases/:did/tables` | 删除表 | admin |

### 数据库备份

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/db-servers/databases/:did/backup` | 创建备份 | admin |
| GET | `/api/db-servers/databases/:did/backups` | 备份列表 | admin |
| GET | `/api/db-servers/backups/:bid/download` | 下载备份 | admin |
| POST | `/api/db-servers/backups/:bid/restore` | 恢复备份 | admin |
| DELETE | `/api/db-servers/backups/:bid` | 删除备份 | admin |

### 数据库用户管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/db-servers/:id/users` | 数据库用户列表 | admin |
| POST | `/api/db-servers/:id/users` | 创建用户 | admin |
| DELETE | `/api/db-servers/:id/users/:uid` | 删除用户 | admin |
| POST | `/api/db-servers/:id/users/:uid/grant` | 授权 | admin |

### 配置管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/db-servers/mysql/config` | MySQL 配置 | admin |
| POST | `/api/db-servers/mysql/config` | 保存 MySQL 配置 | admin |
| GET | `/api/db-servers/mysql/common-params` | MySQL 常用参数 | admin |
| GET | `/api/db-servers/postgresql/config` | PostgreSQL 配置 | admin |
| POST | `/api/db-servers/postgresql/config` | 保存 PostgreSQL 配置 | admin |
| GET | `/api/db-servers/postgresql/common-params` | PostgreSQL 常用参数 | admin |
| GET | `/api/db-servers/redis/config` | Redis 配置 | admin |
| POST | `/api/db-servers/redis/config` | 保存 Redis 配置 | admin |
| GET | `/api/db-servers/redis/common-params` | Redis 常用参数 | admin |

---

## 部署管理

### 服务器

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/deploy/servers` | 服务器列表 | admin |
| GET | `/api/deploy/servers/:id` | 服务器详情 | admin |
| POST | `/api/deploy/servers` | 创建服务器 | admin |
| PUT | `/api/deploy/servers/:id` | 更新服务器 | admin |
| DELETE | `/api/deploy/servers/:id` | 删除服务器 | admin |
| POST | `/api/deploy/servers/:id/test` | 测试连接 | admin |

### 任务

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/deploy/tasks` | 任务列表 | admin |
| GET | `/api/deploy/tasks/:id` | 任务详情 | admin |
| POST | `/api/deploy/tasks` | 创建任务 | admin |
| DELETE | `/api/deploy/tasks/:id` | 删除任务 | admin |
| POST | `/api/deploy/tasks/:id/exec` | 执行任务 | admin |

### 版本

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/deploy/versions` | 版本列表 | admin |
| POST | `/api/deploy/versions/:id/rollback` | 回滚版本 | admin |

---

## 运行环境管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/runtime` | 已安装环境列表 | admin |
| GET | `/api/runtime/:name` | 环境详情 | admin |
| GET | `/api/runtime/detect` | 检测已安装环境 | admin |
| GET | `/api/runtime/check-deps/:name` | 检查依赖 | admin |
| POST | `/api/runtime/install` | 安装环境 | admin |
| POST | `/api/runtime/uninstall` | 卸载环境 | admin |
| POST | `/api/runtime/set-default` | 设置默认版本 | admin |
| POST | `/api/runtime/import-detected` | 导入检测到的环境 | admin |
| GET | `/api/runtime/logs/:id` | 安装日志 | admin |
| GET | `/api/runtime/progress/:id` | 安装进度 | admin |
| GET | `/api/runtime/cleanup/:id` | 清理安装文件 | admin |

---

## 版本管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/runtime-versions/:name` | 版本列表 | admin |
| GET | `/api/runtime-versions/:name/resolve/:alias` | 解析版本别名 | admin |
| GET | `/api/runtime-versions/:name/suggestions` | 版本建议 | admin |
| POST | `/api/runtime-versions/:name/fetch` | 获取可用版本 | admin |

---

## 包管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/packages` | 已安装包列表 | admin |
| GET | `/api/packages/search` | 搜索包 | admin |
| GET | `/api/packages/versions/:name` | 包版本列表 | admin |
| GET | `/api/packages/scan/:id` | 扫描漏洞 | admin |
| POST | `/api/packages/install` | 安装包 | admin |
| POST | `/api/packages/uninstall` | 卸载包 | admin |
| POST | `/api/packages/update` | 更新包 | admin |

---

## 环境配置管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/env-config` | 环境变量列表 | admin |
| GET | `/api/env-config/:id` | 环境变量详情 | admin |
| POST | `/api/env-config` | 创建环境变量 | admin |
| PUT | `/api/env-config/:id` | 更新环境变量 | admin |
| DELETE | `/api/env-config/:id` | 删除环境变量 | admin |
| GET | `/api/env-config/path` | PATH 条目列表 | admin |
| POST | `/api/env-config/path` | 添加 PATH 条目 | admin |
| DELETE | `/api/env-config/path/:id` | 删除 PATH 条目 | admin |
| GET | `/api/env-config/script` | 生成环境脚本 | admin |

---

## 全局配置

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/global-config` | 全局配置列表 | admin |
| GET | `/api/global-config/:id` | 全局配置详情 | admin |
| POST | `/api/global-config` | 创建全局配置 | admin |
| PUT | `/api/global-config/:id` | 更新全局配置 | admin |
| DELETE | `/api/global-config/:id` | 删除全局配置 | admin |

---

## 云服务管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/cloud/instances` | 实例列表 | admin |
| GET | `/api/cloud/instances/:id` | 实例详情 | admin |
| POST | `/api/cloud/instances/:id/start` | 启动实例 | admin |
| POST | `/api/cloud/instances/:id/stop` | 停止实例 | admin |
| POST | `/api/cloud/instances/:id/restart` | 重启实例 | admin |
| GET | `/api/cloud/firewall/:id` | 防火墙规则 | admin |
| POST | `/api/cloud/firewall/:id` | 添加防火墙规则 | admin |
| DELETE | `/api/cloud/firewall/:id/:ruleId` | 删除防火墙规则 | admin |
| GET | `/api/cloud/snapshots` | 快照列表 | admin |
| POST | `/api/cloud/snapshots` | 创建快照 | admin |
| POST | `/api/cloud/snapshots/:id/apply` | 回滚快照 | admin |
| GET | `/api/cloud/monitor/:id` | 监控数据 | admin |
| GET | `/api/cloud/traffic` | 流量统计 | admin |

---

## 审计日志

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/audit-logs` | 审计日志列表 | admin |
| GET | `/api/audit-logs/stats` | 审计统计 | admin |
| GET | `/api/audit-logs/actions` | 操作类型列表 | admin |
| GET | `/api/audit-logs/export` | 导出日志 | admin |
| DELETE | `/api/audit-logs/clean` | 清理日志 | admin |
| GET | `/api/audit-logs/clean-policy` | 清理策略 | admin |
| GET | `/api/audit-logs/verify` | 验证完整性 | admin |

---

## 计划任务

### 任务管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/cron/tasks` | 任务列表 | admin |
| GET | `/api/cron/tasks/:id` | 任务详情 | admin |
| POST | `/api/cron/tasks` | 创建任务 | admin |
| PUT | `/api/cron/tasks/:id` | 更新任务 | admin |
| DELETE | `/api/cron/tasks/:id` | 删除任务 | admin |
| POST | `/api/cron/tasks/:id/enable` | 启用任务 | admin |
| POST | `/api/cron/tasks/:id/disable` | 禁用任务 | admin |
| POST | `/api/cron/tasks/:id/run` | 立即执行 | admin |
| GET | `/api/cron/tasks/:id/logs` | 执行日志 | admin |

### 脚本管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/cron/scripts` | 脚本列表 | admin |
| GET | `/api/cron/scripts/:id` | 脚本详情 | admin |
| POST | `/api/cron/scripts` | 创建脚本 | admin |
| PUT | `/api/cron/scripts/:id` | 更新脚本 | admin |
| DELETE | `/api/cron/scripts/:id` | 删除脚本 | admin |

### 文档管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/cron/docs` | 文档列表 | admin |
| GET | `/api/cron/docs/:id` | 文档详情 | admin |
| POST | `/api/cron/docs` | 创建文档 | admin |
| PUT | `/api/cron/docs/:id` | 更新文档 | admin |
| DELETE | `/api/cron/docs/:id` | 删除文档 | admin |

### 辅助接口

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/cron/presets` | Cron 预设 | admin |
| GET | `/api/cron/describe` | 表达式描述 | admin |
| GET | `/api/cron/next-runs` | 执行时间预览 | admin |

---

## 防火墙管理

### 基本操作

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/status` | 防火墙状态 | admin |
| POST | `/api/firewall/enable` | 启用防火墙 | admin |
| POST | `/api/firewall/disable` | 禁用防火墙 | admin |
| POST | `/api/firewall/default-policy` | 设置默认策略（INPUT/OUTPUT） | admin |

### 规则管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/rules` | 规则列表 | admin |
| GET | `/api/firewall/rules/:id` | 规则详情 | admin |
| POST | `/api/firewall/rules` | 创建规则 | admin |
| PUT | `/api/firewall/rules/:id` | 更新规则 | admin |
| DELETE | `/api/firewall/rules/:id` | 删除规则 | admin |
| POST | `/api/firewall/rules/:id/enable` | 启用规则 | admin |
| POST | `/api/firewall/rules/:id/disable` | 禁用规则 | admin |
| POST | `/api/firewall/rules/:id/move-up` | 规则上移 | admin |
| POST | `/api/firewall/rules/:id/move-down` | 规则下移 | admin |

### 批量操作

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/firewall/rules/bulk-enable` | 批量启用规则 | admin |
| POST | `/api/firewall/rules/bulk-disable` | 批量禁用规则 | admin |
| POST | `/api/firewall/rules/bulk-delete` | 批量删除规则 | admin |

### 导入导出

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/rules/export` | 导出规则（JSON 下载） | admin |
| POST | `/api/firewall/rules/import` | 导入规则 | admin |

### 模板

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/templates` | 获取规则模板列表 | admin |
| POST | `/api/firewall/templates/apply` | 应用规则模板 | admin |

### 系统规则

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/system-rules` | 系统规则 | admin |
| POST | `/api/firewall/system-rules/delete` | 删除系统规则 | admin |

### 日志

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/firewall/logs` | 防火墙日志 | admin |

---

## 面板设置

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/settings` | 获取设置 | admin |
| GET | `/api/settings/system` | 系统信息 | admin |
| PUT | `/api/settings/server` | 更新服务器配置 | admin |
| PUT | `/api/settings/auth` | 更新认证配置 | admin |
| PUT | `/api/settings/monitor` | 更新监控配置 | admin |
| PUT | `/api/settings/audit` | 更新审计配置 | admin |
| PUT | `/api/settings/notify` | 更新通知配置 | admin |
| POST | `/api/settings/notify/test` | 测试 Webhook | admin |
| GET | `/api/alerts/rules` | 获取告警规则 | admin |
| PUT | `/api/alerts/rules` | 更新告警规则 | admin |
| PUT | `/api/settings/cloud` | 更新腾讯云配置 | admin |
| POST | `/api/settings/cloud/test` | 测试腾讯云连接 | admin |
| POST | `/api/settings/restart` | 重启面板 | admin |

---

## SSH 管理

### SSH 配置

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/ssh/config` | 获取 SSH 配置 | admin |
| PUT | `/api/ssh/config` | 保存 SSH 配置 | admin |
| POST | `/api/ssh/config/test` | 测试 SSH 配置 | admin |
| POST | `/api/ssh/config/reload` | 重载 SSH 服务 | admin |

### SSH 会话

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/ssh/sessions` | SSH 会话列表 | admin |
| POST | `/api/ssh/sessions/:pid/kill` | 终止 SSH 会话 | admin |

### SSH 登录历史

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/ssh/logins` | SSH 登录历史 | admin |

---

## 容器管理

### Docker 管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/docker/status` | 检测 Docker 安装状态 | admin |
| POST | `/api/docker/install` | 安装 Docker | admin |
| POST | `/api/docker/start` | 启动 Docker | admin |
| POST | `/api/docker/stop` | 停止 Docker | admin |
| POST | `/api/docker/restart` | 重启 Docker | admin |
| GET | `/api/docker/info` | Docker 信息 | admin |
| POST | `/api/docker/mirror` | 配置镜像加速 | admin |

### 容器管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/containers` | 容器列表 | admin |
| GET | `/api/containers/:id` | 容器详情 | admin |
| POST | `/api/containers` | 创建容器 | admin |
| POST | `/api/containers/:id/start` | 启动容器 | admin |
| POST | `/api/containers/:id/stop` | 停止容器 | admin |
| POST | `/api/containers/:id/restart` | 重启容器 | admin |
| POST | `/api/containers/:id/pause` | 暂停容器 | admin |
| POST | `/api/containers/:id/unpause` | 恢复容器 | admin |
| DELETE | `/api/containers/:id` | 删除容器 | admin |
| GET | `/api/containers/:id/logs` | 容器日志 | admin |
| POST | `/api/containers/:id/exec` | 在容器中执行命令 | admin |
| GET | `/api/containers/:id/stats` | 容器资源统计 | admin |
| GET | `/api/containers/:id/top` | 容器进程列表 | admin |
| POST | `/api/containers/:id/copy-to` | 复制文件到容器 | admin |
| POST | `/api/containers/:id/copy-from` | 从容器复制文件 | admin |
| POST | `/api/containers/:id/rename` | 重命名容器 | admin |
| PUT | `/api/containers/:id/update` | 更新容器配置 | admin |

### 镜像管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/images` | 镜像列表 | admin |
| POST | `/api/images/pull` | 拉取镜像 | admin |
| DELETE | `/api/images/:id` | 删除镜像 | admin |

### Compose 管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/compose/projects` | Compose 项目列表 | admin |
| POST | `/api/compose/up` | 启动 Compose 项目 | admin |
| POST | `/api/compose/down` | 停止 Compose 项目 | admin |
| POST | `/api/compose/restart` | 重启 Compose 项目 | admin |
| GET | `/api/compose/logs` | Compose 日志 | admin |
| GET | `/api/compose/config` | 获取 Compose 配置 | admin |
| PUT | `/api/compose/config` | 保存 Compose 配置 | admin |

### 存储卷管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/volumes` | 存储卷列表 | admin |
| POST | `/api/volumes` | 创建存储卷 | admin |
| DELETE | `/api/volumes/:name` | 删除存储卷 | admin |

### 网络管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/networks` | 网络列表 | admin |
| POST | `/api/networks` | 创建网络 | admin |
| DELETE | `/api/networks/:id` | 删除网络 | admin |

---

## 模板管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/templates/docker-images` | Docker 镜像模板 | admin |
| GET | `/api/templates/categories` | 模板分类 | admin |
| GET | `/api/templates/scripts` | 脚本模板 | admin |

---

## 进程守护管理

### 进程 CRUD

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/processes` | 托管进程列表 | admin |
| POST | `/api/processes` | 创建托管进程 | admin |
| GET | `/api/processes/:id` | 托管进程详情 | admin |
| PUT | `/api/processes/:id` | 更新托管进程 | admin |
| DELETE | `/api/processes/:id` | 删除托管进程 | admin |

### 进程生命周期

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/processes/:id/start` | 启动进程 | admin |
| POST | `/api/processes/:id/stop` | 停止进程 | admin |
| POST | `/api/processes/:id/restart` | 重启进程 | admin |

### 日志与统计

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/processes/:id/logs` | 进程日志（支持分页 ?limit=&offset=） | admin |
| GET | `/api/processes/:id/stats` | 进程资源统计 | admin |

### 批量操作

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/processes/batch/start` | 批量启动 | admin |
| POST | `/api/processes/batch/stop` | 批量停止 | admin |
| POST | `/api/processes/batch/restart` | 批量重启 | admin |

### 进程分组

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/process-groups` | 分组列表 | admin |
| POST | `/api/process-groups` | 创建分组 | admin |
| PUT | `/api/process-groups/:id` | 更新分组 | admin |
| DELETE | `/api/process-groups/:id` | 删除分组 | admin |

### 导入导出

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/processes/export` | 导出进程配置 | admin |
| POST | `/api/processes/import` | 导入进程配置 | admin |

---

## WebSocket

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| WS | `/ws/monitor` | 监控数据推送 | admin |
| WS | `/ws/terminal/:id` | 终端会话 | admin |
| WS | `/ws/services/:name/logs` | 服务日志流 | admin |

---

## 通用响应格式

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... }
}
```

### 错误码

| code | 说明 |
|------|------|
| 0 | 成功 |
| 40000 | 参数错误 |
| 40100 | 未认证 |
| 40101 | Token 过期 |
| 40300 | 权限不足 |
| 40400 | 资源不存在 |
| 40900 | 冲突 |
| 42900 | 请求过于频繁 |
| 50000 | 服务器内部错误 |

---

## 认证方式

所有需要认证的接口通过 `Authorization` Header 传递 JWT Token：

```
Authorization: Bearer ***
```

WebSocket 通过查询参数传递：

```
ws://host/ws/monitor?token=<token>
```

---

## 角色权限

| 角色 | 说明 |
|------|------|
| admin | 管理员，拥有所有权限 |
| operator | 操作员，可操作服务/终端/文件 |
| viewer | 观察者，只读权限 |

> **注意**：后端当前使用 `RequireSingleAdmin()` 中间件，所有 protected 路由仅允许 admin 角色访问。认证相关接口（登录、登出、TOTP 验证、个人信息管理）仅需 JWT 认证，不限角色。

---

## 接口统计

| 模块 | HTTP 接口数 |
|------|------------|
| 健康检查 | 1 |
| 认证 | 13 |
| 系统监控 | 2 |
| 服务管理 | 8 |
| 终端 | 1 |
| 系统信息 | 4 |
| 系统进程管理 | 11 |
| 通知管理 | 6 |
| 文件管理 | 19 |
| Web 服务器管理 | 29 |
| 数据库管理 | 41 |
| 部署管理 | 13 |
| 运行环境管理 | 11 |
| 版本管理 | 4 |
| 包管理 | 7 |
| 环境配置管理 | 9 |
| 全局配置 | 5 |
| 云服务管理 | 13 |
| 审计日志 | 7 |
| 计划任务 | 22 |
| 防火墙管理 | 23 |
| 面板设置 | 13 |
| SSH 管理 | 7 |
| 容器管理 | 40 |
| 模板管理 | 3 |
| 进程守护管理 | 22 |
| **HTTP 合计** | **334** |
| WebSocket | 3 |
| **总计** | **337** |

---

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-06-26 | 补充：模板管理新增脚本模板接口（+1）；系统进程管理新增服务白名单添加接口（+1）；接口总数 335 → 337 |
| 2026-06-25 | 新增：系统进程管理（10 接口）、通知管理（6 接口）、进程守护管理（22 接口）；防火墙新增：规则排序、批量操作、导入导出、模板、默认策略、日志（11 接口）；接口总数 290 → 335 |
| 2026-06-21 | 初始版本，290 个接口 |
