import { useState, useEffect } from 'react';
import {
  Table, Card, Button, Space, Tag, Modal, Form, Input, Select,
  message, Popconfirm, Switch, Tooltip, Row, Col, Statistic,
} from 'antd';
import {
  PlusOutlined, EditOutlined, DeleteOutlined, UnlockOutlined,
  LockOutlined, UserOutlined, LogoutOutlined, KeyOutlined,
  HistoryOutlined, DesktopOutlined, FieldTimeOutlined, SafetyOutlined,
} from '@ant-design/icons';
import { userApi } from '../services/api';
import type { User } from '../types';
import { useAuthStore } from '../store/useAuthStore';
import { COLORS } from '../utils/theme';

export default function Users() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [form] = Form.useForm();
  const { user: currentUser } = useAuthStore();

  // 重置密码
  const [resetPwdVisible, setResetPwdVisible] = useState(false);
  const [resetPwdUser, setResetPwdUser] = useState<User | null>(null);
  const [resetPwdForm] = Form.useForm();

  // 用户活动日志
  const [activitiesVisible, setActivitiesVisible] = useState(false);
  const [activities, setActivities] = useState<any[]>([]);
  const [activitiesLoading, setActivitiesLoading] = useState(false);
  const [activitiesUser, setActivitiesUser] = useState<User | null>(null);

  // 会话管理
  const [sessionsVisible, setSessionsVisible] = useState(false);
  const [sessions, setSessions] = useState<any[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);

  // 账号过期
  const [expiryVisible, setExpiryVisible] = useState(false);
  const [expiryUser, setExpiryUser] = useState<User | null>(null);
  const [expiryForm] = Form.useForm();

  // IP 白名单
  const [ipWhitelistVisible, setIpWhitelistVisible] = useState(false);
  const [ipWhitelistUser, setIpWhitelistUser] = useState<User | null>(null);
  const [ipWhitelistForm] = Form.useForm();

  useEffect(() => {
    fetchUsers();
  }, []);

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const res = await userApi.list();
      setUsers(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch users:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = () => {
    setEditingUser(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleEdit = (user: User) => {
    setEditingUser(user);
    form.setFieldsValue({
      username: user.username,
      role: user.role,
      is_locked: user.is_locked,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      await userApi.delete(id);
      message.success('删除成功');
      fetchUsers();
    } catch (error: any) {
      message.error(error.message || '删除失败');
    }
  };

  const handleUnlock = async (id: number) => {
    try {
      await userApi.unlock(id);
      message.success('解锁成功');
      fetchUsers();
    } catch (error: any) {
      message.error(error.message || '解锁失败');
    }
  };

  const handleToggleLock = async (user: User, locked: boolean) => {
    try {
      await userApi.update(user.id, { is_locked: locked });
      if (locked) {
        message.success(`已锁定用户 ${user.username}，该用户的所有会话已被强制登出`);
      } else {
        message.success(`已解锁用户 ${user.username}`);
      }
      fetchUsers();
    } catch (error: any) {
      message.error(error.message || '操作失败');
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();

      if (editingUser) {
        const updateData: any = { role: values.role };
        if (values.is_locked !== undefined) {
          updateData.is_locked = values.is_locked;
        }
        await userApi.update(editingUser.id, updateData);
        message.success('更新成功');
      } else {
        await userApi.create(values.username, values.password, values.role);
        message.success('创建成功');
      }

      setModalVisible(false);
      fetchUsers();
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    }
  };

  // 重置密码
  const showResetPassword = (user: User) => {
    setResetPwdUser(user);
    resetPwdForm.resetFields();
    setResetPwdVisible(true);
  };

  const handleResetPassword = async () => {
    try {
      const values = await resetPwdForm.validateFields();
      await userApi.resetPassword(resetPwdUser!.id, values.password);
      message.success(`已重置用户 ${resetPwdUser!.username} 的密码`);
      setResetPwdVisible(false);
    } catch (error: any) {
      message.error(error.message || '重置失败');
    }
  };

  // 用户活动日志
  const showActivities = async (user: User) => {
    setActivitiesUser(user);
    setActivitiesVisible(true);
    setActivitiesLoading(true);
    try {
      const res = await userApi.getActivities(user.id, 50);
      setActivities(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch activities:', error);
    } finally {
      setActivitiesLoading(false);
    }
  };

  // 会话管理
  const showSessions = async () => {
    setSessionsVisible(true);
    setSessionsLoading(true);
    try {
      const res = await userApi.getSessions();
      setSessions(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
    } finally {
      setSessionsLoading(false);
    }
  };

  // 账号过期
  const showExpiry = (user: User) => {
    setExpiryUser(user);
    expiryForm.resetFields();
    setExpiryVisible(true);
  };

  const handleSetExpiry = async () => {
    try {
      const values = await expiryForm.validateFields();
      const expiresAt = values.expires_at ? values.expires_at.toISOString() : null;
      await userApi.setExpiry(expiryUser!.id, expiresAt);
      message.success('已设置账号过期时间');
      setExpiryVisible(false);
    } catch (error: any) {
      message.error(error.message || '设置失败');
    }
  };

  // IP 白名单
  const showIPWhitelist = (user: User) => {
    setIpWhitelistUser(user);
    ipWhitelistForm.setFieldsValue({ ip_whitelist: user.ip_whitelist || '' });
    setIpWhitelistVisible(true);
  };

  const handleSetIPWhitelist = async () => {
    try {
      const values = await ipWhitelistForm.validateFields();
      await userApi.setIPWhitelist(ipWhitelistUser!.id, values.ip_whitelist || '');
      message.success('已设置 IP 白名单');
      setIpWhitelistVisible(false);
      fetchUsers(); // refresh to show updated whitelist
    } catch (error: any) {
      message.error(error.message || '设置失败');
    }
  };

  // 统计
  const stats = {
    total: users.length,
    admin: users.filter(u => u.role === 'admin').length,
    operator: users.filter(u => u.role === 'operator').length,
    viewer: users.filter(u => u.role === 'viewer').length,
    locked: users.filter(u => u.is_locked).length,
  };

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: '用户名',
      dataIndex: 'username',
      key: 'username',
      render: (text: string, record: User) => (
        <Space>
          <UserOutlined style={{ color: record.role === 'admin' ? COLORS.ERROR : COLORS.PRIMARY }} />
          {text}
          {record.id === currentUser?.id && <Tag color="blue">当前</Tag>}
        </Space>
      ),
    },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 100,
      render: (role: string) => {
        const colorMap: Record<string, string> = { admin: 'red', operator: 'blue', viewer: 'green' };
        const labelMap: Record<string, string> = { admin: '管理员', operator: '操作员', viewer: '观察者' };
        return <Tag color={colorMap[role]}>{labelMap[role] || role}</Tag>;
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (_: any, record: User) => (
        record.is_locked ? (
          <Tag icon={<LockOutlined />} color="error">已锁定</Tag>
        ) : (
          <Tag color="success">正常</Tag>
        )
      ),
    },
    {
      title: 'IP 白名单',
      dataIndex: 'ip_whitelist',
      key: 'ip_whitelist',
      width: 150,
      render: (wl: string) => {
        if (!wl) return <Tag>全部允许</Tag>;
        const ips = wl.split(',').length;
        return <Tooltip title={wl}><Tag color="orange">{ips} 个 IP</Tag></Tooltip>;
      },
    },
    {
      title: '最后登录',
      dataIndex: 'last_login_at',
      key: 'last_login_at',
      width: 180,
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 350,
      render: (_: any, record: User) => (
        <Space size="small" wrap>
          <Tooltip title="编辑">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(record)} disabled={record.id === currentUser?.id} />
          </Tooltip>
          <Tooltip title="重置密码">
            <Button type="link" size="small" icon={<KeyOutlined />} onClick={() => showResetPassword(record)} />
          </Tooltip>
          <Tooltip title="活动日志">
            <Button type="link" size="small" icon={<HistoryOutlined />} onClick={() => showActivities(record)} />
          </Tooltip>
          <Tooltip title="设置过期时间">
            <Button type="link" size="small" icon={<FieldTimeOutlined />} onClick={() => showExpiry(record)} />
          </Tooltip>
          <Tooltip title="IP 白名单">
            <Button type="link" size="small" icon={<SafetyOutlined />} onClick={() => showIPWhitelist(record)} />
          </Tooltip>
          {record.is_locked ? (
            <Tooltip title="解锁">
              <Button type="link" size="small" icon={<UnlockOutlined />} onClick={() => handleUnlock(record.id)} />
            </Tooltip>
          ) : (
            <Tooltip title={record.id === currentUser?.id ? '不能锁定自己' : '锁定'}>
              <Button type="link" size="small" icon={<LockOutlined />} onClick={() => handleToggleLock(record, true)} disabled={record.id === currentUser?.id} />
            </Tooltip>
          )}
          <Popconfirm title="确定要强制登出此用户吗？" onConfirm={() => handleToggleLock(record, true)} disabled={record.id === currentUser?.id}>
            <Tooltip title="强制登出">
              <Button type="link" size="small" icon={<LogoutOutlined />} disabled={record.id === currentUser?.id} />
            </Tooltip>
          </Popconfirm>
          <Popconfirm title="确定要删除此用户吗？" onConfirm={() => handleDelete(record.id)} disabled={record.id === currentUser?.id}>
            <Tooltip title="删除">
              <Button type="link" size="small" danger icon={<DeleteOutlined />} disabled={record.id === currentUser?.id} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {/* 统计卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small"><Statistic title="总用户" value={stats.total} prefix={<UserOutlined />} /></Card>
        </Col>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small"><Statistic title="管理员" value={stats.admin} styles={{ content: { color: '#ff4d4f' } }} /></Card>
        </Col>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small"><Statistic title="操作员" value={stats.operator} styles={{ content: { color: '#1890ff' } }} /></Card>
        </Col>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small"><Statistic title="观察者" value={stats.viewer} styles={{ content: { color: '#52c41a' } }} /></Card>
        </Col>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small"><Statistic title="已锁定" value={stats.locked} styles={{ content: { color: '#ff4d4f' } }} prefix={<LockOutlined />} /></Card>
        </Col>
        <Col xs={12} sm={8} lg={4}>
          <Card size="small" style={{ cursor: 'pointer' }} onClick={showSessions}>
            <Statistic title="在线会话" value={sessions.length} prefix={<DesktopOutlined />} />
          </Card>
        </Col>
      </Row>

      <Card
        title="用户管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>创建用户</Button>
        }
      >
        <Table
          columns={columns}
          dataSource={users}
          rowKey="id"
          loading={loading}
          pagination={{
            defaultPageSize: 20,
            showTotal: (total) => `共 ${total} 个用户`,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50'],
          }}
          size="small"
        />
      </Card>

      {/* 创建/编辑用户 */}
      <Modal
        title={editingUser ? '编辑用户' : '创建用户'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmit}
        okText="确定"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="username" label="用户名" rules={[{ required: !editingUser, message: '请输入用户名' }]}>
            <Input disabled={!!editingUser} />
          </Form.Item>
          {!editingUser && (
            <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }, { min: 6, message: '密码至少6位' }]}>
              <Input.Password />
            </Form.Item>
          )}
          <Form.Item name="role" label="角色" rules={[{ required: true, message: '请选择角色' }]}>
            <Select>
              <Select.Option value="admin">管理员</Select.Option>
              <Select.Option value="operator">操作员</Select.Option>
              <Select.Option value="viewer">观察者</Select.Option>
            </Select>
          </Form.Item>
          {editingUser && (
            <Form.Item name="is_locked" label="锁定状态" valuePropName="checked">
              <Switch checkedChildren="锁定" unCheckedChildren="正常" disabled={editingUser.id === currentUser?.id} />
            </Form.Item>
          )}
        </Form>
      </Modal>

      {/* 重置密码 */}
      <Modal
        title={`重置密码 - ${resetPwdUser?.username}`}
        open={resetPwdVisible}
        onCancel={() => setResetPwdVisible(false)}
        onOk={handleResetPassword}
        okText="重置"
        cancelText="取消"
      >
        <Form form={resetPwdForm} layout="vertical">
          <Form.Item name="password" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 6, message: '密码至少6位' }]}>
            <Input.Password placeholder="请输入新密码" />
          </Form.Item>
          <div style={{ color: '#666', fontSize: 12 }}>重置后用户需要使用新密码登录，且必须修改密码</div>
        </Form>
      </Modal>

      {/* 用户活动日志 */}
      <Modal
        title={`活动日志 - ${activitiesUser?.username}`}
        open={activitiesVisible}
        onCancel={() => setActivitiesVisible(false)}
        footer={null}
        width={700}
      >
        <Table
          dataSource={activities}
          rowKey="id"
          loading={activitiesLoading}
          size="small"
          pagination={{ pageSize: 20 }}
          columns={[
            { title: '时间', dataIndex: 'created_at', width: 180, render: (t: string) => new Date(t).toLocaleString() },
            { title: '操作', dataIndex: 'action', width: 120, render: (a: string) => <Tag color={a.includes('LOGIN') ? 'blue' : a.includes('PASSWORD') ? 'orange' : 'default'}>{a}</Tag> },
            { title: 'IP', dataIndex: 'ip', width: 130 },
            { title: 'User-Agent', dataIndex: 'user_agent', ellipsis: true },
          ]}
        />
      </Modal>

      {/* 会话管理 */}
      <Modal
        title="在线会话"
        open={sessionsVisible}
        onCancel={() => setSessionsVisible(false)}
        footer={null}
        width={800}
      >
        <Table
          dataSource={sessions}
          rowKey={(record) => `${record.user_id}-${record.login_at}`}
          loading={sessionsLoading}
          size="small"
          pagination={{ pageSize: 20 }}
          columns={[
            { title: '用户', dataIndex: 'username', width: 100 },
            { title: 'IP', dataIndex: 'ip', width: 130 },
            { title: '登录时间', dataIndex: 'login_at', width: 180, render: (t: string) => new Date(t).toLocaleString() },
            { title: '过期时间', dataIndex: 'expires_at', width: 180, render: (t: string) => new Date(t).toLocaleString() },
            { title: 'User-Agent', dataIndex: 'user_agent', ellipsis: true },
          ]}
        />
      </Modal>

      {/* 账号过期 */}
      <Modal
        title={`设置过期时间 - ${expiryUser?.username}`}
        open={expiryVisible}
        onCancel={() => setExpiryVisible(false)}
        onOk={handleSetExpiry}
        okText="确定"
        cancelText="取消"
      >
        <Form form={expiryForm} layout="vertical">
          <Form.Item name="expires_at" label="过期时间">
            <Input type="datetime-local" />
          </Form.Item>
          <div style={{ color: '#666', fontSize: 12 }}>留空表示永不过期</div>
        </Form>
      </Modal>

      {/* IP 白名单 */}
      <Modal
        title={`IP 白名单 - ${ipWhitelistUser?.username}`}
        open={ipWhitelistVisible}
        onCancel={() => setIpWhitelistVisible(false)}
        onOk={handleSetIPWhitelist}
        okText="确定"
        cancelText="取消"
      >
        <Form form={ipWhitelistForm} layout="vertical">
          <Form.Item name="ip_whitelist" label="允许的 IP 地址">
            <Input.TextArea
              placeholder="多个 IP 用逗号分隔，如: 192.168.1.1,10.0.0.1&#10;留空表示允许所有 IP"
              rows={3}
            />
          </Form.Item>
          <div style={{ color: '#666', fontSize: 12 }}>留空表示允许所有 IP 地址登录</div>
        </Form>
      </Modal>
    </div>
  );
}
