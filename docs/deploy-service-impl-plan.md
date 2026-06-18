# 部署服务（Deploy Service）实现方案

> 状态：待开发  
> 优先级：P1（高）  
> 关联审查报告：`F:\Project\memory\journal\review_20260616_iteration5.md`  
> 关联代码：`internal/service/deploy.go`、`internal/api/deploy.go`、`web/src/pages/Deploy.tsx`

---

## 一、背景与目标

### 1.1 背景
EasyServer 目前已实现部署服务的**数据骨架**：
- 可增删改查远程服务器（`DeployServer`）
- 可创建/执行部署任务（`DeployTask`）
- 可记录部署版本（`DeployVersion`）

但核心能力仍是 TODO/占位实现：SSH 连接测试、文件同步、远程命令执行、版本回滚均未真正实现；且部署凭据明文存储，存在严重安全隐患。

### 1.2 目标
将部署服务从“数据骨架”升级为**可用的远程部署自动化模块**，具备：
1. 安全的远程服务器凭据管理（加密存储）
2. 真实可靠的 SSH/SFTP 连接与文件同步
3. 远程命令执行与输出捕获
4. 版本备份与回滚能力
5. 任务执行状态实时反馈（同步阻塞执行即可，后续可扩展异步）

---

## 二、当前状态

### 2.1 已具备基础
| 项目 | 状态 | 说明 |
|------|------|------|
| 数据表结构 | ✅ | `deploy_servers` / `deploy_tasks` / `deploy_versions` |
| CRUD API | ✅ | `internal/api/deploy.go` |
| 前端页面框架 | ✅ | `web/src/pages/Deploy.tsx` |
| 路由权限 | ✅ | 仅 admin 可访问 |

### 2.2 当前缺陷
| 项目 | 状态 | 风险 |
|------|------|------|
| 凭据加密存储 | ❌ | `AuthData` 明文写入 SQLite |
| SSH 连接测试 | ❌ | 仅修改状态，未真正连接 |
| 文件同步 | ❌ | 仅返回占位字符串 |
| 远程命令执行 | ❌ | 仅返回占位字符串 |
| 回滚 | ❌ | 直接返回未实现错误 |
| 前端凭据输入 | ⚠️ | 表单缺少 `auth_data` 字段 |
| 任务并发安全 | ⚠️ | 无锁，同任务可能被重复执行 |

---

## 三、功能范围

### 3.1 第一阶段（MVP，推荐优先实现）
1. 部署服务器凭据加密存储
2. 真实 SSH 连接测试
3. 远程命令执行任务（`command`）
4. 文件同步任务（`sync`），基于 SFTP
5. 任务执行结果与日志持久化
6. 前端表单支持凭据录入

### 3.2 第二阶段（增强）
1. 部署前自动备份 + 版本记录
2. 一键回滚到历史版本
3. 任务执行锁（防止并发执行同一任务）
4. 任务执行历史/日志查询
5. 批量任务串行/并行执行

### 3.3 明确不做（本期）
- 容器化部署（Docker/K8s）
- 流水线编排（多阶段 Pipeline）
- Git 集成与构建
- 异步任务队列（可后续引入）

---

## 四、技术方案

### 4.1 依赖
新增 Go 依赖：

```bash
go get golang.org/x/crypto/ssh
go get github.com/pkg/sftp
```

### 4.2 模块划分

```
internal/deploy/
├── crypto.go          # 凭据加解密（AES-GCM）
├── ssh_client.go      # SSH 客户端封装
├── sftp_client.go     # SFTP 上传/下载
├── executor.go        # 任务执行器
├── backup.go          # 备份/回滚逻辑
└── errors.go          # 错误定义
```

原有文件职责调整：
- `internal/service/deploy.go`：保留数据层 CRUD + 业务编排
- `internal/api/deploy.go`：HTTP 接口，负责参数校验与响应
- `web/src/pages/Deploy.tsx`：前端交互

### 4.3 主密钥来源
凭据加密需要主密钥（Master Key），优先级：

