import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Popconfirm, Typography, Progress } from 'antd';
import { PlusOutlined, DeleteOutlined, CheckCircleOutlined, SyncOutlined, ReloadOutlined, AppstoreOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';

const { Title } = Typography;

interface RuntimeEnvironment {
  id: number;
  name: string;
  version: string;
  path: string;
  is_default: boolean;
  status: string;
  progress: number;
  progress_step: string;
  error_message: string;
  installed_at: string;
}

interface DetectedRuntime {
  name: string;
  versions: string[];
}

interface VersionInfo {
  version: string;
  lts: boolean;
  stable: boolean;
  installed: boolean;
  is_default: boolean;
}

export default function Runtime() {
  const [environments, setEnvironments] = useState<RuntimeEnvironment[]>([]);
  const [loading, setLoading] = useState(false);
  const [installVisible, setInstallVisible] = useState(false);
  const [detectVisible, setDetectVisible] = useState(false);
  const [logsVisible, setLogsVisible] = useState(false);
  const [cleanupVisible, setCleanupVisible] = useState(false);
  const [packageVisible, setPackageVisible] = useState(false);
  const [logsData, setLogsData] = useState<any>(null);
  const [cleanupData, setCleanupData] = useState<any>(null);
  const [packageData, setPackageData] = useState<any[]>([]);
  const [packageLoading, setPackageLoading] = useState(false);
  const [selectedRuntimeForPackage, setSelectedRuntimeForPackage] = useState<any>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  const [cleanupLoading, setCleanupLoading] = useState(false);
  const [detectedRuntimes, setDetectedRuntimes] = useState<DetectedRuntime[]>([]);
  const [availableVersions, setAvailableVersions] = useState<VersionInfo[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [selectedRuntime, setSelectedRuntime] = useState<string>('');
  const [aliasSuggestions, setAliasSuggestions] = useState<string[]>([]);
  const [dependencies, setDependencies] = useState<{ installed: string[]; missing: string[]; optional: string[] } | null>(null);
  const [depsLoading, setDepsLoading] = useState(false);
  const [form] = Form.useForm();
  const [packageForm] = Form.useForm();
  const [packageSearchResults, setPackageSearchResults] = useState<any[]>([]);
  const [packageVersions, setPackageVersions] = useState<string[]>([]);
  const [packageVersionsLoading, setPackageVersionsLoading] = useState(false);

  useEffect(() => {
    fetchEnvironments();
  }, []);

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

  const handleInstall = async (values: { name: string; version: string }) => {
    try {
      await api.post('/runtime/install', values);
      message.success('安装已开始，请稍候...');
      setInstallVisible(false);
      form.resetFields();
      // Refresh immediately to show installing status
      fetchEnvironments();
    } catch (error: any) {
      message.error(error.message || '安装失败');
    }
  };

  // Poll for progress when there are installing environments
  useEffect(() => {
    const installingEnvs = environments.filter(e => e.status === 'installing');
    if (installingEnvs.length === 0) return;

    const interval = setInterval(() => {
      fetchEnvironments();
    }, 2000); // Poll every 2 seconds

    return () => clearInterval(interval);
  }, [environments]);

  const handleUninstall = async (name: string, version: string) => {
    try {
      await api.post('/runtime/uninstall', { name, version });
      message.success('卸载成功');
      fetchEnvironments();
    } catch (error: any) {
      message.error(error.message || '卸载失败');
    }
  };

  const handleSetDefault = async (name: string, version: string) => {
    try {
      await api.post('/runtime/set-default', { name, version });
      message.success('默认版本已设置');
      fetchEnvironments();
    } catch (error: any) {
      message.error(error.message || '设置失败');
    }
  };

  const handleRetry = async (name: string, version: string) => {
    try {
      // First uninstall the failed installation
      await api.post('/runtime/uninstall', { name, version });
      // Then reinstall
      await api.post('/runtime/install', { name, version });
      message.success('重新安装已开始...');
      fetchEnvironments();
    } catch (error: any) {
      message.error(error.message || '重试失败');
    }
  };

  const handleDetect = async () => {
    try {
      const res = await api.get('/runtime/detect');
      setDetectedRuntimes(res.data.data?.detected || []);
      setDetectVisible(true);
    } catch (error: any) {
      message.error(error.message || '检测失败');
    }
  };

  const handleViewLogs = async (id: number) => {
    setLogsLoading(true);
    try {
      const res = await api.get(`/runtime/logs/${id}`);
      setLogsData(res.data.data);
      setLogsVisible(true);
    } catch (error: any) {
      message.error(error.message || '获取日志失败');
    } finally {
      setLogsLoading(false);
    }
  };

  const handleViewCleanup = async (id: number) => {
    setCleanupLoading(true);
    try {
      const res = await api.get(`/runtime/cleanup/${id}`);
      setCleanupData(res.data.data);
      setCleanupVisible(true);
    } catch (error: any) {
      message.error(error.message || '获取清理信息失败');
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
    } catch (error: any) {
      message.error(error.message || '卸载失败');
    }
  };

  const handleOpenPackageManager = async (runtime: any) => {
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
    } catch (error: any) {
      message.error(error.message || '扫描失败');
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
      packageForm.resetFields();
      setTimeout(() => fetchPackages(selectedRuntimeForPackage.id), 3000);
    } catch (error: any) {
      message.error(error.message || '安装失败');
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
    } catch (error: any) {
      message.error(error.message || '卸载失败');
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
    } catch (error: any) {
      message.error(error.message || '更新失败');
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
    } catch (error: any) {
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
    } catch (error: any) {
      console.error('Get versions failed:', error);
      setPackageVersions([]);
    } finally {
      setPackageVersionsLoading(false);
    }
  };

  const handleSelectPackage = (packageName: string) => {
    packageForm.setFieldsValue({ name: packageName });
    setPackageSearchResults([]);
    handleGetPackageVersions(packageName);
  };

  const handleImportDetected = async () => {
    try {
      const res = await api.post('/runtime/import-detected');
      const imported = res.data.data?.imported || 0;
      message.success(`成功导入 ${imported} 个环境`);
      setDetectVisible(false);
      fetchEnvironments();
    } catch (error: any) {
      message.error(error.message || '导入失败');
    }
  };

  const fetchVersions = async (runtimeName: string, forceRefresh = false) => {
    setVersionsLoading(true);
    try {
      let versions: VersionInfo[] = [];

      if (!forceRefresh) {
        // Try to get cached versions first
        const cacheRes = await api.get(`/runtime-versions/${runtimeName}`);
        versions = cacheRes.data.data?.versions || [];
      }

      // If no cached versions or force refresh, fetch from external source
      if (!versions || versions.length === 0 || forceRefresh) {
        const fetchRes = await api.post(`/runtime-versions/${runtimeName}/fetch`);
        versions = fetchRes.data.data?.versions || [];
      }

      setAvailableVersions(versions);
    } catch (error: any) {
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
    form.setFieldsValue({ version: undefined });
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
    } catch (error: any) {
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
    } catch (error: any) {
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
        form.setFieldsValue({ version: resolved });
        message.success(`别名 "${alias}" 解析为版本 ${resolved}`);
      }
    } catch (error: any) {
      message.error(error.message || '别名解析失败');
    }
  };

  const getStatusTag = (status: string, record: RuntimeEnvironment) => {
    switch (status) {
      case 'installed':
        return <Tag color="green">已安装</Tag>;
      case 'installing':
        return (
          <Space direction="vertical" size={0}>
            <Tag color="blue" icon={<SyncOutlined spin />}>安装中</Tag>
            {record.progress > 0 && (
              <Progress percent={record.progress} size="small" style={{ width: 100 }} />
            )}
            {record.progress_step && <span style={{ fontSize: 12, color: '#999' }}>{record.progress_step}</span>}
          </Space>
        );
      case 'failed':
        return (
          <Space direction="vertical" size={0}>
            <Tag color="red">安装失败</Tag>
            {record.error_message && <span style={{ fontSize: 12, color: '#ff4d4f' }}>{record.error_message}</span>}
          </Space>
        );
      default:
        return <Tag>{status}</Tag>;
    }
  };

  const getRuntimeIcon = (name: string) => {
    const icons: Record<string, string> = {
      java: '☕',
      node: '🟢',
      go: '🔵',
      python: '🐍',
      php: '🐘',
    };
    return icons[name] || '📦';
  };

  const columns = [
    {
      title: '运行环境',
      key: 'name',
      render: (_: any, record: RuntimeEnvironment) => (
        <Space>
          <span>{getRuntimeIcon(record.name)}</span>
          <span style={{ textTransform: 'capitalize' }}>{record.name}</span>
        </Space>
      ),
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      render: (version: string, record: RuntimeEnvironment) => (
        <Space>
          <span>{version}</span>
          {record.is_default && <Tag color="blue">默认</Tag>}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string, record: RuntimeEnvironment) => getStatusTag(status, record),
    },
    {
      title: '安装路径',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
    },
    {
      title: '安装时间',
      dataIndex: 'installed_at',
      key: 'installed_at',
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: any, record: RuntimeEnvironment) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {record.status === 'installed' && (
            <>
              {!record.is_default ? (
                <Button
                  type="link"
                  size="small"
                  icon={<CheckCircleOutlined />}
                  onClick={() => handleSetDefault(record.name, record.version)}
                >
                  设为默认
                </Button>
              ) : (
                <Tag color="green">当前默认</Tag>
              )}
              <Button
                type="link"
                size="small"
                icon={<AppstoreOutlined />}
                onClick={() => handleOpenPackageManager(record)}
              >
                包管理
              </Button>
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => handleViewCleanup(record.id)}
                loading={cleanupLoading}
              >
                卸载
              </Button>
            </>
          )}
          {record.status === 'installing' && (
            <Button
              type="link"
              size="small"
              onClick={() => handleViewLogs(record.id)}
              loading={logsLoading}
            >
              查看日志
            </Button>
          )}
          {record.status === 'failed' && (
            <>
              <Button
                type="link"
                size="small"
                icon={<ReloadOutlined />}
                onClick={() => handleRetry(record.name, record.version)}
              >
                重试
              </Button>
              <Button
                type="link"
                size="small"
                onClick={() => handleViewLogs(record.id)}
                loading={logsLoading}
              >
                查看日志
              </Button>
              <Popconfirm
                title="确定要删除此记录吗？"
                onConfirm={() => handleUninstall(record.name, record.version)}
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            </>
          )}
        </div>
      ),
    },
  ];

  return (
    <div>
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
        <Table
          columns={columns}
          dataSource={environments}
          rowKey="id"
          loading={loading}
          pagination={false}
        />
      </Card>

      {/* 安装环境弹窗 */}
      <Modal
        title="安装运行环境"
        open={installVisible}
        onCancel={() => {
          setInstallVisible(false);
          setAvailableVersions([]);
          setSelectedRuntime('');
          form.resetFields();
        }}
        footer={null}
      >
        <Form form={form} onFinish={handleInstall} layout="vertical">
          <Form.Item
            name="name"
            label="运行环境"
            rules={[{ required: true, message: '请选择运行环境' }]}
          >
            <Select placeholder="选择运行环境" onChange={handleRuntimeChange}>
              <Select.Option value="java">Java</Select.Option>
              <Select.Option value="node">Node.js</Select.Option>
              <Select.Option value="go">Go</Select.Option>
              <Select.Option value="python">Python</Select.Option>
              <Select.Option value="php">PHP</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="version"
            label={
              <Space>
                <span>版本号</span>
                {selectedRuntime && versionsLoading && <Tag color="blue">加载中...</Tag>}
                {selectedRuntime && !versionsLoading && availableVersions.length > 0 && (
                  <Tag color="green">{selectedRuntime} 可用版本 {availableVersions.length} 个</Tag>
                )}
              </Space>
            }
            rules={[{ required: true, message: '请选择版本号' }]}
          >
            {selectedRuntime && versionsLoading ? (
              <Select placeholder="正在获取版本列表..." loading={true} />
            ) : availableVersions.length > 0 ? (
              <div>
                <Select
                  placeholder={`选择 ${selectedRuntime} 版本号`}
                  showSearch
                  filterOption={(input, option) => {
                    const value = option?.value as string;
                    return value?.toLowerCase().includes(input.toLowerCase()) ?? false;
                  }}
                  options={availableVersions.map(v => ({
                    label: (
                      <Space>
                        <span>{v.version}</span>
                        {v.installed && <Tag color="green" style={{ fontSize: 10 }}>已安装</Tag>}
                        {v.is_default && <Tag color="blue" style={{ fontSize: 10 }}>默认</Tag>}
                        {v.lts && <Tag color="orange" style={{ fontSize: 10 }}>LTS</Tag>}
                      </Space>
                    ),
                    value: v.version,
                    disabled: v.installed,
                  }))}
                />
                <Button
                  type="link"
                  size="small"
                  icon={<SyncOutlined />}
                  onClick={() => fetchVersions(selectedRuntime, true)}
                  style={{ padding: '4px 0' }}
                >
                  刷新版本列表
                </Button>
                {aliasSuggestions.length > 0 && (
                  <div style={{ marginTop: 8 }}>
                    <span style={{ fontSize: 12, color: '#999' }}>快速选择: </span>
                    <Space size={4} wrap>
                      {aliasSuggestions.map(alias => (
                        <Tag
                          key={alias}
                          color="blue"
                          style={{ cursor: 'pointer' }}
                          onClick={() => handleAliasClick(alias)}
                        >
                          {alias}
                        </Tag>
                      ))}
                    </Space>
                  </div>
                )}
              </div>
            ) : selectedRuntime ? (
              <div>
                <Input placeholder="输入版本号或别名，例如：17、lts、latest" />
                <Button
                  type="link"
                  size="small"
                  icon={<SyncOutlined />}
                  onClick={() => fetchVersions(selectedRuntime, true)}
                  loading={versionsLoading}
                  style={{ padding: '4px 0' }}
                >
                  从网络获取版本列表
                </Button>
                {aliasSuggestions.length > 0 && (
                  <div style={{ marginTop: 8 }}>
                    <span style={{ fontSize: 12, color: '#999' }}>快速选择: </span>
                    <Space size={4} wrap>
                      {aliasSuggestions.map(alias => (
                        <Tag
                          key={alias}
                          color="blue"
                          style={{ cursor: 'pointer' }}
                          onClick={() => handleAliasClick(alias)}
                        >
                          {alias}
                        </Tag>
                      ))}
                    </Space>
                  </div>
                )}
              </div>
            ) : (
              <Input placeholder="请先选择运行环境" disabled />
            )}
          </Form.Item>

          {/* 依赖检测结果 */}
          {selectedRuntime && (
            <Form.Item label="依赖检测">
              {depsLoading ? (
                <Space>
                  <SyncOutlined spin />
                  <span>正在检测依赖...</span>
                </Space>
              ) : dependencies ? (
                <div>
                  {dependencies.missing.length === 0 ? (
                    <Tag color="green">所有必需依赖已满足</Tag>
                  ) : (
                    <div>
                      <Tag color="red">缺少必需依赖</Tag>
                      <div style={{ marginTop: 8 }}>
                        <span style={{ color: '#ff4d4f' }}>缺失: </span>
                        {dependencies.missing.map(dep => (
                          <Tag key={dep} color="red" style={{ marginBottom: 4 }}>{dep}</Tag>
                        ))}
                      </div>
                      <div style={{ marginTop: 4, color: '#999', fontSize: 12 }}>
                        请先安装缺失的依赖后再安装运行环境
                      </div>
                    </div>
                  )}
                  {dependencies.installed.length > 0 && (
                    <div style={{ marginTop: 4 }}>
                      <span style={{ color: '#52c41a' }}>已安装: </span>
                      {dependencies.installed.map(dep => (
                        <Tag key={dep} color="green" style={{ marginBottom: 4 }}>{dep}</Tag>
                      ))}
                    </div>
                  )}
                  {dependencies.optional && dependencies.optional.length > 0 && (
                    <div style={{ marginTop: 4 }}>
                      <span style={{ color: '#faad14' }}>可选: </span>
                      {dependencies.optional.map(dep => (
                        <Tag key={dep} color="warning" style={{ marginBottom: 4 }}>{dep}</Tag>
                      ))}
                    </div>
                  )}
                </div>
              ) : (
                <span style={{ color: '#999' }}>选择运行环境后自动检测</span>
              )}
            </Form.Item>
          )}

          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              block
              disabled={dependencies ? dependencies.missing.length > 0 : false}
            >
              开始安装
            </Button>
          </Form.Item>
        </Form>
      </Modal>

      {/* 检测环境弹窗 */}
      <Modal
        title="系统已安装的运行环境"
        open={detectVisible}
        onCancel={() => setDetectVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setDetectVisible(false)}>
            关闭
          </Button>,
          <Button key="import" type="primary" onClick={handleImportDetected}>
            导入到管理列表
          </Button>,
        ]}
      >
        {detectedRuntimes.length === 0 ? (
          <p>未检测到已安装的运行环境</p>
        ) : (
          <div>
            {detectedRuntimes.map((runtime) => (
              <Card key={runtime.name} size="small" style={{ marginBottom: 16 }}>
                <Title level={5}>
                  {getRuntimeIcon(runtime.name)} {runtime.name}
                </Title>
                <Space>
                  {runtime.versions.map((version) => (
                    <Tag key={version}>{version}</Tag>
                  ))}
                </Space>
              </Card>
            ))}
          </div>
        )}
      </Modal>

      {/* 安装日志弹窗 */}
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

      {/* 清理信息弹窗 */}
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
                  {cleanupData.env_configs?.map((config: any) => (
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
                  {cleanupData.path_entries?.map((entry: any) => (
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

      {/* 包管理弹窗 */}
      <Modal
        title={
          <Space>
            <AppstoreOutlined />
            <span>包管理</span>
            {selectedRuntimeForPackage && (
              <Tag color="blue">
                {selectedRuntimeForPackage.name} {selectedRuntimeForPackage.version}
              </Tag>
            )}
          </Space>
        }
        open={packageVisible}
        onCancel={() => {
          setPackageVisible(false);
          setSelectedRuntimeForPackage(null);
          setPackageData([]);
        }}
        footer={null}
        width={800}
      >
        {selectedRuntimeForPackage && (
          <div>
            <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
              <Space>
                <Button
                  icon={<SearchOutlined />}
                  onClick={handleScanPackages}
                  loading={packageLoading}
                >
                  扫描已安装的包
                </Button>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => fetchPackages(selectedRuntimeForPackage.id)}
                >
                  刷新
                </Button>
              </Space>
            </div>

            <div style={{ marginBottom: 16, padding: 16, background: '#f5f5f5', borderRadius: 4 }}>
              <div style={{ marginBottom: 8, fontWeight: 'bold' }}>安装新包</div>
              <Form form={packageForm} onFinish={handleInstallPackage} layout="inline">
                <div style={{ position: 'relative' }}>
                  <Form.Item name="name" rules={[{ required: true, message: '请输入包名' }]}>
                    <Input
                      placeholder="输入包名搜索..."
                      size="small"
                      style={{ width: 200 }}
                      onChange={(e) => handleSearchPackages(e.target.value)}
                    />
                  </Form.Item>
                  {packageSearchResults.length > 0 && (
                    <div style={{
                      position: 'absolute',
                      top: '100%',
                      left: 0,
                      right: 0,
                      background: 'white',
                      border: '1px solid #d9d9d9',
                      borderRadius: 4,
                      maxHeight: 200,
                      overflow: 'auto',
                      zIndex: 1000,
                    }}>
                      {packageSearchResults.map((pkg, index) => (
                        <div
                          key={index}
                          style={{
                            padding: '8px 12px',
                            cursor: 'pointer',
                            borderBottom: '1px solid #f0f0f0',
                          }}
                          onClick={() => handleSelectPackage(pkg.name)}
                        >
                          <div style={{ fontWeight: 'bold' }}>{pkg.name}</div>
                          <div style={{ fontSize: 12, color: '#999' }}>{pkg.description}</div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                <Form.Item name="version">
                  <Select
                    placeholder="选择版本"
                    size="small"
                    style={{ width: 150 }}
                    loading={packageVersionsLoading}
                    allowClear
                    showSearch
                  >
                    {packageVersions.map((v, index) => (
                      <Select.Option key={index} value={v}>{v}</Select.Option>
                    ))}
                  </Select>
                </Form.Item>
                <Form.Item>
                  <Button type="primary" htmlType="submit" size="small" icon={<PlusOutlined />}>
                    安装
                  </Button>
                </Form.Item>
              </Form>
            </div>

            {packageData.length === 0 ? (
              <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
                暂无已安装的包，点击"扫描"按钮检测
              </div>
            ) : (
              <Table
                dataSource={packageData}
                rowKey="id"
                loading={packageLoading}
                pagination={{ pageSize: 10 }}
                size="small"
                columns={[
                  { title: '包名', dataIndex: 'name', key: 'name' },
                  {
                    title: '版本',
                    dataIndex: 'version',
                    key: 'version',
                    render: (v: string) => <Tag color="blue">{v}</Tag>,
                  },
                  {
                    title: '范围',
                    dataIndex: 'scope',
                    key: 'scope',
                    render: (s: string) => <Tag color={s === 'global' ? 'green' : 'orange'}>{s}</Tag>,
                  },
                  { title: '来源', dataIndex: 'source', key: 'source' },
                  {
                    title: '操作',
                    key: 'action',
                    width: 120,
                    render: (_: any, record: any) => (
                      <Space>
                        <Button
                          type="link"
                          size="small"
                          icon={<SyncOutlined />}
                          onClick={() => handleUpdatePackage(record.name)}
                        >
                          更新
                        </Button>
                        <Popconfirm
                          title={`确定要卸载 ${record.name} 吗？`}
                          onConfirm={() => handleUninstallPackage(record.name)}
                        >
                          <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                            卸载
                          </Button>
                        </Popconfirm>
                      </Space>
                    ),
                  },
                ]}
              />
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
