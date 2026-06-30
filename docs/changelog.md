# EasyServer 更新日志

## [1.0.0] - 2026-06-15

### 新增

#### 单二进制部署
- 实现 `//go:embed` 嵌入前端资源
- 生产模式自动提供前端页面
- 开发模式使用 Vite dev server

#### 安全加固
- JWT Secret 启动时检查（禁止默认值）
- 安全头中间件（X-Frame-Options, CSP, HSTS）
- IP 白名单中间件（基于 CIDR）
- 密码强度验证
- 速率限制配置化

#### 测试
- 中间件单元测试（IP白名单、密码强度）
- 文件管理器单元测试（路径校验、文件复制）

### 修复

- monitor_linux.go 重复变量声明
- FileManager basePath 路径限制
- Copy src==dest 检查
- JWT 算法验证
- 错误格式统一
- Cloud handler nil 检查
- 终端空闲超时锁优化

### 构建

- 更新 Makefile（支持 build/build-dev）
- 更新 Dockerfile（多阶段构建）
- Go 版本要求：1.25+

---

## [0.3.0] - 2026-06-15

### 新增

- 腾讯云集成（实例/防火墙/快照）
- 部署同步（服务器/任务/版本管理）

---

## [0.2.0] - 2026-06-15

### 新增

- 系统监控（CPU/内存/磁盘/网络）
- 服务管理（systemctl）
- Web 终端（PTY）
- 文件管理（CRUD）

---

## [0.1.0] - 2026-06-15

### 新增

- 项目骨架（Go + React）
- JWT 认证系统
- 用户管理
- 基础中间件
