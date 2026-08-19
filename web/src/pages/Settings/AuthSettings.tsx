import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import {
  Card, Form, Button, message, InputNumber, Switch, Select, Row, Col,
  Modal, Input, Typography, Space, Alert, Table, Popconfirm, Tag, QRCode, Avatar,
} from 'antd';
import {
  SafetyOutlined, CopyOutlined, DownloadOutlined,
  LockOutlined, DesktopOutlined, DeleteOutlined, LogoutOutlined, ReloadOutlined,
  UserOutlined, EditOutlined, KeyOutlined,
} from '@ant-design/icons';
import { settingsApi, authApi } from '../../services/api';
import { useAuthStore } from '../../store/useAuthStore';
import { copyToClipboard } from '../../utils/clipboard';
import { formatDateTime } from '../../utils/format';
import type { Settings } from './types';

const { Text, Paragraph } = Typography;

interface Session {
  user_id: number;
  ip: string;
  user_agent: string;
  client_type: string;
  is_current: boolean;
  login_at: string;
  expires_at: string;
  token?: string;
}

export interface AuthSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function AuthSettings({ settings, onRefresh }: AuthSettingsProps) {
  const location = useLocation();
  const { user, updateUser, logout } = useAuthStore();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  // User & Password modal state
  const [showChangeUserModal, setShowChangeUserModal] = useState(false);
  const [changeUserLoading, setChangeUserLoading] = useState(false);
  const [changeUserForm] = Form.useForm();

  const [showChangePassModal, setShowChangePassModal] = useState(false);
  const [changePassLoading, setChangePassLoading] = useState(false);
  const [changePassForm] = Form.useForm();

