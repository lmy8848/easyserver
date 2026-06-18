# EasyServer 项目状态

> 日期：2026-06-15 | 版本：v1.0.0

---

## 当前状态

**登录功能修复中，待测试**

---

## 已完成功能 (10/10)

| 模块 | 状态 | 说明 |
|------|------|------|
| 用户认证 | ✅ | JWT + bcrypt + 锁定机制 |
| 系统监控 | ✅ | CPU/内存/磁盘/网络实时监控 |
| 服务管理 | ✅ | systemctl 启停 + 日志流 |
| Web 终端 | ✅ | PTY + xterm.js |
| 文件管理 | ✅ | CRUD + 路径安全校验 |
| 用户管理 | ✅ | RBAC 权限控制 |
| 腾讯云 | ✅ | 实例/防火墙/快照管理 |
| 部署同步 | ✅ | 框架完成 (5个TODO) |
| 安全加固 | ✅ | JWT检查/安全头/IP白名单 |
| 单二进制部署 | ✅ | //go:embed 嵌入前端 |

---

## 今日修复 (2026-06-15)

| 问题 | 状态 |
|------|------|
| /api/auth/me 路由配置 | ✅ 已修复 |
| SQLite 时间格式问题 | ✅ 已修复 |
| must_change_pass 类型转换 | ✅ 已修复 |
| 速率限制太严格 | ✅ 已修复 |
| 前端 loadUser 问题 | ✅ 已修复 |
| 纯 Go SQLite 驱动迁移 | ✅ 已完成 |

---

## 待办事项

1. 测试登录功能是否正常
2. 项目整体自动化测试
3. 测试通过后上线部署

---

## 技术栈

| 组件 | 版本 |
|------|------|
| Go | 1.25.0 |
| SQLite | modernc.org/sqlite (纯Go) |
| React | 19.x |
| Ant Design | 6.x |
| Vite | 8.x |

---

## 部署信息

| 项目 | 值 |
|------|-----|
| 服务器 IP | 124.221.35.180 |
| 端口 | 8080 |
| 部署目录 | /opt/easyserver |
| 二进制文件 | easyserver-linux-amd64 (31MB) |
| 默认账号 | admin / admin |

---

## 构建命令

```bash
# Linux 版本
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o easyserver-linux-amd64 ./cmd/server

# 前端
cd web && npm run build

# 复制前端到 embed 目录
mkdir -p internal/api/web/dist
cp -r web/dist/* internal/api/web/dist/
```

---

## 注意事项

- 不要构建 Windows 版本（用户不需要）
- 只上传二进制文件，不要上传整个压缩包
- SQLite 时间字段使用 sql.NullString 接收，手动解析
- 速率限制默认 1000 次/分钟

---

**项目状态：登录功能修复中，待测试** ⏳
