import { useState } from 'react';
import { Table, Button, Space, Tag, Modal, Form, Input, message } from 'antd';
import { ReloadOutlined, CameraOutlined } from '@ant-design/icons';
import { cloudApi } from '../../services/api';
import type { Snapshot } from '../../types';

interface Props {
  snapshots: Snapshot[];
  selectedInstance: string;
  onRefresh: () => void;
}

export default function CloudSnapshots({ snapshots, selectedInstance, onRefresh }: Props) {
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();

  const handleCreateSnapshot = async () => {
    try {
      const values = await form.validateFields();
      await cloudApi.createSnapshot(selectedInstance, values.name);
      message.success('快照已创建');
      setModalVisible(false);
      form.resetFields();
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const columns = [
    {
      title: '快照 ID',
      dataIndex: 'snapshot_id',
      key: 'snapshot_id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'NORMAL' ? 'success' : 'processing'}>{status}</Tag>
      ),
    },
    {
      title: '大小',
      dataIndex: 'disk_gb',
      key: 'disk_gb',
      render: (gb: number) => `${gb} GB`,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Snapshot) => (
        <Button
          type="link"
          size="small"
          onClick={async () => {
            try {
              await cloudApi.applySnapshot(record.snapshot_id);
              message.success('快照回滚中');
            } catch (error: unknown) {
              message.error((error instanceof Error ? error.message : '回滚失败'));
            }
          }}
        >
          回滚
        </Button>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Button
            type="primary"
            icon={<CameraOutlined />}
            onClick={() => setModalVisible(true)}
            disabled={!selectedInstance}
          >
            创建快照
          </Button>
          <Button icon={<ReloadOutlined />} onClick={onRefresh}>
            刷新
          </Button>
        </Space>
      </div>
      <Table
        columns={columns}
        dataSource={snapshots}
        rowKey="snapshot_id"
        pagination={false}
      />
      <Modal
        title="创建快照"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleCreateSnapshot}
        okText="创建"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="快照名称"
            rules={[{ required: true, message: '请输入快照名称' }]}
          >
            <Input placeholder="如: pre-update-backup" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
