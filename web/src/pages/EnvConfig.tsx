import { useState } from 'react';
import { Card, Table, Button, Space, Modal, Form, Input, Select, message, Popconfirm, Tabs } from 'antd';
import { PlusOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons';
import api from '../services/api';

interface EnvConfig {
  id: number;
  name: string;
  value: string;
  runtime_id: number;
  is_global: boolean;
  created_at: string;
  updated_at: string;
}

interface PathEntry {
  id: number;
  path: string;
  runtime_id: number;
  is_global: boolean;
  order: number;
  created_at: string;
}

interface GlobalConfig {
  id: number;
  category: string;
  key: string;
  value: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export default function EnvConfig() {
  const [envConfigs, setEnvConfigs] = useState<EnvConfig[]>([]);
  const [pathEntries, setPathEntries] = useState<PathEntry[]>([]);
  const [globalConfigs, setGlobalConfigs] = useState<GlobalConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [envModalVisible, setEnvModalVisible] = useState(false);
  const [pathModalVisible, setPathModalVisible] = useState(false);
  const [globalModalVisible, setGlobalModalVisible] = useState(false);
  const [editingEnv, setEditingEnv] = useState<EnvConfig | null>(null);
  const [editingGlobal, setEditingGlobal] = useState<GlobalConfig | null>(null);
  const [envForm] = Form.useForm();
  const [pathForm] = Form.useForm();
  const [globalForm] = Form.useForm();
  const [activeTab, setActiveTab] = useState('env');
  const [selectedCategory, setSelectedCategory] = useState<string>('');

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

  const fetchGlobalConfigs = async (category?: string) => {
    try {
      const url = category ? `/global-config?category=${category}` : '/global-config';
      const res = await api.get(url);
      setGlobalConfigs(res.data.data?.configs || []);
    } catch (error) {
      message.error('获取全局配置失败');
    }
  };

  const handleCreateEnv = async (values: { name: string; value: string }) => {
    try {
      await api.post('/env-config', { ...values, runtime_id: 0, is_global: true });
      message.success('环境变量创建成功');
      setEnvModalVisible(false);
      envForm.resetFields();
      fetchEnvConfigs();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '创建失败'));
    }
  };

  const handleUpdateEnv = async (values: { name: string; value: string }) => {
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

  const handleCreatePath = async (values: { path: string }) => {
    try {
      await api.post('/env-config/path', { ...values, runtime_id: 0, is_global: true });
      message.success('PATH 条目创建成功');
      setPathModalVisible(false);
      pathForm.resetFields();
      fetchPathEntries();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '创建失败'));
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

  const handleCreateGlobal = async (values: { category: string; key: string; value: string; description: string }) => {
    try {
      await api.post('/global-config', values);
      message.success('全局配置创建成功');
      setGlobalModalVisible(false);
      globalForm.resetFields();
      fetchGlobalConfigs(selectedCategory);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '创建失败'));
    }
  };

  const handleUpdateGlobal = async (values: { value: string; description: string }) => {
    if (!editingGlobal) return;
    try {
      await api.put(`/global-config/${editingGlobal.id}`, values);
      message.success('全局配置更新成功');
      setGlobalModalVisible(false);
      setEditingGlobal(null);
      globalForm.resetFields();
      fetchGlobalConfigs(selectedCategory);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '更新失败'));
    }
  };

  const handleDeleteGlobal = async (id: number) => {
    try {
      await api.delete(`/global-config/${id}`);
      message.success('全局配置删除成功');
      fetchGlobalConfigs(selectedCategory);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const openEditEnv = (config: EnvConfig) => {
    setEditingEnv(config);
    envForm.setFieldsValue({ name: config.name, value: config.value });
    setEnvModalVisible(true);
  };

  const openEditGlobal = (config: GlobalConfig) => {
    setEditingGlobal(config);
    globalForm.setFieldsValue({ value: config.value, description: config.description });
    setGlobalModalVisible(true);
  };

  const envColumns = [
    { title: '变量名', dataIndex: 'name', key: 'name' },
    { title: '值', dataIndex: 'value', key: 'value', ellipsis: true },
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
    { title: '优先级', dataIndex: 'order', key: 'order', width: 80 },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: unknown, record: PathEntry) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Popconfirm title="确定要删除吗？" onConfirm={() => handleDeletePath(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  const globalColumns = [
    { title: '分类', dataIndex: 'category', key: 'category', width: 100 },
    { title: '配置项', dataIndex: 'key', key: 'key', width: 150 },
    { title: '值', dataIndex: 'value', key: 'value', ellipsis: true },
    { title: '说明', dataIndex: 'description', key: 'description', ellipsis: true },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: unknown, record: GlobalConfig) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditGlobal(record)}>编辑</Button>
          <Popconfirm title="确定要删除吗？" onConfirm={() => handleDeleteGlobal(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  const categories = ['maven', 'npm', 'pip', 'go', 'composer', 'ruby'];

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
            {
              key: 'global',
              label: '全局配置',
              children: (
                <>
                  <div style={{ marginBottom: 16 }}>
                    <Space>
                      <Select
                        placeholder="筛选分类"
                        allowClear
                        style={{ width: 150 }}
                        onChange={(value) => {
                          setSelectedCategory(value || '');
                          fetchGlobalConfigs(value);
                        }}
                      >
                        {categories.map(cat => (
                          <Select.Option key={cat} value={cat}>{cat}</Select.Option>
                        ))}
                      </Select>
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => {
                        setEditingGlobal(null);
                        globalForm.resetFields();
                        setGlobalModalVisible(true);
                      }}>
                        添加全局配置
                      </Button>
                    </Space>
                  </div>
                  <Table columns={globalColumns} dataSource={globalConfigs} rowKey="id" pagination={false} />
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
          <Form.Item>
            <Button type="primary" htmlType="submit" block>{editingEnv ? '更新' : '创建'}</Button>
          </Form.Item>
        </Form>
      </Modal>

      {/* PATH 条目弹窗 */}
      <Modal
        title="添加 PATH 条目"
        open={pathModalVisible}
        onCancel={() => {
          setPathModalVisible(false);
          pathForm.resetFields();
        }}
        footer={null}
      >
        <Form form={pathForm} onFinish={handleCreatePath} layout="vertical">
          <Form.Item name="path" label="路径" rules={[{ required: true, message: '请输入路径' }]}>
            <Input placeholder="例如：/usr/lib/jvm/java-17/bin" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>添加</Button>
          </Form.Item>
        </Form>
      </Modal>

      {/* 全局配置弹窗 */}
      <Modal
        title={editingGlobal ? '编辑全局配置' : '添加全局配置'}
        open={globalModalVisible}
        onCancel={() => {
          setGlobalModalVisible(false);
          setEditingGlobal(null);
          globalForm.resetFields();
        }}
        footer={null}
      >
        <Form form={globalForm} onFinish={editingGlobal ? handleUpdateGlobal : handleCreateGlobal} layout="vertical">
          {!editingGlobal && (
            <>
              <Form.Item name="category" label="分类" rules={[{ required: true, message: '请选择分类' }]}>
                <Select placeholder="选择分类">
                  {categories.map(cat => (
                    <Select.Option key={cat} value={cat}>{cat}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
              <Form.Item name="key" label="配置项" rules={[{ required: true, message: '请输入配置项' }]}>
                <Input placeholder="例如：registry" />
              </Form.Item>
            </>
          )}
          <Form.Item name="value" label="值" rules={[{ required: true, message: '请输入值' }]}>
            <Input placeholder="例如：https://registry.npmmirror.com" />
          </Form.Item>
          <Form.Item name="description" label="说明">
            <Input placeholder="配置项说明" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>{editingGlobal ? '更新' : '创建'}</Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
