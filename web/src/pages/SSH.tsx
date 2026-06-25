import { useState, useEffect } from 'react';
import {
  Card, Tabs, Form, Input, InputNumber, Select, Button, Space, message,
  Table, Tag, Popconfirm, Alert, Spin,
} from 'antd';
import {
  SettingOutlined, TeamOutlined, HistoryOutlined,
  ReloadOutlined, SaveOutlined, DeleteOutlined,
} from '@ant-design/icons';
import api from '../services/api';

interface SSHConfig {
  port: number;
  permit_root_login: string;
  password_auth: string;
  pubkey_auth: string;
  max_auth_tries: number;
  login_grace_time: number;
  client_alive_interval: number;
  client_alive_count_max: number;
  allow_users: string;
  deny_users: string;
}

interface SSHSession {
  pid: number;
  user: string;
  tty: string;
  from: string;
  login_time: string;
}

interface SSHLoginRecord {
  time: string;
  user: string;
  ip: string;
  port: number;
  status: string;
  method: string;
}

export default function SSH() {
  const [configForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [sessions, setSessions] = useState<SSHSession[]>([]);
  const [logins, setLogins] = useState<SSHLoginRecord[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [loginsLoading, setLoginsLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('config');

  // Load SSH config on mount
  useEffect(() => {
    loadConfig();
  }, []);

  // Auto-load data when switching tabs
  useEffect(() => {
    if (activeTab === 'sessions') {
      loadSessions();
    } else if (activeTab === 'history') {
      loadLogins();
    }
  }, [activeTab]);

  const loadConfig = async () => {
    setLoading(true);
    try {
      const res = await api.get('/ssh/config');
      const config = res.data?.data;
      if (config) {
        configForm.setFieldsValue(config);
      }
    } catch {
      message.error('加载 SSH 配置失败');
    } finally {
      setLoading(false);
    }
  };

  // Save config
  const handleSave = async () => {
    try {
      const values = await configForm.validateFields();
      await api.put('/ssh/config', values);
      message.success('SSH 配置已保存');
    } catch {
      message.error('保存失败');
    }
  };

  // Test config
  const handleTest = async () => {
    try {
      const res = await api.post('/ssh/config/test');
      message.success(res.data?.data?.message || '配置有效');
    } catch {
      message.error('配置测试失败');
    }
  };

  // Reload SSH
  const handleReload = async () => {
    try {
      await api.post('/ssh/config/reload');
      message.success('SSH 服务已重载');
    } catch {
      message.error('重载失败');
    }
  };

  // Load sessions
  const loadSessions = async () => {
    setSessionsLoading(true);
    try {
      const res = await api.get('/ssh/sessions');
      setSessions(res.data?.data?.sessions || []);
    } catch {
      message.error('加载会话列表失败');
    } finally {
      setSessionsLoading(false);
    }
  };

  // Kill session
  const handleKillSession = async (pid: number) => {
    try {
      await api.post(`/ssh/sessions/${pid}/kill`);
      message.success('会话已终止');
      loadSessions();
    } catch {
      message.error('终止失败');
    }
  };

  // Load login history
  const loadLogins = async () => {
    setLoginsLoading(true);
    try {
      const res = await api.get('/ssh/logins?limit=100');
      setLogins(res.data?.data?.records || []);
    } catch {
      message.error('加载登录历史失败');
    } finally {
      setLoginsLoading(false);
    }
  };

  // Session columns
  const sessionColumns = [
    { title: '用户', dataIndex: 'user', key: 'user' },
    { title: 'TTY', dataIndex: 'tty', key: 'tty' },
    { title: '来源 IP', dataIndex: 'from', key: 'from' },
    { title: '登录时间', dataIndex: 'login_time', key: 'login_time' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: SSHSession) => (
        <Popconfirm
          title="确定终止此会话？"
          onConfirm={() => handleKillSession(record.pid)}
        >
          <Button icon={<DeleteOutlined />} size="small" danger>
            终止
          </Button>
        </Popconfirm>
      ),
    },
  ];

  // Login history columns
  const loginColumns = [
    { title: '时间', dataIndex: 'time', key: 'time' },
    { title: '用户', dataIndex: 'user', key: 'user' },
    { title: 'IP', dataIndex: 'ip', key: 'ip' },
    { title: '端口', dataIndex: 'port', key: 'port' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'success' ? 'green' : 'red'}>
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '方式',
      dataIndex: 'method',
      key: 'method',
      render: (method: string) => (
        <Tag>{method || 'unknown'}</Tag>
      ),
    },
  ];

  return (
    <div>
      <h2>SSH 管理</h2>
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'config',
            label: <span><SettingOutlined /> 配置</span>,
            children: (
              <Card title="SSH 服务器配置">
                <Alert
                  message="修改配置后需要点击「保存」并「重载服务」才能生效"
                  type="info"
                  showIcon
                  style={{ marginBottom: 16 }}
                />
                {loading ? (
                  <Spin />
                ) : (
                  <Form
                    form={configForm}
                    layout="vertical"
                    initialValues={{
                      port: 22,
                      permit_root_login: 'yes',
                      password_auth: 'yes',
                      pubkey_auth: 'yes',
                      max_auth_tries: 6,
                      login_grace_time: 120,
                      client_alive_interval: 0,
                      client_alive_count_max: 3,
                    }}
                  >
                    <Form.Item name="port" label="监听端口">
                      <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="permit_root_login" label="Root 登录">
                      <Select>
                        <Select.Option value="yes">允许</Select.Option>
                        <Select.Option value="no">禁止</Select.Option>
                        <Select.Option value="prohibit-password">仅密钥</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item name="password_auth" label="密码认证">
                      <Select>
                        <Select.Option value="yes">允许</Select.Option>
                        <Select.Option value="no">禁止</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item name="pubkey_auth" label="密钥认证">
                      <Select>
                        <Select.Option value="yes">允许</Select.Option>
                        <Select.Option value="no">禁止</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item name="max_auth_tries" label="最大尝试次数">
                      <InputNumber min={1} max={10} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="login_grace_time" label="登录超时（秒）">
                      <InputNumber min={30} max={600} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="client_alive_interval" label="心跳间隔（秒）">
                      <InputNumber min={0} max={3600} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="client_alive_count_max" label="心跳次数">
                      <InputNumber min={1} max={10} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="allow_users" label="允许用户" extra="留空表示不限制">
                      <Input placeholder="user1 user2" />
                    </Form.Item>
                    <Form.Item name="deny_users" label="拒绝用户">
                      <Input placeholder="user1 user2" />
                    </Form.Item>
                    <Form.Item>
                      <Space>
                        <Button type="primary" icon={<SaveOutlined />} onClick={handleSave}>
                          保存配置
                        </Button>
                        <Button onClick={handleTest}>
                          测试配置
                        </Button>
                        <Button icon={<ReloadOutlined />} onClick={handleReload}>
                          重载服务
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                )}
              </Card>
            ),
          },
          {
            key: 'sessions',
            label: <span><TeamOutlined /> 在线会话</span>,
            children: (
              <Card
                title="在线 SSH 会话"
                extra={
                  <Button icon={<ReloadOutlined />} onClick={loadSessions} loading={sessionsLoading}>
                    刷新
                  </Button>
                }
              >
                <Table
                  columns={sessionColumns}
                  dataSource={sessions}
                  rowKey="pid"
                  loading={sessionsLoading}
                  locale={{ emptyText: '暂无在线会话' }}
                />
              </Card>
            ),
          },
          {
            key: 'history',
            label: <span><HistoryOutlined /> 登录历史</span>,
            children: (
              <Card
                title="SSH 登录历史"
                extra={
                  <Button icon={<ReloadOutlined />} onClick={loadLogins} loading={loginsLoading}>
                    刷新
                  </Button>
                }
              >
                <Table
                  columns={loginColumns}
                  dataSource={logins}
                  rowKey={(_, index) => String(index)}
                  loading={loginsLoading}
                  locale={{ emptyText: '暂无登录记录' }}
                />
              </Card>
            ),
          },
        ]}
      />
    </div>
  );
}
