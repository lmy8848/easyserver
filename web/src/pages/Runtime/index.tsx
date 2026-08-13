import { useState, useEffect, useRef } from 'react';
import { Card, Button, Space, Modal, Tag, Progress, message } from 'antd';
import { PlusOutlined, GlobalOutlined, ReloadOutlined } from '@ant-design/icons';
import api from '../../services/api';
import RuntimeList from './RuntimeList';
import VersionList from './VersionList';
import PackageManager from './PackageManager';
import PackageRegistryModal from './PackageRegistryModal';
import MirrorPanel from './MirrorPanel';
import type {
  RuntimeEnvironment,
  VersionInfo,
  PackageInfo,
  PackageSearchResult,
  LogsData,
  CleanupData,
  CatalogEntry,
} from './types';

export default function Runtime() {
  // --- Runtime list state ---
  const [environments, setEnvironments] = useState<RuntimeEnvironment[]>([]);
  const [loading, setLoading] = useState(false);

  // --- Catalog (drives the install dialog's language dropdown; loaded once) ---
  const [catalog, setCatalog] = useState<CatalogEntry[]>([]);

  // --- Mirror config modal ---
  const [mirrorVisible, setMirrorVisible] = useState(false);

  // --- Polling ---
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // --- Install modal state ---
  const [installVisible, setInstallVisible] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [selectedRuntime, setSelectedRuntime] = useState<string>('');
  const [availableVersions, setAvailableVersions] = useState<VersionInfo[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);

  // --- Logs modal state ---
  const [logsVisible, setLogsVisible] = useState(false);
  const [logsData, setLogsData] = useState<LogsData | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  // logStream 是 SSE 实时累积的日志内容（DB 的 logs 列不再存日志本体）。
  const [logStream, setLogStream] = useState('');
  const logsContainerRef = useRef<HTMLPreElement>(null);

  // Auto-scroll log <pre> to bottom when new content arrives, but only if the user
  // is already near the bottom — otherwise we'd yank them away from what they're reading.
  useEffect(() => {
    const el = logsContainerRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    if (nearBottom) {
      el.scrollTop = el.scrollHeight;
    }
  }, [logStream]);

  // SSE 日志流：打开弹窗且有绑定（lang@exact）时连接 /runtime/log/stream/:lang@exact，
  // 先回放已缓冲行再收实时行。done 帧更新状态/错误并关闭；关闭弹窗或切换目标时断开。
  useEffect(() => {
    if (!logsVisible || !logsData?.name || !logsData?.version) return;
    const es = new EventSource(`/api/runtime/log/stream/${logsData.name}@${logsData.version}`);
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'line') {
          setLogStream(prev => prev + (prev ? '\n' : '') + msg.text);
        } else if (msg.type === 'done') {
          es.close();
          // 终态成功时任务已成功即清、SSE 无日志可回放——done 带的"日志已丢失"
          // 说明对已完成操作是误导，别覆盖已成功的状态。
          if (msg.error) {
            setLogsData(prev => {
              if (!prev) return prev;
              if (prev.status === 'installed' || prev.status === 'uninstalled') return prev;
              return {
                ...prev,
                error_message: msg.error,
                status: prev.status === 'uninstalling' ? 'uninstall_failed' : 'failed',
              };
            });
          } else {
            setLogsData(prev => prev && ({
              ...prev,
              status: prev.status === 'uninstalling' ? 'uninstalled' : 'installed',
              progress: 100,
              progress_step: 'done',
            }));
          }
        }
      } catch { /* ignore malformed frames */ }
    };
    // 服务端关闭流或瞬断：关闭，让 done 状态接管 UI（EventSource 否则自动重连）。
    es.onerror = () => { es.close(); };
    return () => es.close();
  }, [logsVisible, logsData?.name, logsData?.version]);

  // --- Cleanup modal state ---
  const [cleanupVisible, setCleanupVisible] = useState(false);
  const [cleanupData, setCleanupData] = useState<CleanupData | null>(null);
  const [cleanupLoading, setCleanupLoading] = useState(false);

  // --- Package manager state ---
  const [packageVisible, setPackageVisible] = useState(false);
  const [selectedRuntimeForPackage, setSelectedRuntimeForPackage] = useState<RuntimeEnvironment | null>(null);
  const [packageData, setPackageData] = useState<PackageInfo[]>([]);
  const [packageLoading, setPackageLoading] = useState(false);
  const [packageInstalling, setPackageInstalling] = useState(false);
  const [packageSearchResults, setPackageSearchResults] = useState<PackageSearchResult[]>([]);
  const [packageSearchLoading, setPackageSearchLoading] = useState(false);
  const [packageVersions, setPackageVersions] = useState<string[]>([]);
  const [packageVersionsLoading, setPackageVersionsLoading] = useState(false);
  const [updatingPackageName, setUpdatingPackageName] = useState<string | null>(null);

  // --- Registry modal state ---
  const [registryVisible, setRegistryVisible] = useState(false);

  // ==================== Lifecycle ====================

  // showApiError pops a Modal with the backend message AND any `details` array
  // (e.g. 409 conflict's "Process: api-server" list) — message.error swallows
  // details and auto-dismisses, which AC4 says is not enough.
  const showApiError = (err: unknown, fallback: string) => {
    const e = err as { message?: string; details?: unknown };
    const details = Array.isArray(e?.details) ? (e!.details as string[]) : null;
    if (details && details.length > 0) {
      Modal.error({
        title: e?.message || fallback,
        content: (
          <ul style={{ paddingLeft: 20, margin: 0 }}>
            {details.map((d, i) => <li key={i}>{d}</li>)}
          </ul>
        ),
        width: 480,
      });
      return;
    }
    message.error(e?.message || fallback);
  };

  const fetchEnvironments = async () => {
    setLoading(true);
    try {
      const res = await api.get('/runtime');
      setEnvironments(res.data.data?.environments || []);
    } catch (error) {
      message.error('获取运行环境列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchEnvironments();
    api.get('/runtime/catalog')
      .then(res => setCatalog(res.data.data?.catalog || []))
      .catch(() => {
        setCatalog([]);
        // Without a visible failure the install dialog renders an empty
        // language dropdown and the mirror panel shows "no mirrors" —
        // both look like the backend is fine. Surface the failure.
        message.error('加载运行环境目录失败，请刷新页面或检查后端服务');
      });
  }, []);

  const inProgressEnvs = environments.filter(e => e.status === 'installing' || e.status === 'uninstalling');

  useEffect(() => {
    if (inProgressEnvs.length === 0) {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
      return;
    }

    pollIntervalRef.current = setInterval(() => {
      fetchEnvironments();
    }, 2000);

    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, [inProgressEnvs.length]);

  // ==================== Runtime list actions ====================

  const handleDeleteRecord = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      message.success('删除成功');
      fetchEnvironments();
    } catch (error: unknown) {
      showApiError(error, '删除失败');
    }
  };

  const handleRetry = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      await api.post('/runtime/install', { name, version });
      message.success('重新安装已开始...');
      fetchEnvironments();
    } catch (error: unknown) {
      showApiError(error, '重试失败');
    }
  };

  // ==================== Install modal actions ====================

  const handleInstall = async (values: { name: string; version: string }) => {
    setInstalling(true);
    try {
      await api.post('/runtime/install', values);
      message.success('安装已开始，请稍候...');
      setInstallVisible(false);
      setSelectedRuntime('');
      setAvailableVersions([]);
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    } finally {
      setInstalling(false);
    }
  };

  // fetchVersions now hits a single endpoint that calls `mise ls-remote` directly.
  // We mark installed by joining against the local environments list
  // — startsWith catches the case where remote lists "20" but local has "20.11.0".
  const fetchVersions = async (runtimeName: string) => {
    setVersionsLoading(true);
    try {
      const res = await api.get(`/runtime/${runtimeName}/remote-versions`);
      const raw: string[] = res.data.data?.versions || [];
      const localForLang = environments.filter(e => e.name === runtimeName);
      const versions: VersionInfo[] = raw.map(v => {
        const match = localForLang.find(e => e.version === v || e.version.startsWith(v + '.'));
        return {
          version: v,
          installed: !!match,
        };
      });
      setAvailableVersions(versions);
    } catch (error: unknown) {
      console.error('Failed to fetch versions:', error);
      setAvailableVersions([]);
      message.error((error instanceof Error ? error.message : '获取版本列表失败'));
    } finally {
      setVersionsLoading(false);
    }
  };

  const handleRuntimeChange = (value: string) => {
    setSelectedRuntime(value);
    setAvailableVersions([]);
    fetchVersions(value);
  };


  // ==================== Logs modal actions ====================

  const handleViewLogs = async (binding: string) => {
    setLogsLoading(true);
    setLogStream('');
    try {
      const res = await api.get(`/runtime/logs/${binding}`);
      setLogsData(res.data.data);
      setLogsVisible(true);
    } catch (error: unknown) {
      const bizCode = (error as { response?: { data?: { code?: number } } })?.response?.data?.code;
      if (bizCode === 40400) {
        fetchEnvironments();
        message.info('该记录已被移除');
      } else {
        message.error((error instanceof Error ? error.message : '获取日志失败'));
      }
    } finally {
      setLogsLoading(false);
    }
  };

  // ==================== Cleanup modal actions ====================

  const handleViewCleanup = async (binding: string) => {
    setCleanupLoading(true);
    try {
      const res = await api.get(`/runtime/cleanup/${binding}`);
      setCleanupData(res.data.data);
      setCleanupVisible(true);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取清理信息失败'));
    } finally {
      setCleanupLoading(false);
    }
  };

  const handleUninstallWithCleanup = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      message.success('卸载成功');
      setCleanupVisible(false);
      setCleanupData(null);
      fetchEnvironments();
    } catch (error: unknown) {
      setCleanupVisible(false);
      setCleanupData(null);
      fetchEnvironments();
      showApiError(error, '卸载失败');
    }
  };

  // ==================== Package manager actions ====================

  const handleOpenPackageManager = async (runtime: RuntimeEnvironment) => {
    setSelectedRuntimeForPackage(runtime);
    setPackageVisible(true);
    await fetchPackages(runtime);
  };

  const fetchPackages = async (runtime: RuntimeEnvironment) => {
    const isSupported = catalog.find(c => c.lang === runtime.name)?.supports_global_pkgs;
    if (!isSupported) {
      setPackageData([]);
      return;
    }

    setPackageLoading(true);
    try {
      const res = await api.get(`/packages?runtime=${runtime.name}@${runtime.version}`);
      setPackageData(res.data.data?.packages || []);
    } catch (error) {
      message.error('获取包列表失败');
    } finally {
      setPackageLoading(false);
    }
  };



  const handleInstallPackage = async (values: { name: string; version: string; manager?: string }) => {
    if (!selectedRuntimeForPackage) return;

    setPackageInstalling(true);
    try {
      await api.post(`/packages/install?runtime=${selectedRuntimeForPackage.name}@${selectedRuntimeForPackage.version}`, {
        name: values.name,
        version: values.version || '',
        scope: 'global',
        manager: values.manager || 'npm',
      });
      message.success(`${values.name} 安装成功`);
      await fetchPackages(selectedRuntimeForPackage);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    } finally {
      setPackageInstalling(false);
    }
  };

  const handleUninstallPackage = async (pkg: PackageInfo) => {
    if (!selectedRuntimeForPackage) return;

    try {
      await api.post(`/packages/uninstall?runtime=${selectedRuntimeForPackage.name}@${selectedRuntimeForPackage.version}`, {
        name: pkg.name,
        manager: pkg.source || 'npm',
      });
      message.success(`${pkg.name} 卸载成功`);
      await fetchPackages(selectedRuntimeForPackage);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '卸载失败'));
    }
  };

  const handleUpdatePackage = async (pkg: PackageInfo) => {
    if (!selectedRuntimeForPackage) return;
    setUpdatingPackageName(pkg.name);
    try {
      await api.post(`/packages/update?runtime=${selectedRuntimeForPackage.name}@${selectedRuntimeForPackage.version}`, {
        name: pkg.name,
        manager: pkg.source || 'npm',
      });
      message.success(`${pkg.name} 更新成功`);
      await fetchPackages(selectedRuntimeForPackage);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '更新失败'));
    } finally {
      setUpdatingPackageName(null);
    }
  };

  const handleConfigRegistry = async () => {
    if (!selectedRuntimeForPackage) return;
    setRegistryVisible(true);
  };

  const handleSearchPackages = async (query: string) => {
    if (!selectedRuntimeForPackage || !query || query.length < 2) {
      setPackageSearchResults([]);
      setPackageSearchLoading(false);
      return;
    }

    setPackageSearchLoading(true);
    try {
      const res = await api.get(`/packages/search?runtime=${selectedRuntimeForPackage.name}@${selectedRuntimeForPackage.version}&q=${query}`);
      setPackageSearchResults(res.data.data?.packages || []);
    } catch (error: unknown) {
      console.error('Search failed:', error);
      setPackageSearchResults([]);
    } finally {
      setPackageSearchLoading(false);
    }
  };

  const handleGetPackageVersions = async (packageName: string) => {
    if (!selectedRuntimeForPackage || !packageName) {
      setPackageVersions([]);
      return;
    }

    setPackageVersionsLoading(true);
    try {
      const res = await api.get(`/packages/versions/${packageName}?runtime=${selectedRuntimeForPackage.name}@${selectedRuntimeForPackage.version}`);
      setPackageVersions(res.data.data?.versions || []);
    } catch (error: unknown) {
      console.error('Get versions failed:', error);
      setPackageVersions([]);
    } finally {
      setPackageVersionsLoading(false);
    }
  };

  const handleSelectPackage = (packageName: string) => {
    setPackageSearchResults([]);
    handleGetPackageVersions(packageName);
  };

  // ==================== Render ====================

  return (
    <div>
      {/* Main runtime list card */}
      <Card
        title="运行环境管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchEnvironments} loading={loading}>
              刷新
            </Button>
            <Button icon={<GlobalOutlined />} onClick={() => setMirrorVisible(true)}>
              镜像源配置
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setInstallVisible(true)}>
              安装环境
            </Button>
          </Space>
        }
      >
        <RuntimeList
          environments={environments}
          loading={loading}
          logsLoading={logsLoading}
          cleanupLoading={cleanupLoading}
          onDeleteRecord={handleDeleteRecord}
          onRetry={handleRetry}
          onViewLogs={handleViewLogs}
          onViewCleanup={handleViewCleanup}
          onOpenPackageManager={handleOpenPackageManager}
        />
      </Card>

      <MirrorPanel
        visible={mirrorVisible}
        onClose={() => setMirrorVisible(false)}
        catalog={catalog}
      />

      {/* Install environment modal */}
      <VersionList
        visible={installVisible}
        onClose={() => {
          setInstallVisible(false);
          setSelectedRuntime('');
          setAvailableVersions([]);
        }}
        selectedRuntime={selectedRuntime}
        versionsLoading={versionsLoading}
        availableVersions={availableVersions}
        catalog={catalog}
        onInstall={handleInstall}
        installing={installing}
        onRuntimeChange={handleRuntimeChange}
        onRefreshVersions={fetchVersions}
      />

      {/* Install logs modal */}
      <Modal
        title={
          // Identify the operation: explicit status first, then fall back to log content
          // (status='failed' alone can't tell install-fail from uninstall-fail).
          (logsData?.status === 'uninstalling'
            || logsData?.status === 'uninstalled'
            || logsData?.status === 'uninstall_failed'
            || logStream.includes('正在卸载'))
            ? '卸载日志'
            : '安装日志'
        }
        open={logsVisible}
        onCancel={() => {
          setLogsVisible(false);
          setLogsData(null);
        }}
        footer={[
          <Button key="close" onClick={() => {
            setLogsVisible(false);
            setLogsData(null);
          }}>
            关闭
          </Button>,
        ]}
        width={700}
      >
        {logsData ? (
          <div>
            <div style={{ marginBottom: 16 }}>
              <Space>
                <span><strong>运行环境:</strong> {logsData.name}</span>
                <span><strong>版本:</strong> {logsData.version}</span>
                {(() => {
                  const m: Record<string, { color: string; label: string }> = {
                    installed: { color: 'green', label: '已安装' },
                    installing: { color: 'blue', label: '安装中' },
                    failed: { color: 'red', label: '安装失败' },
                    uninstalling: { color: 'orange', label: '卸载中' },
                    uninstalled: { color: 'default', label: '已卸载' },
                    uninstall_failed: { color: 'red', label: '卸载失败' },
                  };
                  const { color, label } = m[logsData.status] ?? { color: 'default', label: logsData.status };
                  return <Tag color={color}>{label}</Tag>;
                })()}
              </Space>
            </div>
            {logsData.progress > 0 && (
              <div style={{ marginBottom: 16 }}>
                <strong>进度:</strong>
                <Progress
                  percent={logsData.progress}
                  status={
                    logsData.status === 'failed' || logsData.status === 'uninstall_failed'
                      ? 'exception'
                      : logsData.status === 'installed' || logsData.status === 'uninstalled'
                        ? 'success'
                        : 'active'
                  }
                />
                {logsData.progress_step && <span style={{ color: '#999' }}>{logsData.progress_step}</span>}
              </div>
            )}
            {logStream && (
              <div style={{ marginBottom: 16 }}>
                <strong>日志:</strong>
                <pre
                  ref={logsContainerRef}
                  style={{
                    background: '#f5f5f5',
                    padding: 16,
                    borderRadius: 4,
                    maxHeight: 300,
                    overflow: 'auto',
                    fontSize: 12,
                    fontFamily: 'Consolas, Monaco, monospace',
                  }}
                >
                  {logStream}
                </pre>
              </div>
            )}
            {logsData.error_message && (
              <div>
                <strong style={{ color: '#ff4d4f' }}>错误信息:</strong>
                <div style={{ color: '#ff4d4f', marginTop: 8 }}>{logsData.error_message}</div>
              </div>
            )}
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: 20 }}>加载中...</div>
        )}
      </Modal>

      {/* Cleanup confirmation modal */}
      <Modal
        title="卸载确认"
        open={cleanupVisible}
        onCancel={() => {
          setCleanupVisible(false);
          setCleanupData(null);
        }}
        footer={[
          <Button key="cancel" onClick={() => {
            setCleanupVisible(false);
            setCleanupData(null);
          }}>
            取消
          </Button>,
          <Button
            key="uninstall"
            type="primary"
            danger
            onClick={() => {
              if (cleanupData?.runtime) {
                handleUninstallWithCleanup(
                  cleanupData.runtime.name,
                  cleanupData.runtime.version
                );
              }
            }}
          >
            确认卸载
          </Button>,
        ]}
        width={600}
      >
        {cleanupData ? (
          <div>
            <div style={{ marginBottom: 16 }}>
              <p>即将卸载以下运行环境：</p>
              <Tag color="blue" style={{ fontSize: 14, padding: '4px 12px' }}>
                {cleanupData.runtime?.name} {cleanupData.runtime?.version}
              </Tag>
            </div>

            <div style={{ marginTop: 16, padding: 12, background: '#fff7e6', borderRadius: 4 }}>
              <strong style={{ color: '#fa8c16' }}>注意：</strong>
              <span> 此操作将删除运行环境及其关联的配置，卸载后需要重新安装。</span>
            </div>
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: 20 }}>加载中...</div>
        )}
      </Modal>

      {/* Package manager registry modal */}
      <PackageRegistryModal
        visible={registryVisible}
        runtime={selectedRuntimeForPackage}
        onClose={() => setRegistryVisible(false)}
      />

      {/* Package manager modal */}
      <PackageManager
        catalog={catalog}
        visible={packageVisible}
        selectedRuntime={selectedRuntimeForPackage}
        packageData={packageData}
        packageLoading={packageLoading}
        packageInstalling={packageInstalling}
        packageSearchResults={packageSearchResults}
        packageSearchLoading={packageSearchLoading}
        packageVersions={packageVersions}
        packageVersionsLoading={packageVersionsLoading}
        updatingPackageName={updatingPackageName}
        onClose={() => {
          setPackageVisible(false);
          setSelectedRuntimeForPackage(null);
          setPackageData([]);
          setPackageSearchResults([]);
          setPackageVersions([]);
        }}
        onRefreshPackages={async () => {
          if (selectedRuntimeForPackage) await fetchPackages(selectedRuntimeForPackage);
        }}
        onConfigRegistry={handleConfigRegistry}
        onInstallPackage={handleInstallPackage}
        onSearchPackages={handleSearchPackages}
        onSelectPackage={handleSelectPackage}
        onUpdatePackage={handleUpdatePackage}
        onUninstallPackage={handleUninstallPackage}
      />
    </div>
  );
}
