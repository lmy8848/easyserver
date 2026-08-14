import { useState } from 'react';
import {
  Button, Space, Modal, Form, Input, Select, Table, Empty, Popconfirm, Tag,
} from 'antd';
import {
  DeleteOutlined, KeyOutlined, LockOutlined, ReloadOutlined, PlusOutlined,
} from '@ant-design/icons';
import type { UsersTabProps, DBUser } from './types';

const MYSQL_PRIVILEGES = [
  'ALL PRIVILEGES', 'SELECT', 'INSERT', 'UPDATE', 'DELETE',
  'CREATE', 'DROP', 'ALTER', 'INDEX', 'REFERENCES', 'EXECUTE',
  'CREATE VIEW', 'SHOW VIEW', 'CREATE ROUTINE', 'ALTER ROUTINE',
  'CREATE TEMPORARY TABLES', 'LOCK TABLES', 'EVENT', 'TRIGGER', 'GRANT OPTION',
];

const PG_PRIVILEGES = [
  'ALL PRIVILEGES', 'SELECT', 'INSERT', 'UPDATE', 'DELETE',
  'TRUNCATE', 'CREATE', 'CONNECT', 'TEMPORARY', 'EXECUTE',
  'USAGE', 'REFERENCES', 'TRIGGER', 'MAINTAIN',
];

export default function UsersTab({
  server, version, dbUsers, usersLoading, busy, databases,
  onFetchUsers, onOpenCreateUser,
  onDeleteUser,
  userModalVisible, onUserModalVisibleChange, userForm, onCreateUser,
  grantVisible, grantUser, grantForm, onGrantVisibleChange, onGrant, onOpenGrant,
  resetPasswordVisible, resetPasswordUser, resetPasswordForm, onResetPasswordVisibleChange, onResetPassword, onOpenResetPassword,
}: UsersTabProps) {
  const isPg = server?.db_type === 'postgresql';
  const isRedis = server?.db_type === 'redis';
  const privileges = isPg ? PG_PRIVILEGES : MYSQL_PRIVILEGES;
  const [searchText, setSearchText] = useState('');
  const filteredUsers = dbUsers.filter(u => u.username.toLowerCase().includes(searchText.toLowerCase()));

  if (isRedis) {
    return <Empty description="暂不支持 Redis 用户管理" />;
  }

  const userColumns = [
    { title: '用户名', dataIndex: 'username', key: 'username', render: (t: string) => <strong>{t}</strong> },
    ...(!isPg ? [{ title: '主机', dataIndex: 'host', key: 'host', width: 160, render: (t: string) => t || '-' }] : []),
    {
      title: '操作', key: 'action', width: 240,
      render: (_: unknown, record: DBUser) => (
        <Space size="small">
          <Button type="link" size="small" icon={<KeyOutlined />}
            onClick={() => onOpenGrant(record)}>授权</Button>
          <Button type="link" size="small" icon={<LockOutlined />}
            onClick={() => onOpenResetPassword(record)}>重置密码</Button>
          <Popconfirm title="确定删除此用户？" onConfirm={() => onDeleteUser(record)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-user-${record.username}@${record.host}`}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space size="middle">
          <span style={{ fontSize: 16, fontWeight: 'bold' }}>用户列表</span>
          <Tag color="blue">共 {filteredUsers.length} 个</Tag>
        </Space>
        <Space>
          <Input.Search
            placeholder="搜索用户名"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
            allowClear
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={onOpenCreateUser} disabled={!version}>创建用户</Button>
          <Button icon={<ReloadOutlined />} loading={usersLoading} onClick={onFetchUsers}>刷新</Button>
        </Space>
      </div>

      <Table
        columns={userColumns}
        dataSource={filteredUsers}
        rowKey={(r: DBUser) => `${r.username}@${r.host}`}
        loading={usersLoading}
        size="small"
        pagination={{ defaultPageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 条` }}
        locale={{ emptyText: <Empty description="暂无用户" /> }}
      />

      {/* 创建用户弹窗 */}
      <Modal title="创建用户" open={userModalVisible} onCancel={() => onUserModalVisibleChange(false)}
        onOk={onCreateUser} okText="创建" cancelText="取消" confirmLoading={busy === 'create-user'}>
        <Form form={userForm} layout="vertical">
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}><Input placeholder="如：app_user" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }, { min: 6 }]}><Input.Password /></Form.Item>
          {!isPg && (
            <Form.Item name="host" label="主机" initialValue="localhost">
              <Select><Select.Option value="localhost">localhost</Select.Option><Select.Option value="%">任意主机（%）</Select.Option></Select>
            </Form.Item>
          )}
        </Form>
      </Modal>

      {/* 授权弹窗 */}
      <Modal title={`授权 - ${grantUser?.username || ''}`} open={grantVisible} onCancel={() => onGrantVisibleChange(false)}
        onOk={onGrant} okText="授权" cancelText="取消" confirmLoading={busy === 'grant'}>
        <Form form={grantForm} layout="vertical">
          <Form.Item name="database" label="数据库" rules={[{ required: true }]}>
            <Select>{databases.map(db => <Select.Option key={db.name} value={db.name}>{db.name}</Select.Option>)}</Select>
          </Form.Item>
          <Form.Item name="privileges" label="权限" rules={[{ required: true }]}>
            <Select mode="multiple" placeholder="请选择权限">
              {privileges.map(p => (
                <Select.Option key={p} value={p}>{p}</Select.Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 重置密码弹窗 */}
      <Modal title={`重置密码 - ${resetPasswordUser?.username || ''}`} open={resetPasswordVisible} onCancel={() => onResetPasswordVisibleChange(false)}
        onOk={onResetPassword} okText="确定" cancelText="取消" confirmLoading={busy === 'reset-password'}>
        <Form form={resetPasswordForm} layout="vertical">
          <Form.Item name="password" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 6, message: '密码至少 6 位' }]}>
            <Input.Password placeholder="请输入新密码（至少 6 位）" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
