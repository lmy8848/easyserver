import { useState, useEffect, useCallback } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { Input, Button, message, Spin, List, Typography, Space, Breadcrumb } from 'antd';
import {
  FileOutlined, FileImageOutlined, FilePdfOutlined, FileZipOutlined,
  FileTextOutlined, VideoCameraOutlined, AudioOutlined, DownloadOutlined,
  LockOutlined, CloudServerOutlined, WarningOutlined, FolderOutlined, EyeOutlined, HomeOutlined
} from '@ant-design/icons';
import { publicShareApi, authApi } from '../services/api';
import type { ShareInfo, ShareFileEntry } from '../types';
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
.es-share-list-item { transition: background 0.3s; border-radius: 8px; padding: 8px 12px !important; color: rgba(255,255,255,0.88); }
.es-share-list-item:hover { background: rgba(255,255,255,0.08); }
.es-share-list-item .ant-typography { color: rgba(255,255,255,0.92); }
.es-share-card .ant-breadcrumb-separator { color: rgba(255,255,255,0.4); }
.es-share-card .ant-empty-description { color: rgba(255,255,255,0.45); }
`;

function formatSize(bytes: number) {
  if (bytes === undefined || bytes === null || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function iconForFile(name: string, isDir?: boolean) {
  if (isDir) return <FolderOutlined style={{ color: '#faad14' }} />;
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

  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const turnstileEnabled = !!turnstileSiteKey;

  const [ticket, setTicket] = useState('');
  const [fileList, setFileList] = useState<ShareFileEntry[] | null>(null);
  const [loadingList, setLoadingList] = useState(false);
  const [currentSubpath, setCurrentSubpath] = useState('');

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
      const res = await publicShareApi.getShareInfo(token);
      setInfo(res.data.data || null);
    } catch (err: unknown) {
      const msg = (err && typeof err === 'object' && 'response' in err)
        ? String((err as any).response?.data?.message || '')
        : '';
      setError(msg || '分享链接不存在或已失效');
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => { fetchInfo(); }, [fetchInfo]);

  const handleAccess = async (action: 'download_all' | 'view_list', specificSubpath?: string) => {
    if (!info) return;
    if (turnstileEnabled && !turnstileToken && !ticket) {
      message.warning('请先完成人机验证');
      return;
    }
    if (info.needs_password && !password && !ticket) {
      message.warning('请输入访问密码');
      return;
    }

    let activeTicket = ticket;
    if (!activeTicket) {
      try {
        const res = await publicShareApi.getDownloadTicket(token, password);
        activeTicket = res.data.data.ticket;
        setTicket(activeTicket);
      } catch (err: unknown) {
        const msg = (err && typeof err === 'object' && 'response' in err)
          ? String((err as any).response?.data?.message || '')
          : '';
        message.error(msg || '获取下载凭证失败，可能是密码错误或次数超限');
        return;
      }
    }
    
    if (action === 'download_all' || (!info.is_dir && !specificSubpath)) {
      const sp = specificSubpath ? `&subpath=${encodeURIComponent(specificSubpath)}` : '';
      window.location.assign(`/api/shares/public/${token}/download?ticket=${encodeURIComponent(activeTicket)}${sp}`);
    } else if (action === 'view_list') {
      fetchList(activeTicket, specificSubpath || '');
    }
  };

  const fetchList = async (tkt: string, subpath: string) => {
    setLoadingList(true);
    try {
      const res = await publicShareApi.listShareFiles(token, tkt, subpath);
      // Sort: folders first, then by name
      const sorted = (res.data.data || []).sort((a, b) => {
        if (a.is_dir && !b.is_dir) return -1;
        if (!a.is_dir && b.is_dir) return 1;
        return a.name.localeCompare(b.name);
      });
      setFileList(sorted);
      setCurrentSubpath(subpath);
    } catch(err) {
      message.error('获取列表失败');
    } finally {
      setLoadingList(false);
    }
  };

  const blockReason = (): string => {
    if (error) return error;
    if (!info) return '分享链接不存在或已失效';
    if (!info.exists) return '文件不存在或已被移除';
    if (info.expired) return '分享链接已过期';
    if (info.downloads_left === 0 && !ticket) return '下载次数已达上限';
    return '';
  };
  const blocked = blockReason();

  const handleNavigatePath = (index: number) => {
    if (index === -1) {
      fetchList(ticket, '');
      return;
    }
    const parts = currentSubpath.split('/').filter(Boolean);
    const newPath = parts.slice(0, index + 1).join('/');
    fetchList(ticket, newPath);
  };

  const renderBreadcrumb = () => {
    const parts = currentSubpath ? currentSubpath.split('/').filter(Boolean) : [];
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flex: 1, minWidth: 0, height: 40, padding: '0 10px', borderRadius: 6, background: 'rgba(0,0,0,0.15)' }}>
        <Button icon={<HomeOutlined />} onClick={() => handleNavigatePath(-1)} style={{ background: 'rgba(255,255,255,0.1)', border: 'none', color: '#fff' }} />
        {parts.length > 0 && <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 15 }}>/</span>}
        {parts.length > 0 && (
          <Breadcrumb
            style={{ fontSize: 15 }}
            items={parts.map((p, i) => ({
              title: <span style={{ color: i === parts.length - 1 ? 'rgba(255,255,255,0.92)' : 'rgba(255,255,255,0.6)', cursor: 'pointer', fontSize: 15 }}>{p}</span>,
              onClick: () => handleNavigatePath(i)
            }))}
          />
        )}
      </div>
    );
  };

  return (
    <div style={{
      position: 'relative',
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'center',
      paddingTop: '8vh',
      paddingBottom: '4vh',
      overflow: 'hidden',
      background: 'linear-gradient(125deg, #0f1729 0%, #1a1f3a 35%, #2a1a4a 70%, #1a1230 100%)',
      backgroundSize: '300% 300%',
      animation: 'esShareGradient 18s ease infinite',
    }}>
      <style>{PAGE_CSS}</style>
      <div className="es-share-orb" style={{ width: 380, height: 380, top: '-100px', left: '-60px', background: '#1890ff', animation: 'esShareFloat 22s ease-in-out infinite' }} />
      <div className="es-share-orb" style={{ width: 320, height: 320, bottom: '-80px', right: '-40px', background: '#722ed1', animation: 'esShareFloat 26s ease-in-out infinite reverse' }} />

      <div className="es-share-card" style={{
        position: 'relative', zIndex: 1, width: 760, maxWidth: '94vw',
        padding: '48px 44px 36px', borderRadius: 22,
        background: 'rgba(255,255,255,0.06)', backdropFilter: 'blur(24px)', WebkitBackdropFilter: 'blur(24px)',
        border: '1px solid rgba(255,255,255,0.12)', boxShadow: '0 20px 60px rgba(0,0,0,0.45)',
        animation: 'esShareFadeUp 0.5s cubic-bezier(0.22,1,0.36,1)',
      }}>
        {/* Brand */}
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 10, fontSize: 24, fontWeight: 600, letterSpacing: 1, color: 'rgba(255,255,255,0.92)' }}>
            <CloudServerOutlined style={{ color: '#1890ff', fontSize: 26 }} />
            <span>EasyServer</span>
          </div>
          <div style={{ color: 'rgba(255,255,255,0.4)', fontWeight: 400, fontSize: 14, marginTop: 4 }}>文件分享</div>
        </div>

        {loading ? (
          <div style={{ textAlign: 'center', padding: '40px 0' }}>
            <Spin size="large" />
            <div style={{ color: 'rgba(255,255,255,0.45)', marginTop: 16 }}>加载中...</div>
          </div>
        ) : blocked ? (
          <div style={{ textAlign: 'center', padding: '24px 8px' }}>
            <div style={{ fontSize: 48, marginBottom: 12 }}>
              <WarningOutlined style={{ color: '#ff7875' }} />
            </div>
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
              <div style={{ fontSize: 40, lineHeight: 1 }}>{iconForFile(info.file_name, info.is_dir)}</div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ color: '#fff', fontWeight: 600, fontSize: 15, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {info.file_name}
                </div>
                <div style={{ color: 'rgba(255,255,255,0.45)', fontSize: 12, marginTop: 4 }}>
                  {info.is_dir ? '文件夹' : formatSize(info.file_size)}
                  {info.downloads_left >= 0 && <span style={{ marginLeft: 12 }}>剩余 {info.downloads_left} 次</span>}
                  {info.needs_password && !ticket && <span style={{ marginLeft: 12, color: '#faad14' }}><LockOutlined /> 需密码</span>}
                </div>
              </div>
            </div>

            {/* Content area: password input OR file list */}
            {!ticket ? (
              <>
                {info.needs_password && (
                  <Input.Password
                    prefix={<LockOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />}
                    placeholder="访问密码"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    onPressEnter={() => handleAccess(info.is_dir ? 'view_list' : 'download_all')}
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

                {info.is_dir ? (
                  <Space style={{ display: 'flex' }}>
                    <Button type="default" icon={<EyeOutlined />} block
                      onClick={() => handleAccess('view_list')}
                      style={{ height: 46, borderRadius: 10, fontWeight: 600, fontSize: 15, flex: 1, background: 'rgba(255,255,255,0.1)', color: '#fff', border: 'none' }}>
                      查看列表
                    </Button>
                    <Button type="primary" icon={<FileZipOutlined />} block
                      onClick={() => handleAccess('download_all')}
                      style={{ height: 46, borderRadius: 10, fontWeight: 600, fontSize: 15, flex: 1, border: 'none',
                        background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
                        boxShadow: '0 6px 20px rgba(24,144,255,0.35)' }}>
                      打包下载
                    </Button>
                  </Space>
                ) : (
                  <Button type="primary" icon={<DownloadOutlined />} block
                    onClick={() => handleAccess('download_all')}
                    style={{ height: 46, borderRadius: 10, fontWeight: 600, fontSize: 15, border: 'none',
                      background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
                      boxShadow: '0 6px 20px rgba(24,144,255,0.35)' }}>
                    下载文件
                  </Button>
                )}
              </>
            ) : (
              fileList && (
                <div style={{ marginTop: 20 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                    {renderBreadcrumb()}
                    <Button type="primary" icon={<FileZipOutlined />} onClick={() => handleAccess('download_all', currentSubpath)} style={{ flexShrink: 0, height: 40 }}>
                      打包当前目录
                    </Button>
                  </div>
                  <List
                    loading={loadingList}
                    dataSource={fileList}
                    style={{ maxHeight: 300, overflow: 'auto', background: 'rgba(0,0,0,0.15)', borderRadius: 10, padding: 8 }}
                    renderItem={(item) => (
                      <List.Item className="es-share-list-item" style={{ borderBottom: 'none', padding: '12px 16px', display: 'flex', alignItems: 'center' }}>
                        <div style={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 12 }}>
                          <span style={{ fontSize: 20 }}>{iconForFile(item.name, item.is_dir)}</span>
                          <Typography.Text style={{ color: 'rgba(255,255,255,0.92)', flex: 1 }} ellipsis={{ tooltip: item.name }}>
                            {item.name}
                          </Typography.Text>
                        </div>
                        <div style={{ marginLeft: 16, display: 'flex', alignItems: 'center', gap: 16 }}>
                          {!item.is_dir && <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: 12 }}>{formatSize(item.size)}</span>}
                          {item.is_dir ? (
                            <Space size={4}>
                              <Button type="text" style={{ color: '#52c41a' }} icon={<FileZipOutlined />} onClick={() => handleAccess('download_all', currentSubpath ? `${currentSubpath}/${item.name}` : item.name)}>
                                打包
                              </Button>
                              <Button type="text" style={{ color: '#1890ff' }} onClick={() => fetchList(ticket, currentSubpath ? `${currentSubpath}/${item.name}` : item.name)}>
                                打开
                              </Button>
                            </Space>
                          ) : (
                            <Button type="text" style={{ color: '#1890ff' }} onClick={() => handleAccess('download_all', currentSubpath ? `${currentSubpath}/${item.name}` : item.name)}>
                              下载
                            </Button>
                          )}
                        </div>
                      </List.Item>
                    )}
                  />
                </div>
              )
            )}

            <div style={{ textAlign: 'center', marginTop: 16, color: 'rgba(255,255,255,0.3)', fontSize: 12 }}>
              请在有效期内下载 · 链接失效后不可恢复
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
