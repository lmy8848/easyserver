import { useState, useEffect, useRef } from 'react';
import { Card, Button, Space, Modal, Tag, Progress, message } from 'antd';
import { PlusOutlined, SyncOutlined } from '@ant-design/icons';
import api from '../../services/api';
import RuntimeList from './RuntimeList';
import VersionList from './VersionList';
import DetectPanel from './DetectPanel';
import PackageManager from './PackageManager';
import type {
  RuntimeEnvironment,
  DetectedRuntime,
  VersionInfo,
  PackageInfo,
  PackageSearchResult,
  LogsData,
  CleanupData,
  Dependencies,
} from './types';

export default function Runtime() {
  // --- Runtime list state ---
  const [environments, setEnvironments] = useState<RuntimeEnvironment[]>([]);
  const [loading, setLoading] = useState(false);

  // --- Polling ---
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // --- Install modal state ---
  const [installVisible, setInstallVisible] = useState(false);
  const [selectedRuntime, setSelectedRuntime] = useState<string>('');
  const [availableVersions, setAvailableVersions] = useState<VersionInfo[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [aliasSuggestions, setAliasSuggestions] = useState<string[]>([]);
  const [dependencies, setDependencies] = useState<Dependencies | null>(null);
  const [depsLoading, setDepsLoading] = useState(false);

  // --- Detect modal state ---
  const [detectVisible, setDetectVisible] = useState(false);
  const [detectedRuntimes, setDetectedRuntimes] = useState<DetectedRuntime[]>([]);

  // --- Logs modal state ---
  const [logsVisible, setLogsVisible] = useState(false);
  const [logsData, setLogsData] = useState<LogsData | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);

  // --- Cleanup modal state ---
  const [cleanupVisible, setCleanupVisible] = useState(false);
  const [cleanupData, setCleanupData] = useState<CleanupData | null>(null);
  const [cleanupLoading, setCleanupLoading] = useState(false);

  // --- Package manager state ---
  const [packageVisible, setPackageVisible] = useState(false);
  const [selectedRuntimeForPackage, setSelectedRuntimeForPackage] = useState<RuntimeEnvironment | null>(null);
  const [packageData, setPackageData] = useState<PackageInfo[]>([]);
  const [packageLoading, setPackageLoading] = useState(false);
  const [packageSearchResults, setPackageSearchResults] = useState<PackageSearchResult[]>([]);
  const [packageVersions, setPackageVersions] = useState<string[]>([]);
  const [packageVersionsLoading, setPackageVersionsLoading] = useState(false);

  // ==================== Lifecycle ====================

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
  }, []);

  const installingEnvs = environments.filter(e => e.status === 'installing');

  useEffect(() => {
    if (installingEnvs.length === 0) {
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
  }, [installingEnvs.length]);

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

  const handleUninstall = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      message.success('卸载成功');
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '卸载失败'));
    }
  };

  const handleRetry = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      await api.post('/runtime/install', { name, version });
      message.success('重新安装已开始...');
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '重试失败'));
    }
  };

  // ==================== Install modal actions ====================

  const handleInstall = async (values: { name: string; version: string }) => {
    try {
      await api.post('/runtime/install', values);
      message.success('安装已开始，请稍候...');
      setInstallVisible(false);
      setSelectedRuntime('');
      setAvailableVersions([]);
      setAliasSuggestions([]);
      setDependencies(null);
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    }
  };

  const fetchVersions = async (runtimeName: string, forceRefresh = false) => {
    setVersionsLoading(true);
    try {
      let versions: VersionInfo[] = [];

      if (!forceRefresh) {
        const cacheRes = await api.get(`/runtime-versions/${runtimeName}`);
        versions = cacheRes.data.data?.versions || [];
      }

      if (!versions || versions.length === 0 || forceRefresh) {
        const fetchRes = await api.post(`/runtime-versions/${runtimeName}/fetch`);
        versions = fetchRes.data.data?.versions || [];
      }

      setAvailableVersions(versions);
    } catch (error: unknown) {
      console.error('Failed to fetch versions:', error);
      setAvailableVersions([]);
    } finally {
      setVersionsLoading(false);
    }
  };

  const handleRuntimeChange = (value: string) => {
    setSelectedRuntime(value);
    setAvailableVersions([]);
    setAliasSuggestions([]);
    setDependencies(null);
    fetchVersions(value);
    checkDependencies(value);
    fetchAliasSuggestions(value);
  };

  const checkDependencies = async (runtimeName: string) => {
    setDepsLoading(true);
    try {
      const res = await api.get(`/runtime/check-deps/${runtimeName}`);
      setDependencies({
        installed: res.data.data?.installed || [],
        missing: res.data.data?.missing || [],
        optional: res.data.data?.optional || [],
      });
    } catch (error: unknown) {
      console.error('Failed to check dependencies:', error);
      setDependencies(null);
    } finally {
      setDepsLoading(false);
    }
  };

  const fetchAliasSuggestions = async (runtimeName: string) => {
    try {
      const res = await api.get(`/runtime-versions/${runtimeName}/suggestions`);
      setAliasSuggestions(res.data.data?.suggestions || []);
    } catch (error: unknown) {
      console.error('Failed to fetch alias suggestions:', error);
      setAliasSuggestions([]);
    }
  };

  const handleAliasClick = async (alias: string) => {
    if (!selectedRuntime) return;

    try {
      const res = await api.get(`/runtime-versions/${selectedRuntime}/resolve/${alias}`);
      const resolved = res.data.data?.resolved;
      if (resolved) {
        message.success(`别名 "${alias}" 解析为版本 ${resolved}`);
      }
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '别名解析失败'));
    }
  };

  // ==================== Detect modal actions ====================

  const handleDetect = async () => {
    try {
      const res = await api.get('/runtime/detect');
      setDetectedRuntimes(res.data.data?.detected || []);
      setDetectVisible(true);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '检测失败'));
    }
  };

  const handleImportDetected = async () => {
    try {
      const res = await api.post('/runtime/import-detected');
      const imported = res.data.data?.imported || 0;
      message.success(`成功导入 ${imported} 个环境`);
      setDetectVisible(false);
      fetchEnvironments();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '导入失败'));
    }
  };

  // ==================== Logs modal actions ====================

  const handleViewLogs = async (id: number) => {
    setLogsLoading(true);
    try {
      const res = await api.get(`/runtime/logs/${id}`);
      setLogsData(res.data.data);
      setLogsVisible(true);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取日志失败'));
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
      message.error((error instanceof Error ? error.message : '卸载失败'));
    }
  };

  // ==================== Package manager actions ====================

  const handleOpenPackageManager = async (runtime: RuntimeEnvironment) => {
    setSelectedRuntimeForPackage(runtime);
    setPackageVisible(true);
    await fetchPackages(runtime.id);
  };

  const fetchPackages = async (runtimeId: number) => {
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

  const handleScanPackages = async () => {
    if (!selectedRuntimeForPackage) return;

    setPackageLoading(true);
    try {
      const res = await api.get(`/packages/scan/${selectedRuntimeForPackage.id}`);
      const packages = res.data.data?.packages || [];
      setPackageData(packages);
      message.success(`扫描完成，发现 ${packages.length} 个包`);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '扫描失败'));
    } finally {
      setPackageLoading(false);
    }
  };

  const handleInstallPackage = async (values: { name: string; version: string }) => {
    if (!selectedRuntimeForPackage) return;

    try {
      await api.post('/packages/install', {
        runtime_id: selectedRuntimeForPackage.id,
        name: values.name,
        version: values.version || '',
        scope: 'global',
      });
      message.success(`正在安装 ${values.name}...`);
      setTimeout(() => fetchPackages(selectedRuntimeForPackage.id), 3000);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    }
  };

  const handleUninstallPackage = async (name: string) => {
    if (!selectedRuntimeForPackage) return;

    try {
      await api.post('/packages/uninstall', {
        runtime_id: selectedRuntimeForPackage.id,
        name: name,
      });
      message.success(`${name} 卸载成功`);
      fetchPackages(selectedRuntimeForPackage.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '卸载失败'));
    }
  };

  const handleUpdatePackage = async (name: string) => {
    if (!selectedRuntimeForPackage) return;

    try {
      await api.post('/packages/update', {
        runtime_id: selectedRuntimeForPackage.id,
        name: name,
      });
      message.success(`${name} 更新成功`);
      fetchPackages(selectedRuntimeForPackage.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '更新失败'));
    }
  };

  const handleSearchPackages = async (query: string) => {
    if (!selectedRuntimeForPackage || !query || query.length < 2) {
      setPackageSearchResults([]);
      return;
    }

    try {
      const res = await api.get(`/packages/search?runtime_id=${selectedRuntimeForPackage.id}&q=${query}`);
      setPackageSearchResults(res.data.data?.packages || []);
    } catch (error: unknown) {
      console.error('Search failed:', error);
      setPackageSearchResults([]);
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
            <Button icon={<SyncOutlined />} onClick={handleDetect}>
              检测环境
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
          onUninstall={handleUninstall}
          onRetry={handleRetry}
          onViewLogs={handleViewLogs}
          onViewCleanup={handleViewCleanup}
          onOpenPackageManager={handleOpenPackageManager}
        />
      </Card>

      {/* Install environment modal */}
      <VersionList
        visible={installVisible}
        onClose={() => {
          setInstallVisible(false);
          setSelectedRuntime('');
          setAvailableVersions([]);
          setAliasSuggestions([]);
          setDependencies(null);
        }}
        selectedRuntime={selectedRuntime}
        versionsLoading={versionsLoading}
        availableVersions={availableVersions}
        aliasSuggestions={aliasSuggestions}
        dependencies={dependencies}
        depsLoading={depsLoading}
        onInstall={handleInstall}
        onRuntimeChange={handleRuntimeChange}
        onRefreshVersions={fetchVersions}
        onAliasClick={handleAliasClick}
      />

      {/* Detect environment modal */}
      <DetectPanel
        visible={detectVisible}
        detectedRuntimes={detectedRuntimes}
        onClose={() => setDetectVisible(false)}
        onImport={handleImportDetected}
      />

      {/* Install logs modal */}
      <Modal
        title="安装日志"
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
                <Tag color={logsData.status === 'installed' ? 'green' : logsData.status === 'failed' ? 'red' : 'blue'}>
                  {logsData.status === 'installed' ? '已安装' : logsData.status === 'failed' ? '安装失败' : '安装中'}
                </Tag>
              </Space>
            </div>
            {logsData.progress > 0 && (
              <div style={{ marginBottom: 16 }}>
                <strong>进度:</strong>
                <Progress percent={logsData.progress} status={logsData.status === 'failed' ? 'exception' : 'active'} />
                {logsData.progress_step && <span style={{ color: '#999' }}>{logsData.progress_step}</span>}
              </div>
            )}
            {logsData.logs && (
              <div style={{ marginBottom: 16 }}>
                <strong>日志:</strong>
                <pre style={{
                  background: '#f5f5f5',
                  padding: 16,
                  borderRadius: 4,
                  maxHeight: 300,
                  overflow: 'auto',
                  fontSize: 12,
                  fontFamily: 'Consolas, Monaco, monospace',
                }}>
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

      {/* Package manager modal */}
      <PackageManager
        visible={packageVisible}
        selectedRuntime={selectedRuntimeForPackage}
        packageData={packageData}
        packageLoading={packageLoading}
        packageSearchResults={packageSearchResults}
        packageVersions={packageVersions}
        packageVersionsLoading={packageVersionsLoading}
        onClose={() => {
          setPackageVisible(false);
          setSelectedRuntimeForPackage(null);
          setPackageData([]);
          setPackageSearchResults([]);
          setPackageVersions([]);
        }}
        onScanPackages={handleScanPackages}
        onRefreshPackages={() => {
          if (selectedRuntimeForPackage) fetchPackages(selectedRuntimeForPackage.id);
        }}
        onInstallPackage={handleInstallPackage}
        onSearchPackages={handleSearchPackages}
        onSelectPackage={handleSelectPackage}
        onUpdatePackage={handleUpdatePackage}
        onUninstallPackage={handleUninstallPackage}
      />
    </div>
  );
}
