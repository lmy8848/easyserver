import { useState, useEffect } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Popconfirm, Modal, Form, Input,
} from 'antd';
import {
  DeleteOutlined, CloudDownloadOutlined, ReloadOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import { DOCKER_IMAGE_TEMPLATES } from '../../constants/templates';
import type { Image, ImageCategory } from './types';

export default function ImageTab() {
  const [images, setImages] = useState<Image[]>([]);
  const [loading, setLoading] = useState(true);
  const [pullVisible, setPullVisible] = useState(false);
  const [pulling, setPulling] = useState(false);
  const [pullForm] = Form.useForm();
  const templates: ImageCategory[] = DOCKER_IMAGE_TEMPLATES;

  const loadImages = async () => {
    try {
      const res = await api.get('/images');
      setImages(res.data?.data?.images || []);
    } catch {
      message.error('加载镜像列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadImages(); }, []);

  const handlePull = async () => {
    let values;
    try {
      values = await pullForm.validateFields();
    } catch {
      return; // 校验失败由表单自身提示，无需 loading
    }
    setPulling(true);
    try {
      await api.post('/images/pull', values);
      message.success('镜像拉取成功');
      setPullVisible(false);
      pullForm.resetFields();
      setLoading(true);
      loadImages();
    } catch {
      message.error('拉取失败');
    } finally {
      setPulling(false);
    }
  };

  const handleRemove = async (id: string) => {
    try {
      await api.delete(`/images/${id}?force=true`);
      message.success('镜像已删除');
      setLoading(true);
      loadImages();
    } catch {
      message.error('删除失败');
    }
  };

  const columns = [
    { title: '仓库', dataIndex: 'repository', key: 'repository' },
    { title: '标签', dataIndex: 'tag', key: 'tag' },
    { title: '大小', dataIndex: 'size', key: 'size', render: (size: string) => size || '-' },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Image) => (
        <Popconfirm title="确定删除此镜像？" onConfirm={() => handleRemove(record.id)} okText="删除" cancelText="取消">
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
            <Button icon={<CloudDownloadOutlined />} type="primary" onClick={() => setPullVisible(true)}>拉取镜像</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); loadImages(); }}>刷新</Button>
          </Space>
        }
      >
        <Table columns={columns} dataSource={images} rowKey="id" loading={loading} locale={{ emptyText: '暂无镜像' }} />
      </Card>

      <Modal
        title="拉取镜像"
        open={pullVisible}
        onOk={handlePull}
        onCancel={() => setPullVisible(false)}
        confirmLoading={pulling}
        cancelButtonProps={{ disabled: pulling }}
        maskClosable={!pulling}
        closable={!pulling}
        keyboard={!pulling}
        width={600}
        destroyOnClose
      >
        <Form form={pullForm} layout="vertical">
          <Form.Item name="image" label="镜像名称" rules={[{ required: true }]}>
            <Input placeholder="nginx:latest" />
          </Form.Item>
        </Form>
        {templates.length > 0 && (
          <>
            <div style={{ marginBottom: 8, fontWeight: 500 }}>常用镜像</div>
            <Space wrap>
              {templates.map(cat => cat.images.map(img => (
                <Tag
                  key={img.image}
                  style={{ cursor: 'pointer' }}
                  onClick={() => pullForm.setFieldsValue({ image: img.image })}
                >
                  {img.name}
                </Tag>
              )))}
            </Space>
          </>
        )}
      </Modal>
    </>
  );
}
