import { useState, useEffect } from 'react';
import { Card, Table, Button, Modal, Form, Input, message, Popconfirm, Tabs, Switch } from 'antd';
import { PlusOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons';
import api from '../services/api';

interface EnvConfig {
  id: number;
  name: string;
  value: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface PathEntry {
  id: number;
  path: string;
  enabled: boolean;
  order: number;
  created_at: string;
}

export default function EnvConfig() {
  const [envConfigs, setEnvConfigs] = useState<EnvConfig[]>([]);
  const [pathEntries, setPathEntries] = useState<PathEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [envModalVisible, setEnvModalVisible] = useState(false);
  const [pathModalVisible, setPathModalVisible] = useState(false);
  const [editingEnv, setEditingEnv] = useState<EnvConfig | null>(null);
  const [editingPath, setEditingPath] = useState<PathEntry | null>(null);
  const [envForm] = Form.useForm();
  const [pathForm] = Form.useForm();
  const [activeTab, setActiveTab] = useState('env');



  const fetchEnvConfigs = async () => {
    setLoading(true);
    try {
      const res = await api.get('/env-config');
      setEnvConfigs(res.data.data?.configs || []);
    } catch (error) {
      message.error('获取环境变量失败');
    } finally {
      setLoading(false);
    }
  };

  const fetchPathEntries = async () => {
    try {
      const res = await api.get('/env-config/path');
      setPathEntries(res.data.data?.entries || []);
    } catch (error) {
      message.error('获取 PATH 条目失败');
    }
  };

  useEffect(() => {
    fetchEnvConfigs();
    fetchPathEntries();
  }, []);

  const handleCreateEnv = async (values: { name: string; value: string; enabled?: boolean }) => {
    try {
      await api.post('/env-config', values);
      message.success('环境变量创建成功');
      setEnvModalVisible(false);
      envForm.resetFields();
      fetchEnvConfigs();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '创建失败'));
    }
  };

  const handleUpdateEnv = async (values: { name: string; value: string; enabled?: boolean }) => {
    if (!editingEnv) return;
    try {
      await api.put(`/env-config/${editingEnv.id}`, values);
      message.success('环境变量更新成功');
      setEnvModalVisible(false);
      setEditingEnv(null);
      envForm.resetFields();
      fetchEnvConfigs();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '更新失败'));
    }
  };

  const handleDeleteEnv = async (id: number) => {
    try {
      await api.delete(`/env-config/${id}`);
      message.success('环境变量删除成功');
      fetchEnvConfigs();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleCreatePath = async (values: { path: string; enabled?: boolean }) => {
    try {
      await api.post('/env-config/path', values);
      message.success('PATH 条目创建成功');
      setPathModalVisible(false);
      pathForm.resetFields();
      fetchPathEntries();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '创建失败'));
    }
  };

  const handleUpdatePath = async (values: { path: string; enabled?: boolean }) => {
    if (!editingPath) return;
    try {
      await api.put(`/env-config/path/${editingPath.id}`, values);
      message.success('PATH 条目更新成功');
      setPathModalVisible(false);
      setEditingPath(null);
      pathForm.resetFields();
      fetchPathEntries();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '更新失败'));
    }
  };

  const handleDeletePath = async (id: number) => {
    try {
      await api.delete(`/env-config/path/${id}`);
      message.success('PATH 条目删除成功');
      fetchPathEntries();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const openEditEnv = (config: EnvConfig) => {
    setEditingEnv(config);
    envForm.setFieldsValue({ name: config.name, value: config.value, enabled: config.enabled });
    setEnvModalVisible(true);
  };

  const openEditPath = (entry: PathEntry) => {
    setEditingPath(entry);
    pathForm.setFieldsValue({ path: entry.path, enabled: entry.enabled });
    setPathModalVisible(true);
  };

  const envColumns = [
    { title: '变量名', dataIndex: 'name', key: 'name' },
    { title: '值', dataIndex: 'value', key: 'value', ellipsis: true },
    { title: '状态', dataIndex: 'enabled', key: 'enabled', render: (enabled: boolean) => enabled ? '启用' : '禁用', width: 80 },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: unknown, record: EnvConfig) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditEnv(record)}>编辑</Button>
          <Popconfirm title="确定要删除吗？" onConfirm={() => handleDeleteEnv(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  const pathColumns = [
    { title: '路径', dataIndex: 'path', key: 'path' },
    { title: '状态', dataIndex: 'enabled', key: 'enabled', render: (enabled: boolean) => enabled ? '启用' : '禁用', width: 80 },
    { title: '优先级', dataIndex: 'order', key: 'order', width: 80 },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: unknown, record: PathEntry) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditPath(record)}>编辑</Button>
          <Popconfirm title="确定要删除吗？" onConfirm={() => handleDeletePath(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  return (
    <div>
      <Card title="环境配置管理">
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'env',
              label: '环境变量',
              children: (
                <>
                  <div style={{ marginBottom: 16 }}>
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => {
                      setEditingEnv(null);
                      envForm.resetFields();
                      setEnvModalVisible(true);
                    }}>
                      添加环境变量
                    </Button>
                  </div>
                  <Table columns={envColumns} dataSource={envConfigs} rowKey="id" loading={loading} pagination={false} />
                </>
              ),
            },
            {
              key: 'path',
              label: 'PATH 管理',
              children: (
                <>
                  <div style={{ marginBottom: 16 }}>
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => {
                      setEditingPath(null);
                      pathForm.resetFields();
                      setPathModalVisible(true);
                    }}>
                      添加 PATH 条目
                    </Button>
                  </div>
                  <Table columns={pathColumns} dataSource={pathEntries} rowKey="id" pagination={false} />
                </>
              ),
            },
          ]}
        />
      </Card>

      {/* 环境变量弹窗 */}
      <Modal
        title={editingEnv ? '编辑环境变量' : '添加环境变量'}
        open={envModalVisible}
        onCancel={() => {
          setEnvModalVisible(false);
          setEditingEnv(null);
          envForm.resetFields();
        }}
        footer={null}
      >
        <Form form={envForm} onFinish={editingEnv ? handleUpdateEnv : handleCreateEnv} layout="vertical">
          <Form.Item name="name" label="变量名" rules={[{ required: true, message: '请输入变量名' }]}>
            <Input placeholder="例如：JAVA_HOME" disabled={!!editingEnv} />
          </Form.Item>
          <Form.Item name="value" label="值" rules={[{ required: true, message: '请输入值' }]}>
            <Input placeholder="例如：/usr/lib/jvm/java-17" />
          </Form.Item>
          <Form.Item name="enabled" label="启用状态" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>{editingEnv ? '更新' : '创建'}</Button>
          </Form.Item>
        </Form>
      </Modal>

      {/* PATH 条目弹窗 */}
      <Modal
        title={editingPath ? '编辑 PATH 条目' : '添加 PATH 条目'}
        open={pathModalVisible}
        onCancel={() => {
          setPathModalVisible(false);
          setEditingPath(null);
          pathForm.resetFields();
        }}
        footer={null}
      >
        <Form form={pathForm} onFinish={editingPath ? handleUpdatePath : handleCreatePath} layout="vertical">
          <Form.Item name="path" label="路径" rules={[{ required: true, message: '请输入路径' }]}>
            <Input placeholder="例如：/usr/lib/jvm/java-17/bin" disabled={!!editingPath} />
          </Form.Item>
          <Form.Item name="enabled" label="启用状态" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>{editingPath ? '更新' : '添加'}</Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
