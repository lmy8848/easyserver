import { useState, useEffect, useCallback } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Popconfirm, Modal, Form, Input, Select,
} from 'antd';
import {
  DeleteOutlined, PlusOutlined, ReloadOutlined,
} from '@ant-design/icons';
import { containerApi } from '../../services/container';
import { useAsyncRun } from '../../hooks/useAsyncRun';
import type { Network } from './types';

export default function NetworkTab({ engine }: { engine: string }) {
  const [networks, setNetworks] = useState<Network[]>([]);
  const [loading, setLoading] = useState(true);
  const [createVisible, setCreateVisible] = useState(false);
  const [createForm] = Form.useForm();
  const [removing, setRemoving] = useState<string>('');
  const [createLoading, runCreate] = useAsyncRun();

  const loadNetworks = useCallback(async () => {
    try {
      const res = await containerApi.listNetworks(engine);
      setNetworks(res.data?.data?.items ?? []);
    } catch {
      message.error('加载网络列表失败');
    } finally {
      setLoading(false);
    }
  }, [engine]);

  useEffect(() => { loadNetworks(); }, [loadNetworks]);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      await runCreate(() => containerApi.createNetwork(values, engine));
      message.success('网络创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      setLoading(true);
      loadNetworks();
    } catch {
      message.error('创建失败');
    }
  };

  const handleRemove = async (id: string) => {
    setRemoving(id);
    try {
      await containerApi.deleteNetwork(id, engine);
      message.success('网络已删除');
      setLoading(true);
      loadNetworks();
    } catch {
      message.error('删除失败');
    } finally {
      setRemoving('');
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
            <Button icon={<DeleteOutlined />} size="small" danger loading={removing === record.id} disabled={!!removing}>删除</Button>
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
            <Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); loadNetworks(); }}>刷新</Button>
          </Space>
        }
      >
        <Table columns={columns} dataSource={networks} rowKey="id" loading={loading} locale={{ emptyText: '暂无网络' }} />
      </Card>

      <Modal title="创建网络" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)} confirmLoading={createLoading}>
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
