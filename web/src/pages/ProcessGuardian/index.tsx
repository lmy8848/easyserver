import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber,
  Switch, message, Popconfirm, Table, Empty, Tooltip, Tabs,
  Typography, Badge, Row, Col, Statistic, Drawer, Descriptions,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  CaretRightOutlined, PauseOutlined, RedoOutlined,
  AppstoreOutlined, ClusterOutlined,
  ThunderboltOutlined, CloudServerOutlined, SettingOutlined,
  FileTextOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import type { Service, ManagedServiceSpec } from '../../types';
import { serviceApi } from '../../services/api';
import RuntimeVersionSelect from '../../components/RuntimeVersionSelect';

const { Text } = Typography;
const { TextArea } = Input;

const MODAL_TOP_OFFSET = 40;

// 表单中间类型：env 是 JSON 字符串（TextArea），runtime 是 {id,lang,exact} 对象。
// 提交时 handleSubmit 再转成 ManagedServiceSpec（env -> object, runtime -> 三字段）。
type ManagedServiceForm = Omit<ManagedServiceSpec, 'env' | 'runtime_version_id' | 'runtime_lang' | 'runtime_exact'> & {
  env: string;
  runtime?: { id: number; lang: string; exact: string };
};

const STATUS_CONFIG: Record<string, { color: string; label: string }> = {
  active: { color: 'green', label: '运行中' },
  inactive: { color: 'default', label: '已停止' },
  failed: { color: 'red', label: '失败' },
  activating: { color: 'processing', label: '启动中' },
  deactivating: { color: 'warning', label: '停止中' },
};

// formatBytes 把字节数格式化为人类可读（如 1.5 MB）。
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

// formatUptime 把秒数格式化为人类可读（如 2d 3h 10m）。
function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

// ============================================================
// 共用表格列构造：面板托管和系统服务复用，差异通过参数控制
// ============================================================

type ServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable';

interface ColumnProps {
  operating: string;                       // 操作中状态标记
  managed: boolean;                        // 托管 Tab 传 true，系统 Tab 传 false
  onAction: (name: string, action: ServiceAction) => void;
  onEdit?: (s: Service) => void;           // 托管才有
  onDelete?: (name: string) => void;       // 托管才有
  onLogs: (name: string) => void;
  onDetail: (s: Service) => void;
}

