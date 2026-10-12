# 部署流程规范（Deployment）

线上面板：`https://panel.0xdead.icu`（Cloudflare 回源）。密钥 `server.pem` 在仓库根。
服务器无法直连 GitHub，采用 **git archive → staging 构建 → 替换二进制** 的部署方式。

## 拓扑

- 面板进程 `easyserver.service`（systemd）监听 **443（TLS 直连，CF 回源）**，
  工作目录 `/opt/easyserver`，二进制 `/usr/local/bin/easyserver`，配置 `/opt/easyserver/config.toml`。
- `domain = panel.0xdead.icu`、`force_domain = true`（Host 不匹配会 301 到规范域名）。
- nginx 在 8080 是**无关残留**，面板不在其后面。
- 数据目录 `/opt/easyserver`（config/data/certs/plugins）**部署时不动**。

## 构建约束

- **前端只能在本地构建**（服务器无 node/pnpm）→ `cd web && pnpm run build`，产物 `web/dist`。
- **后端/插件在服务器构建**（项目约定后端在 Linux 构建）。服务器 GOPROXY=腾讯镜像，
  `GOTOOLCHAIN=auto` 用缓存的 go1.26.9。
- **必须用 go1.26.9**（修复 GO-2026-6617 stdlib 漏洞），显式
  `GOTOOLCHAIN=go1.26.9`，别用 go1.26.0。
- 本地 Windows 缺工具链/依赖时：`GOPROXY=https://goproxy.cn,direct` + `GOTOOLCHAIN=go1.26.0`
  可下载工具链与依赖做编译/测试验证（`go env -w GOPROXY=...` 已持久化）。

## 部署步骤

```bash
# 1. 本地：构建前端 + 打包
cd web && pnpm run build
cd .. && git archive --format=tar.gz -o /tmp/es-src.tgz master
tar -czf /tmp/es-webdist.tgz -C web/dist .

# 2. 传送到服务器，staging 组装
scp -i server.pem /tmp/es-src.tgz /tmp/es-webdist.tgz root@124.221.35.180:/tmp/
ssh -i server.pem root@124.221.35.180 '
  rm -rf /tmp/es-build && mkdir -p /tmp/es-build
  tar -xzf /tmp/es-src.tgz -C /tmp/es-build
  mkdir -p /tmp/es-build/web/dist /tmp/es-build/internal/api/web/dist
  tar -xzf /tmp/es-webdist.tgz -C /tmp/es-build/web/dist
  cp -a /tmp/es-build/web/dist/. /tmp/es-build/internal/api/web/dist/

  # 3. 构建（web/dist 内嵌进二进制）
  cd /tmp/es-build
  GOTOOLCHAIN=go1.26.9 go build -buildvcs=false \
    -ldflags "-X easyserver/internal/infra.Version=deploy-$(date +%Y%m%d)" -o easyserver ./cmd/server
  # 插件（独立 module）
  cd /tmp/es-build/plugins/tunnel-frp && GOTOOLCHAIN=go1.26.9 go build -o tunnel-frp .

  # 4. 替换（运行中的二进制不能原地覆盖，先 rm 再 cp）
  rm -f /usr/local/bin/easyserver
  cp -a /tmp/es-build/easyserver /usr/local/bin/easyserver
  PDIR=/opt/easyserver/plugins/tunnel-frp
  rm -f $PDIR/tunnel-frp
  cp -a /tmp/es-build/plugins/tunnel-frp/tunnel-frp $PDIR/tunnel-frp
  chmod +x /usr/local/bin/easyserver $PDIR/tunnel-frp

  # 5. 重启 + 验证
  systemctl restart easyserver && sleep 4
  systemctl is-active easyserver && /usr/local/bin/easyserver --version
'
```

要点：
- **先备份再替换**（`cp -a` 到 `.bak-<时间戳>`），可回滚。
- **`rm -f` + `cp`**：运行中的可执行文件 `cp` 覆盖会报 `Text file busy`。
- 只改了后端某文件时，scp 单个文件到 `/tmp/es-build` 对应路径再重建即可，不必重打全量 archive。
- 部署后 `curl https://panel.0xdead.icu/health` + 插件 `curl --unix-socket /run/easyserver/plugins/<id>.sock http://plugin/health` 验证。

## 依赖安全

- Go 依赖漏洞：`govulncheck ./...`（插件模块内单独跑）确认；主模块与插件都保持 go1.26.9 构建。
- npm 依赖漏洞：在 `web/pnpm-workspace.yaml` 的 `overrides` 钉修复版
  （pnpm v11 已不读 `package.json.pnpm` 字段！）。`brace-expansion` 因 minimatch@10 依赖其 5.x API
  **不能全局 override**，用 `pnpm update brace-expansion` 单独升 2.x。
- GitHub dependabot 告警以**版本**为准（不分析可达性），重扫是批量调度、可能滞后数小时。
