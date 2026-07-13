import { useState, useEffect, useRef } from 'react';
import { Card, Button, Space, Modal, Tag, Progress, message } from 'antd';
import { PlusOutlined, GlobalOutlined } from '@ant-design/icons';
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
  }, [logsData?.logs]);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;

    const pollLogs = async () => {
      if (!active || !logsVisible || !logsData?.id) return;
      try {
        const res = await api.get(`/runtime/logs/${logsData.id}`);
        if (!active) return;
        const data = res.data.data;
        setLogsData(data);
        if (data?.status === 'installing' || data?.status === 'uninstalling') {
          timer = setTimeout(pollLogs, 2000);
        }
      } catch (e: unknown) {
        if (!active) return;
        const code = (e as { code?: number })?.code;
        // Backend returns code=40400 when the row is gone. Either uninstall succeeded
        // and the record was deleted, or it was wiped out of band. Either way, stop polling.
        if (code === 40400) {
          if (logsData?.status === 'uninstalling') {
            setLogsData(prev => prev && ({
              ...prev,
              status: 'uninstalled',
              progress: 100,
              progress_step: 'done',
              logs: (prev.logs ? prev.logs + '\n' : '') + '卸载完成',
            }));
          }
          return;
        }
        // Other errors: keep polling silently (pre-existing behavior)
        timer = setTimeout(pollLogs, 2000);
      }
    };

    if (logsVisible && logsData?.id && (logsData?.status === 'installing' || logsData?.status === 'uninstalling')) {
      timer = setTimeout(pollLogs, 2000);
    }

    return () => {
      active = false;
      if (timer) clearTimeout(timer);
    };
     
  }, [logsVisible, logsData?.id, logsData?.status]);

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

  const handleSetDefault = async (name: string, version: string) => {
    try {
      await api.post('/runtime/set-default', { name, version });
      message.success('默认版本已设置');
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '设置失败'));
    }
  };

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
  // We mark installed/is_default by joining against the local environments list
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
          is_default: !!match?.is_default,
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

  const handleViewLogs = async (id: number) => {
    setLogsLoading(true);
    try {
      const res = await api.get(`/runtime/logs/${id}`);
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

  const handleViewCleanup = async (id: number) => {
    setCleanupLoading(true);
    try {
      const res = await api.get(`/runtime/cleanup/${id}`);
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
    await fetchPackages(runtime.id);
  };

  const fetchPackages = async (runtimeId: number) => {
    const runtime = environments.find(r => r.id === runtimeId);
    if (!runtime) return;
    
    const isSupported = catalog.find(c => c.lang === runtime.name)?.supports_global_pkgs;
    if (!isSupported) {
      setPackageData([]);
      return;
    }

    setPackageLoading(true);
    try {
      const res = await api.get(`/packages?runtime_id=${runtimeId}`);
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
      await api.post('/packages/install', {
        runtime_id: selectedRuntimeForPackage.id,
        name: values.name,
        version: values.version || '',
        scope: 'global',
        manager: values.manager || 'npm',
      });
      message.success(`${values.name} 安装成功`);
      await fetchPackages(selectedRuntimeForPackage.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    } finally {
      setPackageInstalling(false);
    }
  };

  const handleUninstallPackage = async (pkg: PackageInfo) => {
    if (!selectedRuntimeForPackage) return;

    try {
      await api.post('/packages/uninstall', {
        runtime_id: selectedRuntimeForPackage.id,
        name: pkg.name,
        manager: pkg.source || 'npm',
      });
      message.success(`${pkg.name} 卸载成功`);
      await fetchPackages(selectedRuntimeForPackage.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '卸载失败'));
    }
  };

  const handleUpdatePackage = async (pkg: PackageInfo) => {
    if (!selectedRuntimeForPackage) return;
    setUpdatingPackageName(pkg.name);
    try {
      await api.post('/packages/update', {
        runtime_id: selectedRuntimeForPackage.id,
        name: pkg.name,
        manager: pkg.source || 'npm',
      });
      message.success(`${pkg.name} 更新成功`);
      await fetchPackages(selectedRuntimeForPackage.id);
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
      const res = await api.get(`/packages/search?runtime_id=${selectedRuntimeForPackage.id}&q=${query}`);
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
      const res = await api.get(`/packages/versions/${packageName}?runtime_id=${selectedRuntimeForPackage.id}`);
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
          onSetDefault={handleSetDefault}
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
            || logsData?.logs?.includes('正在卸载'))
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
            {logsData.logs && (
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
                  {logsData.logs}
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

            {cleanupData.will_cleanup?.env_configs_count > 0 && (
              <div style={{ marginBottom: 16 }}>
                <strong>将清理以下环境变量：</strong>
                <ul style={{ marginTop: 8 }}>
                  {cleanupData.env_configs?.map((config) => (
                    <li key={config.id}>
                      <Tag color="orange">{config.name}</Tag> = {config.value}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {cleanupData.will_cleanup?.path_entries_count > 0 && (
              <div style={{ marginBottom: 16 }}>
                <strong>将清理以下 PATH 条目：</strong>
                <ul style={{ marginTop: 8 }}>
                  {cleanupData.path_entries?.map((entry) => (
                    <li key={entry.id}>
                      <Tag color="orange">{entry.path}</Tag>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {cleanupData.will_cleanup?.env_configs_count === 0 &&
             cleanupData.will_cleanup?.path_entries_count === 0 && (
              <p style={{ color: '#999' }}>没有关联的环境变量或 PATH 条目需要清理</p>
            )}

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
          if (selectedRuntimeForPackage) await fetchPackages(selectedRuntimeForPackage.id);
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
