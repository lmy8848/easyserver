import { useState, useEffect, useRef } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber, Select, Alert,
  message, Popconfirm, Tooltip, Row, Col,
  Table, Tabs, Empty, Spin,
} from 'antd';
import {
  GlobalOutlined, PlusOutlined, EditOutlined, DeleteOutlined,
  PlayCircleOutlined, PauseCircleOutlined, SafetyOutlined,
  FileTextOutlined, ArrowLeftOutlined, CloudServerOutlined,
  RocketOutlined, StopOutlined, ReloadOutlined, DownloadOutlined,
  UndoOutlined, CodeOutlined, ToolOutlined, SettingOutlined,
  CheckCircleOutlined, CloseCircleOutlined, CopyOutlined, FolderOutlined,
} from '@ant-design/icons';
import { webServerApi, websiteApi } from '../services/api';
import { usePortCheck } from '../hooks/usePortCheck';
import type { WebServer, Website } from '../types';

export default function WebsitePage() {
  const [servers, setServers] = useState<WebServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedServer, setSelectedServer] = useState<WebServer | null>(null);

  // Website state
  const [websites, setWebsites] = useState<Website[]>([]);
  const [sitesLoading, setSitesLoading] = useState(false);

  // Create/Edit modal
  const [modalVisible, setModalVisible] = useState(false);
  const [editingSite, setEditingSite] = useState<Website | null>(null);
  const [form] = Form.useForm();

  // Log modal
  const [logVisible, setLogVisible] = useState(false);
  const [logSite, setLogSite] = useState<Website | null>(null);
  const [logContent, setLogContent] = useState('');
  const [logType, setLogType] = useState('access');
  const [logLoading, setLogLoading] = useState(false);

  // SSL modal
  const [sslVisible, setSslVisible] = useState(false);
  const [sslSite, setSslSite] = useState<Website | null>(null);
  const [sslForm] = Form.useForm();

  // Operation loading state
  const [operating, setOperating] = useState<string>(''); // e.g. "install-1", "stop-2"

  // Config modal
  const [configVisible, setConfigVisible] = useState(false);
  const [configContent, setConfigContent] = useState('');
  const [configLoading, setConfigLoading] = useState(false);
  const [configTestResult, setConfigTestResult] = useState<{ valid: boolean; message: string } | null>(null);

  // Service logs modal
  const [svcLogVisible, setSvcLogVisible] = useState(false);
  const [svcLogContent, setSvcLogContent] = useState('');
  const [svcLogLoading, setSvcLogLoading] = useState(false);
  const [svcLogFollow, setSvcLogFollow] = useState(true);
  const svcLogRef = useRef<HTMLDivElement>(null);
  const pathTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Highlight log line based on content
  // Log level styles
  const LOG_STYLES = {
    error:  { color: '#cf1322', bg: '#fff1f0', border: '#ffa39e' },
    warn:   { color: '#ad6800', bg: '#fffbe6', border: '#ffe58f' },
    info:   { color: '#006d75', bg: '#e6fffb', border: '#b5f5ec' },
    debug:  { color: '#8c8c8c', bg: 'transparent', border: 'transparent' },
    default:{ color: '#262626', bg: 'transparent', border: 'transparent' },
  } as const;

  // Project types
  const [projectTypes, setProjectTypes] = useState<Array<{ name: string; label: string; description: string; default_port: number; proxy: boolean }>>([]);

  // Port check
  const { result: portCheck, checking: portChecking, checkPort, clearResult: clearPortCheck } = usePortCheck();

  // Directory browser state
  const [dirBrowserVisible, setDirBrowserVisible] = useState(false);
  const [dirBrowserPath, setDirBrowserPath] = useState('/var/www');
  const [dirEntries, setDirEntries] = useState<Array<{ name: string; path: string; is_dir: boolean; has_items: boolean; project: string }>>([]);
  const [dirLoading, setDirLoading] = useState(false);
  const [pathValidation, setPathValidation] = useState<{ valid: boolean; message: string; exists?: boolean; writable?: boolean; project?: string } | null>(null);

  const highlightLogLine = (line: string) => {
    const lower = line.toLowerCase();
    if (lower.includes('error') || lower.includes('fatal') || lower.includes('panic')) return LOG_STYLES.error;
    if (lower.includes('warn')) return LOG_STYLES.warn;
    if (lower.includes('info') || lower.includes('started') || lower.includes('listening')) return LOG_STYLES.info;
    if (lower.includes('debug')) return LOG_STYLES.debug;
    return LOG_STYLES.default;
  };

  // Directory browser functions
  const openDirBrowser = async (currentPath?: string) => {
    const path = currentPath || form.getFieldValue('root_path') || '/var/www';
    setDirBrowserVisible(true);
    await browseTo(path);
  };

  const browseTo = async (path: string) => {
    setDirLoading(true);
    try {
      const res = await webServerApi.browseDirs(path);
      const data = res.data.data;
      setDirBrowserPath(data?.current || path);
      setDirEntries(data?.entries || []);
    } catch (error: any) {
      message.error(error.message || '无法浏览目录');
    } finally {
      setDirLoading(false);
    }
  };

  const selectDir = (path: string) => {
    form.setFieldsValue({ root_path: path });
    setDirBrowserVisible(false);
    validateRootPath(path);
  };

  const validateRootPath = async (path: string) => {
    if (!path) { setPathValidation(null); return; }
    try {
      const res = await webServerApi.validatePath(path);
      setPathValidation(res.data.data || null);
    } catch {
      setPathValidation(null);
    }
  };

  const projectLabel: Record<string, string> = {
    nodejs: 'Node.js', php: 'PHP', python: 'Python', django: 'Django',
    java: 'Java', go: 'Go', ruby: 'Ruby', static: '静态网站',
  };

  useEffect(() => {
    fetchServers();
    fetchProjectTypes();
    return () => { if (pathTimerRef.current) clearTimeout(pathTimerRef.current); };
  }, []);

  const fetchProjectTypes = async () => {
    try {
      const res = await webServerApi.getProjectTypes();
      setProjectTypes(res.data.data || []);
    } catch {}
  };

  // Auto-refresh process info when viewing server detail (every 10s)
  useEffect(() => {
    if (!selectedServer || selectedServer.status === 'not_installed') return;

    const refresh = async () => {
      try {
        const [procRes, statusRes] = await Promise.all([
          webServerApi.getProcessInfo(selectedServer.id),
          webServerApi.status(selectedServer.id),
        ]);
        const proc = procRes.data.data;
        const status = statusRes.data.data;
        if (proc || status) {
          setSelectedServer(prev => prev ? { ...prev, ...proc, ...status } : prev);
          setServers(prev => prev.map(s =>
            s.id === selectedServer.id ? { ...s, ...proc, ...status } : s
          ));
        }
      } catch {}
    };

    refresh(); // immediate
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, [selectedServer?.id, selectedServer?.status]);

  // Auto-refresh service logs when modal is open (every 5s)
  useEffect(() => {
    if (!svcLogVisible || !selectedServer) return;

    const refresh = async () => {
      try {
        const res = await webServerApi.getServiceLogs(selectedServer.id, 200);
        setSvcLogContent(res.data.data?.logs || '(empty)');
      } catch {}
    };

    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, [svcLogVisible, selectedServer?.id]);

  // Auto-scroll to bottom when follow mode is on and content changes
  useEffect(() => {
    if (svcLogFollow && svcLogRef.current) {
      svcLogRef.current.scrollTop = svcLogRef.current.scrollHeight;
    }
  }, [svcLogContent, svcLogFollow]);

  const fetchServers = async () => {
    setLoading(true);
    try {
      const res = await webServerApi.list();
      setServers(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch servers:', error);
    } finally {
      setLoading(false);
    }
  };

  // Enter server detail
  const enterServer = async (server: WebServer) => {
    setSelectedServer(server);
    setSitesLoading(true);
    try {
      const [sitesRes, procRes] = await Promise.all([
        websiteApi.list(server.id),
        webServerApi.getProcessInfo(server.id).catch(() => null),
      ]);
      setWebsites(sitesRes.data.data || []);
      if (procRes?.data?.data) {
        setSelectedServer(prev => prev ? { ...prev, ...procRes.data.data } : prev);
      }
    } catch (error) {
      console.error('Failed to fetch websites:', error);
    } finally {
      setSitesLoading(false);
    }
  };

  const goBack = () => {
    setSelectedServer(null);
    setWebsites([]);
  };

  // Refresh a single server in the list (and selectedServer if viewing it)
  const refreshServer = async (serverId: number) => {
    try {
      const res = await webServerApi.get(serverId);
      const updated = res.data.data;
      setServers(prev => prev.map(s => s.id === serverId ? { ...s, ...updated } : s));
      setSelectedServer(prev => prev && prev.id === serverId ? { ...prev, ...updated } : prev);
    } catch {}
  };

  // Server actions with loading and dynamic refresh
  const handleInstall = async (server: WebServer) => {
    setOperating(`install-${server.id}`);
    try {
      await webServerApi.install(server.id);
      message.success(`${server.display_name} 安装成功`);
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '安装失败');
    } finally {
      setOperating('');
    }
  };

  const handleUninstall = async (server: WebServer) => {
    setOperating(`uninstall-${server.id}`);
    try {
      await webServerApi.uninstall(server.id);
      message.success(`${server.display_name} 已卸载`);
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '卸载失败');
    } finally {
      setOperating('');
    }
  };

  const handleStart = async (server: WebServer) => {
    setOperating(`start-${server.id}`);
    try {
      await webServerApi.start(server.id);
      message.success(`${server.display_name} 已启动`);
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '启动失败');
    } finally {
      setOperating('');
    }
  };

  const handleStop = async (server: WebServer) => {
    setOperating(`stop-${server.id}`);
    try {
      await webServerApi.stop(server.id);
      message.success(`${server.display_name} 已停止`);
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '停止失败');
    } finally {
      setOperating('');
    }
  };

  const handleRestart = async (server: WebServer) => {
    setOperating(`restart-${server.id}`);
    try {
      await webServerApi.restart(server.id);
      message.success(`${server.display_name} 已重启`);
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '重启失败');
    } finally {
      setOperating('');
    }
  };

  // Reload config
  const handleReload = async (server: WebServer) => {
    setOperating(`reload-${server.id}`);
    try {
      await webServerApi.reload(server.id);
      message.success('配置已重载');
      await refreshServer(server.id);
    } catch (error: any) {
      message.error(error.message || '重载失败');
    } finally {
      setOperating('');
    }
  };

  // Test config
  const handleTestConfig = async (server: WebServer) => {
    setOperating(`test-${server.id}`);
    try {
      const res = await webServerApi.testConfig(server.id);
      setConfigTestResult(res.data.data || null);
    } catch (error: any) {
      setConfigTestResult({ valid: false, message: error.message || 'test failed' });
    } finally {
      setOperating('');
    }
  };

  // View/edit config
  const showConfig = async (server: WebServer) => {
    setConfigVisible(true);
    setConfigLoading(true);
    setConfigTestResult(null);
    try {
      const res = await webServerApi.getConfig(server.id);
      setConfigContent(res.data.data?.content || '');
    } catch (error: any) {
      setConfigContent('# Failed to load: ' + (error.message || 'unknown'));
    } finally {
      setConfigLoading(false);
    }
  };

  const handleSaveConfig = async () => {
    const server = selectedServer;
    if (!server) return;
    try {
      await webServerApi.saveConfig(server.id, configContent);
      message.success('配置已保存（已自动备份原文件）');
    } catch (error: any) {
      message.error(error.message || '保存失败');
    }
  };

  // Service logs
  const showServiceLogs = async (server: WebServer) => {
    setSvcLogVisible(true);
    setSvcLogLoading(true);
    try {
      const res = await webServerApi.getServiceLogs(server.id, 200);
      setSvcLogContent(res.data.data?.logs || '(empty)');
    } catch (error: any) {
      setSvcLogContent('Failed: ' + (error.message || 'unknown'));
    } finally {
      setSvcLogLoading(false);
    }
  };

  // Auto-start toggle
  const handleAutoStart = async (server: WebServer, enabled: boolean) => {
    try {
      await webServerApi.setAutoStart(server.id, enabled);
      message.success(enabled ? '已设置开机自启' : '已取消开机自启');
      fetchServers();
    } catch (error: any) {
      message.error(error.message || '设置失败');
    }
  };

  // Website CRUD
  const handleCreateSite = () => {
    setEditingSite(null);
    form.resetFields();
    form.setFieldsValue({ port: 80 });
    setModalVisible(true);
  };

  const handleEditSite = (site: Website) => {
    setEditingSite(site);
    form.setFieldsValue({
      name: site.name,
      domain: site.domain,
      root_path: site.root_path,
      port: site.port,
      proxy_enabled: site.proxy_enabled,
      proxy_pass: site.proxy_pass,
      custom_config: site.custom_config,
    });
    setModalVisible(true);
  };

  const handleSubmitSite = async () => {
    const server = selectedServer;
    if (!server) return;
    try {
      const values = await form.validateFields();
      if (editingSite) {
        await websiteApi.update(server.id, editingSite.id, values);
        message.success('更新成功');
      } else {
        await websiteApi.create(server.id, values);
        message.success('创建成功');
      }
      setModalVisible(false);
      enterServer(server);
    } catch (error: any) {
      if (error.message) message.error(error.message);
    }
  };

  const handleDeleteSite = async (id: number) => {
    const server = selectedServer;
    if (!server) return;
    try {
      await websiteApi.delete(server.id, id);
      message.success('删除成功');
      enterServer(server);
    } catch (error: any) {
      message.error(error.message || '删除失败');
    }
  };

  const handleToggleSite = async (site: Website) => {
    const server = selectedServer;
    if (!server) return;
    try {
      if (site.status === 'active') {
        await websiteApi.disable(server.id, site.id);
        message.success('已禁用');
      } else {
        await websiteApi.enable(server.id, site.id);
        message.success('已启用');
      }
      enterServer(server);
    } catch (error: any) {
      message.error(error.message || '操作失败');
    }
  };

  // Logs
  const showLogs = async (site: Website, type: string = 'access') => {
    const server = selectedServer;
    if (!server) return;
    setLogSite(site);
    setLogType(type);
    setLogVisible(true);
    setLogLoading(true);
    try {
      const res = await websiteApi.getLogs(server.id, site.id, type, 200);
      setLogContent(res.data.data?.logs || '(empty)');
    } catch (error: any) {
      setLogContent('Failed to load logs: ' + (error.message || 'unknown'));
    } finally {
      setLogLoading(false);
    }
  };

  // SSL
  const showSSL = (site: Website) => {
    setSslSite(site);
    sslForm.resetFields();
    setSslVisible(true);
  };

  const handleApplySSL = async () => {
    const server = selectedServer;
    const ssl = sslSite;
    if (!server || !ssl) return;
    try {
      const values = await sslForm.validateFields();
      await websiteApi.applySSL(server.id, ssl.id, values.email);
      message.success('SSL 证书申请成功');
      setSslVisible(false);
      enterServer(server);
    } catch (error: any) {
      message.error(error.message || 'SSL 申请失败');
    }
  };

  // Status helpers
  const statusTag = (status: string) => {
    switch (status) {
      case 'running': return <Tag color="success">运行中</Tag>;
      case 'stopped': return <Tag color="error">已停止</Tag>;
      case 'not_installed': return <Tag color="default">未安装</Tag>;
      default: return <Tag color="default">{status}</Tag>;
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'running': return '#52c41a';
      case 'stopped': return '#ff4d4f';
      default: return '#999';
    }
  };

  // Level 1: Web Server cards
  if (!selectedServer) {
    return (
      <div>
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
          <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchServers}>
            刷新
          </Button>
        </div>
        <Row gutter={[16, 16]}>
          {servers.map(server => (
            <Col xs={24} sm={12} lg={6} key={server.id}>
              <Card
                hoverable
                onClick={() => server.status !== 'not_installed' && enterServer(server)}
                style={{ borderColor: statusColor(server.status) }}
                actions={[
                  server.status === 'not_installed' ? (
                    <Tooltip title="安装" key="install">
                      <Button type="link" icon={<DownloadOutlined />} loading={operating === `install-${server.id}`} onClick={(e) => { e.stopPropagation(); handleInstall(server); }}>
                        安装
                      </Button>
                    </Tooltip>
                  ) : server.status === 'running' ? (
                    <Tooltip title="停止" key="stop">
                      <Button type="link" danger icon={<StopOutlined />} loading={operating === `stop-${server.id}`} onClick={(e) => { e.stopPropagation(); handleStop(server); }}>
                        停止
                      </Button>
                    </Tooltip>
                  ) : (
                    <Tooltip title="启动" key="start">
                      <Button type="link" icon={<PlayCircleOutlined />} loading={operating === `start-${server.id}`} onClick={(e) => { e.stopPropagation(); handleStart(server); }}>
                        启动
                      </Button>
                    </Tooltip>
                  ),
                  server.status !== 'not_installed' && (
                    <Tooltip title="重启" key="restart">
                      <Button type="link" icon={<ReloadOutlined />} loading={operating === `restart-${server.id}`} onClick={(e) => { e.stopPropagation(); handleRestart(server); }}>
                        重启
                      </Button>
                    </Tooltip>
                  ),
                ].filter(Boolean)}
              >
                <Card.Meta
                  avatar={<CloudServerOutlined style={{ fontSize: 32, color: statusColor(server.status) }} />}
                  title={
                    <Space>
                      {server.display_name}
                      {statusTag(server.status)}
                    </Space>
                  }
                  description={
                    <div>
                      <p style={{ margin: '8px 0', color: '#666' }}>{server.description}</p>
                      {server.version && <Tag color="blue">{server.version}</Tag>}
                    </div>
                  }
                />
              </Card>
            </Col>
          ))}
        </Row>
      </div>
    );
  }

  // Level 2: Website list for selected server
  const siteColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', key: 'name', width: 120, render: (t: string) => <strong>{t}</strong> },
    {
      title: '域名', dataIndex: 'domain', key: 'domain',
      render: (text: string, record: Website) => (
        <a href={`http://${text}:${record.port}`} target="_blank" rel="noreferrer">
          <GlobalOutlined /> {text}
        </a>
      ),
    },
    { title: '端口', dataIndex: 'port', key: 'port', width: 70 },
    {
      title: '项目类型', dataIndex: 'project_type', key: 'project_type', width: 100,
      render: (pt: string) => <Tag>{projectLabel[pt] || pt || '静态'}</Tag>,
    },
    { title: '根目录', dataIndex: 'root_path', key: 'root_path', ellipsis: true },
    {
      title: 'SSL', key: 'ssl', width: 80,
      render: (_: any, r: Website) => r.ssl_enabled
        ? <Tag icon={<SafetyOutlined />} color="success">已启用</Tag>
        : <Tag color="default">未启用</Tag>,
    },
    {
      title: '反代', key: 'proxy', width: 80,
      render: (_: any, r: Website) => r.proxy_enabled
        ? <Tooltip title={r.proxy_pass}><Tag color="blue">已启用</Tag></Tooltip>
        : <Tag color="default">关闭</Tag>,
    },
    {
      title: '状态', key: 'status', width: 80,
      render: (_: any, r: Website) => r.status === 'active'
        ? <Tag color="success">运行中</Tag>
        : <Tag color="error">已禁用</Tag>,
    },
    {
      title: '操作', key: 'action', width: 320,
      render: (_: any, record: Website) => (
        <Space size="small" wrap>
          <Tooltip title="编辑">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEditSite(record)} />
          </Tooltip>
          <Tooltip title={record.status === 'active' ? '禁用' : '启用'}>
            <Button type="link" size="small" icon={record.status === 'active' ? <PauseCircleOutlined /> : <PlayCircleOutlined />} onClick={() => handleToggleSite(record)} />
          </Tooltip>
          <Tooltip title="访问日志">
            <Button type="link" size="small" icon={<FileTextOutlined />} onClick={() => showLogs(record, 'access')} />
          </Tooltip>
          <Tooltip title="错误日志">
            <Button type="link" size="small" icon={<FileTextOutlined />} style={{ color: '#ff4d4f' }} onClick={() => showLogs(record, 'error')} />
          </Tooltip>
          <Tooltip title="SSL 证书">
            <Button type="link" size="small" icon={<SafetyOutlined />} onClick={() => showSSL(record)} />
          </Tooltip>
          <Popconfirm title="确定删除此网站？" onConfirm={() => handleDeleteSite(record.id)}>
            <Tooltip title="删除">
              <Button type="link" size="small" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {/* Header with back button and server info */}
      <Card style={{ marginBottom: 16 }}>
        <Row justify="space-between" align="middle">
          <Col>
            <Space size="middle">
              <Button icon={<ArrowLeftOutlined />} onClick={goBack}>返回</Button>
              <CloudServerOutlined style={{ fontSize: 28, color: statusColor(selectedServer.status) }} />
              <div>
                <Space>
                  <span style={{ fontSize: 18, fontWeight: 'bold' }}>{selectedServer.display_name}</span>
                  {operating && operating.endsWith(`-${selectedServer.id}`)
                    ? <Tag color="processing">操作中...</Tag>
                    : statusTag(selectedServer.status)
                  }
                </Space>
                <div style={{ color: '#999', fontSize: 12, marginTop: 4 }}>{selectedServer.description}</div>
              </div>
            </Space>
          </Col>
          <Col>
            <Space wrap>
              {selectedServer.status === 'running' && (
                <>
                  <Button icon={<StopOutlined />} danger loading={operating === `stop-${selectedServer.id}`} onClick={() => handleStop(selectedServer)}>停止</Button>
                  <Button icon={<ReloadOutlined />} loading={operating === `restart-${selectedServer.id}`} onClick={() => handleRestart(selectedServer)}>重启</Button>
                  <Button icon={<ReloadOutlined />} loading={operating === `reload-${selectedServer.id}`} onClick={() => handleReload(selectedServer)}>重载配置</Button>
                </>
              )}
              {selectedServer.status === 'stopped' && (
                <Button type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${selectedServer.id}`} onClick={() => handleStart(selectedServer)}>启动</Button>
              )}
              {selectedServer.status === 'not_installed' && (
                <Button type="primary" icon={<DownloadOutlined />} loading={operating === `install-${selectedServer.id}`} onClick={() => handleInstall(selectedServer)}>安装</Button>
              )}
              {selectedServer.status !== 'not_installed' && (
                <>
                  <Button icon={<CodeOutlined />} onClick={() => showConfig(selectedServer)}>配置文件</Button>
                  <Button icon={<FileTextOutlined />} onClick={() => showServiceLogs(selectedServer)}>服务日志</Button>
                  <Button icon={<ToolOutlined />} loading={operating === `test-${selectedServer.id}`} onClick={() => handleTestConfig(selectedServer)}>测试配置</Button>
                  <Popconfirm title="确定卸载？需要先删除所有网站。" onConfirm={() => handleUninstall(selectedServer)}>
                    <Button icon={<UndoOutlined />} danger loading={operating === `uninstall-${selectedServer.id}`}>卸载</Button>
                  </Popconfirm>
                </>
              )}
            </Space>
          </Col>
        </Row>

        {/* Runtime info bar */}
        {selectedServer.status !== 'not_installed' && (
          <div style={{ marginTop: 12, padding: '8px 0', borderTop: '1px solid #f0f0f0' }}>
            <Row justify="space-between" align="middle">
              <Col>
                <Space size="large">
                  {selectedServer.version && <span>版本: <strong>{selectedServer.version}</strong></span>}
                  {selectedServer.pid > 0 && <span>PID: <strong>{selectedServer.pid}</strong></span>}
                  {selectedServer.memory_bytes > 0 && <span>内存: <strong>{(selectedServer.memory_bytes / 1024 / 1024).toFixed(1)} MB</strong></span>}
                  {selectedServer.uptime && <span>运行时间: <strong>{selectedServer.uptime}</strong></span>}
                  <span>默认端口: <strong>{selectedServer.default_port}</strong></span>
                  <span>配置目录: <Tag>{selectedServer.config_path}</Tag></span>
                  <span>日志目录: <Tag>{selectedServer.log_dir}</Tag></span>
                  {configTestResult && (
                    configTestResult.valid
                      ? <Tag icon={<CheckCircleOutlined />} color="success">配置正常</Tag>
                      : <Tooltip title={configTestResult.message}><Tag icon={<CloseCircleOutlined />} color="error">配置错误</Tag></Tooltip>
                  )}
                </Space>
              </Col>
              <Col>
                <Space>
                  <span style={{ color: '#999', fontSize: 12 }}>每 10 秒自动刷新</span>
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => refreshServer(selectedServer.id)}>
                    刷新
                  </Button>
                </Space>
              </Col>
            </Row>
          </div>
        )}
      </Card>

      {/* Website list */}
      <Card
        title={`${selectedServer.display_name} - 网站列表`}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} loading={sitesLoading} onClick={() => enterServer(selectedServer)}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateSite}
              disabled={selectedServer.status === 'not_installed'}>
              添加网站
            </Button>
          </Space>
        }
      >
        <Table
          columns={siteColumns}
          dataSource={websites}
          rowKey="id"
          loading={sitesLoading}
          pagination={{ defaultPageSize: 20, showTotal: (t) => `共 ${t} 个网站` }}
          size="small"
          locale={{ emptyText: selectedServer.status === 'not_installed'
            ? <Empty description="请先安装 Web 服务器" />
            : <Empty description="暂无网站，点击上方按钮添加" />
          }}
        />
      </Card>

      {/* Create/Edit Modal */}
      <Modal
        title={editingSite ? '编辑网站' : '添加网站'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmitSite}
        okText="确定"
        cancelText="取消"
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="网站名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如：我的博客" />
          </Form.Item>
          <Form.Item name="domain" label="域名" rules={[{ required: true, message: '请输入域名' }]}
            extra={editingSite ? '修改域名会同时更新 Nginx 配置文件' : ''}>
            <Input placeholder="如：example.com 或 IP 地址" />
          </Form.Item>
          <Form.Item name="root_path" label="根目录" rules={[{ required: true, message: '请输入根目录' }]}
            extra={pathValidation ? (
              <span style={{ color: pathValidation.valid ? '#52c41a' : '#ff4d4f' }}>
                {pathValidation.message}
                {pathValidation.project && ` (${projectLabel[pathValidation.project] || pathValidation.project})`}
              </span>
            ) : undefined}
          >
            <Input
              placeholder="如：/var/www/html"
              addonAfter={
                <Button type="link" size="small" icon={<FolderOutlined />} style={{ padding: 0 }}
                  onClick={() => openDirBrowser()}>
                  浏览
                </Button>
              }
              onChange={(e) => { if (pathTimerRef.current) clearTimeout(pathTimerRef.current); pathTimerRef.current = setTimeout(() => validateRootPath(e.target.value), 500); }}
            />
          </Form.Item>
          <Form.Item name="project_type" label="项目类型" initialValue="static">
            <Select
              onChange={(val: string) => {
                const pt = projectTypes.find(p => p.name === val);
                if (pt) {
                  form.setFieldsValue({
                    port: 80,
                    app_port: pt.default_port,
                    proxy_enabled: pt.proxy,
                    proxy_pass: pt.proxy ? `http://127.0.0.1:${pt.default_port}` : '',
                  });
                }
              }}
            >
              {projectTypes.map(pt => (
                <Select.Option key={pt.name} value={pt.name}>
                  <div>
                    <strong>{pt.label}</strong>
                    <span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{pt.description}</span>
                  </div>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="port" label="网站端口" initialValue={80}
            extra={portCheck && (
              portCheck.available
                ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
            )}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }}
              onChange={(val) => val && checkPort(val as number)} />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(prev, cur) => prev.project_type !== cur.project_type}>
            {({ getFieldValue }) => {
              const pt = getFieldValue('project_type') || 'static';
              const needsProxy = ['nodejs', 'python', 'java', 'proxy'].includes(pt);
              return needsProxy ? (
                <Form.Item name="app_port" label="应用端口" initialValue={3000}
                  extra="Web 服务器将代理到此端口">
                  <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                </Form.Item>
              ) : pt === 'php' ? (
                <Form.Item name="app_port" label="php-fpm 端口" initialValue={9000}>
                  <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                </Form.Item>
              ) : null;
            }}
          </Form.Item>
          <Form.Item name="custom_config" label="自定义配置（留空使用默认模板）">
            <Input.TextArea rows={4} placeholder="留空则根据项目类型自动生成配置" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Log Modal */}
      <Modal
        title={`${logSite?.domain} - ${logType === 'error' ? '错误' : '访问'}日志`}
        open={logVisible}
        onCancel={() => setLogVisible(false)}
        footer={null}
        width={900}
      >
        <Tabs
          activeKey={logType}
          onChange={(key) => { setLogType(key); showLogs(logSite!, key); }}
          items={[
            { key: 'access', label: '访问日志' },
            { key: 'error', label: '错误日志' },
          ]}
        />
        <Input.TextArea
          value={logLoading ? 'Loading...' : logContent}
          readOnly
          rows={20}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
        />
      </Modal>

      {/* SSL Modal */}
      <Modal
        title={`SSL 证书 - ${sslSite?.domain}`}
        open={sslVisible}
        onCancel={() => setSslVisible(false)}
        onOk={handleApplySSL}
        okText="申请证书"
        cancelText="取消"
      >
        <Form form={sslForm} layout="vertical">
          <Form.Item name="email" label="邮箱（可选）">
            <Input placeholder="admin@example.com" />
          </Form.Item>
        </Form>
        {sslSite?.ssl_enabled && (
          <Tag color="success">SSL 已启用: {sslSite.ssl_cert_path}</Tag>
        )}
      </Modal>

      {/* Config Editor Modal */}
      <Modal
        title={`${selectedServer?.display_name} - 配置文件`}
        open={configVisible}
        onCancel={() => setConfigVisible(false)}
        width={900}
        footer={
          <Space>
            <Button onClick={() => handleTestConfig(selectedServer!)}>测试配置</Button>
            <Button onClick={() => setConfigVisible(false)}>关闭</Button>
            <Button type="primary" onClick={handleSaveConfig}>保存</Button>
          </Space>
        }
      >
        {configTestResult && (
          <div style={{ marginBottom: 12 }}>
            {configTestResult.valid
              ? <Tag icon={<CheckCircleOutlined />} color="success">{configTestResult.message}</Tag>
              : <Tag icon={<CloseCircleOutlined />} color="error">{configTestResult.message}</Tag>
            }
          </div>
        )}
        <div style={{ marginBottom: 8, color: '#999', fontSize: 12 }}>
          文件路径: {selectedServer?.config_file}（保存时自动备份原文件）
        </div>
        <Input.TextArea
          value={configLoading ? 'Loading...' : configContent}
          onChange={(e) => setConfigContent(e.target.value)}
          rows={25}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
        />
      </Modal>

      {/* Service Logs Modal */}
      <Modal
        title={
          <Space>
            <FileTextOutlined />
            <span>{selectedServer?.display_name} - 服务日志</span>
            {svcLogLoading && <Spin size="small" />}
          </Space>
        }
        open={svcLogVisible}
        onCancel={() => setSvcLogVisible(false)}
        footer={
          <Row justify="space-between" align="middle" style={{ gap: 12 }}>
            <Col>
              <Space size="middle">
                <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
                <span style={{ color: svcLogFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>
                  {svcLogFollow ? '● 自动滚动' : '○ 已暂停'}
                </span>
              </Space>
            </Col>
            <Col>
              <Space size="small">
                <Button
                  size="small"
                  type={svcLogFollow ? 'primary' : 'default'}
                  onClick={() => setSvcLogFollow(!svcLogFollow)}
                >
                  {svcLogFollow ? 'Follow ON' : 'Follow OFF'}
                </Button>
                <Button
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(svcLogContent);
                    message.success('日志已复制到剪贴板');
                  }}
                >
                  复制
                </Button>
                <Button size="small" icon={<ReloadOutlined />} onClick={() => selectedServer && showServiceLogs(selectedServer)}>
                  刷新
                </Button>
                <Button size="small" onClick={() => setSvcLogVisible(false)}>关闭</Button>
              </Space>
            </Col>
          </Row>
        }
        width="90vw"
        style={{ maxWidth: 960 }}
      >
        {svcLogLoading && !svcLogContent ? (
          <div style={{ padding: 16 }}>
            {Array.from({ length: 12 }).map((_, i) => (
              <div key={i} style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
                <div style={{ width: 40, height: 14, background: '#f5f5f5', borderRadius: 2 }} />
                <div style={{ width: 160, height: 14, background: '#f5f5f5', borderRadius: 2 }} />
                <div style={{ flex: 1, height: 14, background: '#f5f5f5', borderRadius: 2, opacity: 0.5 }} />
              </div>
            ))}
          </div>
        ) : (
          <div
            ref={svcLogRef}
            style={{
              background: '#fafafa',
              border: '1px solid #e8e8e8',
              fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
              fontSize: 13,
              lineHeight: 1.8,
              padding: '8px 0',
              borderRadius: 6,
              maxHeight: '60vh',
              overflowY: 'auto',
              overflowX: 'auto',
            }}
          >
            {svcLogContent.split('\n').map((line, i) => {
              const style = highlightLogLine(line);
              return (
                <div
                  key={i}
                  style={{
                    display: 'flex',
                    alignItems: 'baseline',
                    color: style.color,
                    background: style.bg,
                    borderLeft: `3px solid ${style.border}`,
                    padding: '0 12px',
                    minHeight: 22,
                  }}
                >
                  <span style={{ color: '#bfbfbf', minWidth: 36, width: 36, flexShrink: 0, textAlign: 'right', marginRight: 16, userSelect: 'none', fontSize: 11 }}>
                    {i + 1}
                  </span>
                  <span style={{ whiteSpace: 'nowrap' }}>
                    {line || ' '}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </Modal>

      {/* Directory Browser Modal */}
      <Modal
        title={
          <Space>
            <FolderOutlined />
            <span>选择根目录</span>
          </Space>
        }
        open={dirBrowserVisible}
        onCancel={() => setDirBrowserVisible(false)}
        footer={
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ color: '#8c8c8c', fontSize: 12, fontFamily: 'monospace' }}>
              {dirBrowserPath}
            </span>
            <Space>
              <Button onClick={() => setDirBrowserVisible(false)}>取消</Button>
              <Button type="primary" onClick={() => selectDir(dirBrowserPath)}>选择当前目录</Button>
            </Space>
          </div>
        }
        width={640}
      >
        {/* Breadcrumb */}
        <div style={{ marginBottom: 12, padding: '6px 12px', background: '#f5f5f5', borderRadius: 4, fontSize: 13, fontFamily: 'monospace' }}>
          {dirBrowserPath.split('/').filter(Boolean).map((seg, i, arr) => {
            const path = '/' + arr.slice(0, i + 1).join('/');
            return (
              <span key={path}>
                {i > 0 && <span style={{ color: '#bfbfbf', margin: '0 4px' }}>/</span>}
                <span
                  style={{ color: '#1890ff', cursor: 'pointer' }}
                  onClick={() => browseTo(path)}
                >
                  {seg}
                </span>
              </span>
            );
          })}
        </div>

        {/* Directory list */}
        <div style={{ maxHeight: 400, overflowY: 'auto', border: '1px solid #f0f0f0', borderRadius: 4 }}>
          {dirLoading ? (
            <div style={{ padding: 40, textAlign: 'center' }}>
              <Spin />
            </div>
          ) : dirEntries.length === 0 ? (
            <div style={{ padding: 40, textAlign: 'center', color: '#999' }}>
              空目录
            </div>
          ) : (
            dirEntries.map((entry) => (
              <div
                key={entry.path}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  padding: '8px 12px',
                  borderBottom: '1px solid #f5f5f5',
                  cursor: 'pointer',
                  transition: 'background 0.2s',
                }}
                onClick={() => {
                  if (entry.name === '..') {
                    browseTo(entry.path);
                  } else {
                    browseTo(entry.path);
                  }
                }}
                onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.background = '#f5f5f5'; }}
                onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
              >
                <span style={{ marginRight: 8, fontSize: 16 }}>
                  {entry.name === '..' ? '⬆️' : '📁'}
                </span>
                <span style={{ flex: 1, fontWeight: entry.name === '..' ? 'normal' : 500 }}>
                  {entry.name}
                </span>
                {entry.project && (
                  <Tag color="blue" style={{ marginLeft: 8 }}>
                    {projectLabel[entry.project] || entry.project}
                  </Tag>
                )}
                {entry.has_items && !entry.project && (
                  <Tag color="green" style={{ marginLeft: 8 }}>有项目文件</Tag>
                )}
                <Button
                  type="link" size="small"
                  style={{ marginLeft: 8, padding: 0 }}
                  onClick={(e) => { e.stopPropagation(); selectDir(entry.path); }}
                >
                  选择
                </Button>
              </div>
            ))
          )}
        </div>
      </Modal>
    </div>
  );
}
