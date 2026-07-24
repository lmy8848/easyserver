import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Tag, Modal, Form, Input, InputNumber, message, Popconfirm, Tooltip, Checkbox } from 'antd';
import { LinkOutlined, DeleteOutlined, PlusOutlined, ReloadOutlined, CopyOutlined, EditOutlined, LockOutlined } from '@ant-design/icons';
import { fileShareApi } from '../../services/api';
import type { FileShare } from '../../types';
import { copyToClipboard } from '../../utils/clipboard';

export default function FileShares() {
  const [shares, setShares] = useState<FileShare[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();
  const [createLoading, setCreateLoading] = useState(false);

  // Edit modal
  const [editVisible, setEditVisible] = useState(false);
  const [editingShare, setEditingShare] = useState<FileShare | null>(null);
  const [editForm] = Form.useForm();
  const [editLoading, setEditLoading] = useState(false);
  const [editClearExpiry, setEditClearExpiry] = useState(false);

  const fetchShares = async () => {
    setLoading(true);
    try {
      const res = await fileShareApi.list();
      setShares(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch shares:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchShares();
  }, []);

  const handleCreate = () => {
    form.resetFields();
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setCreateLoading(true);
      await fileShareApi.create(values);
      message.success('外链生成成功');
      setModalVisible(false);
      fetchShares();
    } catch (error: unknown) {
      if (error instanceof Error) message.error(error.message);
    } finally {
      setCreateLoading(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await fileShareApi.delete(id);
      message.success('外链已撤销');
      fetchShares();
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '撤销失败');
    }
  };

  const handleCleanup = async () => {
    Modal.confirm({
      title: '确认清理',
      content: '确定要清理所有已过期或达到下载上限的外链吗？',
      okText: '确认清理',
      cancelText: '取消',
      onOk: async () => {
        try {
          const res = await fileShareApi.cleanupExpired();
          message.success(`已清理 ${res.data.data?.deleted || 0} 个外链`);
          fetchShares();
        } catch (error: unknown) {
          message.error(error instanceof Error ? error.message : '清理失败');
        }
      },
    });
  };

  // 复制干净链接(不带密码)，下载页会引导输入密码。
  const copyShareLink = (token: string) => {
    const link = `${window.location.origin}/share/${token}`;
    writeClipboard(link);
  };

  // 复制带密码链接(直达下载，跳过下载页)。需先拉取完整 share 取到密码。
  const copyShareLinkWithPassword = async (id: number, token: string) => {
    try {
      const res = await fileShareApi.get(id);
      const share = res.data.data;
      const pwd = share?.password || '';
      if (!pwd) {
        message.info('该外链未设置密码，已复制普通链接');
        writeClipboard(`${window.location.origin}/share/${token}`);
        return;
      }
      // 复制带有 password 参数的前端页面链接，前端页面会自动读取并填入密码
      writeClipboard(`${window.location.origin}/share/${token}?password=${encodeURIComponent(pwd)}`);
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '获取外链信息失败');
    }
  };

  const writeClipboard = (text: string) => {
    copyToClipboard(text, '链接已复制');
  };

  const handleEdit = async (share: FileShare) => {
    setEditingShare(share);
    setEditClearExpiry(false);
    editForm.resetFields();
    editForm.setFieldsValue({
      max_downloads: share.max_downloads || 0,
      expires_at: share.expires_at || '',
    });
    setEditVisible(true);
    
    // Fetch full info to echo password
    try {
      const res = await fileShareApi.get(share.id);
      editForm.setFieldsValue({
        password: res.data.data?.password || '',
      });
    } catch (error) {
      console.error('Failed to fetch share password', error);
    }
  };

  const handleEditSubmit = async () => {
    if (!editingShare) return;
    try {
      const values = await editForm.validateFields();
      const v = values as { password?: string; expires_at?: string; max_downloads?: number };
      setEditLoading(true);
      const payload: {
        password?: string;
        expires_at?: string;
        max_downloads: number;
        clear_expiry?: boolean;
      } = { max_downloads: v.max_downloads ?? 0, password: v.password || '' };
      
      // 过期时间：勾选永久 -> clear_expiry；输入新值 -> 替换；否则不传
      if (editClearExpiry) {
        payload.clear_expiry = true;
      } else if (v.expires_at) {
        payload.expires_at = v.expires_at;
      }
      await fileShareApi.update(editingShare.id, payload);
      message.success('外链已更新');
      setEditVisible(false);
      fetchShares();
    } catch (error: unknown) {
      if (error instanceof Error) message.error(error.message);
    } finally {
      setEditLoading(false);
    }
  };

  const formatExpiry = (expiresAt: string) => {
    if (!expiresAt) return <Tag color="default">永久有效</Tag>;
    const expired = new Date(expiresAt) < new Date();
    return (
      <Tag color={expired ? 'error' : 'processing'}>
        {expired ? '已过期' : new Date(expiresAt).toLocaleString()}
      </Tag>
    );
  };

  const formatSize = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const columns = [
    { title: '文件名', dataIndex: 'file_name', key: 'file_name', ellipsis: true },
    {
      title: '源文件', key: 'source', width: 80,
      render: (_: unknown, record: FileShare) => record.file_exists === false
        ? <Tag color="error">已不存在</Tag>
        : record.current_size !== undefined && record.current_size !== record.file_size
          ? <Tooltip title={`创建时 ${formatSize(record.file_size)}，当前 ${formatSize(record.current_size)}`}>
              <Tag color="warning">已变更</Tag>
            </Tooltip>
          : <Tag color="success">正常</Tag>,
    },
    {
      title: '创建时大小', dataIndex: 'file_size', key: 'file_size', width: 90,
      render: (size: number) => formatSize(size),
    },
    {
      title: '当前大小', key: 'current_size', width: 90,
      render: (_: unknown, record: FileShare) => record.current_size !== undefined
        ? <span style={{ color: record.current_size !== record.file_size ? '#faad14' : undefined }}>
            {formatSize(record.current_size)}
          </span>
        : '-',
    },
    {
      title: '下载次数', key: 'downloads', width: 90,
      render: (_: unknown, record: FileShare) => (
        <span>{record.download_count}{record.max_downloads > 0 ? ` / ${record.max_downloads}` : ''}</span>
      ),
    },
    {
      title: '密码', key: 'password', width: 70, align: 'center' as const,
      render: (_: unknown, record: FileShare) => record.has_password
        ? <Tooltip title="已设置访问密码"><Tag color="orange" style={{ margin: 0 }}><LockOutlined /> 已设</Tag></Tooltip>
        : <Tag color="default" style={{ margin: 0 }}>无</Tag>,
    },
    {
      title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 150,
      render: (t: string) => formatExpiry(t),
    },
    {
      title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 150,
      render: (t: string) => new Date(t).toLocaleString(),
    },
    {
      title: '操作', key: 'action', width: 280,
      render: (_: unknown, record: FileShare) => (
        <Space size="small" wrap>
          <Tooltip title="复制链接（下载页，需输入密码）">
            <Button type="link" size="small" icon={<CopyOutlined />} onClick={() => copyShareLink(record.token)}>
              复制链接
            </Button>
          </Tooltip>
          {record.has_password && (
            <Tooltip title="复制带密码链接（直达下载）">
              <Button type="link" size="small" icon={<LinkOutlined />} onClick={() => copyShareLinkWithPassword(record.id, record.token)}>
                带密码
              </Button>
            </Tooltip>
          )}
          <Tooltip title="编辑外链">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
          <Popconfirm title="确定撤销此分享链接？" onConfirm={() => handleDelete(record.id)}>
            <Tooltip title="撤销">
              <Button type="link" size="small" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title={<span><LinkOutlined style={{ marginRight: 8 }} />文件外链管理</span>}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchShares}>刷新</Button>
            <Button onClick={handleCleanup}>清理过期</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>生成外链</Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={shares}
          rowKey="id"
          loading={loading}
          pagination={{ defaultPageSize: 20, showTotal: (t) => `共 ${t} 个外链` }}
          size="small"
          scroll={{ x: 1000 }}
        />
      </Card>

      <Modal
        title="生成文件外链"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmit}
        okText="生成"
        confirmLoading={createLoading}
        cancelText="取消"
        width={500}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="file_path" label="文件路径" rules={[{ required: true, message: '请输入文件路径' }]}>
            <Input placeholder="如：/var/www/html/file.zip" />
          </Form.Item>
          <Form.Item name="expires_at" label="过期时间" extra="留空为永久有效。支持：1h, 1d, 7d 或具体时间 2026-07-01 12:00:00">
            <Input placeholder="留空、1h、7d 或 2026-07-01 12:00:00" />
          </Form.Item>
          <Form.Item name="max_downloads" label="最大下载次数" extra="0 表示不限制">
            <InputNumber min={0} max={100000} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="password" label="访问密码（可选）">
            <Input.Password placeholder="留空表示不需要密码" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="编辑外链"
        open={editVisible}
        onCancel={() => setEditVisible(false)}
        onOk={handleEditSubmit}
        okText="保存"
        confirmLoading={editLoading}
        cancelText="取消"
        width={500}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item label="文件路径">
            <Input value={editingShare?.file_path} disabled />
          </Form.Item>
          <Form.Item name="password" label="访问密码" extra="留空可清除密码">
            <Input.Password placeholder="未设置密码" />
          </Form.Item>
          <Form.Item name="expires_at" label="过期时间" extra="留空不修改；支持 1h, 7d 或 2026-07-01 12:00:00">
            <Input placeholder="留空不修改、1h、7d 或 2026-07-01 12:00:00" disabled={editClearExpiry} />
          </Form.Item>
          <Form.Item style={{ marginBottom: 16 }}>
            <Checkbox checked={editClearExpiry} onChange={(e) => setEditClearExpiry(e.target.checked)}>设为永久有效</Checkbox>
          </Form.Item>
          <Form.Item name="max_downloads" label="最大下载次数" extra="0 表示不限制">
            <InputNumber min={0} max={100000} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
