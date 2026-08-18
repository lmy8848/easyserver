# EasyServer 测试计划

> 日期：2026-06-16 | 版本：v1.0.0

---

## 测试目标

确保项目功能完整、稳定、安全，通过后上线部署。

---

## 测试范围

### 1. 单元测试

| 模块 | 测试文件 | 测试内容 |
|------|----------|----------|
| 认证 | auth_test.go | 登录/登出/密码修改/JWT生成验证 |
| 监控 | monitor_test.go | 数据采集/历史查询 |
| 文件管理 | filemanager_test.go | 路径校验/CRUD/复制/移动 |
| 中间件 | middleware_test.go | JWT验证/限流/IP白名单/密码强度 |

### 2. 集成测试

| 场景 | 测试步骤 | 预期结果 |
|------|----------|----------|
| 用户注册登录 | 创建用户 → 登录 → 获取用户信息 | 成功登录，返回正确用户信息 |
| 权限控制 | 不同角色访问不同接口 | 按角色返回数据或拒绝访问 |
| 监控数据流 | 启动服务 → 采集数据 → 查询历史 | 数据正确存储和返回 |
| 文件操作 | 创建/读取/更新/删除文件 | 操作成功，数据一致 |
| WebSocket | 连接监控/终端/日志WS | 连接成功，数据正常传输 |

### 3. API 测试

| API | 测试内容 | 预期结果 |
|-----|----------|----------|
| POST /api/auth/login | 登录成功/失败 | 返回token/错误信息 |
| GET /api/auth/me | 获取用户信息 | 返回当前用户信息 |
| GET /api/monitor/stats | 获取监控数据 | 返回CPU/内存/磁盘/网络数据 |
| GET /api/services | 获取服务列表 | 返回systemd服务列表 |
| GET /api/files | 浏览文件 | 返回目录内容 |
| POST /api/files/upload | 上传文件 | 文件上传成功 |
| WebSocket /ws/monitor | 实时监控 | 推送监控数据 |

### 4. 前端测试

| 页面 | 测试内容 | 预期结果 |
|------|--------------------|
| 登录页 | 输入用户名密码登录 | 登录成功，跳转主页 |
| 监控页 | 查看实时监控图表 | 图表正常显示数据 |
| 服务页 | 启停服务 | 服务状态正确更新 |
| 终端页 | 执行命令 | 命令执行，输出正确 |
| 文件页 | 浏览/编辑文件 | 操作正常 |
| 用户页 | 创建/编辑用户 | 操作成功 |

### 5. 安全测试

| 测试项 | 测试方法 | 预期结果 |
|--------|----------|----------|
| JWT 验证 | 无token/错误token访问 | 返回401 |
| 权限控制 | 低权限访问高权限接口 | 返回403 |
| 路径遍历 | 尝试访问 ../ 路径 | 拒绝访问 |
| 密码强度 | 设置弱密码 | 返回错误 |
| 限流 | 快速请求 | 返回429 |

---

## 测试工具

- Go testing 包
- curl 命令
- 浏览器开发者工具
- WebSocket 测试工具

---

## 测试步骤

### Step 1: 运行单元测试

```bash
cd F:\Project\code\EasyServer
go test ./... -v
```

### Step 2: 启动服务

```bash
./easyserver-linux-amd64 -config config.yaml -dev
```

### Step 3: API 测试

```bash
# 登录
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'

# 获取用户信息
curl -X GET http://localhost:8080/api/auth/me \
  -H "Authorization: Bearer <token>"

# 获取监控数据
curl -X GET http://localhost:8080/api/monitor/stats \
  -H "Authorization: Bearer <token>"
```

### Step 4: 前端测试

1. 访问 http://localhost:8080
2. 使用 admin/admin 登录
3. 测试各个页面功能

### Step 5: 安全测试

1. 无 token 访问受保护接口
2. 使用错误 token 访问
3. 低权限用户访问高权限接口
4. 尝试路径遍历攻击

---

## 通过标准

- ✅ 所有单元测试通过
- ✅ 所有 API 测试通过
- ✅ 前端功能正常
- ✅ 安全测试通过
- ✅ 无严重 bug

---

## 测试完成后

1. 修复发现的问题
2. 重新编译部署包
3. 部署到生产服务器
4. 配置 Nginx 和 HTTPS
5. 监控和日志

---

**测试通过后即可上线部署！** ✅
