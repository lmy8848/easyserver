import { useState, useEffect } from 'react';
import { Card, Tabs, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Popconfirm } from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  SyncOutlined,
  ApiOutlined,
} from '@ant-design/icons';
import api from '../services/api';

interface DeployServer {
  id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  auth_type: string;
  status: string;
  last_ping: string;
  created_at: string;
}

interface DeployTask {
  id: number;
  server_id: number;
  server_name: string;
  name: string;
  type: string;
  source_path: string;
  dest_path: string;
  command: string;
  status: string;
  result: string;
  created_at: string;
}

interface DeployVersion {
  id: number;
  server_id: number;
  server_name: string;
  version: string;
  files: string;
  created_at: string;
}

export default function Deploy() {
  const [servers, setServers] = useState<DeployServer[]>([]);
  const [tasks, setTasks] = useState<DeployTask[]>([]);
  const [versions, setVersions] = useState<DeployVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [serverModalVisible, setServerModalVisible] = useState(false);
  const [taskModalVisible, setTaskModalVisible] = useState(false);
  const [editingServer, setEditingServer] = useState<DeployServer | null>(null);
  const [form] = Form.useForm();

  const fetchServers = async () => {
    try {
      const res = await api.get('/deploy/servers');
      setServers(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch servers:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchTasks = async () => {
    try {
      const res = await api.get('/deploy/tasks');
      setTasks(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch tasks:', error);
    }
  };

  useEffect(() => {
    fetchServers();
    fetchTasks();
  }, []);

  const fetchVersions = async (serverId: number) => {
    try {
      const res = await api.get(`/deploy/versions?server_id=${serverId}`);
      setVersions(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch versions:', error);
    }
  };

  const handleCreateServer = () => {
    setEditingServer(null);
    form.resetFields();
    setServerModalVisible(true);
  };

  const handleEditServer = (server: DeployServer) => {
    setEditingServer(server);
    form.setFieldsValue(server);
    setServerModalVisible(true);
  };

  const handleSaveServer = async () => {
    try {
      const values = await form.validateFields();
      if (editingServer) {
        await api.put(`/deploy/servers/${editingServer.id}`, values);
        message.success('服务器已更新');
      } else {
        await api.post('/deploy/servers', values);
        message.success('服务器已添加');
      }
      setServerModalVisible(false);
      setLoading(true);
      fetchServers();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const handleDeleteServer = async (id: number) => {
    try {
      await api.delete(`/deploy/servers/${id}`);
      message.success('服务器已删除');
      setLoading(true);
      fetchServers();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleTestConnection = async (id: number) => {
    try {
      await api.post(`/deploy/servers/${id}/test`);
      message.success('连接成功');
      setLoading(true);
      fetchServers();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '连接失败'));
    }
  };

  const handleCreateTask = () => {
    form.resetFields();
    setTaskModalVisible(true);
  };

  const handleSaveTask = async () => {
    try {
      const values = await form.validateFields();
      await api.post('/deploy/tasks', values);
      message.success('任务已创建');
      setTaskModalVisible(false);
      fetchTasks();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const handleExecuteTask = async (id: number) => {
    try {
      await api.post(`/deploy/tasks/${id}/exec`);
      message.success('任务已开始执行');
      fetchTasks();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '执行失败'));
    }
  };

  const handleDeleteTask = async (id: number) => {
    try {
      await api.delete(`/deploy/tasks/${id}`);
      message.success('任务已删除');
      fetchTasks();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleRollback = async (id: number) => {
    try {
      await api.post(`/deploy/versions/${id}/rollback`);
      message.success('回滚已开始');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '回滚失败'));
    }
  };

  const serverColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '地址', key: 'address', render: (_: unknown, r: DeployServer) => `${r.host}:${r.port}` },
    { title: '用户名', dataIndex: 'username', key: 'username' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'online' ? 'success' : status === 'offline' ? 'error' : 'default'}>
          {status}
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: DeployServer) => (
        <Space>
          <Button size="small" icon={<ApiOutlined />} onClick={() => handleTestConnection(record.id)}>
            测试
          </Button>
          <Button size="small" icon={<EditOutlined />} onClick={() => handleEditServer(record)}>
            编辑
          </Button>
          <Popconfirm title="确定删除?" onConfirm={() => handleDeleteServer(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const taskColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '服务器', dataIndex: 'server_name', key: 'server_name' },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => {
        const colorMap: Record<string, string> = { sync: 'blue', command: 'green', rollback: 'orange' };
        return <Tag color={colorMap[type]}>{type}</Tag>;
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          pending: 'default',
          running: 'processing',
          success: 'success',
          failed: 'error',
        };
        return <Tag color={colorMap[status]}>{status}</Tag>;
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: DeployTask) => (
        <Space>
          <Button
            size="small"
            type="primary"
            icon={<PlayCircleOutlined />}
            onClick={() => handleExecuteTask(record.id)}
            disabled={record.status === 'running'}
          >
            执行
          </Button>
          <Popconfirm title="确定删除?" onConfirm={() => handleDeleteTask(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const versionColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '版本', dataIndex: 'version', key: 'version' },
    { title: '服务器', dataIndex: 'server_name', key: 'server_name' },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: DeployVersion) => (
        <Button size="small" icon={<SyncOutlined />} onClick={() => handleRollback(record.id)}>
          回滚
        </Button>
      ),
    },
  ];

  const tabItems = [
    {
      key: 'servers',
      label: '服务器管理',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateServer}>
              添加服务器
            </Button>
          </div>
          <Table columns={serverColumns} dataSource={servers} rowKey="id" loading={loading} />
        </div>
      ),
    },
    {
      key: 'tasks',
      label: '部署任务',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTask}>
              创建任务
            </Button>
          </div>
          <Table columns={taskColumns} dataSource={tasks} rowKey="id" />
        </div>
      ),
    },
    {
      key: 'versions',
      label: '版本历史',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <Select
              placeholder="选择服务器查看版本"
              style={{ width: 200 }}
              onChange={(value) => fetchVersions(value)}
            >
              {servers.map((s) => (
                <Select.Option key={s.id} value={s.id}>
                  {s.name}
                </Select.Option>
              ))}
            </Select>
          </div>
          <Table columns={versionColumns} dataSource={versions} rowKey="id" />
        </div>
      ),
    },
  ];

  return (
    <Card title="部署同步">
      <Tabs items={tabItems} />

      {/* Server Modal */}
      <Modal
        title={editingServer ? '编辑服务器' : '添加服务器'}
        open={serverModalVisible}
        onCancel={() => setServerModalVisible(false)}
        onOk={handleSaveServer}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="host" label="地址" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="port" label="端口" initialValue={22}>
            <Input type="number" />
          </Form.Item>
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="auth_type" label="认证方式" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="password">密码</Select.Option>
              <Select.Option value="key">密钥</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="auth_data" label="密码/密钥路径" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>

      {/* Task Modal */}
      <Modal
        title="创建任务"
        open={taskModalVisible}
        onCancel={() => setTaskModalVisible(false)}
        onOk={handleSaveTask}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="server_id" label="服务器" rules={[{ required: true }]}>
            <Select>
              {servers.map((s) => (
                <Select.Option key={s.id} value={s.id}>
                  {s.name}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="type" label="任务类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="sync">文件同步</Select.Option>
              <Select.Option value="command">执行命令</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="source_path" label="本地路径">
            <Input placeholder="文件同步时填写" />
          </Form.Item>
          <Form.Item name="dest_path" label="远程路径">
            <Input placeholder="文件同步时填写" />
          </Form.Item>
          <Form.Item name="command" label="命令">
            <Input.TextArea placeholder="执行命令时填写" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