function buildServiceColumns(props: ColumnProps) {
  const { operating, managed, onAction, onEdit, onDelete, onLogs, onDetail } = props;

  const cols: object[] = [
    {
      title: '名称', dataIndex: 'name', key: 'name', ellipsis: true, width: 180,
      render: (t: string) => <Text strong style={{ fontSize: 13 }}>{t}</Text>,
    },
    {
      title: '描述', dataIndex: 'description', key: 'description', ellipsis: true, width: 160,
      render: (t: string) => t ? <Text type="secondary" style={{ fontSize: 12 }}>{t}</Text> : null,
    },
    {
      title: '状态', key: 'status', width: 120,
      render: (_: unknown, r: Service) => {
        const cfg = STATUS_CONFIG[r.state] || { color: 'default', label: r.state };
        const text = r.sub_state && r.sub_state !== r.state
          ? `${cfg.label} (${r.sub_state})` : cfg.label;
        return <Badge status={cfg.color as any} text={text} />;
      },
    },
    {
      title: 'PID', dataIndex: 'pid', key: 'pid', width: 70,
      render: (pid: number) => pid > 0 ? pid : '-',
    },
    {
      title: '内存', dataIndex: 'memory_bytes', key: 'memory', width: 85,
      render: (m: number) => m > 0 ? formatBytes(m) : '-',
    },
  ];

  // 系统服务 Tab：自启开关放列里（托管的在编辑表单里配）
  if (!managed) {
    cols.push({
      title: '自启', key: 'enabled', width: 70,
      render: (_: unknown, r: Service) => (
        <Switch size="small" checked={r.enabled}
          loading={operating === `enable-${r.name}` || operating === `disable-${r.name}`}
          disabled={r.name === 'easyserver'}
          onChange={(checked) => onAction(r.name, checked ? 'enable' : 'disable')} />
      ),
    });
  } else {
    // 托管 Tab：自启用 Tag 展示
    cols.push({
      title: '自启', dataIndex: 'enabled', key: 'enabled', width: 70,
      render: (en: boolean) => en ? <Tag color="blue" style={{ margin: 0 }}>是</Tag> : <Tag style={{ margin: 0 }}>否</Tag>,
    });
  }

  cols.push({
    title: '操作', key: 'action', width: managed ? 260 : 220, fixed: 'right' as const,
    render: (_: unknown, r: Service) => {
      const isRunning = r.state === 'active';
      const isBusy = r.state === 'activating' || r.state === 'deactivating';
      const isSelf = !managed && r.name === 'easyserver';
      const busy = (key: string) => operating === `${key}-${r.name}`;
      const disabledAny = isBusy || isSelf || operating.startsWith(`start-${r.name}`)
        || operating.startsWith(`stop-${r.name}`) || operating.startsWith(`restart-${r.name}`);

      return (
        <Space wrap>
          {/* 启动/停止互斥：只显示可操作的那个 */}
          {isRunning ? (
            <>
              <Button icon={<PauseOutlined />} size="small"
                loading={busy('stop')} disabled={disabledAny}
                onClick={() => onAction(r.name, 'stop')}>停止</Button>
              <Button icon={<RedoOutlined />} size="small"
                loading={busy('restart')} disabled={disabledAny}
                onClick={() => onAction(r.name, 'restart')}>重启</Button>
            </>
          ) : (
            <Button icon={<CaretRightOutlined />} size="small"
              loading={busy('start')} disabled={disabledAny}
              onClick={() => onAction(r.name, 'start')}>启动</Button>
          )}
          {/* 辅助操作：纯图标 + Tooltip */}
          <Tooltip title="日志">
            <Button icon={<FileTextOutlined />} size="small" onClick={() => onLogs(r.name)} />
          </Tooltip>
          <Tooltip title="详情">
            <Button icon={<InfoCircleOutlined />} size="small" onClick={() => onDetail(r)} />
          </Tooltip>
          {managed && onEdit && (
            <Tooltip title="编辑">
              <Button icon={<EditOutlined />} size="small" onClick={() => onEdit(r)} />
            </Tooltip>
          )}
          {managed && onDelete && (
            <Popconfirm title="确定删除此服务？" okText="删除" cancelText="取消" onConfirm={() => onDelete(r.name)}>
              <Button icon={<DeleteOutlined />} size="small" danger disabled={isRunning} />
            </Popconfirm>
          )}
        </Space>
      );
    },
  });

  return cols;
}

export default function ProcessGuardian() {
  const [activeTab, setActiveTab] = useState<'managed' | 'system'>('managed');

  return (
    <>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'managed' | 'system')}
        items={[
          {
            key: 'managed',
            label: <span><ClusterOutlined /> 面板托管</span>,
          },
          {
            key: 'system',
            label: <span><CloudServerOutlined /> 系统服务</span>,
          },
        ]}
        style={{ marginBottom: 16 }}
      />
      {activeTab === 'managed' ? <ManagedTab /> : <SystemTab />}
    </>
  );
}

// ============================================================
// 面板托管 Tab
// ============================================================

