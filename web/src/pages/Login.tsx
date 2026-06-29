import { useState, useEffect } from 'react';
import { Form, Input, Button, Card, message, Typography, Space } from 'antd';
import { UserOutlined, LockOutlined, SafetyOutlined, KeyOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/useAuthStore';
import { authApi } from '../services/api';
import { COLORS } from '../utils/theme';

const { Title, Text } = Typography;

export default function Login() {
  const [loading, setLoading] = useState(false);
  const [step, setStep] = useState<'login' | 'totp' | 'backup'>('login');
  const [tempToken, setTempToken] = useState<string>('');
  const navigate = useNavigate();
  const { isAuthenticated } = useAuthStore();

  // If already authenticated, redirect to home
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  /**
   * Shared login success handler.
   * must_change_pass is stored on the user object from the server,
   * not in a separate localStorage key (prevents client tampering).
   */
  const handleLoginSuccess = (data: { token: string; user: { id: number; username: string; role: string }; must_change_pass?: boolean }) => {
    useAuthStore.setState({
      user: { ...data.user, must_change_pass: data.must_change_pass },
      token: data.token,
      isAuthenticated: true,
    });
    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));

    message.success('登录成功');

    if (data.must_change_pass) {
      navigate('/change-password', { replace: true });
    } else {
      navigate('/');
    }
  };

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const response = await authApi.login(values.username, values.password);
      const data = response.data.data;

      if (data.requires_totp) {
        setTempToken(data.temp_token!);
        setStep('totp');
        message.info('请输入验证码');
      } else {
        handleLoginSuccess(data);
      }
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '登录失败'));
    } finally {
      setLoading(false);
    }
  };

  const onTOTPFinish = async (values: { code: string }) => {
    setLoading(true);
    try {
      const response = await authApi.verifyTOTP(tempToken, values.code);
      handleLoginSuccess(response.data.data);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '验证码错误'));
    } finally {
      setLoading(false);
    }
  };

  const onBackupCodeFinish = async (values: { backup_code: string }) => {
    setLoading(true);
    try {
      const response = await authApi.verifyBackupCode(tempToken, values.backup_code);
      handleLoginSuccess(response.data.data);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '备份码错误'));
    } finally {
      setLoading(false);
    }
  };

  const renderLoginForm = () => (
    <>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <Title level={2} style={{ margin: 0 }}>EasyServer</Title>
        <p style={{ color: COLORS.TEXT_SECONDARY }}>Linux 服务器管理面板</p>
      </div>

      <Form
        name="login"
        onFinish={onFinish}
        autoComplete="off"
        size="large"
      >
        <Form.Item
          name="username"
          rules={[{ required: true, message: '请输入用户名' }]}
        >
          <Input
            prefix={<UserOutlined />}
            placeholder="用户名"
          />
        </Form.Item>

        <Form.Item
          name="password"
          rules={[{ required: true, message: '请输入密码' }]}
        >
          <Input.Password
            prefix={<LockOutlined />}
            placeholder="密码"
          />
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            登录
          </Button>
        </Form.Item>
      </Form>
    </>
  );

  const renderTOTPForm = () => (
    <>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <SafetyOutlined style={{ fontSize: 48, color: COLORS.PRIMARY }} />
        <Title level={3} style={{ marginTop: 16 }}>双因素验证</Title>
        <Text type="secondary">请输入验证器应用中的验证码</Text>
      </div>

      <Form
        name="totp"
        onFinish={onTOTPFinish}
        autoComplete="off"
        size="large"
      >
        <Form.Item
          name="code"
          rules={[
            { required: true, message: '请输入验证码' },
            { len: 6, message: '验证码为6位数字' },
          ]}
        >
          <Input
            prefix={<KeyOutlined />}
            placeholder="6位验证码"
            maxLength={6}
            style={{ textAlign: 'center', fontSize: 24, letterSpacing: 8 }}
          />
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center' }}>
          <Space>
            <Button type="link" onClick={() => setStep('backup')}>
              使用备份码
            </Button>
            <Button type="link" onClick={() => {
              setStep('login');
              setTempToken('');
            }}>
              返回登录
            </Button>
          </Space>
        </Form.Item>
      </Form>
    </>
  );

  const renderBackupCodeForm = () => (
    <>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <KeyOutlined style={{ fontSize: 48, color: COLORS.PRIMARY }} />
        <Title level={3} style={{ marginTop: 16 }}>备份码验证</Title>
        <Text type="secondary">请输入您的备份码</Text>
      </div>

      <Form
        name="backup"
        onFinish={onBackupCodeFinish}
        autoComplete="off"
        size="large"
      >
        <Form.Item
          name="backup_code"
          rules={[{ required: true, message: '请输入备份码' }]}
        >
          <Input
            prefix={<KeyOutlined />}
            placeholder="备份码"
          />
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center' }}>
          <Space>
            <Button type="link" onClick={() => setStep('totp')}>
              使用验证码
            </Button>
            <Button type="link" onClick={() => {
              setStep('login');
              setTempToken('');
            }}>
              返回登录
            </Button>
          </Space>
        </Form.Item>
      </Form>
    </>
  );

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      minHeight: '100vh',
      background: COLORS.LOGIN_BG,
    }}>
      <Card style={{ width: 400, boxShadow: '0 8px 32px rgba(0,0,0,0.1)' }}>
        {step === 'login' && renderLoginForm()}
        {step === 'totp' && renderTOTPForm()}
        {step === 'backup' && renderBackupCodeForm()}
      </Card>
    </div>
  );
}
