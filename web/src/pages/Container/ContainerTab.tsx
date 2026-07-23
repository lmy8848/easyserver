import { useState, useEffect } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Popconfirm, Modal, Form,
  Input, Select, Tabs, Descriptions, Tooltip,
} from 'antd';
import {
  PlayCircleOutlined, PauseCircleOutlined, StopOutlined,
  ReloadOutlined, DeleteOutlined, PlusOutlined,
  CodeOutlined, InfoCircleOutlined, LineChartOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import { DOCKER_IMAGE_TEMPLATES } from '../../constants/templates';
import type { Container, ContainerStats, ImageCategory } from './types';
import { formatBytes, getStatusColor } from './types';

export default function ContainerTab() {
  const [containers, setContainers] = useState<Container[]>([]);
  const [loading, setLoading] = useState(true);
  const [createVisible, setCreateVisible] = useState(false);
  const [execVisible, setExecVisible] = useState(false);
  const [logsVisible, setLogsVisible] = useState(false);
  const [statsVisible, setStatsVisible] = useState(false);
  const [selectedContainer, setSelectedContainer] = useState('');
  const [logs, setLogs] = useState('');
  const [stats, setStats] = useState<ContainerStats | null>(null);
  const [actionLoading, setActionLoading] = useState<string>('');
  const [createForm] = Form.useForm();
  const [execForm] = Form.useForm();
  const templates: ImageCategory[] = DOCKER_IMAGE_TEMPLATES;

  const loadContainers = async () => {
    try {
      const res = await api.get('/containers?all=true');
      setContainers(res.data?.data?.containers || []);
    } catch {
      message.error('加载容器列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadContainers(); }, []);

  const handleAction = async (action: string, id: string) => {
    setActionLoading(`${id}:${action}`);
    try {
      await api.post(`/containers/${id}/${action}`);
      message.success('操作成功');
      await loadContainers();
    } catch {
      message.error('操作失败');
    } finally {
      setActionLoading('');
    }
  };

  const handleRemove = async (id: string, force: boolean) => {
    setActionLoading(`${id}:remove`);
    try {
      await api.delete(`/containers/${id}?force=${force}`);
      message.success('容器已删除');
      await loadContainers();
    } catch {
      message.error('删除失败');
    } finally {
      setActionLoading('');
    }
  };

  const [createLoading, setCreateLoading] = useState(false);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreateLoading(true);
      const res = await api.post('/containers', values, { timeout: 600000 }); // 10 min: docker pull may take time
      const resultData = res.data?.data;
      const createdId = resultData?.id || resultData; // Backend might return { id: ... } or string
      message.success(createdId ? `容器创建成功 (ID: ${String(createdId).substring(0, 12)})` : '容器创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      await loadContainers();
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { message?: string } } };
      const errMsg = axiosErr.response?.data?.message || (err instanceof Error ? err.message : '创建失败');
      message.error(`创建失败: ${errMsg}`);
    } finally {
      setCreateLoading(false);
    }
  };

  const handleExec = async () => {
    try {
      const values = await execForm.validateFields();
      const res = await api.post(`/containers/${selectedContainer}/exec`, values);
      Modal.info({
        title: '执行结果',
        content: <pre style={{ maxHeight: 400, overflow: 'auto' }}>{res.data?.data?.output}</pre>,
        width: 600,
      });
      setExecVisible(false);
      execForm.resetFields();
    } catch {
      message.error('执行失败');
    }
  };

  const handleLogs = async (id: string) => {
    try {
      const res = await api.get(`/containers/${id}/logs?tail=200`);
      setLogs(res.data?.data?.logs || '');
      setSelectedContainer(id);
      setLogsVisible(true);
    } catch {
      message.error('获取日志失败');
    }
  };

  const handleStats = async (id: string) => {
    try {
      const res = await api.get(`/containers/${id}/stats`);
      setStats(res.data?.data);
      setSelectedContainer(id);
      setStatsVisible(true);
    } catch {
      message.error('获取状态失败');
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => name || '-',
    },
    { title: '镜像', dataIndex: 'image', key: 'image' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string, record: Container) => <Tag color={getStatusColor(record.state)}>{status}</Tag>,
    },
    {
      title: '端口',
      dataIndex: 'ports',
      key: 'ports',
      render: (ports: Container['ports']) => (
        <Space wrap>
          {ports?.map((p, i) => (
            <Tag key={i}>{p.container_port ? `${p.host_port}:${p.container_port}/${p.protocol}` : p.host_port}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 360,
      render: (_: unknown, record: Container) => (
        <Space wrap>
          {record.state === 'running' ? (
            <>
              <Button icon={<StopOutlined />} size="small" 
                loading={actionLoading === `${record.id}:stop`}
                disabled={actionLoading.startsWith(`${record.id}:`)}
                onClick={() => handleAction('stop', record.id)}>停止</Button>
              <Button icon={<ReloadOutlined />} size="small" 
                loading={actionLoading === `${record.id}:restart`}
                disabled={actionLoading.startsWith(`${record.id}:`)}
                onClick={() => handleAction('restart', record.id)}>重启</Button>
              <Button icon={<PauseCircleOutlined />} size="small" 
                loading={actionLoading === `${record.id}:pause`}
                disabled={actionLoading.startsWith(`${record.id}:`)}
                onClick={() => handleAction('pause', record.id)}>暂停</Button>
            </>
          ) : record.state === 'paused' ? (
            <Button icon={<PlayCircleOutlined />} size="small" 
              loading={actionLoading === `${record.id}:unpause`}
              disabled={actionLoading.startsWith(`${record.id}:`)}
              onClick={() => handleAction('unpause', record.id)}>恢复</Button>
          ) : (
            <Button icon={<PlayCircleOutlined />} size="small" 
              loading={actionLoading === `${record.id}:start`}
              disabled={actionLoading.startsWith(`${record.id}:`)}
              onClick={() => handleAction('start', record.id)}>启动</Button>
          )}
          <Tooltip title="执行命令">
            <Button icon={<CodeOutlined />} size="small" onClick={() => { setSelectedContainer(record.id); setExecVisible(true); }} />
          </Tooltip>
          <Tooltip title="日志">
            <Button icon={<InfoCircleOutlined />} size="small" onClick={() => handleLogs(record.id)} />
          </Tooltip>
          <Tooltip title="资源监控">
            <Button icon={<LineChartOutlined />} size="small" onClick={() => handleStats(record.id)} />
          </Tooltip>
          <Popconfirm title="确定删除此容器？" onConfirm={() => handleRemove(record.id, true)} okText="删除" cancelText="取消">
            <Button icon={<DeleteOutlined />} size="small" danger 
              loading={actionLoading === `${record.id}:remove`}
              disabled={actionLoading.startsWith(`${record.id}:`)} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      {/* 常用镜像快速部署 */}
      {templates.length > 0 && (
        <Card title="常用镜像" style={{ marginBottom: 16 }}>
          <Tabs
            size="small"
            items={templates.map(cat => ({
              key: cat.name,
              label: cat.name,
              children: (
                <Space wrap>
                  {cat.images.map(img => (
                    <Card key={img.image} size="small" hoverable style={{ width: 200 }}
                      onClick={() => {
                        const parsedName = img.name ? String(img.name).toLowerCase().replace(/[^a-z0-9_.-]/g, '-').replace(/-+/g, '-').replace(/^[^a-z0-9]+/, '').replace(/[^a-z0-9_.-]+$/, '') : '';
                        createForm.setFieldsValue({ image: img.image, name: parsedName });
                        setCreateVisible(true);
                      }}
                    >
                      <Card.Meta
                        title={img.name}
                        description={<>
                          <div style={{ fontSize: 12, color: '#666', marginBottom: 4 }}>{img.description}</div>
                          <Tag>{img.image}</Tag>
                        </>}
                      />
                    </Card>
                  ))}
                </Space>
              ),
            }))}
          />
        </Card>
      )}

      <Card
        extra={
          <Space>
            <Button icon={<PlusOutlined />} type="primary" onClick={() => setCreateVisible(true)}>创建容器</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); loadContainers(); }}>刷新</Button>
          </Space>
        }
      >
        <Table columns={columns} dataSource={containers} rowKey="id" loading={loading} locale={{ emptyText: '暂无容器' }} />
      </Card>

      {/* Create Modal */}
      <Modal title="创建容器" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)} width={600} destroyOnHidden confirmLoading={createLoading}>
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="容器名称"><Input placeholder="my-container" /></Form.Item>
          <Form.Item name="image" label="镜像" rules={[{ required: true }]}><Input placeholder="nginx:latest" /></Form.Item>
          <Form.Item name="command" label="命令"><Input placeholder="可选" /></Form.Item>
          <Form.Item name="restart_policy" label="重启策略">
            <Select placeholder="选择重启策略">
              <Select.Option value="no">不重启</Select.Option>
              <Select.Option value="always">总是重启</Select.Option>
              <Select.Option value="on-failure">失败时重启</Select.Option>
              <Select.Option value="unless-stopped">除非手动停止</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* Exec Modal */}
      <Modal title="在容器中执行命令" open={execVisible} onOk={handleExec} onCancel={() => setExecVisible(false)} destroyOnHidden>
        <Form form={execForm} layout="vertical">
          <Form.Item name="command" label="命令" rules={[{ required: true }]}><Input placeholder="ls -la" /></Form.Item>
        </Form>
      </Modal>

      {/* Logs Modal */}
      <Modal title={`容器日志 - ${selectedContainer}`} open={logsVisible} onCancel={() => setLogsVisible(false)} footer={null} width={800} destroyOnHidden>
        <pre style={{ maxHeight: 500, overflow: 'auto', background: '#f5f5f5', padding: 16, fontSize: 12, whiteSpace: 'pre-wrap' }}>
          {logs || '暂无日志'}
        </pre>
      </Modal>

      {/* Stats Modal */}
      <Modal title={`资源监控 - ${selectedContainer}`} open={statsVisible} onCancel={() => setStatsVisible(false)} footer={null} width={600} destroyOnHidden>
        {stats && (
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="CPU 使用率">{stats.cpu_percent.toFixed(2)}%</Descriptions.Item>
            <Descriptions.Item label="内存使用率">{stats.mem_percent.toFixed(2)}%</Descriptions.Item>
            <Descriptions.Item label="内存使用">{formatBytes(stats.mem_usage)}</Descriptions.Item>
            <Descriptions.Item label="内存限制">{formatBytes(stats.mem_limit)}</Descriptions.Item>
            <Descriptions.Item label="网络接收">{formatBytes(stats.net_rx)}</Descriptions.Item>
            <Descriptions.Item label="网络发送">{formatBytes(stats.net_tx)}</Descriptions.Item>
            <Descriptions.Item label="磁盘读取">{formatBytes(stats.block_read)}</Descriptions.Item>
            <Descriptions.Item label="磁盘写入">{formatBytes(stats.block_write)}</Descriptions.Item>
            <Descriptions.Item label="进程数">{stats.pids}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </>
  );
}
