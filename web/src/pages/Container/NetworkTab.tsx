import { useState, useEffect } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Popconfirm, Modal, Form, Input, Select,
} from 'antd';
import {
  DeleteOutlined, PlusOutlined, ReloadOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { Network } from './types';

export default function NetworkTab() {
  const [networks, setNetworks] = useState<Network[]>([]);
  const [loading, setLoading] = useState(false);
  const [createVisible, setCreateVisible] = useState(false);
  const [createForm] = Form.useForm();

  const loadNetworks = async () => {
    setLoading(true);
    try {
      const res = await api.get('/networks');
      setNetworks(res.data?.data?.networks || []);
    } catch {
      message.error('加载网络列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadNetworks(); }, []);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      await api.post('/networks', values);
      message.success('网络创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      loadNetworks();
    } catch {
      message.error('创建失败');
    }
  };

  const handleRemove = async (id: string) => {
    try {
      await api.delete(`/networks/${id}`);
      message.success('网络已删除');
      loadNetworks();
    } catch {
      message.error('删除失败');
    }
  };

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '驱动', dataIndex: 'driver', key: 'driver' },
    { title: '作用域', dataIndex: 'scope', key: 'scope' },
    { title: '子网', dataIndex: 'subnet', key: 'subnet' },
    { title: '网关', dataIndex: 'gateway', key: 'gateway' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Network) => {
        const isProtected = ['bridge', 'host', 'none'].includes(record.name);
        return isProtected ? (
          <Tag>系统网络</Tag>
        ) : (
          <Popconfirm title="确定删除此网络？" onConfirm={() => handleRemove(record.id)} okText="删除" cancelText="取消">
            <Button icon={<DeleteOutlined />} size="small" danger>删除</Button>
          </Popconfirm>
        );
      },
    },
  ];

  return (
    <>
      <Card
        extra={
          <Space>
            <Button icon={<PlusOutlined />} type="primary" onClick={() => setCreateVisible(true)}>创建网络</Button>
            <Button icon={<ReloadOutlined />} onClick={loadNetworks}>刷新</Button>
          </Space>
        }
      >
        <Table columns={columns} dataSource={networks} rowKey="id" loading={loading} locale={{ emptyText: '暂无网络' }} />
      </Card>

      <Modal title="创建网络" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)}>
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input placeholder="my-network" /></Form.Item>
          <Form.Item name="driver" label="驱动" initialValue="bridge">
            <Select>
              <Select.Option value="bridge">bridge</Select.Option>
              <Select.Option value="overlay">overlay</Select.Option>
              <Select.Option value="macvlan">macvlan</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