  const handleChangeUsername = async (values: { new_username: string; password: string }) => {
    setChangeUserLoading(true);
    try {
      const res = await authApi.changeUsername(values.new_username, values.password);
      if (res.data?.data?.user) {
        updateUser(res.data.data.user);
      }
      message.success('用户名修改成功');
      setShowChangeUserModal(false);
      changeUserForm.resetFields();
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '用户名修改失败');
    } finally {
      setChangeUserLoading(false);
    }
  };

  const handleChangePassword = async (values: { old_password: string; new_password: string; confirm_password: string }) => {
    if (values.old_password === values.new_password) {
      message.error('新密码不能与当前密码相同');
      return;
    }
    if (values.new_password !== values.confirm_password) {
      message.error('两次输入的密码不一致');
      return;
    }
    setChangePassLoading(true);
    try {
      await authApi.changePassword(values.old_password, values.new_password);
      message.success('密码修改成功，请重新登录');
      setShowChangePassModal(false);
      changePassForm.resetFields();
      localStorage.removeItem('must_change_pass');
      logout();
      window.location.href = '/login';
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '密码修改失败');
    } finally {
      setChangePassLoading(false);
    }
  };

  // 2FA / TOTP state
  const [totpLoading, setTotpLoading] = useState(false);
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [setupData, setSetupData] = useState<{
    secret: string;
    otpauth_url: string;
  } | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [showSetupModal, setShowSetupModal] = useState(false);
  const [showDisableModal, setShowDisableModal] = useState(false);
  const [verifyCode, setVerifyCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [setupStep, setSetupStep] = useState<'qr' | 'verify' | 'backup'>('qr');

  // Session state
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);

  const checkTOTPStatus = async () => {
    try {
      const response = await authApi.getTOTPStatus();
      setTotpEnabled(response.data.data.enabled);
    } catch (error) {
      console.error('Failed to check TOTP status:', error);
    }
  };

  const fetchSessions = async () => {
    setSessionsLoading(true);
    try {
      const response = await authApi.getSessions();
      setSessions(response.data.data || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取会话列表失败'));
    } finally {
      setSessionsLoading(false);
    }
  };

  useEffect(() => {
    checkTOTPStatus();
    fetchSessions();
  }, []);

  useEffect(() => {
    const hash = location.hash;
    let timer: ReturnType<typeof setTimeout> | undefined;
    if (hash === '#2fa') {
      timer = setTimeout(() => {
        const el = document.getElementById('2fa');
        if (el) {
          el.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 100);
    } else if (hash === '#admin-account' || hash === '#account') {
      timer = setTimeout(() => {
        const el = document.getElementById('admin-account');
        if (el) {
          el.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
      }, 100);
    }
    return () => {
      if (timer) clearTimeout(timer);
    };
  }, [location.key, location.hash]);

  useEffect(() => {
    if (settings?.auth) {
      form.setFieldsValue({
        session_timeout: settings.auth.session_timeout,
        idle_timeout: settings.auth.idle_timeout,
        max_login_attempts: settings.auth.max_login_attempts,
        lockout_duration: settings.auth.lockout_duration,
        rate_limit: settings.auth.rate_limit,
        rate_interval: settings.auth.rate_interval,
        login_rate_limit: settings.auth.login_rate_limit,
        login_rate_interval: settings.auth.login_rate_interval,
        allow_multi_session: settings.auth.allow_multi_session,
        ip_whitelist: settings.auth.ip_whitelist ?? [],
        session_cleanup_interval: settings.auth.session_cleanup_interval,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      const payload = {
        ...values,
        ip_whitelist: (values.ip_whitelist ?? []).map((s: string) => s.trim()).filter(Boolean),
      };
      await settingsApi.updateAuth(payload);
      message.success('认证配置已保存');
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    } finally {
      setSaving(false);
    }
  };

  const handleEnableTOTP = async () => {
    setTotpLoading(true);
    try {
      const response = await authApi.setupTOTP();
      setSetupData(response.data.data);
      setShowSetupModal(true);
      setSetupStep('qr');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : 'TOTP 设置失败'));
    } finally {
      setTotpLoading(false);
    }
  };

  const handleVerifyCode = async () => {
    if (!verifyCode || verifyCode.length !== 6) {
      message.error('请输入6位验证码');
      return;
    }

    setTotpLoading(true);
    try {
      const response = await authApi.enableTOTP(verifyCode);
      setBackupCodes(response.data.data.backup_codes);
      setSetupStep('backup');
      setTotpEnabled(true);
      message.success('2FA 已启用');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '验证码错误'));
    } finally {
      setTotpLoading(false);
    }
  };

  const handleDisableTOTP = async () => {
    if (!disablePassword) {
      message.error('请输入密码');
      return;
    }

    setTotpLoading(true);
    try {
      await authApi.disableTOTP(disablePassword);
      setTotpEnabled(false);
      setShowDisableModal(false);
      setDisablePassword('');
      message.success('2FA 已禁用');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '密码错误'));
    } finally {
      setTotpLoading(false);
    }
  };

  const copyBackupCodes = () => {
    const text = backupCodes.join('\n');
    copyToClipboard(text, '备份码已复制到剪贴板');
  };

  const downloadBackupCodes = () => {
    const text = `EasyServer 备份码\n\n${backupCodes.join('\n')}\n\n请妥善保管，每个备份码只能使用一次。`;
    const blob = new Blob([text], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'easyserver-backup-codes.txt';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    message.success('备份码已下载');
  };

  const handleCloseSetupModal = () => {
    setShowSetupModal(false);
    setSetupData(null);
    setVerifyCode('');
    setBackupCodes([]);
    setSetupStep('qr');
    checkTOTPStatus();
  };

  const handleKickSession = async (token: string) => {
    try {
      await authApi.kickSession(token);
      message.success('已踢出该会话');
      fetchSessions();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '踢出会话失败'));
    }
  };

  const handleKickAllOtherSessions = async () => {
    try {
      await authApi.kickAllOtherSessions();
      message.success('已踢出所有其他会话');
      fetchSessions();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '踢出会话失败'));
    }
  };

  return (
    <div>
      {/* Administrator Account Card */}
      <Card
        id="admin-account"
        title={
          <Space>
            <UserOutlined />
            <span>管理员账户</span>
          </Space>
        }
        style={{
          marginBottom: 16,
          ...(location.hash === '#admin-account' || location.hash === '#account'
            ? { borderColor: '#6366f1', boxShadow: '0 0 0 2px rgba(99, 102, 241, 0.2)' }
            : {}),
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 16 }}>
          <Space size="middle">
            <Avatar size={48} style={{ backgroundColor: '#6366f1' }}>
              {user?.username?.[0]?.toUpperCase() || 'A'}
            </Avatar>
            <div>
              <div style={{ fontSize: 16, fontWeight: 600 }}>{user?.username || 'admin'}</div>
              <Text type="secondary">角色: 管理员 · 账户 ID: {user?.id || 1}</Text>
            </div>
          </Space>
          <Space>
            <Button icon={<EditOutlined />} onClick={() => setShowChangeUserModal(true)}>
              修改用户名
            </Button>
            <Button icon={<KeyOutlined />} onClick={() => setShowChangePassModal(true)}>
              修改密码
            </Button>
          </Space>
        </div>
      </Card>

      <Card title="认证安全配置">
        <Form
          form={form}
          layout="vertical"
        >
          <Row gutter={[24, 0]}>
            <Col xs={24} sm={12}>
              <Form.Item
                name="session_timeout"
                label="会话超时"
                extra="用户会话的有效持续时间（秒，默认 86400 秒 = 24 小时）"
                rules={[{ required: true, message: '请输入会话超时秒数' }]}
              >
                <InputNumber min={300} suffix="秒" style={{ width: '100%' }} placeholder="86400" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="idle_timeout"
                label="空闲超时"
                extra="用户无操作后自动登出的时间（秒，默认 1800 秒 = 30 分钟）"
                rules={[{ required: true, message: '请输入空闲超时秒数' }]}
              >
                <InputNumber min={60} suffix="秒" style={{ width: '100%' }} placeholder="1800" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="max_login_attempts"
                label="最大登录尝试次数"
                extra="登录失败多少次后锁定账户"
                rules={[{ required: true, message: '请输入最大次数' }]}
              >
                <InputNumber min={3} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="lockout_duration"
                label="锁定时长"
                extra="账户锁定的持续时间（秒，默认 900 秒 = 15 分钟）"
                rules={[{ required: true, message: '请输入锁定时长秒数' }]}
              >
                <InputNumber min={60} max={86400} suffix="秒" style={{ width: '100%' }} placeholder="900" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="rate_limit"
                label="速率限制"
                extra="每个时间窗口内允许的最大请求数"
                rules={[{ required: true, message: '请输入速率限制' }]}
              >
                <InputNumber min={10} style={{ width: '100%' }} />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="rate_interval"
                label="速率限制时间窗口"
                extra="通用 API 速率限制的时间窗口（秒，默认 60 秒 = 1 分钟）"
                rules={[{ required: true, message: '请输入时间窗口秒数' }]}
              >
                <InputNumber min={1} suffix="秒" style={{ width: '100%' }} placeholder="60" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="login_rate_limit"
                label="登录速率限制"
                extra="每个时间窗口内登录接口允许的最大请求数"
                rules={[{ required: true, message: '请输入登录速率限制' }]}
              >
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="login_rate_interval"
                label="登录限流时间窗口"
                extra="登录速率限制的时间窗口（秒，默认 60 秒 = 1 分钟）"
                rules={[{ required: true, message: '请输入登录限流时间窗口秒数' }]}
              >
                <InputNumber min={1} max={3600} suffix="秒" style={{ width: '100%' }} placeholder="60" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="session_cleanup_interval"
                label="会话清理间隔"
                extra="过期会话的清理周期（秒，默认 300 秒 = 5 分钟）。仅在重启后生效。"
                rules={[{ required: true, message: '请输入会话清理间隔秒数' }]}
              >
                <InputNumber min={60} max={86400} suffix="秒" style={{ width: '100%' }} placeholder="300" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="allow_multi_session"
                label="允许多端同时登录"
                valuePropName="checked"
                extra="开启后新登录不会踢出其他设备会话；关闭后新登录使其他设备下线。扫码登录始终共存。"
              >
                <Switch />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item
                name="ip_whitelist"
                label="访问 IP 白名单"
                extra="仅允许列出的 IP/CIDR 访问面板，留空则放行所有 IP。每行一个（如 192.168.1.0/24 或 10.0.0.1）。⚠️ 配置错误可能导致自身被拒之门外，请谨慎。"
                rules={[
                  {
                    validator: (_, v: string[]) => {
                      const list = (v ?? []).map((s) => s.trim()).filter(Boolean);
                      if (list.every((s) => /^[\d.:a-fA-F/]+$/.test(s))) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error('存在非法 IP 或 CIDR 格式'));
                    },
                  },
                ]}
              >
                <Select
                  mode="tags"
                  open={false}
                  suffixIcon={null}
                  placeholder="每行一个 IP 或 CIDR，回车添加"
                  tokenSeparators={[',', '，']}
                />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item>
                <Button
                  type="primary"
                  onClick={handleSave}
                  loading={saving}
                >
                  保存配置
                </Button>
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Card>

      {/* 2FA Card */}
      <Card
        id="2fa"
        style={{
          marginTop: 16,
          transition: 'border-color 0.3s, box-shadow 0.3s',
          ...(location.hash === '#2fa' ? { borderColor: '#6366f1', boxShadow: '0 0 0 2px rgba(99, 102, 241, 0.2)' } : {}),
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Space>
            <SafetyOutlined style={{ fontSize: 24, color: totpEnabled ? '#52c41a' : '#d9d9d9' }} />
            <div>
              <Text strong style={{ fontSize: 16 }}>双因素认证 (2FA)</Text>
              <br />
              <Text type="secondary">
                {totpEnabled ? '已启用 - 使用验证器应用生成验证码' : '未启用 - 添加额外的安全层'}
              </Text>
            </div>
          </Space>
          <Switch
            checked={totpEnabled}
            onChange={(checked) => {
              if (checked) {
                handleEnableTOTP();
              } else {
                setShowDisableModal(true);
              }
            }}
            loading={totpLoading}
          />
        </div>

        {totpEnabled && (
          <Alert
            title="2FA 已启用"
            description="您的账户已启用双因素认证。登录时需要输入验证器应用中的验证码。"
            type="success"
            showIcon
            style={{ marginTop: 16 }}
          />
        )}
      </Card>

      {/* Setup 2FA Modal */}
      <Modal
        title="设置双因素认证"
        open={showSetupModal}
        onCancel={handleCloseSetupModal}
        footer={null}
        width={500}
      >
        {setupStep === 'qr' && setupData && (
          <div style={{ textAlign: 'center' }}>
            <Paragraph>
              使用验证器应用（如 Google Authenticator、Microsoft Authenticator）扫描下方二维码：
            </Paragraph>
            <div style={{ margin: '24px 0', display: 'flex', justifyContent: 'center' }}>
              <QRCode
                value={setupData.otpauth_url}
                size={200}
                bordered={false}
              />
            </div>
            <div style={{ margin: '16px 0 24px' }}>
              <Paragraph type="secondary" style={{ marginBottom: 8 }}>
                或在验证器应用中手动输入密钥：
              </Paragraph>
              <Space.Compact block>
                <Input
                  readOnly
                  value={setupData.secret}
                  style={{ textAlign: 'center', fontFamily: 'monospace', fontWeight: 600, letterSpacing: 1 }}
                />
                <Button
                  icon={<CopyOutlined />}
                  onClick={() => copyToClipboard(setupData.secret, '密钥已复制到剪贴板')}
                >
                  复制
                </Button>
              </Space.Compact>
            </div>
            <Button type="primary" onClick={() => setSetupStep('verify')} block>
              下一步
            </Button>
          </div>
        )}

        {setupStep === 'verify' && (
          <div style={{ textAlign: 'center' }}>
            <Paragraph style={{ marginBottom: 20 }}>
              输入验证器应用中显示的 6 位验证码：
            </Paragraph>
            <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 24 }}>
              <Input.OTP
                size="large"
                length={6}
                value={verifyCode}
                onChange={(text) => setVerifyCode(text)}
              />
            </div>
            <Button
              type="primary"
              size="large"
              onClick={handleVerifyCode}
              loading={totpLoading}
              block
            >
              验证并启用
            </Button>
          </div>
        )}

        {setupStep === 'backup' && (
          <div>
            <Alert
              title="请保存备份码"
              description="这些备份码可以在您无法使用验证器应用时用于登录。每个备份码只能使用一次。"
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <div
              style={{
                background: '#fafafa',
                border: '1px solid #f0f0f0',
                borderRadius: 8,
                padding: 12,
                marginBottom: 16,
              }}
            >
              <Row gutter={[12, 12]}>
                {backupCodes.map((code, idx) => (
                  <Col span={12} key={idx}>
                    <div
                      style={{
                        background: '#fff',
                        border: '1px solid #e8e8e8',
                        borderRadius: 6,
                        padding: '8px 0',
                        textAlign: 'center',
                        fontFamily: 'monospace',
                        fontSize: 15,
                        fontWeight: 600,
                        letterSpacing: 1,
                        color: '#262626',
                      }}
                    >
                      {code}
                    </div>
                  </Col>
                ))}
              </Row>
            </div>
            <Space style={{ width: '100%', justifyContent: 'center', marginBottom: 16 }}>
              <Button icon={<CopyOutlined />} onClick={copyBackupCodes}>
                复制
              </Button>
              <Button icon={<DownloadOutlined />} onClick={downloadBackupCodes}>
                下载
              </Button>
            </Space>
            <Button
              type="primary"
              onClick={handleCloseSetupModal}
              block
            >
              完成
            </Button>
          </div>
        )}
      </Modal>

      {/* Disable 2FA Modal */}
      <Modal
        title="禁用双因素认证"
        open={showDisableModal}
        onCancel={() => {
          setShowDisableModal(false);
          setDisablePassword('');
        }}
        footer={null}
      >
        <Alert
          title="确定要禁用 2FA？"
          description="禁用后，登录时将不再需要验证码。这会降低您的账户安全性。"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Paragraph>请输入密码以确认：</Paragraph>
        <Input.Password
          prefix={<LockOutlined />}
          placeholder="密码"
          value={disablePassword}
          onChange={(e) => setDisablePassword(e.target.value)}
          style={{ marginBottom: 16 }}
        />
        <Button
          type="primary"
          danger
          onClick={handleDisableTOTP}
          loading={totpLoading}
          block
        >
          禁用 2FA
        </Button>
      </Modal>

      {/* Session Management */}
      <Card title={<Space><DesktopOutlined /> 会话管理</Space>} style={{ marginTop: 16 }}>
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text type="secondary">查看当前活跃会话，管理登录设备</Text>
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchSessions} loading={sessionsLoading}>
              刷新
            </Button>
            <Popconfirm
              title="确定要踢出所有其他设备？"
              onConfirm={handleKickAllOtherSessions}
              okText="确定"
              cancelText="取消"
            >
              <Button icon={<LogoutOutlined />} danger>
                踢出所有其他设备
              </Button>
            </Popconfirm>
          </Space>
        </div>
        <Table
          columns={[
            { title: 'IP 地址', dataIndex: 'ip', key: 'ip', width: 150 },
            { title: '类型', dataIndex: 'client_type', key: 'client_type', width: 80,
              render: (t: string) => t === 'mobile' ? <Tag color="blue">移动</Tag> : <Tag>Web</Tag> },
            { title: '设备', key: 'device', ellipsis: true,
              render: (_: unknown, r: Session) => r.user_agent || '-' },
            { title: '登录时间', dataIndex: 'login_at', key: 'login_at', width: 180,
              render: (t: string) => t ? formatDateTime(t) : '-' },
            { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 180,
              render: (t: string) => t ? formatDateTime(t) : '-' },
            { title: '操作', key: 'action', width: 100,
              render: (_: unknown, record: Session) => (
                record.token && !record.is_current ? (
                  <Popconfirm
                    title="确定要踢出此设备？"
                    onConfirm={() => handleKickSession(record.token!)}
                    okText="确定"
                    cancelText="取消"
                  >
                    <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                      踢出
                    </Button>
                  </Popconfirm>
                ) : record.is_current ? <Text type="secondary">当前</Text> : null
              ),
            },
          ]}
          dataSource={sessions}
          rowKey={(record) => record.token || record.ip + record.login_at}
          loading={sessionsLoading}
          size="small"
          pagination={false}
        />
      </Card>

      {/* 修改用户名弹窗 */}
      <Modal
        title="修改用户名"
        open={showChangeUserModal}
        onCancel={() => {
          if (!changeUserLoading) {
            setShowChangeUserModal(false);
            changeUserForm.resetFields();
          }
        }}
        footer={null}
        destroyOnClose
        centered
        width={420}
      >
        <Form
          form={changeUserForm}
          layout="vertical"
          onFinish={handleChangeUsername}
          autoComplete="off"
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="new_username"
            label="新用户名"
            rules={[
              { required: true, message: '请输入新用户名' },
              { min: 3, max: 32, message: '用户名长度需为 3-32 位' },
              { pattern: /^[a-zA-Z0-9_-]+$/, message: '仅支持字母、数字、下划线或短横线' },
            ]}
          >
            <Input placeholder="请输入新用户名" />
          </Form.Item>

          <Form.Item
            name="password"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码以验证身份' }]}
            extra="为了保障账户安全，修改用户名需要验证当前密码"
          >
            <Input.Password placeholder="请输入当前密码" />
          </Form.Item>

          <Form.Item style={{ marginBottom: 0, textAlign: 'right', marginTop: 24 }}>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <Button onClick={() => { setShowChangeUserModal(false); changeUserForm.resetFields(); }} disabled={changeUserLoading}>
                取消
              </Button>
              <Button type="primary" htmlType="submit" loading={changeUserLoading}>
                确认修改
              </Button>
            </div>
          </Form.Item>
        </Form>
      </Modal>

      {/* 修改密码弹窗 */}
      <Modal
        title="修改密码"
        open={showChangePassModal}
        onCancel={() => {
          if (!changePassLoading) {
            setShowChangePassModal(false);
            changePassForm.resetFields();
          }
        }}
        footer={null}
        destroyOnClose
        centered
        width={420}
      >
        <Form
          form={changePassForm}
          layout="vertical"
          onFinish={handleChangePassword}
          autoComplete="off"
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="old_password"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password placeholder="请输入当前密码" />
          </Form.Item>

          <Form.Item
            name="new_password"
            label="新密码"
            dependencies={['old_password']}
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '密码至少8个字符' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (value && getFieldValue('old_password') === value) {
                    return Promise.reject(new Error('新密码不能与当前密码相同'));
                  }
                  return Promise.resolve();
                },
              }),
            ]}
            extra="密码需包含大写字母、小写字母和数字，至少8位"
          >
            <Input.Password placeholder="请输入新密码" />
          </Form.Item>

          <Form.Item
            name="confirm_password"
            label="确认新密码"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请确认新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('new_password') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'));
                },
              }),
            ]}
          >
            <Input.Password placeholder="请再次输入新密码" />
          </Form.Item>

          <Form.Item style={{ marginBottom: 0, textAlign: 'right', marginTop: 24 }}>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <Button onClick={() => { setShowChangePassModal(false); changePassForm.resetFields(); }} disabled={changePassLoading}>
                取消
              </Button>
              <Button type="primary" htmlType="submit" loading={changePassLoading}>
                确认修改
              </Button>
            </div>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
