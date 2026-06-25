# EasyServer

基于 Go + React 的 Linux 服务器管理面板。

![EasyServer 面板截图](docs/screenshot.jpg)

## 特性

- 🖥️ 系统监控（CPU、内存、磁盘、网络）
- 📦 进程管理（守护进程、自动重启、日志查看）
- 🔧 系统服务管理（systemd 集成）
- 📁 文件管理（在线编辑、上传下载）
- 🔐 用户认证（JWT + 两步验证）
- 📊 操作日志审计
- 🔔 通知告警系统

## 技术栈

**后端：**
- Go + Gin + SQLite
- WebSocket + JWT + PTY
- 自动数据库迁移

**前端：**
- React 19 + TypeScript
- Ant Design 6 + Vite
- 响应式设计

## 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+
- Linux 服务器（推荐 Ubuntu 20.04+）

### 安装

```bash
# 克隆项目
git clone https://github.com/your-username/easyserver.git
cd easyserver

# 编译后端
go build -o easyserver ./cmd/server

# 编译前端
cd web
npm install
npm run build
cd ..

# 配置
cp config.example.yaml config.yaml
# 编辑 config.yaml 设置密码和端口

# 运行
./easyserver -config config.yaml
```

### 开发模式

```bash
# 后端（开发模式，前端单独服务）
go build -tags dev -o easyserver ./cmd/server
./easyserver -config config.yaml -dev

# 前端（热更新）
cd web
npm run dev
# 访问 http://localhost:5173
```

## 项目结构

```
EasyServer/
├── cmd/
│   ├── server/          # 后端入口
│   └── cli/             # CLI 工具
├── internal/
│   ├── api/             # HTTP 处理器
│   ├── config/          # 配置管理
│   ├── database/        # 数据库连接
│   ├── middleware/       # 中间件
│   ├── model/           # 数据模型
│   ├── repository/      # 数据访问层
│   └── service/         # 业务逻辑
├── migrations/          # 数据库迁移
├── web/
│   ├── src/
│   │   ├── pages/       # 页面组件
│   │   ├── services/    # API 服务
│   │   └── types/       # TypeScript 类型
│   └── dist/            # 构建输出
└── config.yaml          # 配置文件
```

## 配置说明

配置文件 `config.yaml` 示例：

```yaml
server:
  port: 8080
  host: 0.0.0.0

database:
  path: ./data/easyserver.db

auth:
  username: admin
  password: your-secure-password
  jwt_secret: your-jwt-secret

security:
  totp_enabled: false
  session_timeout: 3600
```

## API 文档

启动服务后访问：`http://your-server:8080/api/docs`

## 部署

### 生产部署

```bash
# 编译生产版本
cd web && npm run build && cd ..
go build -o easyserver ./cmd/server

# 上传到服务器
scp easyserver root@your-server:/opt/easyserver/
scp -r web/dist root@your-server:/opt/easyserver/web/
scp config.yaml root@your-server:/opt/easyserver/

# 服务器上运行
ssh root@your-server
cd /opt/easyserver
./easyserver -config config.yaml
```

### Systemd 服务

创建 `/etc/systemd/system/easyserver.service`：

```ini
[Unit]
Description=EasyServer Management Panel
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/easyserver
ExecStart=/opt/easyserver/easyserver -config config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable easyserver
sudo systemctl start easyserver
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！