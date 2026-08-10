import {
  Button, Space, Modal, Form, Input, Select, Table, Empty, Popconfirm,
} from 'antd';
import {
  PlusOutlined, DeleteOutlined, ReloadOutlined, KeyOutlined,
} from '@ant-design/icons';
import type { UsersTabProps, DBUser } from './types';

// 用户 tab — 用户列表 + 创建用户/授权弹窗，随 tab 走。
export default function UsersTab({
  version, dbUsers, usersLoading, busy, databases,
  onRefreshUsers, onDeleteUser,
  userModalVisible, onUserModalVisibleChange, userForm, onCreateUser,
  grantVisible, grantUser, grantForm, onGrantVisibleChange, onGrant, onOpenGrant,
}: UsersTabProps) {
  const userColumns = [
    { title: '用户名', dataIndex: 'username', key: 'username', render: (t: string) => <strong>{t}</strong> },
    { title: '主机', dataIndex: 'host', key: 'host', width: 160, render: (t: string) => t || '-' },
    {
      title: '操作', key: 'action', width: 180,
      render: (_: unknown, record: DBUser) => (
        <Space size="small">
          <Button type="link" size="small" icon={<KeyOutlined />}
            onClick={() => onOpenGrant(record)}>授权</Button>
          <Popconfirm title="确定删除此用户？" onConfirm={() => onDeleteUser(record)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-user-${record.username}@${record.host}`}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <Button icon={<ReloadOutlined />} loading={usersLoading} onClick={onRefreshUsers}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />}
          onClick={() => { userForm.resetFields(); onUserModalVisibleChange(true); }}
          disabled={version.status !== 'running'}>创建用户</Button>
      </div>
      <Table columns={userColumns} dataSource={dbUsers} rowKey={(r: DBUser) => `${r.username}@${r.host}`} loading={usersLoading} size="small"
        locale={{ emptyText: <Empty description="暂无用户" /> }} />

      {/* 创建用户弹窗 */}
      <Modal title="创建用户" open={userModalVisible} onCancel={() => onUserModalVisibleChange(false)}
        onOk={onCreateUser} okText="创建" cancelText="取消" confirmLoading={busy === 'create-user'}>
        <Form form={userForm} layout="vertical">
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}><Input placeholder="如：app_user" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }, { min: 6 }]}><Input.Password /></Form.Item>
          <Form.Item name="host" label="主机" initialValue="localhost">
            <Select><Select.Option value="localhost">localhost</Select.Option><Select.Option value="%">任意主机（%）</Select.Option></Select>
          </Form.Item>
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
            <Select mode="multiple">
              <Select.Option value="ALL PRIVILEGES">全部权限</Select.Option>
              <Select.Option value="SELECT">SELECT</Select.Option>
              <Select.Option value="INSERT">INSERT</Select.Option>
              <Select.Option value="UPDATE">UPDATE</Select.Option>
              <Select.Option value="DELETE">DELETE</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
