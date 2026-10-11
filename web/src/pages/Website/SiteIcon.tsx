import { useState } from 'react';
import { GlobalOutlined } from '@ant-design/icons';
import type { Website } from '../../types';

// SiteIcon 依次尝试：自定义图标（面板上传）→ 站点自身 favicon → 默认图标。
// 面板为 HTTPS 时浏览器会拦截 http:// 图片，故 favicon 回退只尝试 https。
export default function SiteIcon({ site, size = 16 }: { site: Website; size?: number }) {
  const [stage, setStage] = useState(0);
  const v = encodeURIComponent(site.updated_at || '');
  if (stage === 0) {
    return (
      <img
        src={`/web-servers/${site.web_server_id}/websites/${site.id}/icon?v=${v}`}
        width={size} height={size} alt=""
        style={{ verticalAlign: -3, borderRadius: 3, objectFit: 'contain' }}
        onError={() => setStage(1)}
      />
    );
  }
  if (stage === 1) {
    return (
      <img
        src={`https://${site.domain}/favicon.ico`}
        width={size} height={size} alt=""
        style={{ verticalAlign: -3 }}
        onError={() => setStage(2)}
      />
    );
  }
  return <GlobalOutlined style={{ fontSize: size }} />;
}
