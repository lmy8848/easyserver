import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber, Select,
  message, Popconfirm, Tooltip, Row, Col,
  Table, Tabs, Empty,
} from 'antd';
import {
  GlobalOutlined, PlusOutlined, EditOutlined, DeleteOutlined,
  PlayCircleOutlined, PauseCircleOutlined, SafetyOutlined,
  FileTextOutlined, ArrowLeftOutlined, CloudServerOutlined,
  StopOutlined, ReloadOutlined, DownloadOutlined,
  UndoOutlined, CodeOutlined, ToolOutlined,
  CheckCircleOutlined, CloseCircleOutlined, FolderOutlined,
} from '@ant-design/icons';
import { webServerApi, websiteApi } from '../../services/api';
import { usePortCheck } from '../../hooks/usePortCheck';
import type { WebServer, Website } from '../../types';
import type { ProjectType, DirEntry, PathValidation, ConfigTestResult } from './types';
import { getServiceStatusColor, ServiceStatusTag } from '../../utils/status';

interface WebsiteListProps {
  selectedServer: WebServer;
  operating: string;
  configTestResult: ConfigTestResult | null;
  onGoBack: () => void;
  onStop: (server: WebServer) => void;
  onStart: (server: WebServer) => void;
  onRestart: (server: WebServer) => void;
  onReload: (server: WebServer) => void;
  onInstall: (server: WebServer) => void;
  onUninstall: (server: WebServer) => void;
  onTestConfig: (server: WebServer) => void;
  onRefreshServer: (serverId: number) => void;
  onShowConfig: () => void;
  onShowServiceLogs: () => void;
}

const projectLabel: Record<string, string> = {
  nodejs: 'Node.js', php: 'PHP', python: 'Python', django: 'Django',
  java: 'Java', go: 'Go', ruby: 'Ruby', static: '静态网站',
};

function statusTag(status: string) {
  return <ServiceStatusTag status={status} />;
}

function statusColor(status: string) {
  const colorName = getServiceStatusColor(status);
  const colorMap: Record<string, string> = {
    success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
  };
  return colorMap[colorName] || '#999';
}

