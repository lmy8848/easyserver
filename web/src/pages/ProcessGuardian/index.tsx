import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber,
  Switch, message, Popconfirm, Table, Empty, Tooltip, Tabs,
  Typography, Badge, Row, Col, Statistic,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  CaretRightOutlined, PauseOutlined, RedoOutlined,
  AppstoreOutlined, ClusterOutlined,
  ThunderboltOutlined, CloudServerOutlined, SettingOutlined,
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
    // 后端 ParseUnitMeta 已从 [Service] 段回填 command/args/dir/env/auto_restart，
    // 编辑时直接用，不再清空。
    form.setFieldsValue({
      name: s.name,
      description: s.description,
      command: s.command || '',
      args: s.args || '',
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
        command: values.command,
        args: values.args || '',
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

  const handleAction = async (name: string, action: 'start' | 'stop' | 'restart') => {
    const fn = action === 'start' ? serviceApi.start
      : action === 'stop' ? serviceApi.stop : serviceApi.restart;
    setOperating(`${action}-${name}`);
    try {
      await fn(name);
      message.success(`${action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'}成功`);
      fetch();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败');
    } finally {
      setOperating('');
    }
  };

  const columns = [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (t: string) => <Text strong>{t}</Text>,
    },
    {
      title: '描述', dataIndex: 'description', key: 'description', ellipsis: true,
      render: (t: string) => t ? <Text type="secondary">{t}</Text> : <Text type="secondary" style={{ fontSize: 12 }}>easyserver-{t}</Text>,
    },
    {
      title: '状态', key: 'status', width: 100,
      render: (_: unknown, r: Service) => {
        const cfg = STATUS_CONFIG[r.state] || STATUS_CONFIG['inactive']!;
        return <Badge status={cfg.color as any} text={cfg.label} />;
      },
    },
    {
      title: 'PID', dataIndex: 'pid', key: 'pid', width: 70,
      render: (pid: number) => pid > 0 ? pid : '-',
    },
    {
      title: '开机自启', dataIndex: 'enabled', key: 'enabled', width: 90,
      render: (en: boolean) => en ? <Tag color="blue">已启用</Tag> : <Tag>未启用</Tag>,
    },
    {
      title: '操作', key: 'action', width: 220,
      render: (_: unknown, r: Service) => {
        const isRunning = r.state === 'active';
        const isBusy = r.state === 'activating' || r.state === 'deactivating';
        return (
          <Space size="small">
            {!isRunning && (
              <Tooltip title="启动">
                <Button type="link" size="small" icon={<CaretRightOutlined />}
                  loading={operating === `start-${r.name}`} disabled={isBusy}
                  onClick={() => handleAction(r.name, 'start')} />
              </Tooltip>
            )}
            {isRunning && (
              <Tooltip title="停止">
                <Button type="link" size="small" icon={<PauseOutlined />}
                  loading={operating === `stop-${r.name}`} disabled={isBusy}
                  onClick={() => handleAction(r.name, 'stop')} />
              </Tooltip>
            )}
            <Tooltip title="重启">
              <Button type="link" size="small" icon={<RedoOutlined />}
                loading={operating === `restart-${r.name}`} disabled={isBusy}
                onClick={() => handleAction(r.name, 'restart')} />
            </Tooltip>
            <Tooltip title="编辑">
              <Button type="link" size="small" icon={<EditOutlined />}
                onClick={() => handleEdit(r)} />
            </Tooltip>
            <Popconfirm title="确定删除?" onConfirm={() => handleDelete(r.name)}>
              <Tooltip title="删除">
                <Button type="link" size="small" danger icon={<DeleteOutlined />}
                  disabled={isRunning} />
              </Tooltip>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

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
        <Form.Item name="command" label="启动命令" rules={[{ required: true, message: '请输入启动命令' }]}>
          <Input placeholder="例如: node /app/server.js" />
        </Form.Item>
        <Form.Item name="args" label="参数">
          <Input placeholder="命令行参数（空格分隔）" />
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
  const [actingService, setActingService] = useState<string | null>(null);
  const [searchText, setSearchText] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);

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

  const handleAction = async (name: string, action: string) => {
    setActingService(name);
    try {
      const fn = (action === 'start' ? serviceApi.start
        : action === 'stop' ? serviceApi.stop
        : action === 'restart' ? serviceApi.restart
        : action === 'enable' ? serviceApi.enable
        : serviceApi.disable);
      await fn(name);
      message.success(`服务 ${name} 已${action === 'start' ? '启动' : action === 'stop' ? '停止' : action === 'restart' ? '重启' : action === 'enable' ? '设置开机自启' : '取消开机自启'}`);
      fetch();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败');
    } finally {
      setActingService(null);
    }
  };

  const filtered = services.filter(s =>
    !searchText || s.name.toLowerCase().includes(searchText.toLowerCase()) ||
    s.description?.toLowerCase().includes(searchText.toLowerCase())
  );

  const columns = [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (t: string) => <Text strong>{t}</Text>,
    },
    {
      title: '描述', dataIndex: 'description', key: 'description', ellipsis: true,
      render: (t: string) => <Text type="secondary">{t}</Text>,
    },
    {
      title: '状态', key: 'state', width: 100,
      render: (_: unknown, r: Service) => {
        const cfg = STATUS_CONFIG[r.state] || { color: 'default', label: r.state };
        return <Badge status={cfg.color as any} text={cfg.label} />;
      },
    },
    {
      title: '开机自启', dataIndex: 'enabled', key: 'enabled', width: 100,
      render: (en: boolean, r: Service) => (
        <Switch size="small" checked={en} loading={actingService === r.name}
          onChange={(checked) => handleAction(r.name, checked ? 'enable' : 'disable')} />
      ),
    },
    {
      title: '操作', key: 'action', width: 180,
      render: (_: unknown, r: Service) => {
        // 保护面板自身服务，禁止操作避免锁死自己
        const isSelf = r.name === 'easyserver';
        return (
          <Space size="small">
            <Button size="small" icon={<CaretRightOutlined />}
              disabled={isSelf || r.state === 'active'} loading={actingService === r.name}
              onClick={() => handleAction(r.name, 'start')}>启动</Button>
            <Button size="small" icon={<PauseOutlined />}
              disabled={isSelf || r.state === 'inactive'} loading={actingService === r.name}
              onClick={() => handleAction(r.name, 'stop')}>停止</Button>
            <Button size="small" icon={<RedoOutlined />}
              disabled={isSelf} loading={actingService === r.name}
              onClick={() => handleAction(r.name, 'restart')}>重启</Button>
          </Space>
        );
      },
    },
  ];

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
          rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys as string[]) }}
          pagination={{ pageSize: 50, showSizeChanger: true, showTotal: (t) => `共 ${t} 个服务` }}
        />
      </Card>
    </>
  );
}
