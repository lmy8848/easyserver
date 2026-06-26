import { useState, useEffect } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Popconfirm, Modal, Form,
  Input, Select, Tabs, Descriptions, Tooltip,
} from 'antd';
import {
  PlayCircleOutlined, PauseCircleOutlined, StopOutlined,
  ReloadOutlined, DeleteOutlined, PlusOutlined,
  CodeOutlined, InfoCircleOutlined, SettingOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
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
  const [createForm] = Form.useForm();
  const [execForm] = Form.useForm();
  const [templates, setTemplates] = useState<ImageCategory[]>([]);

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

  const loadTemplates = async () => {
    try {
      const res = await api.get('/templates/docker-images');
      setTemplates(res.data?.data?.categories || []);
    } catch {
      // ignore
    }
  };

  useEffect(() => { loadContainers(); loadTemplates(); }, []);

  const handleAction = async (action: string, id: string) => {
    try {
      await api.post(`/containers/${id}/${action}`);
      message.success('操作成功');
      setLoading(true);
      loadContainers();
    } catch {
      message.error('操作失败');
    }
  };

  const handleRemove = async (id: string, force: boolean) => {
    try {
      await api.delete(`/containers/${id}?force=${force}`);
      message.success('容器已删除');
      setLoading(true);
      loadContainers();
    } catch {
      message.error('删除失败');
    }
  };

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      await api.post('/containers', values);
      message.success('容器创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      setLoading(true);
      loadContainers();
    } catch {
      message.error('创建失败');
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
      render: (name: string) => name.replace(/^\//, ''),
    },
    { title: '镜像', dataIndex: 'image', key: 'image' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <Tag color={getStatusColor(status)}>{status}</Tag>,
    },
    {
      title: '端口',
      dataIndex: 'ports',
      key: 'ports',
      render: (ports: Container['ports']) => (
        <Space wrap>
          {ports?.map((p, i) => (
            <Tag key={i}>{p.host_port}:{p.container_port}/{p.protocol}</Tag>
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
          {!record.status.includes('running') ? (
            <Button icon={<PlayCircleOutlined />} size="small" onClick={() => handleAction('start', record.id)}>启动</Button>
          ) : (
            <>
              <Button icon={<StopOutlined />} size="small" onClick={() => handleAction('stop', record.id)}>停止</Button>
              <Button icon={<ReloadOutlined />} size="small" onClick={() => handleAction('restart', record.id)}>重启</Button>
              <Button icon={<PauseCircleOutlined />} size="small" onClick={() => handleAction('pause', record.id)}>暂停</Button>
            </>
          )}
          <Tooltip title="执行命令">
            <Button icon={<CodeOutlined />} size="small" onClick={() => { setSelectedContainer(record.id); setExecVisible(true); }} />
          </Tooltip>
          <Tooltip title="日志">
            <Button icon={<InfoCircleOutlined />} size="small" onClick={() => handleLogs(record.id)} />
          </Tooltip>
          <Tooltip title="资源监控">
            <Button icon={<SettingOutlined />} size="small" onClick={() => handleStats(record.id)} />
          </Tooltip>
          <Popconfirm title="确定删除此容器？" onConfirm={() => handleRemove(record.id, true)} okText="删除" cancelText="取消">
            <Button icon={<DeleteOutlined />} size="small" danger />
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
                        createForm.setFieldsValue({ image: img.image, name: img.name.toLowerCase().replace(/\s+/g, '-') });
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
      <Modal title="创建容器" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)} width={600}>
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
      <Modal title="在容器中执行命令" open={execVisible} onOk={handleExec} onCancel={() => setExecVisible(false)}>
        <Form form={execForm} layout="vertical">
          <Form.Item name="command" label="命令" rules={[{ required: true }]}><Input placeholder="ls -la" /></Form.Item>
        </Form>
      </Modal>

      {/* Logs Modal */}
      <Modal title={`容器日志 - ${selectedContainer}`} open={logsVisible} onCancel={() => setLogsVisible(false)} footer={null} width={800}>
        <pre style={{ maxHeight: 500, overflow: 'auto', background: '#f5f5f5', padding: 16, fontSize: 12, whiteSpace: 'pre-wrap' }}>
          {logs || '暂无日志'}
        </pre>
      </Modal>

      {/* Stats Modal */}
      <Modal title={`资源监控 - ${selectedContainer}`} open={statsVisible} onCancel={() => setStatsVisible(false)} footer={null} width={600}>
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