1. 环境变量 `EASYSERVER_DEPLOY_MASTER_KEY`（生产推荐）
2. 配置文件 `deploy.master_key`（不推荐生产使用）
3. 若均未设置，启动时拒绝加载部署模块或强制进入只读模式

主密钥要求：32 字节以上，启动时校验长度。

---

## 五、数据模型变更

### 5.1 新增配置项
`config.yaml`：

```yaml
deploy:
  enabled: true
  master_key: ""  # 生产环境应使用 EASYSERVER_DEPLOY_MASTER_KEY 环境变量
```

### 5.2 `deploy_servers` 表结构（不变）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 自增 |
| name | TEXT | 服务器别名 |
| host | TEXT | 主机地址 |
| port | INTEGER | SSH 端口，默认 22 |
| username | TEXT | 登录用户 |
| auth_type | TEXT | `password` / `key` |
| auth_data | TEXT | **加密后的密码或私钥内容** |
| status | TEXT | online / offline / unknown |
| last_ping | TEXT | 上次连通时间 |
| created_at | TEXT | 创建时间 |

> 注意：`auth_type = key` 时，`auth_data` 存私钥**内容**（不是路径），避免文件路径管理复杂度。

### 5.3 `deploy_tasks` 表结构（不变）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 自增 |
| server_id | FK | 关联服务器 |
| name | TEXT | 任务名称 |
| type | TEXT | sync / command / rollback |
| source_path | TEXT | 本地源路径（sync） |
| dest_path | TEXT | 远程目标路径（sync） |
| command | TEXT | 远程命令（command） |
| status | TEXT | pending / running / success / failed |
| result | TEXT | 执行结果/日志 |
| created_at | TEXT | 创建时间 |

### 5.4 `deploy_versions` 表结构（建议扩展）
新增字段：
- `backup_path` 改为非空，记录远程服务器上的备份目录
- `created_by` 可选：记录执行用户

---

## 六、API 设计

### 6.1 现有 API 调整

#### POST /api/deploy/servers
创建服务器时前端需传入 `auth_data`（密码或私钥内容）。

```json
{
  "name": "prod-web-01",
  "host": "10.0.0.5",
  "port": 22,
  "username": "deploy",
  "auth_type": "password",
  "auth_data": "SuperSecretPassword"
}
```

**注意**：`auth_data` 在写入数据库前必须加密，返回给前端时**不携带** `auth_data`。

#### GET /api/deploy/servers/:id
响应中不包含 `auth_data`，其余字段正常返回。

#### PUT /api/deploy/servers/:id
- 若请求中包含 `auth_data`，则更新并重新加密
- 若请求中不包含 `auth_data`，则保留原加密值不变

### 6.2 新增/调整 API

#### POST /api/deploy/servers/:id/test
真实 SSH 连接测试，返回结果：

```json
{
  "code": 0,
  "data": {
    "status": "online",
    "message": "SSH connection succeeded",
    "latency_ms": 45
  }
}
```

#### POST /api/deploy/tasks/:id/exec
执行任务（同步阻塞模式），返回任务最终状态：

```json
{
  "code": 0,
  "data": {
    "id": 1,
    "status": "success",
    "result": "Executed command on 10.0.0.5: ls -la /opt/app\ntotal 128\n...",
    "duration_ms": 1200
  }
}
```

#### POST /api/deploy/versions/:id/rollback
回滚指定版本：

```json
{
  "code": 0,
  "data": {
    "status": "success",
    "message": "Rollback to version v20260616-120000 completed"
  }
}
```

---

## 七、关键实现细节

### 7.1 凭据加密（AES-GCM）

```go
// internal/deploy/crypto.go

// Encrypt 使用主密钥加密明文，返回 base64 编码的密文
func Encrypt(plaintext, masterKey string) (string, error)

// Decrypt 使用主密钥解密密文，返回明文
func Decrypt(ciphertext, masterKey string) (string, error)
```

要求：
- 主密钥派生：SHA-256(masterKey) 取 32 字节作为 AES 密钥
- 每次加密随机生成 12 字节 nonce，密文格式：`base64(nonce || ciphertext || tag)`

### 7.2 SSH 客户端封装

