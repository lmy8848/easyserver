import { useState, useEffect, useCallback } from 'react';
import { Form, Input, Button, message, Typography, Space, Segmented, QRCode, Dropdown, Tooltip } from 'antd';
import { UserOutlined, LockOutlined, SafetyOutlined, KeyOutlined, CloudServerOutlined } from '@ant-design/icons';
import { SunMoon, Sun, Moon } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/useAuthStore';
import { useThemeStore, type ThemeMode } from '../store/useThemeStore';
import { authApi } from '../services/api';
import type { User } from '../types';
import Turnstile from '../components/Turnstile';

const { Title, Text } = Typography;

// 登录页动画 keyframes 与深浅色主题样式
const LOGIN_ANIM_CSS = `
@keyframes esLoginGradient {
  0%   { background-position: 0% 50%; }
  50%  { background-position: 100% 50%; }
  100% { background-position: 0% 50%; }
}
@keyframes esLoginFloat {
  0%   { transform: translate(0, 0) scale(1); }
  33%  { transform: translate(30px, -40px) scale(1.1); }
  66%  { transform: translate(-20px, 20px) scale(0.95); }
  100% { transform: translate(0, 0) scale(1); }
}
@keyframes esLoginFadeUp {
  from { opacity: 0; transform: translateY(16px); }
  to   { opacity: 1; transform: translateY(0); }
}
@keyframes esLoginPulseDark {
  0%, 100% { box-shadow: 0 8px 32px rgba(24,144,255,0.15); }
  50%      { box-shadow: 0 12px 40px rgba(24,144,255,0.30); }
}
@keyframes esLoginPulseLight {
  0%, 100% { box-shadow: 0 20px 60px rgba(99,102,241,0.12), 0 8px 24px rgba(0,0,0,0.06); }
  50%      { box-shadow: 0 24px 70px rgba(99,102,241,0.22), 0 12px 30px rgba(0,0,0,0.08); }
}
@keyframes esLoginStepIn {
  from { opacity: 0; transform: translateX(16px); }
  to   { opacity: 1; transform: translateX(0); }
}
.es-login-step { animation: esLoginStepIn 0.35s cubic-bezier(0.22, 1, 0.36, 1); }
.es-login-orb { position: absolute; border-radius: 50%; filter: blur(60px); pointer-events: none; }

/* === DARK THEME === */
.es-login-wrapper.is-dark {
  background: linear-gradient(125deg, #0f1729 0%, #1a1f3a 35%, #2a1a4a 70%, #1a1230 100%);
  background-size: 300% 300%;
}
.es-login-wrapper.is-dark .es-login-card {
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.12);
  animation: esLoginPulseDark 6s ease-in-out infinite, esLoginFadeUp 0.6s cubic-bezier(0.22,1,0.36,1);
}
.es-login-wrapper.is-dark input::placeholder { color: rgba(255,255,255,0.35); }
.es-login-wrapper.is-dark input:-webkit-autofill,
.es-login-wrapper.is-dark input:-webkit-autofill:hover,
.es-login-wrapper.is-dark input:-webkit-autofill:focus {
  -webkit-text-fill-color: #fff;
  -webkit-box-shadow: 0 0 0 1000px rgba(255,255,255,0.08) inset;
  transition: background-color 5000s ease-in-out 0s;
}
.es-login-wrapper.is-dark .ant-input-affix-wrapper {
  background: rgba(255,255,255,0.08);
  border-color: rgba(255,255,255,0.15);
  transition: all 0.2s ease-in-out;
}
.es-login-wrapper.is-dark .ant-input-affix-wrapper > input.ant-input {
  background: transparent;
  color: #fff;
}
.es-login-wrapper.is-dark .ant-input-affix-wrapper:hover {
  border-color: #40a9ff;
}
.es-login-wrapper.is-dark .ant-input-affix-wrapper-focused,
.es-login-wrapper.is-dark .ant-input-affix-wrapper:focus-within {
  border-color: #1890ff !important;
  box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.2) !important;
}
.es-login-wrapper.is-dark .ant-input-affix-wrapper-focused .ant-input-prefix,
.es-login-wrapper.is-dark .ant-input-affix-wrapper:focus-within .ant-input-prefix {
  color: #1890ff !important;
}
.es-login-wrapper.is-dark .ant-form-item-explain-error { color: #ff7875; }
.es-login-wrapper.is-dark .ant-segmented { background: rgba(255,255,255,0.08); padding: 3px;}
.es-login-wrapper.is-dark .ant-segmented .ant-segmented-item { color: rgba(255,255,255,0.55); }
.es-login-wrapper.is-dark .ant-segmented .ant-segmented-item:not(.ant-segmented-item-selected):hover { color: #fff; }
.es-login-wrapper.is-dark .ant-segmented .ant-segmented-item-selected { color: #fff; font-weight: 600; }
.es-login-wrapper.is-dark .ant-segmented .ant-segmented-thumb { background: rgba(255,255,255,0.18); }
.es-login-wrapper.is-dark .ant-otp { gap: 8px; justify-content: center; }
.es-login-wrapper.is-dark .ant-otp input {
  background: rgba(255,255,255,0.08);
  border-color: rgba(255,255,255,0.15);
  color: #fff;
  font-size: 20px;
  font-weight: 600;
  text-align: center;
  border-radius: 10px;
  transition: all 0.2s ease-in-out;
}
.es-login-wrapper.is-dark .ant-otp input:hover { border-color: #40a9ff; }
.es-login-wrapper.is-dark .ant-otp input:focus,
.es-login-wrapper.is-dark .ant-otp input:focus-visible {
  border-color: #1890ff !important;
  box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.2) !important;
  outline: none;
}

/* === LIGHT THEME === */
.es-login-wrapper.is-light {
  background: linear-gradient(125deg, #f0f4ff 0%, #e2e8f8 35%, #ede9fe 70%, #f5f3ff 100%);
  background-size: 300% 300%;
}
.es-login-wrapper.is-light .es-login-card {
  background: rgba(255,255,255,0.78);
  border: 1px solid rgba(255,255,255,0.9);
  animation: esLoginPulseLight 6s ease-in-out infinite, esLoginFadeUp 0.6s cubic-bezier(0.22,1,0.36,1);
}
.es-login-wrapper.is-light input::placeholder { color: #94a3b8; }
.es-login-wrapper.is-light input:-webkit-autofill,
.es-login-wrapper.is-light input:-webkit-autofill:hover,
.es-login-wrapper.is-light input:-webkit-autofill:focus {
  -webkit-text-fill-color: #1e293b;
  -webkit-box-shadow: 0 0 0 1000px #f8fafc inset;
  transition: background-color 5000s ease-in-out 0s;
}
.es-login-wrapper.is-light .ant-input-affix-wrapper {
  background: rgba(255,255,255,0.9);
  border-color: #e2e8f0;
  transition: all 0.2s ease-in-out;
}
.es-login-wrapper.is-light .ant-input-affix-wrapper > input.ant-input {
  background: transparent;
  color: #1e293b;
}
.es-login-wrapper.is-light .ant-input-affix-wrapper:hover {
  border-color: #6366f1;
}
.es-login-wrapper.is-light .ant-input-affix-wrapper-focused,
.es-login-wrapper.is-light .ant-input-affix-wrapper:focus-within {
  border-color: #6366f1 !important;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2) !important;
}
.es-login-wrapper.is-light .ant-input-affix-wrapper-focused .ant-input-prefix,
.es-login-wrapper.is-light .ant-input-affix-wrapper:focus-within .ant-input-prefix {
  color: #6366f1 !important;
}
.es-login-wrapper.is-light .ant-form-item-explain-error { color: #ef4444; }
.es-login-wrapper.is-light .ant-segmented { background: rgba(241,245,249,0.95); padding: 3px;}
.es-login-wrapper.is-light .ant-segmented .ant-segmented-item { color: #64748b; }
.es-login-wrapper.is-light .ant-segmented .ant-segmented-item:not(.ant-segmented-item-selected):hover { color: #1e293b; }
.es-login-wrapper.is-light .ant-segmented .ant-segmented-item-selected { color: #1e293b; font-weight: 600; }
.es-login-wrapper.is-light .ant-segmented .ant-segmented-thumb { background: #ffffff; box-shadow: 0 2px 6px rgba(0,0,0,0.08); }
.es-login-wrapper.is-light .ant-otp { gap: 8px; justify-content: center; }
.es-login-wrapper.is-light .ant-otp input {
  background: rgba(255,255,255,0.9);
  border-color: #e2e8f0;
  color: #1e293b;
  font-size: 20px;
  font-weight: 600;
  text-align: center;
  border-radius: 10px;
  transition: all 0.2s ease-in-out;
}
.es-login-wrapper.is-light .ant-otp input:hover { border-color: #6366f1; }
.es-login-wrapper.is-light .ant-otp input:focus,
.es-login-wrapper.is-light .ant-otp input:focus-visible {
  border-color: #6366f1 !important;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2) !important;
  outline: none;
}
`;

