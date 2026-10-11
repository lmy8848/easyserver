import { Typography, Alert } from 'antd';

const { Title, Paragraph, Text } = Typography;

// 代码块样式
const code = (s: string) => (
  <pre style={{ background: 'rgba(0,0,0,0.04)', padding: 12, borderRadius: 6, overflowX: 'auto', fontSize: 12, lineHeight: 1.6 }}>
    <code>{s}</code>
  </pre>
);

// 插件开发手册（在线版；同内容的 Markdown 源见 gh-pages 站点）
export default function PluginManual() {
  return (
    <div style={{ maxWidth: 860 }}>
      <Alert
        type="info" showIcon
        message="插件是独立进程（任意语言可执行文件），经 Unix socket 与面板通信：崩溃/慢/泄漏绝不拖垮主面板。启用即批准插件声明的全部权限，默认拒绝未声明能力。"
        style={{ marginBottom: 20 }}
      />

      <Title level={4}>一、插件目录结构</Title>
      <Paragraph>插件是一个目录，放入面板的 <Text code>/opt/easyserver/plugins/&lt;插件名&gt;/</Text>（或通过面板「导入插件」上传 zip）。</Paragraph>
      {code(`my-plugin/            ← 目录名须与 plugin.yaml 的 name 一致（小写字母/数字/横线）
├── plugin.yaml     ← 清单（必填）
├── my-plugin       ← 可执行文件（任意语言，须可执行）
└── web/            ← 可选：插件前端页面资源（经插件自身 HTTP 服务返回）
`)}

      <Title level={4}>二、plugin.yaml 清单</Title>
      {code(`api_version: 1              # 面板支持的协议版本（当前 1）
manifest_schema_version: 1
name: my-plugin
version: 0.1.0
description: 插件说明

runtime:
  exec: my-plugin              # 可执行文件（插件目录内相对路径）
  args: []
  env: {}
  socket: my-plugin.sock       # 插件 socket 名（面板探测 /run/easyserver/plugins/ 下）
  startup_timeout: 15s
  request_timeout: 30s

health:
  path: /health                # 健康检查端点（插件 HTTP 服务提供）
  interval: 30s
  fail_threshold: 3            # 连续失败 N 次重启

permissions:                   # 申请的能力（默认拒绝，按需声明）
  - config.readwrite
  - log.write
  - notify.send
  - audit.write
  # - exec.restricted          # 等同 root 执行，慎用
  # - event.publish
  # - storage.readwrite

contributions:                 # 向面板扩展点贡献内容（全部可选）
  menus:
    - { id: main, title: 我的插件, icon: rocket, path: /plugins/my-plugin, order: 150 }
  pages:
    - { id: main, path: /plugins/my-plugin }
  routes:
    - { id: api, prefix: /api/plugins/my-plugin }
  settings:
    - { id: default, title: 设置, config_schema: { type: object, properties: {} } }
  eventsubs:
    - { event: easyserver.auth.login }
  eventpubs:
    - { event: easyserver.my-plugin.done }
`)}

      <Title level={4}>三、生命周期与状态</Title>
      <Paragraph>
        面板扫描注册 <Text code>registered</Text> → 启用后拉起进程 <Text code>starting</Text> → 健康检查通过 <Text code>running</Text>。
        进程崩溃自动退避重启（1s→60s，10 分钟窗口 &gt;5 次转 disabled）；面板重启时对已启用插件自动重放。
        面板注入环境变量：<Text code>EASYSERVER_PLUGIN_TOKEN</Text>（Host API 鉴权）、<Text code>EASYSERVER_HOST_SOCKET</Text>、
        <Text code>EASYSERVER_PLUGIN_ID</Text>、<Text code>EASYSERVER_PLUGIN_SOCKET</Text>（插件监听绝对路径）。
      </Paragraph>

      <Title level={4}>四、插件 HTTP 服务（必做）</Title>
      <Paragraph>插件必须提供 HTTP 服务（监听 <Text code>EASYSERVER_PLUGIN_SOCKET</Text> 指定的 Unix socket），至少实现：</Paragraph>
      <ul>
        <li><Text code>GET /health</Text> — 健康检查（非 5xx 即健康）</li>
        <li><Text code>POST /events</Text> — 接收面板事件（可选）</li>
        <li>其余业务路由自由定义，经 <Text code>/api/plugins/&lt;插件名&gt;/*</Text> 由面板反向代理给前端</li>
      </ul>

      <Title level={4}>五、Go SDK 快速开始</Title>
      <Paragraph>SDK 独立模块 <Text code>sdk/plugin-sdk-go</Text>（git 仓库内），封装 socket 服务、事件接收与 Host API 客户端：</Paragraph>
      {code(`package main

import (
  "net/http"
  "github.com/lmy8848/easyserver-plugin-sdk-go"
)

func main() {
  srv := sdk.NewServer(sdk.Config{SocketPath: "my-plugin.sock"})
  srv.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("hello from plugin"))
  })
  srv.OnEvent(func(ctx context.Context, e *sdk.Event) {
    // e.Name / e.Payload —— 面板事件（需幂等）
  })
  go srv.Serve()

  hc := sdk.NewHostClient()   // 读面板注入的环境变量
  hc.Audit(ctx, "插件启动")
  hc.Notify(ctx, "标题", "内容")   // 需 notify.send 权限
  hc.ConfigGet(ctx, "key")        // 需 config.readwrite
  hc.Exec(ctx, "ls", []string{"-la"}, 10)  // 需 exec.restricted，有审计+超时
  select {}
}`)}

      <Title level={4}>六、前端页面（iframe + Host SDK）</Title>
      <Paragraph>
        插件贡献的 <Text code>pages</Text> 会以 iframe 挂载在 <Text code>/plugins/&lt;插件名&gt;</Text>。插件页面在 HTML 引入
        <Text code>&lt;script src="/host-sdk.js"&gt;&lt;/script&gt;</Text>，通过 <Text code>window.EasyServerPlugin</Text> 与面板通信：
      </Paragraph>
      {code(`<script src="/host-sdk.js"></script>
<script>
  const s = window.EasyServerPlugin;
  s.request('/api/plugins/my-plugin/api/status')   // 受限 GET（带面板登录态）
    .then(res => { /* 处理 */ });
  s.navigate('/settings');                          // 面板内导航
  s.notify('标题', '内容');                         // 面板通知
