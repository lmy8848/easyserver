import { useState, useEffect } from 'react';
import {
  Card, Table, Button, Space, message, Popconfirm, Modal, Form, Input, Select,
} from 'antd';
import {
  DeleteOutlined, PlusOutlined, ReloadOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { Volume } from './types';

export default function VolumeTab() {
  const [volumes, setVolumes] = useState<Volume[]>([]);
  const [loading, setLoading] = useState(true);
  const [createVisible, setCreateVisible] = useState(false);
  const [createForm] = Form.useForm();

  const loadVolumes = async () => {
    try {
      const res = await api.get('/volumes');
      setVolumes(res.data?.data?.volumes || []);
    } catch {
      message.error('加载存储卷列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadVolumes(); }, []);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      await api.post('/volumes', values);
      message.success('存储卷创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      setLoading(true);
      loadVolumes();
    } catch {
      message.error('创建失败');
    }
  };

  const handleRemove = async (name: string) => {
    try {
      await api.delete(`/volumes/${name}?force=true`);
      message.success('存储卷已删除');
      setLoading(true);
      loadVolumes();
    } catch {
      message.error('删除失败');
    }
  };

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '驱动', dataIndex: 'driver', key: 'driver' },
    { title: '挂载点', dataIndex: 'mountpoint', key: 'mountpoint', ellipsis: true },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Volume) => (
        <Popconfirm title="确定删除此存储卷？" onConfirm={() => handleRemove(record.name)} okText="删除" cancelText="取消">
          <Button icon={<DeleteOutlined />} size="small" danger>删除</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <>
      <Card
        extra={
          <Space>
            <Button icon={<PlusOutlined />} type="primary" onClick={() => setCreateVisible(true)}>创建存储卷</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); loadVolumes(); }}>刷新</Button>
          </Space>
        }
      >
        <Table columns={columns} dataSource={volumes} rowKey="name" loading={loading} locale={{ emptyText: '暂无存储卷' }} />
      </Card>

      <Modal title="创建存储卷" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)}>
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input placeholder="my-volume" /></Form.Item>
          <Form.Item name="driver" label="驱动" initialValue="local">
            <Select>
              <Select.Option value="local">local</Select.Option>
              <Select.Option value="nfs">nfs</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