export default function WebsiteList({
  selectedServer, operating, configTestResult,
  onGoBack, onStop, onStart, onRestart, onReload, onInstall, onUninstall,
  onTestConfig, onRefreshServer, onShowConfig, onShowServiceLogs,
}: WebsiteListProps) {
  // Website data
  const [websites, setWebsites] = useState<Website[]>([]);
  const [sitesLoading, setSitesLoading] = useState(true);

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

  // Directory browser state
  const [dirBrowserVisible, setDirBrowserVisible] = useState(false);
  const [dirBrowserPath, setDirBrowserPath] = useState('/var/www');
  const [dirEntries, setDirEntries] = useState<DirEntry[]>([]);
  const [dirLoading, setDirLoading] = useState(false);
  const [pathValidation, setPathValidation] = useState<PathValidation | null>(null);

  // Project types
  const [projectTypes, setProjectTypes] = useState<ProjectType[]>([]);

  // Port check
  const { result: portCheck, checkPort } = usePortCheck();

  // Debounce timer for path validation
  const pathTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchWebsites = useCallback(async () => {
    try {
      const res = await websiteApi.list(selectedServer.id);
      setWebsites(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch websites:', error);
    } finally {
      setSitesLoading(false);
    }
  }, [selectedServer.id]);

  const fetchProjectTypes = async () => {
    try {
      const res = await webServerApi.getProjectTypes();
      setProjectTypes(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch project types:', error);
    }
  };

  // Fetch websites on mount
  useEffect(() => {
    fetchWebsites();
    fetchProjectTypes();
    return () => { if (pathTimerRef.current) clearTimeout(pathTimerRef.current); };
  }, [fetchWebsites]);

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
    } catch (e) {
      console.debug('Path validation failed:', e);
      setPathValidation(null);
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
    try {
      const values = await form.validateFields();
      if (editingSite) {
        await websiteApi.update(selectedServer.id, editingSite.id, values);
        message.success('更新成功');
      } else {
        await websiteApi.create(selectedServer.id, values);
        message.success('创建成功');
      }
      setModalVisible(false);
      setSitesLoading(true);
      fetchWebsites();
    } catch (error: any) {
      if (error.message) message.error(error.message);
    }
  };

  const handleDeleteSite = async (id: number) => {
    try {
      await websiteApi.delete(selectedServer.id, id);
      message.success('删除成功');
      setSitesLoading(true);
      fetchWebsites();
    } catch (error: any) {
      message.error(error.message || '删除失败');
    }
  };

  const handleToggleSite = async (site: Website) => {
    try {
      if (site.status === 'active') {
        await websiteApi.disable(selectedServer.id, site.id);
        message.success('已禁用');
      } else {
        await websiteApi.enable(selectedServer.id, site.id);
        message.success('已启用');
      }
      setSitesLoading(true);
      fetchWebsites();
    } catch (error: any) {
      message.error(error.message || '操作失败');
    }
  };

  // Logs
  const showLogs = async (site: Website, type: string = 'access') => {
    setLogSite(site);
    setLogType(type);
    setLogVisible(true);
    setLogLoading(true);
    try {
      const res = await websiteApi.getLogs(selectedServer.id, site.id, type, 200);
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
    const ssl = sslSite;
    if (!ssl) return;
    try {
      const values = await sslForm.validateFields();
      await websiteApi.applySSL(selectedServer.id, ssl.id, values.email);
      message.success('SSL 证书申请成功');
      setSslVisible(false);
      fetchWebsites();
    } catch (error: any) {
      message.error(error.message || 'SSL 申请失败');
    }
  };

  // Site table columns
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
      render: (_: unknown, r: Website) => r.ssl_enabled
        ? <Tag icon={<SafetyOutlined />} color="success">已启用</Tag>
        : <Tag color="default">未启用</Tag>,
    },
    {
      title: '反代', key: 'proxy', width: 80,
      render: (_: unknown, r: Website) => r.proxy_enabled
        ? <Tooltip title={r.proxy_pass}><Tag color="blue">已启用</Tag></Tooltip>
        : <Tag color="default">关闭</Tag>,
    },
    {
      title: '状态', key: 'status', width: 80,
      render: (_: unknown, r: Website) => r.status === 'active'
        ? <Tag color="success">运行中</Tag>
        : <Tag color="error">已禁用</Tag>,
    },
    {
      title: '操作', key: 'action', width: 320,
      render: (_: unknown, record: Website) => (
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
              <Button icon={<ArrowLeftOutlined />} onClick={onGoBack}>返回</Button>
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
                  <Button icon={<StopOutlined />} danger loading={operating === `stop-${selectedServer.id}`} onClick={() => onStop(selectedServer)}>停止</Button>
                  <Button icon={<ReloadOutlined />} loading={operating === `restart-${selectedServer.id}`} onClick={() => onRestart(selectedServer)}>重启</Button>
                  <Button icon={<ReloadOutlined />} loading={operating === `reload-${selectedServer.id}`} onClick={() => onReload(selectedServer)}>重载配置</Button>
                </>
              )}
              {selectedServer.status === 'stopped' && (
                <Button type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${selectedServer.id}`} onClick={() => onStart(selectedServer)}>启动</Button>
              )}
              {selectedServer.status === 'not_installed' && (
                <Button type="primary" icon={<DownloadOutlined />} loading={operating === `install-${selectedServer.id}`} onClick={() => onInstall(selectedServer)}>安装</Button>
              )}
              {selectedServer.status !== 'not_installed' && (
                <>
                  <Button icon={<CodeOutlined />} onClick={onShowConfig}>配置文件</Button>
                  <Button icon={<FileTextOutlined />} onClick={onShowServiceLogs}>服务日志</Button>
                  <Button icon={<ToolOutlined />} loading={operating === `test-${selectedServer.id}`} onClick={() => onTestConfig(selectedServer)}>测试配置</Button>
                  <Popconfirm title="确定卸载？需要先删除所有网站。" onConfirm={() => onUninstall(selectedServer)}>
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
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => onRefreshServer(selectedServer.id)}>
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
            <Button icon={<ReloadOutlined />} loading={sitesLoading} onClick={() => { setSitesLoading(true); fetchWebsites(); }}>
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
          onChange={(key) => { setLogType(key); if (logSite) showLogs(logSite, key); }}
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
              加载中...
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
                onClick={() => browseTo(entry.path)}
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