function ManagedTab() {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(false);
  const [operating, setOperating] = useState<string>('');

  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState<Service | null>(null);
  const [form] = Form.useForm<ManagedServiceForm>();

  const fetch = useCallback(async () => {
    setLoading(true);
    try {
      const res = await serviceApi.list();
      // 只展示托管服务（managed=true）
      setServices((res.data?.data || []).filter(s => s.managed));
    } catch {
      message.error('获取托管服务列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetch(); }, [fetch]);

  // 自动刷新
  useEffect(() => {
    const t = setInterval(fetch, 5000);
    return () => clearInterval(t);
  }, [fetch]);

  const handleCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ auto_restart: true, max_restarts: 10, restart_delay: 5, stop_timeout: 10, auto_start: false, env: '' });
    setModalVisible(true);
  };

  const handleEdit = (s: Service) => {
    setEditing(s);
    // 后端 ParseUnitMeta 已从 [Service] 段回填 exec_start/dir/env/auto_restart，
    // 编辑时直接用。
    form.setFieldsValue({
      name: s.name,
      description: s.description,
      exec_start: s.exec_start || '',
      dir: s.dir || '',
      env: s.env && Object.keys(s.env).length > 0 ? JSON.stringify(s.env, null, 2) : '',
      auto_restart: s.auto_restart,
      max_restarts: 10,
      restart_delay: 5,
      stop_timeout: 10,
      auto_start: s.enabled,
      runtime: (s.runtime_version_id && s.runtime_lang && s.runtime_exact)
        ? { id: s.runtime_version_id, lang: s.runtime_lang, exact: s.runtime_exact }
        : undefined,
    });
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      // env 表单是 TextArea（JSON 字符串），parse 成 object 给后端。
      let env: Record<string, string> = {};
      if (typeof values.env === 'string' && values.env.trim()) {
        try {
          env = JSON.parse(values.env);
        } catch {
          message.error('环境变量不是合法的 JSON');
          return;
        }
      }
      // runtime 表单存的是 {id,lang,exact} 对象，拆成三字段给后端。
      const rt = values.runtime as { id: number; lang: string; exact: string } | undefined;
      const spec: ManagedServiceSpec = {
        name: values.name,
        description: values.description,
        exec_start: values.exec_start,
        dir: values.dir || '',
        env,
        auto_restart: values.auto_restart,
        max_restarts: values.max_restarts,
        restart_delay: values.restart_delay,
        stop_timeout: values.stop_timeout,
        auto_start: values.auto_start,
        runtime_version_id: rt?.id || 0,
        runtime_lang: rt?.lang || '',
        runtime_exact: rt?.exact || '',
      };
      if (editing) {
        await serviceApi.update(editing.name, spec);
        message.success('更新成功');
      } else {
        await serviceApi.create(spec);
        message.success('创建成功');
      }
      setModalVisible(false);
      fetch();
    } catch (e: any) {
      if (e?.errorFields) return;
      message.error(e instanceof Error ? e.message : '操作失败');
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await serviceApi.delete(name);
      message.success('已删除');
      fetch();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '删除失败');
    }
  };

  const handleAction = async (name: string, action: ServiceAction) => {
    const fn = action === 'start' ? serviceApi.start
      : action === 'stop' ? serviceApi.stop
      : action === 'restart' ? serviceApi.restart
      : action === 'enable' ? serviceApi.enable : serviceApi.disable;
    setOperating(`${action}-${name}`);
    try {
      await fn(name);
      const label = action === 'start' ? '启动' : action === 'stop' ? '停止'
        : action === 'restart' ? '重启' : action === 'enable' ? '设置开机自启' : '取消开机自启';
      message.success(`${label}成功`);
      fetch();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败');
    } finally {
      setOperating('');
    }
  };

  // 日志 Drawer
  const [logService, setLogService] = useState<string | null>(null);
  const [logs, setLogs] = useState<Array<{ time: string; message: string; priority: string }>>([]);
  const [logLoading, setLogLoading] = useState(false);

  const fetchLogs = useCallback(async (name: string) => {
    setLogLoading(true);
    try {
      const res = await serviceApi.getLogs(name, 200);
      setLogs(res.data?.data?.lines || []);
    } catch {
      setLogs([]);
    } finally {
      setLogLoading(false);
    }
  }, []);

  const openLogs = (name: string) => {
    setLogService(name);
    fetchLogs(name);
  };

  // 详情 Drawer
  const [detailService, setDetailService] = useState<Service | null>(null);

  const columns = buildServiceColumns({
    operating,
    managed: true,
    onAction: handleAction,
    onEdit: handleEdit,
    onDelete: handleDelete,
    onLogs: openLogs,
    onDetail: setDetailService,
  });

  const runningCount = services.filter(s => s.state === 'active').length;
  const stoppedCount = services.filter(s => s.state === 'inactive').length;
  const failedCount = services.filter(s => s.state === 'failed').length;

  return (
    <>
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card size="small"><Statistic title="托管总数" value={services.length} prefix={<AppstoreOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card size="small"><Statistic title="运行中" value={runningCount} styles={{ content: { color: '#3f8600' } }} prefix={<CaretRightOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card size="small"><Statistic title="已停止" value={stoppedCount} prefix={<PauseOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card size="small"><Statistic title="异常" value={failedCount} styles={{ content: { color: '#cf1322' } }} prefix={<ThunderboltOutlined />} /></Card>
        </Col>
      </Row>

      <Card
        title={<Space><ClusterOutlined />面板托管服务</Space>}
        extra={
          <Space>
            <Button size="small" icon={<ReloadOutlined />} onClick={fetch}>刷新</Button>
            <Button size="small" type="primary" icon={<PlusOutlined />} onClick={handleCreate}>添加服务</Button>
          </Space>
        }
      >
        <Table
          rowKey="name"
          columns={columns}
          dataSource={services}
          loading={loading}
          size="small"
          pagination={false}
          scroll={{ x: 700 }}
          locale={{ emptyText: <Empty description="暂无托管服务，点击「添加服务」开始" /> }}
        />
      </Card>

      <ManagedServiceModal
        visible={modalVisible}
        editing={editing}
        form={form}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
      />

      {/* 日志 Drawer */}
      <Drawer
        title={<Space><FileTextOutlined />{logService} 日志</Space>}
        open={!!logService}
        onClose={() => setLogService(null)}
        width={720}
        extra={<Button size="small" icon={<ReloadOutlined />} loading={logLoading}
          onClick={() => logService && fetchLogs(logService)}>刷新</Button>}
      >
        <pre style={{ fontSize: 12, lineHeight: 1.6, maxHeight: 'calc(100vh - 160px)', overflow: 'auto', margin: 0, padding: 8, background: '#fafafa', borderRadius: 4 }}>
          {logs.length === 0
            ? (logLoading ? '加载中...' : '暂无日志')
            : logs.map((l) => `[${l.time}] ${l.message}`).join('\n')}
        </pre>
      </Drawer>

      {/* 详情 Drawer */}
      <Drawer
        title={<Space><InfoCircleOutlined />{detailService?.name} 详情</Space>}
        open={!!detailService}
        onClose={() => setDetailService(null)}
        width={560}
      >
        {detailService && (
          <Descriptions column={1} bordered size="small" labelStyle={{ width: 120 }}>
            <Descriptions.Item label="状态">
              <Badge status={(STATUS_CONFIG[detailService.state]?.color) as any}
                text={`${STATUS_CONFIG[detailService.state]?.label || detailService.state} (${detailService.sub_state})`} />
            </Descriptions.Item>
            <Descriptions.Item label="描述">{detailService.description || '-'}</Descriptions.Item>
            <Descriptions.Item label="PID">{detailService.pid > 0 ? detailService.pid : '-'}</Descriptions.Item>
            <Descriptions.Item label="内存">{detailService.memory_bytes > 0 ? formatBytes(detailService.memory_bytes) : '-'}</Descriptions.Item>
            <Descriptions.Item label="CPU">{detailService.cpu_percent > 0 ? `${detailService.cpu_percent.toFixed(1)}%` : '-'}</Descriptions.Item>
            <Descriptions.Item label="运行时长">{detailService.uptime_seconds > 0 ? formatUptime(detailService.uptime_seconds) : '-'}</Descriptions.Item>
            <Descriptions.Item label="开机自启">{detailService.enabled ? '是' : '否'}</Descriptions.Item>
            {detailService.managed && <>
              <Descriptions.Item label="启动命令">
                <Text code copyable style={{ fontSize: 12, wordBreak: 'break-all' }}>{detailService.exec_start || '-'}</Text>
              </Descriptions.Item>
              {detailService.dir && <Descriptions.Item label="工作目录">{detailService.dir}</Descriptions.Item>}
              {detailService.runtime_version_id > 0 && (
                <Descriptions.Item label="运行时">{detailService.runtime_lang}@{detailService.runtime_exact}</Descriptions.Item>
              )}
              {detailService.env && Object.keys(detailService.env).length > 0 && (
                <Descriptions.Item label="环境变量">
                  <pre style={{ margin: 0, fontSize: 12 }}>
                    {Object.entries(detailService.env).map(([k, v]) => `${k}=${v}`).join('\n')}
                  </pre>
                </Descriptions.Item>
              )}
            </>}
          </Descriptions>
        )}
      </Drawer>
    </>
  );
}

function ManagedServiceModal({ visible, editing, form, onOk, onCancel }: {
  visible: boolean;
  editing: Service | null;
  form: ReturnType<typeof Form.useForm<ManagedServiceForm>>[0];
  onOk: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal
      title={editing ? '编辑托管服务' : '添加托管服务'}
      open={visible}
      onOk={onOk}
      onCancel={onCancel}
      width={600}
      destroyOnHidden
      style={{ top: MODAL_TOP_OFFSET }}
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item name="name" label="服务名称" rules={[
          { required: true, message: '请输入服务名称' },
          { pattern: /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/, message: '只能包含小写字母、数字、连字符，不能以连字符开头/结尾' },
        ]} extra="生成 unit: easyserver-<名称>.service">
          <Input placeholder="例如: my-app" disabled={!!editing} />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input placeholder="可选，显示用" />
        </Form.Item>
        <Form.Item name="exec_start" label="启动命令" rules={[{ required: true, message: '请输入启动命令' }]}
          extra="完整命令，如 node /app/server.js --port 3000（绑定运行时后自动前置 mise exec）">
          <Input placeholder="例如: node /app/server.js --port 3000" />
        </Form.Item>
        <Form.Item
          name="runtime"
          label="运行时版本"
          extra="启动命令会自动通过 mise exec <lang>@<exact> 包裹"
        >
          <RuntimeVersionSelect />
        </Form.Item>
        <Form.Item name="dir" label="工作目录">
          <Input placeholder="例如: /app" />
        </Form.Item>
        <Form.Item name="env" label="环境变量 (JSON)">
          <TextArea rows={3} placeholder='例如: {"NODE_ENV": "production", "PORT": "3000"}' />
        </Form.Item>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="auto_restart" label="自动重启" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="max_restarts" label="最大重启次数">
              <InputNumber min={0} max={100} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="restart_delay" label="重启延迟(秒)">
              <InputNumber min={1} max={300} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="stop_timeout" label="停止超时(秒)">
              <InputNumber min={1} max={60} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="auto_start" label="开机自启" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Modal>
  );
}

// ============================================================
// 系统服务 Tab（排除 easyserver-* 托管服务）
// ============================================================

function SystemTab() {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [operating, setOperating] = useState<string>('');
  const [searchText, setSearchText] = useState('');

  // 日志 Drawer
  const [logService, setLogService] = useState<string | null>(null);
  const [logs, setLogs] = useState<Array<{ time: string; message: string; priority: string }>>([]);
  const [logLoading, setLogLoading] = useState(false);
  // 详情 Drawer
  const [detailService, setDetailService] = useState<Service | null>(null);

  const fetch = useCallback(async () => {
    setLoading(true);
    try {
      const res = await serviceApi.list();
      // 排除面板托管服务（managed=true）
      setServices((res.data?.data || []).filter(s => !s.managed));
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetch(); }, [fetch]);

  const fetchLogs = useCallback(async (name: string) => {
    setLogLoading(true);
    try {
      const res = await serviceApi.getLogs(name, 200);
      setLogs(res.data?.data?.lines || []);
    } catch {
      setLogs([]);
    } finally {
      setLogLoading(false);
    }
  }, []);

  const openLogs = (name: string) => {
    setLogService(name);
    fetchLogs(name);
  };

  const handleAction = async (name: string, action: ServiceAction) => {
    setOperating(`${action}-${name}`);
    try {
      const fn = action === 'start' ? serviceApi.start
        : action === 'stop' ? serviceApi.stop
        : action === 'restart' ? serviceApi.restart
        : action === 'enable' ? serviceApi.enable
        : serviceApi.disable;
      await fn(name);
      const label = action === 'start' ? '启动' : action === 'stop' ? '停止'
        : action === 'restart' ? '重启' : action === 'enable' ? '设置开机自启' : '取消开机自启';
      message.success(`${label}成功`);
      fetch();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败');
    } finally {
      setOperating('');
    }
  };

  const filtered = services.filter(s =>
    !searchText || s.name.toLowerCase().includes(searchText.toLowerCase()) ||
    s.description?.toLowerCase().includes(searchText.toLowerCase())
  );

  const columns = buildServiceColumns({
    operating,
    managed: false,
    onAction: handleAction,
    onLogs: openLogs,
    onDetail: setDetailService,
  });

  const activeCount = services.filter(s => s.state === 'active').length;
  const failedCount = services.filter(s => s.state === 'failed').length;

  return (
    <>
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={8}><Card size="small"><Statistic title="系统服务总数" value={services.length} prefix={<SettingOutlined />} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="运行中" value={activeCount} styles={{ content: { color: '#3f8600' } }} prefix={<CaretRightOutlined />} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="失败" value={failedCount} styles={{ content: { color: '#cf1322' } }} prefix={<ThunderboltOutlined />} /></Card></Col>
      </Row>

      <Card
        title={<Space><CloudServerOutlined />系统服务</Space>}
        extra={
          <Space>
            <Input.Search placeholder="搜索服务名" allowClear size="small"
              style={{ width: 200 }} value={searchText} onChange={e => setSearchText(e.target.value)} />
            <Button size="small" icon={<ReloadOutlined />} onClick={fetch}>刷新</Button>
          </Space>
        }
      >
        <Table
          rowKey="name"
          columns={columns}
          dataSource={filtered}
          loading={loading}
          size="small"
          scroll={{ x: 800 }}
          pagination={{ pageSize: 50, showSizeChanger: true, showTotal: (t) => `共 ${t} 个服务` }}
        />
      </Card>

      {/* 日志 Drawer */}
      <Drawer
        title={<Space><FileTextOutlined />{logService} 日志</Space>}
        open={!!logService}
        onClose={() => setLogService(null)}
        width={720}
        extra={<Button size="small" icon={<ReloadOutlined />} loading={logLoading}
          onClick={() => logService && fetchLogs(logService)}>刷新</Button>}
      >
        <pre style={{ fontSize: 12, lineHeight: 1.6, maxHeight: 'calc(100vh - 160px)', overflow: 'auto', margin: 0, padding: 8, background: '#fafafa', borderRadius: 4 }}>
          {logs.length === 0
            ? (logLoading ? '加载中...' : '暂无日志')
            : logs.map((l) => `[${l.time}] ${l.message}`).join('\n')}
        </pre>
      </Drawer>

      {/* 详情 Drawer */}
      <Drawer
        title={<Space><InfoCircleOutlined />{detailService?.name} 详情</Space>}
        open={!!detailService}
        onClose={() => setDetailService(null)}
        width={560}
      >
        {detailService && (
          <Descriptions column={1} bordered size="small" labelStyle={{ width: 120 }}>
            <Descriptions.Item label="状态">
              <Badge status={(STATUS_CONFIG[detailService.state]?.color) as any}
                text={`${STATUS_CONFIG[detailService.state]?.label || detailService.state} (${detailService.sub_state})`} />
            </Descriptions.Item>
            <Descriptions.Item label="描述">{detailService.description || '-'}</Descriptions.Item>
            <Descriptions.Item label="PID">{detailService.pid > 0 ? detailService.pid : '-'}</Descriptions.Item>
            <Descriptions.Item label="内存">{detailService.memory_bytes > 0 ? formatBytes(detailService.memory_bytes) : '-'}</Descriptions.Item>
            <Descriptions.Item label="CPU">{detailService.cpu_percent > 0 ? `${detailService.cpu_percent.toFixed(1)}%` : '-'}</Descriptions.Item>
            <Descriptions.Item label="运行时长">{detailService.uptime_seconds > 0 ? formatUptime(detailService.uptime_seconds) : '-'}</Descriptions.Item>
            <Descriptions.Item label="开机自启">{detailService.enabled ? '是' : '否'}</Descriptions.Item>
          </Descriptions>
        )}
      </Drawer>
    </>
  );
}