```go
// internal/deploy/ssh_client.go

type SSHConfig struct {
    Host     string
    Port     int
    Username string
    AuthType string // password / key
    AuthData string // 明文密码或私钥内容
    Timeout  time.Duration
}

func Dial(cfg SSHConfig) (*ssh.Client, error)
func TestConnection(cfg SSHConfig) (latency time.Duration, err error)
func RunCommand(client *ssh.Client, command string, timeout time.Duration) (stdout, stderr string, err error)
```

### 7.3 SFTP 文件同步

```go
// internal/deploy/sftp_client.go

type SFTPClient struct {
    client *sftp.Client
}

func NewSFTPClient(sshClient *ssh.Client) (*SFTPClient, error)
func (c *SFTPClient) UploadFile(localPath, remotePath string) error
func (c *SFTPClient) UploadDir(localDir, remoteDir string) error
func (c *SFTPClient) DownloadFile(remotePath, localPath string) error
```

`sync` 任务执行逻辑：
1. 建立 SSH + SFTP 连接
2. 若 `source_path` 是文件 → `UploadFile`
3. 若 `source_path` 是目录 → `UploadDir`（递归）
4. 校验远程目录存在，不存在则创建
5. 返回传输文件列表与字节数

### 7.4 远程命令执行

```go
func (s *DeployService) executeCommand(srv *DeployServer, task *DeployTask) (string, error) {
    cfg := buildSSHConfig(srv)
    client, err := deploy.Dial(cfg)
    if err != nil {
        return "", err
    }
    defer client.Close()

    stdout, stderr, err := deploy.RunCommand(client, task.Command, 5*time.Minute)
    result := fmt.Sprintf("STDOUT:\n%s\n\nSTDERR:\n%s", stdout, stderr)
    return result, err
}
```

### 7.5 备份与回滚

`sync` 任务执行前自动备份远程目标目录：
1. 生成备份目录：`{dest_path}.backup.{version}`
2. 若远程目标目录存在，则移动到备份目录
3. 执行文件同步
4. 成功后创建版本记录，记录 `backup_path`

回滚逻辑：
1. 查询版本记录的 `backup_path`
2. 删除当前 `dest_path`
3. 将 `backup_path` 移动回 `dest_path`

### 7.6 任务执行状态机

```
pending -> running -> success
                \\-> failed
```

执行任务前更新 `status = 'running'`；执行结束后根据结果更新 `status` 和 `result`。

---

## 八、安全设计

| 风险点 | 防护措施 |
|--------|----------|
| 凭据明文存储 | AES-GCM 加密，`auth_data` 永不返回前端 |
| 私钥泄露 | 私钥内容加密存储，不落地临时文件 |
| 命令注入 | 远程命令由管理员配置，前端/后端做基础校验（禁止 `;`、`&&`、`\|` 等多余字符可后续增强） |
| SSH 连接安全 | 支持密码和私钥两种认证，私钥推荐 ED25519/RSA 2048+ |
| 主密钥泄露 | 主密钥只读环境变量，启动时校验长度，不记录日志 |
| 任务并发 | 执行任务时加内存锁或数据库锁，防止重复执行 |
| 权限控制 | 部署模块路由已限定 admin 角色，保持不变 |

---

## 九、前端改造点

### 9.1 服务器表单
在 `Deploy.tsx` 的服务器新增/编辑弹窗中增加凭据字段：

```tsx
<Form.Item name="auth_type" label="认证方式">
  <Select>
    <Select.Option value="password">密码</Select.Option>
    <Select.Option value="key">私钥</Select.Option>
  </Select>
</Form.Item>

<Form.Item name="auth_data" label="密码 / 私钥">
  <Input.TextArea rows={authType === 'key' ? 8 : 1} />
</Form.Item>
```

### 9.2 任务执行反馈
- 执行按钮加 loading
- 执行完成后显示 `result` 内容（可折叠的日志区域）
- 失败时高亮显示 stderr

### 9.3 回滚操作
- 版本列表增加“回滚”按钮，带二次确认
- 回滚成功后刷新任务列表

---