</script>`)}
      <Paragraph>
        iframe 使用 <Text code>sandbox="allow-scripts allow-forms allow-same-origin"</Text>（不含 allow-top-navigation），插件前端
        <Text code>不持有</Text>面板 token——需要权限的操作经主前端代理执行。
      </Paragraph>

      <Title level={4}>七、Host API 能力清单</Title>
      <Paragraph>插件凭 <Text code>EASYSERVER_PLUGIN_TOKEN</Text> 调面板 host socket（<Text code>/run/easyserver/host.sock</Text>）：</Paragraph>
      {code(`audit.write         POST /api/host/audit
config.read         GET  /api/host/config[/:key]
config.readwrite    PUT  /api/host/config
log.write           POST /api/host/log
event.publish       POST /api/host/event/publish   （事件名须以 easyserver.<插件名>. 为域）
exec.restricted     POST /api/host/exec            （有审计+超时的 root 执行，等同 root shell）
notify.send         POST /api/host/notify
storage.readwrite   GET/PUT/DELETE /api/host/storage/:path  （插件私有文件区）`)}

      <Title level={4}>八、打包与安装</Title>
      <Paragraph>把插件目录打成 zip（顶层直接是 plugin.yaml，不要套一层目录），面板「导入插件」上传即可；或手动放入插件目录后重启面板。</Paragraph>
      {code(`cd my-plugin && zip -r ../my-plugin.zip .   # 顶层含 plugin.yaml
# 面板 → 插件管理 → 导入插件 → 选 my-plugin.zip
# 安装后点「启用」启动进程`)}
    </div>
  );
}
