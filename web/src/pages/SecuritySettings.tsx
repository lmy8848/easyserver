import { useState, useEffect } from 'react';
import { Card, Button, Switch, Modal, Input, message, Typography, Space, List, Alert, Table, Popconfirm, Tag } from 'antd';
import { SafetyOutlined, KeyOutlined, CopyOutlined, DownloadOutlined, LockOutlined, DesktopOutlined, DeleteOutlined, LogoutOutlined, ReloadOutlined } from '@ant-design/icons';
import { authApi } from '../services/api';

const { Title, Text, Paragraph } = Typography;

interface Session {
  user_id: number;
  username: string;
  role: string;
  ip: string;
  user_agent: string;
  client_type: string;
  device_id?: string;
  device_info?: string;
  is_current: boolean;
  login_at: string;
  expires_at: string;
  token?: string;
}

export default function SecuritySettings() {
  const [loading, setLoading] = useState(false);
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [setupData, setSetupData] = useState<{
    secret: string;
    otpauth_url: string;
    qr_code_base64: string;
  } | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [showSetupModal, setShowSetupModal] = useState(false);
  const [showDisableModal, setShowDisableModal] = useState(false);
  const [verifyCode, setVerifyCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [setupStep, setSetupStep] = useState<'qr' | 'verify' | 'backup'>('qr');
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

  const handleEnableTOTP = async () => {
    setLoading(true);
    try {
      const response = await authApi.setupTOTP();
      setSetupData(response.data.data);
      setShowSetupModal(true);
      setSetupStep('qr');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : 'TOTP 设置失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyCode = async () => {
    if (!verifyCode || verifyCode.length !== 6) {
      message.error('请输入6位验证码');
      return;
    }

    setLoading(true);
    try {
      const response = await authApi.enableTOTP(verifyCode);
      setBackupCodes(response.data.data.backup_codes);
      setSetupStep('backup');
      setTotpEnabled(true);
      message.success('2FA 已启用');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '验证码错误'));
    } finally {
      setLoading(false);
    }
  };

  const handleDisableTOTP = async () => {
    if (!disablePassword) {
      message.error('请输入密码');
      return;
    }

    setLoading(true);
    try {
      await authApi.disableTOTP(disablePassword);
      setTotpEnabled(false);
      setShowDisableModal(false);
      setDisablePassword('');
      message.success('2FA 已禁用');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '密码错误'));
    } finally {
      setLoading(false);
    }
  };

  const copyBackupCodes = () => {
    const text = backupCodes.join('\n');
    navigator.clipboard.writeText(text).then(() => {
      message.success('备份码已复制到剪贴板');
    }).catch(() => {
      message.error('复制失败，请手动复制');
    });
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

  return (
    <div style={{ padding: 24 }}>
      <Title level={2}>安全设置</Title>

      <Card style={{ marginTop: 24 }}>
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
            loading={loading}
          />
        </div>

        {totpEnabled && (
          <Alert
            message="2FA 已启用"
            description="您的账户已启用双因素认证。登录时需要输入验证器应用中的验证码。"
            type="success"
            showIcon
            style={{ marginTop: 16 }}
          />
        )}
      </Card>

      {/* Setup Modal */}
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
            <div style={{ margin: '24px 0' }}>
              <img
                src={setupData.qr_code_base64}
                alt="TOTP QR Code"
                style={{ maxWidth: 256, maxHeight: 256 }}
              />
            </div>
            <Paragraph type="secondary">
              或手动输入密钥：<Text code>{setupData.secret}</Text>
            </Paragraph>
            <Button type="primary" onClick={() => setSetupStep('verify')} block>
              下一步
            </Button>
          </div>
        )}

        {setupStep === 'verify' && (
          <div style={{ textAlign: 'center' }}>
            <Paragraph>
              输入验证器应用中显示的 6 位验证码：
            </Paragraph>
            <Input
              prefix={<KeyOutlined />}
              placeholder="6位验证码"
              maxLength={6}
              value={verifyCode}
              onChange={(e) => setVerifyCode(e.target.value)}
              style={{
                textAlign: 'center',
                fontSize: 24,
                letterSpacing: 8,
                marginBottom: 24,
              }}
            />
            <Button
              type="primary"
              onClick={handleVerifyCode}
              loading={loading}
              block
            >
              验证并启用
            </Button>
          </div>
        )}

        {setupStep === 'backup' && (
          <div>
            <Alert
              message="请保存备份码"
              description="这些备份码可以在您无法使用验证器应用时用于登录。每个备份码只能使用一次。"
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <List
              bordered
              dataSource={backupCodes}
              renderItem={(code) => (
                <List.Item>
                  <Text code style={{ fontSize: 16 }}>{code}</Text>
                </List.Item>
              )}
              style={{ marginBottom: 16 }}
            />
            <Space style={{ width: '100%', justifyContent: 'center' }}>
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
              style={{ marginTop: 16 }}
            >
              完成
            </Button>
          </div>
        )}
      </Modal>

      {/* Disable Modal */}
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
          message="确定要禁用 2FA？"
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
          loading={loading}
          block
        >
          禁用 2FA
        </Button>
      </Modal>

      {/* Session Management */}
      <Card title={<Space><DesktopOutlined /> 会话管理</Space>} style={{ marginTop: 24 }}>
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
              render: (_: unknown, r: Session) => r.client_type === 'mobile' ? (r.device_info || '移动设备') : r.user_agent },
            { title: '登录时间', dataIndex: 'login_at', key: 'login_at', width: 180,
              render: (t: string) => t ? new Date(t).toLocaleString('zh-CN') : '-' },
            { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 180,
              render: (t: string) => t ? new Date(t).toLocaleString('zh-CN') : '-' },
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
    </div>
  );
}
