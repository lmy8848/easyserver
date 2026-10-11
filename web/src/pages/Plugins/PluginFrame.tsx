import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { message } from 'antd';
import api from '../../services/client';

// 插件前端页面宿主：以 iframe 挂载插件页面（sandbox 隔离，插件前端不持有
// 面板 token），并作为 Host SDK 的 postMessage 代理（文档 §9.2）。
export default function PluginFrame() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  useEffect(() => {
    if (!id) return;
    const handler = (e: MessageEvent) => {
      const data = e.data as { __esplugin?: boolean; type?: string; payload?: { path?: string; title?: string; content?: string; method?: string; url?: string; body?: unknown } } | undefined;
      if (!data?.__esplugin) return;
      switch (data.type) {
        case 'navigate':
          if (data.payload?.path) navigate(data.payload.path);
          break;
        case 'notify':
          message.info(`${data.payload?.title || '插件通知'}: ${data.payload?.content || ''}`);
          break;
        case 'request':
          // Host SDK 的受限 API 代理（v1 仅支持面板同源 GET）
          if (data.payload?.method === 'GET' && typeof data.payload.url === 'string') {
            api.get(data.payload.url)
              .then((res) => e.source?.postMessage({ __esplugin_reply: true, result: res.data }, { targetOrigin: e.origin }))
              .catch((err: unknown) => e.source?.postMessage({ __esplugin_reply: true, err: String(err) }, { targetOrigin: e.origin }));
          }
          break;
        default:
      }
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [id, navigate]);

  return (
    <iframe
      src={`/api/plugins/${id}/`}
      title={`插件 ${id}`}
      style={{ width: '100%', height: 'calc(100vh - 120px)', border: 'none' }}
      sandbox="allow-scripts allow-forms allow-same-origin"
    />
  );
}
