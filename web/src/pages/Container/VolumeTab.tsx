import { useState, useEffect } from 'react';
import {
  Card, Table, Button, Space, message, Popconfirm, Modal, Form, Input, Select,
} from 'antd';
import {
  DeleteOutlined, PlusOutlined, ReloadOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { Volume } from './types';
import { withEngine } from './types';
import { formatCreatedAt } from '../../utils/format';

export default function VolumeTab({ engine }: { engine: string }) {
  const [volumes, setVolumes] = useState<Volume[]>([]);
  const [loading, setLoading] = useState(true);
  const [createVisible, setCreateVisible] = useState(false);
  const [createForm] = Form.useForm();
  const [createLoading, setCreateLoading] = useState(false);
  const [removing, setRemoving] = useState<string>('');

  const loadVolumes = async () => {
    try {
      const res = await api.get(withEngine('/container/volumes', engine));
      setVolumes(res.data?.data?.volumes || []);
    } catch {
      message.error('加载存储卷列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadVolumes(); }, [engine]);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      const labels: Record<string, string> = {};
      (values.labels || []).forEach((l: { key?: string; value?: string }) => {
        if (l.key) labels[l.key] = l.value || '';
      });
      setCreateLoading(true);
      await api.post(withEngine('/container/volumes', engine), { ...values, labels });
      message.success('存储卷创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      setLoading(true);
      loadVolumes();
    } catch {
      message.error('创建失败');
    } finally {
      setCreateLoading(false);
    }
  };

  const handleRemove = async (name: string) => {
    setRemoving(name);
    try {
      await api.delete(withEngine(`/container/volumes/${name}?force=true`, engine));
      message.success('存储卷已删除');
      setLoading(true);
      loadVolumes();
    } catch {
      message.error('删除失败');
    } finally {
      setRemoving('');
    }
  };

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '驱动', dataIndex: 'driver', key: 'driver' },
    { title: '挂载点', dataIndex: 'mountpoint', key: 'mountpoint', ellipsis: true },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => formatCreatedAt(v) },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Volume) => (
        <Popconfirm title="确定删除此存储卷？" onConfirm={() => handleRemove(record.name)} okText="删除" cancelText="取消">
          <Button icon={<DeleteOutlined />} size="small" danger loading={removing === record.name} disabled={!!removing}>删除</Button>
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

      <Modal title="创建存储卷" open={createVisible} onOk={handleCreate} onCancel={() => setCreateVisible(false)} confirmLoading={createLoading}>
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input placeholder="my-volume" /></Form.Item>
          <Form.Item name="driver" label="驱动" initialValue="local">
            <Select disabled>
              <Select.Option value="local">local</Select.Option>
            </Select>
          </Form.Item>
          <Form.List name="labels">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <div key={key} style={{ display: 'flex', gap: 8, alignItems: 'baseline', marginBottom: 8 }}>
                    <Form.Item {...restField} name={[name, 'key']} style={{ marginBottom: 0, flex: 1 }}>
                      <Input placeholder="标签键"/>
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'value']} style={{ marginBottom: 0, flex: 1 }}>
                      <Input placeholder="标签值"/>
                    </Form.Item>
                    <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} />
                  </div>
                ))}
                <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加标签</Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>
    </>
  );
}
