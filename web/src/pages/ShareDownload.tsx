import { useState, useEffect, useCallback } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { Input, Button, message, Spin } from 'antd';
import {
  FileOutlined, FileImageOutlined, FilePdfOutlined, FileZipOutlined,
  FileTextOutlined, VideoCameraOutlined, AudioOutlined, DownloadOutlined,
  LockOutlined, CloudServerOutlined,
} from '@ant-design/icons';
import { publicShareApi, authApi } from '../services/api';
import type { ShareInfo } from '../types';
import Turnstile from '../components/Turnstile';

const PAGE_CSS = `
@keyframes esShareGradient {
  0% { background-position: 0% 50%; }
  50% { background-position: 100% 50%; }
  100% { background-position: 0% 50%; }
}
@keyframes esShareFadeUp {
  from { opacity: 0; transform: translateY(16px); }
  to   { opacity: 1; transform: translateY(0); }
}
@keyframes esShareFloat {
  0%, 100% { transform: translate(0, 0) scale(1); }
  50%      { transform: translate(20px, -30px) scale(1.08); }
}
.es-share-orb { position: absolute; border-radius: 50%; filter: blur(60px); opacity: 0.5; pointer-events: none; }
.es-share-card input::placeholder { color: rgba(255,255,255,0.35); }
.es-share-card .ant-input-affix-wrapper { background: rgba(255,255,255,0.08); border-color: rgba(255,255,255,0.15); }
.es-share-card .ant-input-affix-wrapper > input.ant-input { background: transparent; color: #fff; }
`;

