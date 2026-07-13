import { useState, useEffect, useCallback } from 'react';
import { Form, Input, Button, message, Typography, Space, Segmented } from 'antd';
import { UserOutlined, LockOutlined, SafetyOutlined, KeyOutlined, CloudServerOutlined, ReloadOutlined, QrcodeOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/useAuthStore';
import { authApi } from '../services/api';
import type { User } from '../types';

const { Title, Text } = Typography;

// 登录页动画 keyframes。通过 <style> 注入,避免引入额外依赖。
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
@keyframes esLoginPulse {
  0%, 100% { box-shadow: 0 8px 32px rgba(24,144,255,0.15); }
  50%      { box-shadow: 0 12px 40px rgba(24,144,255,0.30); }
}
@keyframes esLoginStepIn {
  from { opacity: 0; transform: translateX(16px); }
  to   { opacity: 1; transform: translateX(0); }
}
@keyframes esQrBreath {
  0%, 100% { transform: scale(1); opacity: 1; }
  50%      { transform: scale(1.02); opacity: 0.92; }
}
.es-login-step { animation: esLoginStepIn 0.35s cubic-bezier(0.22, 1, 0.36, 1); }
.es-login-orb { position: absolute; border-radius: 50%; filter: blur(60px); opacity: 0.55; pointer-events: none; }
/* 深色玻璃卡片下的输入框占位符 / 自动填充配色 */
.es-login-card input::placeholder { color: rgba(255,255,255,0.35); }
.es-login-card input:-webkit-autofill,
.es-login-card input:-webkit-autofill:hover,
.es-login-card input:-webkit-autofill:focus {
  -webkit-text-fill-color: #fff;
  -webkit-box-shadow: 0 0 0 1000px rgba(255,255,255,0.08) inset;
  transition: background-color 5000s ease-in-out 0s;
}
.es-login-card .ant-input-affix-wrapper { background: rgba(255,255,255,0.08); border-color: rgba(255,255,255,0.15); }
.es-login-card .ant-input-affix-wrapper > input.ant-input { background: transparent; color: #fff; }
.es-login-card .ant-form-item-explain-error { color: #ff7875; }
/* Segmented 适配深色玻璃 */
.es-login-card .ant-segmented { background: rgba(255,255,255,0.08); padding: 3px; border-radius: 10px; }
.es-login-card .ant-segmented .ant-segmented-item { color: rgba(255,255,255,0.55); }
.es-login-card .ant-segmented .ant-segmented-item-selected { color: #fff; }
.es-login-card .ant-segmented .ant-segmented-thumb { background: rgba(255,255,255,0.14); }
`;

interface QRData {
  qr_token: string;
  qr_code_base64: string;
  expires_at: string;
}

export default function Login() {
  const [loading, setLoading] = useState(false);
  const [step, setStep] = useState<'login' | 'totp' | 'backup'>('login');
  const [tempToken, setTempToken] = useState<string>('');
  const navigate = useNavigate();
  const { isAuthenticated } = useAuthStore();

  // 扫码登录状态
  const [loginMode, setLoginMode] = useState<'password' | 'qr'>('password');
  const [qrData, setQrData] = useState<QRData | null>(null);
  const [qrLoading, setQrLoading] = useState(false);
  const [qrStatus, setQrStatus] = useState<'pending' | 'expired'>('pending');

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
  const handleLoginSuccess = useCallback((data: { token: string; user: { id: number; username: string; role: string }; must_change_pass?: boolean }) => {
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
  }, [navigate]);

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

  // 生成新的扫码登录二维码
  const startQrLogin = async () => {
    setQrLoading(true);
    setQrStatus('pending');
    try {
      const res = await authApi.createQRSession();
      setQrData(res.data.data);
    } catch {
      message.error('生成二维码失败');
      setQrData(null);
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
    }
  };

  // 轮询扫码状态。pollQRStatus 随 qrData 变化重建,effect 依赖它自动重跑。
  const pollQRStatus = useCallback(async () => {
    if (!qrData) return;
    try {
      const res = await authApi.getQRStatus(qrData.qr_token);
      const d = res.data.data;
      if (!d) return;
      if (d.status === 'confirmed' && d.token && d.user) {
        handleLoginSuccess({ token: d.token, user: d.user as User, must_change_pass: d.must_change_pass });
      } else if (d.status === 'expired' || d.status === 'cancelled') {
        setQrStatus('expired');
        setQrData(null);
      }
    } catch {
      /* 网络抖动忽略，继续轮询 */
    }
  }, [qrData, handleLoginSuccess]);

  useEffect(() => {
    if (loginMode !== 'qr' || !qrData) return;
    pollQRStatus();
    const id = setInterval(pollQRStatus, 2000);
    return () => clearInterval(id);
  }, [loginMode, qrData, pollQRStatus]);

  const inputStyle: React.CSSProperties = {
    height: 46,
    borderRadius: 10,
    background: 'rgba(255,255,255,0.08)',
    borderColor: 'rgba(255,255,255,0.15)',
    color: '#fff',
  };

  const primaryBtnStyle: React.CSSProperties = {
    height: 46, borderRadius: 10, fontWeight: 600, fontSize: 15, border: 'none',
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
      <Title level={2} style={{ margin: 0, color: '#fff', letterSpacing: 1 }}>EasyServer</Title>
      <p style={{ color: 'rgba(255,255,255,0.55)', marginTop: 6, fontSize: 13, letterSpacing: 2 }}>LINUX 服务器管理面板</p>
    </div>
  );

  const renderPasswordForm = () => (
    <Form name="login" onFinish={onFinish} autoComplete="off" size="large">
      <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
        <Input prefix={<UserOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />} placeholder="用户名" style={inputStyle} />
      </Form.Item>

      <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
        <Input.Password prefix={<LockOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />} placeholder="密码" style={inputStyle} />
      </Form.Item>

      <Form.Item style={{ marginBottom: 0 }}>
        <Button type="primary" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
          登录
        </Button>
      </Form.Item>
    </Form>
  );

  const renderQRForm = () => (
    <div style={{ textAlign: 'center' }}>
      <div style={{
        display: 'inline-block', padding: 12, borderRadius: 16,
        background: 'rgba(255,255,255,0.95)', marginBottom: 16,
        boxShadow: '0 8px 24px rgba(0,0,0,0.3)',
      }}>
        {qrLoading || !qrData ? (
          <div style={{ width: 200, height: 200, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' }}>
            生成中...
          </div>
        ) : qrStatus === 'expired' ? (
          <div style={{ width: 200, height: 200, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12 }}>
            <QrcodeOutlined style={{ fontSize: 56, color: '#bbb' }} />
            <span style={{ color: '#999', fontSize: 13 }}>二维码已过期</span>
          </div>
        ) : (
          <img src={qrData.qr_code_base64} alt="登录二维码"
            style={{ width: 200, height: 200, display: 'block', animation: 'esQrBreath 3s ease-in-out infinite' }} />
        )}
      </div>
      <div style={{ color: 'rgba(255,255,255,0.65)', fontSize: 14, marginBottom: 8 }}>
        {qrStatus === 'expired' ? '二维码已失效' : '请使用手机 App 扫码登录'}
      </div>
      {qrStatus === 'expired' && (
        <Button icon={<ReloadOutlined />} onClick={startQrLogin} loading={qrLoading}
          style={{ background: 'rgba(255,255,255,0.1)', borderColor: 'rgba(255,255,255,0.2)', color: '#fff' }}>
          刷新二维码
        </Button>
      )}
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
        <Title level={3} style={{ margin: 0, color: '#fff' }}>双因素验证</Title>
        <Text style={{ color: 'rgba(255,255,255,0.55)' }}>请输入验证器应用中的验证码</Text>
      </div>

      <Form name="totp" onFinish={onTOTPFinish} autoComplete="off" size="large">
        <Form.Item name="code" rules={[{ required: true, message: '请输入验证码' }, { len: 6, message: '验证码为6位数字' }]}>
          <Input prefix={<KeyOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />} placeholder="6位验证码"
            maxLength={6} style={{ ...inputStyle, textAlign: 'center', fontSize: 26, letterSpacing: 12 }} />
        </Form.Item>

        <Form.Item style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center', marginTop: 8 }}>
          <Space>
            <Button type="link" style={{ color: 'rgba(255,255,255,0.65)' }} onClick={() => setStep('backup')}>使用备份码</Button>
            <Button type="link" style={{ color: 'rgba(255,255,255,0.65)' }} onClick={() => { setStep('login'); setTempToken(''); }}>返回登录</Button>
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
        <Title level={3} style={{ margin: 0, color: '#fff' }}>备份码验证</Title>
        <Text style={{ color: 'rgba(255,255,255,0.55)' }}>请输入您的备份码</Text>
      </div>

      <Form name="backup" onFinish={onBackupCodeFinish} autoComplete="off" size="large">
        <Form.Item name="backup_code" rules={[{ required: true, message: '请输入备份码' }]}>
          <Input prefix={<KeyOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />} placeholder="备份码" style={inputStyle} />
        </Form.Item>

        <Form.Item style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" loading={loading} block style={primaryBtnStyle}>
            验证
          </Button>
        </Form.Item>

        <Form.Item style={{ marginBottom: 0, textAlign: 'center', marginTop: 8 }}>
          <Space>
            <Button type="link" style={{ color: 'rgba(255,255,255,0.65)' }} onClick={() => setStep('totp')}>使用验证码</Button>
            <Button type="link" style={{ color: 'rgba(255,255,255,0.65)' }} onClick={() => { setStep('login'); setTempToken(''); }}>返回登录</Button>
          </Space>
        </Form.Item>
      </Form>
    </div>
  );

  return (
    <div style={{
      position: 'relative',
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      minHeight: '100vh',
      overflow: 'hidden',
      background: 'linear-gradient(125deg, #0f1729 0%, #1a1f3a 35%, #2a1a4a 70%, #1a1230 100%)',
      backgroundSize: '300% 300%',
      animation: 'esLoginGradient 18s ease infinite',
    }}>
      <style>{LOGIN_ANIM_CSS}</style>

      {/* 浮动光斑装饰 */}
      <div className="es-login-orb" style={{ width: 420, height: 420, top: '-120px', left: '-80px', background: '#1890ff', animation: 'esLoginFloat 20s ease-in-out infinite' }} />
      <div className="es-login-orb" style={{ width: 360, height: 360, bottom: '-100px', right: '-60px', background: '#722ed1', animation: 'esLoginFloat 24s ease-in-out infinite reverse' }} />
      <div className="es-login-orb" style={{ width: 260, height: 260, top: '40%', right: '20%', background: '#13c2c2', opacity: 0.3, animation: 'esLoginFloat 28s ease-in-out infinite' }} />

      {/* 玻璃拟态卡片 */}
      <div className="es-login-card" style={{
        position: 'relative',
        zIndex: 1,
        width: 400,
        maxWidth: '92vw',
        padding: '40px 36px 32px',
        borderRadius: 20,
        background: 'rgba(255,255,255,0.06)',
        backdropFilter: 'blur(24px)',
        WebkitBackdropFilter: 'blur(24px)',
        border: '1px solid rgba(255,255,255,0.12)',
        boxShadow: '0 20px 60px rgba(0,0,0,0.45)',
        animation: 'esLoginPulse 6s ease-in-out infinite, esLoginFadeUp 0.6s cubic-bezier(0.22,1,0.36,1)',
      }}>
        {step === 'login' && (
          <>
            <div style={{ animation: 'esLoginFadeUp 0.5s cubic-bezier(0.22,1,0.36,1)' }}>
              {renderBrand()}
              <Segmented
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

        <div style={{ textAlign: 'center', marginTop: 24, color: 'rgba(255,255,255,0.3)', fontSize: 12 }}>
          EasyServer © {new Date().getFullYear()}
        </div>
      </div>
    </div>
  );
}