## 十、实现阶段与任务拆分

### Phase 1：凭据安全（2-3 天）
- [ ] 新增 `internal/deploy/crypto.go`，实现 AES-GCM 加解密
- [ ] 新增 `EASYSERVER_DEPLOY_MASTER_KEY` 环境变量支持
- [ ] 修改 `DeployServer` 结构体，新增 `AuthDataEncrypted` 字段（或复用 `AuthData` 但加密存储）
- [ ] 修改 `CreateServer` / `UpdateServer`：写入前加密 `auth_data`
- [ ] 修改 `GetServer` / `ListServers`：响应中不包含 `auth_data`
- [ ] 前端表单增加 `auth_type` 和 `auth_data` 输入

### Phase 2：SSH/SFTP 基础设施（2-3 天）
- [ ] 新增 `internal/deploy/ssh_client.go`
- [ ] 新增 `internal/deploy/sftp_client.go`
- [ ] 实现 `TestConnection` 真实 SSH 连接
- [ ] 更新 `POST /deploy/servers/:id/test` 返回真实结果

### Phase 3：任务执行（3-4 天）
- [ ] 实现 `executeCommand`
- [ ] 实现 `executeSync`（SFTP 上传）
- [ ] 任务执行结果与日志写入 `result` 字段
- [ ] 增加任务执行锁（内存 `sync.Map` 或数据库状态校验）
- [ ] 前端展示执行结果/日志

### Phase 4：备份与回滚（2-3 天）
- [ ] `sync` 任务执行前自动备份远程目录
- [ ] 完善 `DeployVersion` 记录
- [ ] 实现 `RollbackVersion`
- [ ] 前端增加回滚按钮

### Phase 5：测试与文档（2-3 天）
- [ ] 单元测试：加解密、SSH 配置解析
- [ ] 集成测试：本地起 SSH 容器测试 sync/command
- [ ] 更新 `docs/api-reference.md` 中部署相关 API
- [ ] 更新 `docs/project-status.md`

**预计总工期**：11 ~ 16 天（1 人）

---

## 十一、验收标准

### 11.1 功能验收
- [ ] 可创建带密码认证的服务器，连接测试成功
- [ ] 可创建带私钥认证的服务器，连接测试成功
- [ ] 可执行 `command` 任务，真实在远程服务器运行命令并返回输出
- [ ] 可执行 `sync` 任务，真实把本地文件/目录上传到远程服务器
- [ ] `sync` 执行前自动创建备份，可在版本列表中查看
- [ ] 可执行回滚，远程目录恢复到备份状态

### 11.2 安全验收
- [ ] SQLite 中 `deploy_servers.auth_data` 字段不是明文
- [ ] `GET /deploy/servers/:id` 响应不包含凭据
- [ ] 未配置主密钥时，服务启动给出明确错误并拒绝部署功能
- [ ] 部署模块路由仍仅允许 admin 访问

### 11.3 性能验收
- [ ] 单文件 100MB 上传不 OOM
- [ ] 命令执行支持 5 分钟超时
- [ ] 同一任务并发执行被阻止

---

## 十二、风险与依赖

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 主密钥丢失 | 无法解密已有凭据 | 文档强调备份主密钥；提供重新录入凭据功能 |
| 私钥格式兼容 | 连接失败 | 支持 OpenSSH PEM/PKCS#8 格式，错误信息清晰 |
| 大文件上传超时 | 任务失败 | 支持分块上传或增加超时配置 |
| 远程命令风险 | 误操作破坏生产 | 增加命令白名单/黑名单（后续增强） |
| 依赖 golang.org/x/crypto | 版本兼容 | 使用稳定版本，go.mod 锁定 |

---

## 十三、相关文件

- 数据层：`internal/service/deploy.go`
- 接口层：`internal/api/deploy.go`
- 前端页面：`web/src/pages/Deploy.tsx`
- 路由配置：`internal/api/router.go:241-261`
- 本方案：`docs/deploy-service-impl-plan.md`

---

*本文档为待开发项，实现前建议团队就 Phase 1 的加密方案和主密钥管理策略进行确认。*