function formatSize(bytes: number) {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Map a file extension to a representative icon.
function iconForFile(name: string) {
  const ext = name.split('.').pop()?.toLowerCase() || '';
  const color = '#1890ff';
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico', 'avif'].includes(ext))
    return <FileImageOutlined style={{ color }} />;
  if (ext === 'pdf') return <FilePdfOutlined style={{ color: '#ff4d4f' }} />;
  if (['zip', 'gz', 'tar', 'bz2', 'xz', '7z', 'rar'].includes(ext))
    return <FileZipOutlined style={{ color: '#faad14' }} />;
  if (['mp4', 'webm', 'avi', 'mov', 'mkv'].includes(ext))
    return <VideoCameraOutlined style={{ color: '#722ed1' }} />;
  if (['mp3', 'wav', 'flac', 'ogg', 'aac'].includes(ext))
    return <AudioOutlined style={{ color: '#13c2c2' }} />;
  if (['txt', 'md', 'json', 'yaml', 'yml', 'go', 'py', 'js', 'ts', 'sh', 'html', 'css'].includes(ext))
    return <FileTextOutlined style={{ color: '#52c41a' }} />;
  return <FileOutlined style={{ color }} />;
}

export default function ShareDownload() {
  const { token = '' } = useParams<{ token: string }>();
  const [searchParams] = useSearchParams();
  const [info, setInfo] = useState<ShareInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [password, setPassword] = useState(searchParams.get('password') || '');
  const [verifying, setVerifying] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const turnstileEnabled = !!turnstileSiteKey;

  // 加载 Turnstile 公开配置
  useEffect(() => {
    authApi.getTurnstileConfig().then((res) => {
      const cfg = res.data.data;
      if (cfg?.enable_public_share && cfg.site_key) {
        setTurnstileSiteKey(cfg.site_key);
      }
    }).catch(() => undefined);
  }, []);

  const fetchInfo = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const res = await publicShareApi.getInfo(token);
      setInfo(res.data.data || null);
    } catch (err: unknown) {
      const msg = (err && typeof err === 'object' && 'response' in err)
        ? String((err as { response?: { data?: { message?: string } } }).response?.data?.message || '')
        : '';
      setError(msg || '分享链接不存在或已失效');
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => { fetchInfo(); }, [fetchInfo]);

  const handleDownload = async () => {
    if (!info) return;
    // Turnstile 启用时需先通过验证(阻止机器人访问下载页,但不下传到下载端点)
    if (turnstileEnabled && !turnstileToken) {
      message.warning('请先完成人机验证');
      return;
    }
    // No password: go straight to the download endpoint.
    if (!info.needs_password) {
      window.location.href = `/share/${token}/download`;
      return;
    }
    if (!password) {
      message.warning('请输入访问密码');
      return;
    }
    // Password-protected: verify first (no download-count increment) so a wrong
    // password shows an inline error instead of a raw JSON page.
    setVerifying(true);
    try {
      await publicShareApi.verify(token, password);
      window.location.href = `/share/${token}/download?password=${encodeURIComponent(password)}`;
    } catch (err: unknown) {
      const msg = (err && typeof err === 'object' && 'response' in err)
        ? String((err as { response?: { data?: { message?: string } } }).response?.data?.message || '')
        : '';
      message.error(msg || '密码错误或链接无效');
    } finally {
      setVerifying(false);
    }
  };

  // Determine a blocking state (expired / exhausted / missing) from the info.
  const blockReason = (): string => {
    if (!info) return '';
    if (error) return error;
    if (!info.exists) return '文件不存在或已被移除';
    if (info.expired) return '分享链接已过期';
    if (info.downloads_left === 0) return '下载次数已达上限';
    return '';
  };
  const blocked = blockReason();

  return (
    <div style={{
      position: 'relative',
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      overflow: 'hidden',
      background: 'linear-gradient(125deg, #0f1729 0%, #1a1f3a 35%, #2a1a4a 70%, #1a1230 100%)',
      backgroundSize: '300% 300%',
      animation: 'esShareGradient 18s ease infinite',
    }}>
      <style>{PAGE_CSS}</style>
      <div className="es-share-orb" style={{ width: 380, height: 380, top: '-100px', left: '-60px', background: '#1890ff', animation: 'esShareFloat 22s ease-in-out infinite' }} />
      <div className="es-share-orb" style={{ width: 320, height: 320, bottom: '-80px', right: '-40px', background: '#722ed1', animation: 'esShareFloat 26s ease-in-out infinite reverse' }} />

      <div className="es-share-card" style={{
        position: 'relative', zIndex: 1, width: 420, maxWidth: '92vw',
        padding: '36px 32px 28px', borderRadius: 20,
        background: 'rgba(255,255,255,0.06)', backdropFilter: 'blur(24px)', WebkitBackdropFilter: 'blur(24px)',
        border: '1px solid rgba(255,255,255,0.12)', boxShadow: '0 20px 60px rgba(0,0,0,0.45)',
        animation: 'esShareFadeUp 0.5s cubic-bezier(0.22,1,0.36,1)',
      }}>
        {/* Brand */}
        <div style={{ textAlign: 'center', marginBottom: 24, color: 'rgba(255,255,255,0.85)' }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 15, fontWeight: 600, letterSpacing: 1 }}>
            <CloudServerOutlined style={{ color: '#1890ff' }} />
            <span>EasyServer</span>
            <span style={{ color: 'rgba(255,255,255,0.35)', fontWeight: 400, fontSize: 12 }}>文件分享</span>
          </div>
        </div>

        {loading ? (
          <div style={{ textAlign: 'center', padding: '40px 0' }}>
            <Spin size="large" />
            <div style={{ color: 'rgba(255,255,255,0.45)', marginTop: 16 }}>加载中...</div>
          </div>
        ) : blocked ? (
          <div style={{ textAlign: 'center', padding: '24px 8px' }}>
            <div style={{ fontSize: 48, marginBottom: 12 }}>⚠️</div>
            <div style={{ color: '#ff7875', fontSize: 16, fontWeight: 500, marginBottom: 8 }}>无法下载</div>
            <div style={{ color: 'rgba(255,255,255,0.5)', fontSize: 13 }}>{blocked}</div>
          </div>
        ) : info ? (
          <>
            {/* File card */}
            <div style={{
              display: 'flex', alignItems: 'center', gap: 16, padding: 16,
              background: 'rgba(255,255,255,0.04)', borderRadius: 14,
              border: '1px solid rgba(255,255,255,0.08)', marginBottom: 20,
            }}>
              <div style={{ fontSize: 40, lineHeight: 1 }}>{iconForFile(info.file_name)}</div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ color: '#fff', fontWeight: 600, fontSize: 15, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {info.file_name}
                </div>
                <div style={{ color: 'rgba(255,255,255,0.45)', fontSize: 12, marginTop: 4 }}>
                  {formatSize(info.file_size)}
                  {info.downloads_left >= 0 && <span style={{ marginLeft: 12 }}>剩余 {info.downloads_left} 次</span>}
                  {info.needs_password && <span style={{ marginLeft: 12, color: '#faad14' }}><LockOutlined /> 需密码</span>}
                </div>
              </div>
            </div>

            {/* Password input */}
            {info.needs_password && (
              <Input.Password
                prefix={<LockOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />}
                placeholder="访问密码"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onPressEnter={handleDownload}
                style={{ height: 44, borderRadius: 10, marginBottom: 16 }}
              />
            )}

            {turnstileEnabled && (
              <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'center' }}>
                <Turnstile
                  siteKey={turnstileSiteKey}
                  onVerify={(t) => setTurnstileToken(t)}
                  onExpire={() => setTurnstileToken('')}
                  theme="auto"
                />
              </div>
            )}

            <Button type="primary" icon={<DownloadOutlined />} block loading={verifying}
              onClick={handleDownload}
              style={{ height: 46, borderRadius: 10, fontWeight: 600, fontSize: 15, border: 'none',
                background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
                boxShadow: '0 6px 20px rgba(24,144,255,0.35)' }}>
              下载文件
            </Button>

            <div style={{ textAlign: 'center', marginTop: 16, color: 'rgba(255,255,255,0.3)', fontSize: 12 }}>
              请在有效期内下载 · 链接失效后不可恢复
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
