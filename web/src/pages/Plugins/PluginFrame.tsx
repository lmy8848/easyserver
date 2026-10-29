import { useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { message } from 'antd';
import api from '../../services/client';

// 插件前端页面宿主：以 iframe 挂载插件页面，并作为 Host SDK 的 postMessage 代理。
//
// 安全边界（重要）：
// 1. 只接受同源、且来自本插件 iframe 的消息。否则任意外部站点都能 window.open
//    打开插件页后，要求面板带登录态发起请求并把结果回传给攻击者。
// 2. request 代理仅放行只读白名单前缀，与后端 panel.api.proxy 的
//    panelAPIReadAllowlist（internal/plugin/host.go）保持一致，避免成为读取全部
//    GET 接口的通用数据通道。
const PANEL_API_ALLOWLIST = [
  '/api/monitor/',
  '/api/audit-logs',
  '/api/alerts/',
  '/api/security/',
];

function panelApiAllowed(path: string): boolean {
  return PANEL_API_ALLOWLIST.some((p) => path.startsWith(p));
}

export default function PluginFrame() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const frameRef = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    if (!id) return;
    const handler = (e: MessageEvent) => {
      if (e.origin !== window.location.origin) return;
      if (!frameRef.current || e.source !== frameRef.current.contentWindow) return;

      const data = e.data as { __esplugin?: boolean; type?: string; payload?: { path?: string; title?: string; content?: string; method?: string; url?: string; body?: unknown } } | undefined;
      if (!data?.__esplugin) return;
      switch (data.type) {
        case 'navigate':
          if (data.payload?.path) navigate(data.payload.path);
          break;
        case 'notify':
          message.info(`${data.payload?.title || '插件通知'}: ${data.payload?.content || ''}`);
          break;
        case 'request': {
          const url = data.payload?.url;
          if (data.payload?.method !== 'GET' || typeof url !== 'string') break;
          if (!panelApiAllowed(url)) {
            e.source?.postMessage(
              { __esplugin_reply: true, err: '该路径不在只读白名单内' },
              { targetOrigin: e.origin },
            );
            break;
          }
          api.get(url)
            .then((res) => e.source?.postMessage({ __esplugin_reply: true, result: res.data }, { targetOrigin: e.origin }))
            .catch((err: unknown) => e.source?.postMessage({ __esplugin_reply: true, err: String(err) }, { targetOrigin: e.origin }));
          break;
        }
        default:
      }
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [id, navigate]);

  return (
    <iframe
      ref={frameRef}
      src={`/api/plugins/${id}/`}
      title={`插件 ${id}`}
      style={{ width: '100%', height: 'calc(100vh - 120px)', border: 'none' }}
      // 插件页面由面板同源提供，需要 allow-same-origin 才能带登录态访问自己的
      // /api/plugins/:id/ 接口；因此"安装插件"即视为引入受信代码（管理员审批权限）。
      // 真正的源隔离需要独立源（子域），见 docs/reviews/2026-10-29-project-review.md。
      sandbox="allow-scripts allow-forms allow-same-origin"
    />
  );
}
