import { Typography, Alert, Button, Space } from 'antd';
import { LinkOutlined } from '@ant-design/icons';

const { Title, Paragraph, Text } = Typography;

// 插件开发手册（面板内精简版）。
// 完整官方手册：docs/plugin-manual.md（仓库）；线上渲染：www.0xdead.icu/plugin-manual。
// 事实源契约见 internal/plugin/contract.go 与 host.go。
export default function PluginManual() {
  return (
    <div style={{ maxWidth: 860 }}>
      <Alert
        type="info" showIcon
        message="插件是独立进程（任意语言可执行文件），经 Unix socket 与面板通信：崩溃/慢/泄漏绝不拖垮主面板。启用即批准插件声明的全部权限，默认拒绝未声明能力。"
        style={{ marginBottom: 20 }}
      />

      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<LinkOutlined />}
          href="https://www.0xdead.icu/plugin-manual"
          target="_blank"
          rel="noreferrer"
        >
          查看完整开发手册（在线）
        </Button>
      </Space>

      <Title level={4}>一、目录结构与清单</Title>
      <Paragraph>
        插件是一个目录，放入 <Text code>/opt/easyserver/plugins/&lt;插件名&gt;/</Text>（或经面板「导入插件」上传 zip）。
        目录名须与 <Text code>plugin.yaml</Text> 的 <Text code>name</Text> 一致（小写字母/数字/横线）。
        <Text code>plugin.yaml</Text> 声明运行方式（exec / socket / 健康检查）、申请的权限（默认拒绝）与贡献点。
      </Paragraph>

      <Title level={4}>二、生命周期</Title>
      <Paragraph>
        <Text code>registered</Text> → 启用后 <Text code>starting</Text> → 健康检查通过 <Text code>running</Text>。
        进程崩溃自动退避重启（1s→60s，10 分钟窗口 &gt;5 次转 disabled）；面板重启时自动重放已启用插件。
        面板注入环境变量：<Text code>EASYSERVER_PLUGIN_TOKEN</Text>、<Text code>EASYSERVER_HOST_SOCKET</Text>、
        <Text code>EASYSERVER_PLUGIN_ID</Text>、<Text code>EASYSERVER_PLUGIN_SOCKET</Text>。
      </Paragraph>

      <Title level={4}>三、插件 HTTP 服务（必做）</Title>
      <Paragraph>插件必须提供 HTTP 服务（监听 <Text code>EASYSERVER_PLUGIN_SOCKET</Text> 指定的 Unix socket），至少实现：</Paragraph>
      <ul>
        <li><Text code>GET /health</Text> — 健康检查（非 5xx 即健康）</li>
        <li><Text code>POST /events</Text> — 接收面板事件（可选，订阅 eventsubs 时需要）</li>
        <li>其余业务路由自由定义，经 <Text code>/api/plugins/&lt;插件名&gt;/*</Text> 由面板反向代理给前端</li>
      </ul>

      <Title level={4}>四、贡献点（12 种）</Title>
      <Paragraph>
        功能补充型扩展点：<Text code>menus</Text>（侧边栏）、<Text code>pages</Text>（iframe 页面）、<Text code>routes</Text>（代理前缀）、
        <Text code>widgets</Text>（仪表盘小部件）、<Text code>settings</Text>（配置表单）、<Text code>services</Text>（定时任务）、
        <Text code>eventsubs</Text> / <Text code>eventpubs</Text>（事件）、<Text code>styles</Text>（前端样式注入）、
        <Text code>web_servers</Text>（Web 服务器类型）、<Text code>commands</Text>（可调用命令）、<Text code>datasources</Text>（数据源）。
        核心面板操作不开放给插件。
      </Paragraph>

      <Title level={4}>五、Host API 与权限（默认拒绝）</Title>
      <Paragraph>
        插件凭 <Text code>EASYSERVER_PLUGIN_TOKEN</Text> 调面板 host socket（<Text code>/run/easyserver/host.sock</Text>）。
        主要权限：<Text code>audit.write</Text> / <Text code>config.read</Text> / <Text code>config.readwrite</Text> /
        <Text code>log.write</Text> / <Text code>event.publish</Text> / <Text code>exec.restricted</Text>（root 执行，慎用）/
        <Text code>notify.send</Text> / <Text code>panel.api.proxy</Text>（只读面板代理）/<Text code>storage.readwrite</Text> /
        <Text code>firewall.manage</Text>（端口白名单）/<Text code>command.invoke</Text>。
        未声明能力一律 403。
      </Paragraph>

      <Title level={4}>六、Go SDK 与前端页面</Title>
      <Paragraph>
        SDK 独立模块 <Text code>sdk/plugin-sdk-go</Text> 封装 socket 服务、事件接收与 Host API 客户端。
        插件前端以 iframe 挂载，引入 <Text code>&lt;script src="/host-sdk.js"&gt;&lt;/script&gt;</Text> 经
        <Text code>window.EasyServerPlugin</Text>（request / navigate / notify）与面板通信；iframe 不持有面板 token。
      </Paragraph>

      <Title level={4}>七、打包与安装</Title>
      <Paragraph>
        打成 zip（顶层直接是 plugin.yaml）→ 面板「导入插件」上传 → 启用。或手动放入插件目录后重启面板。
      </Paragraph>
      <Paragraph style={{ marginTop: 16 }}>
        <Text type="secondary">
          完整字段、示例与安全模型见完整手册：<a href="https://www.0xdead.icu/plugin-manual" target="_blank" rel="noreferrer">www.0xdead.icu/plugin-manual</a>
        </Text>
      </Paragraph>
    </div>
  );
}
