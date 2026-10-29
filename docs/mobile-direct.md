# 移动端直连通道（Mobile Direct）

## 为什么

面板与移动端目前都经 Cloudflare 代理。实测同一接口：CF 冷连接 TLS 握手 1–2.7s、
首字节 3.4–9.4s；直连源站 0.10s 握手、0.14s 首字节。移动端在弱网/重连时极易超时。

直连通道把移动端流量从 CF 摘出来，直连源站端口，同时保证"扫到端口也没有可用入口"。

## 设计

- 独立 TLS 监听（默认 `0.0.0.0:8443`），只代理 `/api/` 业务接口；`/`、面板页面、
  插件宿主 API、密码登录/校验类接口一律 404。
- 每个请求必须携带 Ed25519 设备签名：
  - 签名原文：`METHOD\nREQUEST-URI\nTIMESTAMP\nNONCE\nSHA256(body)`（hex）
  - 请求头：`X-Device-Id`、`X-Device-Timestamp`（Unix 秒）、`X-Device-Nonce`、
    `X-Device-Signature`（base64）
  - 服务端校验：时间窗 ±`clock_skew`（默认 5 分钟）、nonce 防重放、设备未吊销。
- 通过校验后，服务端为该设备铸造 15 分钟短期 JWT 并写入会话表，以 Cookie
  转发给主引擎；因此复用全部既有鉴权、审计与业务路由。设备吊销后最长 15 分钟生效。
- WebSocket（终端）经反代无法升级，移动端终端仍走 CF 通道。

设备公钥的管理接口（走正常登录态，通常经 CF）：

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/devices` | `{device_id, public_key(base64), name}` 注册/更新 |
| GET | `/api/auth/devices` | 列出本人设备（含吊销状态、最近使用时间） |
| DELETE | `/api/auth/devices/:id` | 吊销设备 |

## 配置

```toml
[mobile_direct]
enabled = true
listen = "0.0.0.0:8443"
hostname = "panel.example.com"    # 反代时发给主引擎的 Host（面板域名）
cert_file = "/opt/easyserver/certs/mobile.crt"
key_file = "/opt/easyserver/certs/mobile.key"
clock_skew = "5m"
```

`hostname` 必须是面板自己的域名（`force_domain` 开启时，Host 不符会被重定向）。

## DNS

Cloudflare 的代理开关是**按记录**的，不是按 zone：

1. 新建 `A` 记录 `m.0xdead.icu` → 源站 IP，代理状态选 **DNS only（灰云）**；
2. `panel.0xdead.icu` 保持橙云（继续走 CF）。

灰云记录会暴露源站 IP（扫描者可从 DNS 看到）。这是直连的必然代价；本通道的防护
不依赖隐藏 IP，而依赖设备签名 + 最小接口面。若连 IP 都要隐藏，需要额外的中转
VPS + WireGuard，成本更高。

## 证书

灰云记录不会得到 CF 边缘证书，源站必须自己提供覆盖 `m.0xdead.icu` 的证书：

- **推荐**：Let's Encrypt + DNS-01（certbot 的 cloudflare 插件，需要 CF API Token，
  Zone.DNS 编辑权限）。不依赖 80 端口，可自动续期。
- 备选：HTTP-01（需要该域名能直接解析到源站且 80 端口可达）。
- 不要用 Cloudflare Origin CA：它与 CF 边缘绑定，手机直连会不信任。
- 临时联调可用自签证书，但移动端必须在 App 内做证书 pinning，正式版不建议。

## 防火墙建议

- `443`：只放行 Cloudflare 网段（面板网页、外链、插件自请求都走 CF）。
- `8443`：对公网开放（移动端直连）。若只服务固定地区，可按地域收紧。
- `80`：仅在用 HTTP-01 签证书时临时开放。

## 联调与验收（2026-10-29 已通过）

- 未签名请求 → 401；签名请求 → 200（`/api/auth/me` 返回 admin）。
- 同一 nonce 重放 → 401；`/` 与 `/api/auth/login` → 404。
- 直连单请求耗时 41ms（首次含铸造会话），CF 热连接 37ms、冷连接 3–9s。

## 移动端接入（待做）

1. 首次登录（走 CF）后生成 Ed25519 密钥对，私钥存 Android Keystore，公钥
   `POST /api/auth/devices` 注册。
2. 请求层：直连地址可用时优先直连（带签名），否则回退 CF。
3. 连接探测：启动时对直连 `/api/auth/me` 做一次带签名探测，失败/超时切回 CF。