interface QRData {
  qr_token: string;
  expires_at: string;
}

interface TurnstileConfig {
  site_key: string;
  enable_login: boolean;
  enable_qr_login: boolean;
  enable_public_share: boolean;
}

export default function Login() {
  const { mode: themeMode, isDark, setMode: setThemeMode } = useThemeStore();
  const [totpForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [step, setStep] = useState<'login' | 'totp' | 'backup'>('login');
  const [tempToken, setTempToken] = useState<string>('');
  const navigate = useNavigate();
  const { isAuthenticated } = useAuthStore();

  // Turnstile 配置(公开,无私钥)
  const [turnstileCfg, setTurnstileCfg] = useState<TurnstileConfig | null>(null);
  const [turnstileToken, setTurnstileToken] = useState<string>('');

  // 扫码登录状态
  const [loginMode, setLoginMode] = useState<'password' | 'qr'>('password');
  const [qrData, setQrData] = useState<QRData | null>(null);
  const [qrLoading, setQrLoading] = useState(false);
  const [qrStatus, setQrStatus] = useState<'pending' | 'expired'>('pending');

  // 加载 Turnstile 公开配置
  useEffect(() => {
    authApi.getTurnstileConfig().then((res) => {
      const cfg = res.data.data;
      if (cfg) {
        setTurnstileCfg(cfg);
        setTurnstileToken('');
      }
    }).catch(() => { /* 静默失败,Turnstile 可选 */ });
  }, []);

  // If already authenticated, redirect to home
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  /**
   * Shared login success handler.
   * Web 登录态走 HttpOnly Cookie（后端 Set-Cookie）。user 由登录接口返回
   * （后端登录时已查出用户信息），QR 扫码登录除外——后端确认时拿不到
   * web 端信息，由前端自行从 /auth/me 获取后再走这里。
   */
  const handleLoginSuccess = useCallback((user: User) => {
    useAuthStore.setState({ user, isAuthenticated: true });
    localStorage.setItem('user', JSON.stringify(user));

    message.success('登录成功');

    if (user.must_change_pass) {
      navigate('/change-password', { replace: true });
    } else {
      navigate('/');
    }
  }, [navigate]);

  const onFinish = async (values: { username: string; password: string }) => {
    // Turnstile 启用但未通过验证时,阻止登录
    if (turnstileCfg?.enable_login && !turnstileToken) {
      message.warning('请先完成人机验证');
      return;
    }
    setLoading(true);
    try {
      const response = await authApi.login(values.username, values.password, turnstileToken);
      const data = response.data.data;

      if (data.requires_totp) {
        setTempToken(data.temp_token!);
        setStep('totp');
        message.info('请输入验证码');
      } else {
        handleLoginSuccess(data.user);
      }
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '登录失败'));
    } finally {
      setLoading(false);
    }
  };

  const onTOTPFinish = async (values: { code: string }) => {
    if (turnstileCfg?.enable_login && !turnstileToken) {
      message.warning('请先完成人机验证');
      return;
    }
    setLoading(true);
    try {
      const response = await authApi.verifyTOTP(tempToken, values.code, turnstileToken);
      handleLoginSuccess(response.data.data.user);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '验证码错误'));
    } finally {
      setLoading(false);
    }
  };

  const onBackupCodeFinish = async (values: { backup_code: string }) => {
    if (turnstileCfg?.enable_login && !turnstileToken) {
      message.warning('请先完成人机验证');
      return;
    }
    setLoading(true);
    try {
      const response = await authApi.verifyBackupCode(tempToken, values.backup_code, turnstileToken);
      handleLoginSuccess(response.data.data.user);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '备份码错误'));
    } finally {
      setLoading(false);
    }
  };

  // 生成新的扫码登录二维码
  const startQrLogin = async () => {
    setQrLoading(true);
    try {
      const res = await authApi.createQRSession();
      setQrData(res.data.data);
      setQrStatus('pending');
    } catch {
      message.error('生成二维码失败');
    } finally {
      setQrLoading(false);
    }
  };

  // 切到扫码模式时自动生成；切回密码模式时取消未完成的会话
  const switchMode = (mode: 'password' | 'qr') => {
    setLoginMode(mode);
    if (mode === 'qr') {
      startQrLogin();
    } else if (qrData) {
      authApi.cancelQRLogin(qrData.qr_token).catch(() => undefined);
      setQrData(null);
      setQrStatus('pending');
    }
  };

  // 轮询扫码状态。pollQRStatus 随 qrData 变化重建,effect 依赖它自动重跑。
  const pollQRStatus = useCallback(async () => {
    if (!qrData) return;
    try {
      const res = await authApi.getQRStatus(qrData.qr_token);
      const d = res.data.data;
      if (!d) return;
      if (d.status === 'confirmed') {
        // QR 登录的 web token 已由后端 Set-Cookie；用户快照随状态返回。
        handleLoginSuccess(d.user!);
      } else if (d.status === 'expired' || d.status === 'cancelled') {
        setQrStatus('expired');
      }
    } catch {
      /* 网络抖动忽略，继续轮询 */
    }
  }, [qrData, handleLoginSuccess]);

  useEffect(() => {
    if (loginMode !== 'qr' || !qrData || qrStatus === 'expired') return;
    pollQRStatus();
    const id = setInterval(pollQRStatus, 3000);
    return () => clearInterval(id);
  }, [loginMode, qrData, qrStatus, pollQRStatus]);

  const primaryBtnStyle: React.CSSProperties = {
    borderRadius: 8, fontWeight: 600, fontSize: 16, border: 'none',
    background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
    boxShadow: '0 6px 20px rgba(24,144,255,0.35)',
  };

  const renderBrand = () => (
    <div style={{ textAlign: 'center', marginBottom: 28 }}>
      <div style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        width: 64, height: 64, borderRadius: 18, marginBottom: 16,
        background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
        boxShadow: '0 8px 24px rgba(24,144,255,0.45)',
      }}>
        <CloudServerOutlined style={{ fontSize: 34, color: '#fff' }} />
      </div>
      <Title level={2} style={{ margin: 0, color: isDark ? '#fff' : '#1e293b', letterSpacing: 1 }}>EasyServer</Title>
      <p style={{ color: isDark ? 'rgba(255,255,255,0.55)' : '#64748b', marginTop: 6, fontSize: 13, letterSpacing: 2 }}>LINUX 服务器管理面板</p>
    </div>
  );

  const renderPasswordForm = () => (
    <Form name="login" onFinish={onFinish} autoComplete="off">
      <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
        <Input size="large" prefix={<UserOutlined style={{ color: isDark ? 'rgba(255,255,255,0.45)' : '#94a3b8' }} />} placeholder="用户名" />
      </Form.Item>

      <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
        <Input.Password size="large" prefix={<LockOutlined style={{ color: isDark ? 'rgba(255,255,255,0.45)' : '#94a3b8' }} />} placeholder="密码" />
      </Form.Item>

      {turnstileCfg?.enable_login && turnstileCfg.site_key && (
        <Form.Item style={{ marginBottom: 12 }}>
          <Turnstile
            siteKey={turnstileCfg.site_key}
            onVerify={(t) => setTurnstileToken(t)}
            onExpire={() => setTurnstileToken('')}
            theme="auto"
          />
        </Form.Item>
      )}

      <Form.Item style={{ marginBottom: 0 }}>
        <Button type="primary" size="large" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
          登录
        </Button>
      </Form.Item>
    </Form>
  );

  const renderQRForm = () => (
    <div style={{ textAlign: 'center' }}>
      <div style={{
        display: 'inline-block',
        padding: 12,
        borderRadius: 12,
        background: isDark ? 'rgba(255, 255, 255, 0.06)' : '#ffffff',
        border: isDark ? '1px solid rgba(255, 255, 255, 0.15)' : '1px solid #e2e8f0',
        boxShadow: isDark ? '0 2px 8px rgba(0, 0, 0, 0.15)' : '0 2px 8px rgba(0, 0, 0, 0.04)',
        marginBottom: 16,
        backdropFilter: 'blur(12px)',
      }}>
        <QRCode
          value={qrData ? `esqr:${qrData.qr_token}` : '-'}
          size={200}
          bordered={false}
          color={isDark ? '#ffffff' : '#1e293b'}
          bgColor="transparent"
          status={qrStatus === 'expired' ? 'expired' : qrLoading || !qrData ? 'loading' : 'active'}
          onRefresh={startQrLogin}
        />
      </div>
      <div style={{ color: isDark ? 'rgba(255,255,255,0.65)' : '#64748b', fontSize: 14 }}>
        请使用手机 App 扫码登录
      </div>
    </div>
  );

  const renderTOTPForm = () => (
    <div className="es-login-step">
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <div style={{
          display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          width: 64, height: 64, borderRadius: 18, marginBottom: 16,
          background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
          boxShadow: '0 8px 24px rgba(24,144,255,0.45)',
        }}>
          <SafetyOutlined style={{ fontSize: 32, color: '#fff' }} />
        </div>
        <Title level={3} style={{ margin: 0, color: isDark ? '#fff' : '#1e293b' }}>双因素验证</Title>
        <Text style={{ color: isDark ? 'rgba(255,255,255,0.55)' : '#64748b' }}>请输入验证器应用中的验证码</Text>
      </div>

      <Form form={totpForm} name="totp" onFinish={onTOTPFinish} autoComplete="off">
        <Form.Item
          name="code"
          rules={[
            { required: true, message: '请输入验证码' },
            { len: 6, message: '验证码为6位数字' },
          ]}
          style={{ textAlign: 'center' }}
        >
          <Input.OTP
            size="large"
            length={6}
            autoFocus
            onChange={(val) => {
              if (val.length === 6 && !loading) {
                totpForm.submit();
              }
            }}
          />
        </Form.Item>

        {turnstileCfg?.enable_login && turnstileCfg.site_key && (
          <Form.Item style={{ marginBottom: 12 }}>
            <Turnstile
              siteKey={turnstileCfg.site_key}
              onVerify={(t) => setTurnstileToken(t)}
              onExpire={() => setTurnstileToken('')}
              theme="auto"
            />
          </Form.Item>
        )}

        <Form.Item style={{ marginBottom: 0 }}>
          <Button type="primary" size="large" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center', marginTop: 8 }}>
          <Space>
            <Button type="link" style={{ color: isDark ? 'rgba(255,255,255,0.65)' : '#6366f1' }} onClick={() => setStep('backup')}>使用备份码</Button>
            <Button type="link" style={{ color: isDark ? 'rgba(255,255,255,0.65)' : '#6366f1' }} onClick={() => { setStep('login'); setTempToken(''); }}>返回登录</Button>
          </Space>
        </Form.Item>
      </Form>
    </div>
  );

  const renderBackupCodeForm = () => (
    <div className="es-login-step">
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <div style={{
          display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          width: 64, height: 64, borderRadius: 18, marginBottom: 16,
          background: 'linear-gradient(135deg, #1890ff 0%, #722ed1 100%)',
          boxShadow: '0 8px 24px rgba(24,144,255,0.45)',
        }}>
          <KeyOutlined style={{ fontSize: 32, color: '#fff' }} />
        </div>
        <Title level={3} style={{ margin: 0, color: isDark ? '#fff' : '#1e293b' }}>备份码验证</Title>
        <Text style={{ color: isDark ? 'rgba(255,255,255,0.55)' : '#64748b' }}>请输入您的备份码</Text>
      </div>

      <Form name="backup" onFinish={onBackupCodeFinish} autoComplete="off">
        <Form.Item name="backup_code" rules={[{ required: true, message: '请输入备份码' }]}>
          <Input size="large" prefix={<KeyOutlined style={{ color: isDark ? 'rgba(255,255,255,0.45)' : '#94a3b8' }} />} placeholder="备份码" />
        </Form.Item>

        {turnstileCfg?.enable_login && turnstileCfg.site_key && (
          <Form.Item style={{ marginBottom: 12 }}>
            <Turnstile
              siteKey={turnstileCfg.site_key}
              onVerify={(t) => setTurnstileToken(t)}
              onExpire={() => setTurnstileToken('')}
              theme="auto"
            />
          </Form.Item>
        )}

        <Form.Item style={{ marginBottom: 0 }}>
          <Button type="primary" size="large" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center', marginTop: 8 }}>
          <Space>
            <Button type="link" style={{ color: isDark ? 'rgba(255,255,255,0.65)' : '#6366f1' }} onClick={() => setStep('totp')}>使用验证码</Button>
            <Button type="link" style={{ color: isDark ? 'rgba(255,255,255,0.65)' : '#6366f1' }} onClick={() => { setStep('login'); setTempToken(''); }}>返回登录</Button>
          </Space>
        </Form.Item>
      </Form>
    </div>
  );

  return (
    <div
      className={`es-login-wrapper ${isDark ? 'is-dark' : 'is-light'}`}
      style={{
        position: 'relative',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        overflow: 'hidden',
        animation: 'esLoginGradient 18s ease infinite',
      }}
    >
      <style>{LOGIN_ANIM_CSS}</style>

      {/* 顶部右侧主题切换 */}
      <div style={{ position: 'fixed', top: 20, right: 24, zIndex: 100 }}>
        <Dropdown
          menu={{
            items: [
              {
                key: 'auto',
                icon: <SunMoon size={16} />,
                label: '跟随系统',
              },
              {
                key: 'light',
                icon: <Sun size={16} />,
                label: '浅色模式',
              },
              {
                key: 'dark',
                icon: <Moon size={16} />,
                label: '暗色模式',
              },
            ],
            selectedKeys: [themeMode],
            onClick: ({ key }) => setThemeMode(key as ThemeMode),
          }}
          trigger={['click']}
          placement="bottomRight"
        >
          <Tooltip title={`主题：${themeMode === 'auto' ? '跟随系统' : themeMode === 'dark' ? '暗色模式' : '浅色模式'}`}>
            <Button
              type="text"
              style={{
                color: isDark ? '#fff' : '#1e293b',
                background: isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(255, 255, 255, 0.8)',
                border: isDark ? '1px solid rgba(255, 255, 255, 0.15)' : '1px solid rgba(226, 232, 240, 0.9)',
                boxShadow: isDark ? 'none' : '0 4px 12px rgba(0,0,0,0.06)',
                backdropFilter: 'blur(12px)',
                WebkitBackdropFilter: 'blur(12px)',
                borderRadius: 10,
                width: 38,
                height: 38,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
              icon={themeMode === 'auto' ? <SunMoon size={18} /> : themeMode === 'dark' ? <Moon size={18} /> : <Sun size={18} />}
            />
          </Tooltip>
        </Dropdown>
      </div>

      {/* 浮动光斑装饰 */}
      <div className="es-login-orb" style={{ width: 420, height: 420, top: '-120px', left: '-80px', background: isDark ? '#1890ff' : '#6366f1', opacity: isDark ? 0.55 : 0.25, animation: 'esLoginFloat 20s ease-in-out infinite' }} />
      <div className="es-login-orb" style={{ width: 360, height: 360, bottom: '-100px', right: '-60px', background: isDark ? '#722ed1' : '#a855f7', opacity: isDark ? 0.55 : 0.25, animation: 'esLoginFloat 24s ease-in-out infinite reverse' }} />
      <div className="es-login-orb" style={{ width: 260, height: 260, top: '40%', right: '20%', background: isDark ? '#13c2c2' : '#38bdf8', opacity: isDark ? 0.3 : 0.2, animation: 'esLoginFloat 28s ease-in-out infinite' }} />

      {/* 玻璃拟态卡片 */}
      <div className="es-login-card" style={{
        position: 'relative',
        zIndex: 1,
        width: 400,
        maxWidth: '92vw',
        padding: '40px 36px 32px',
        borderRadius: 20,
        backdropFilter: 'blur(24px)',
        WebkitBackdropFilter: 'blur(24px)',
      }}>
        {step === 'login' && (
          <>
            <div style={{ animation: 'esLoginFadeUp 0.5s cubic-bezier(0.22,1,0.36,1)' }}>
              {renderBrand()}
              <Segmented
                size="large"
                value={loginMode}
                onChange={(v) => switchMode(v as 'password' | 'qr')}
                options={[
                  { label: '密码登录', value: 'password' },
                  { label: '扫码登录', value: 'qr' },
                ]}
                block
                style={{ marginBottom: 24 }}
              />
            </div>
            {loginMode === 'password' ? renderPasswordForm() : renderQRForm()}
          </>
        )}
        {step === 'totp' && renderTOTPForm()}
        {step === 'backup' && renderBackupCodeForm()}

        <div style={{ textAlign: 'center', marginTop: 24, color: isDark ? 'rgba(255,255,255,0.3)' : '#94a3b8', fontSize: 12 }}>
          EasyServer © {new Date().getFullYear()}
        </div>
      </div>
    </div>
  );
}
