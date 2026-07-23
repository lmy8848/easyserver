import { useState, useEffect } from 'react';
import {
  Card, Form, Button, message, InputNumber, Switch,
} from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

export interface AuthSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function AuthSettings({ settings, onRefresh }: AuthSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.auth) {
      form.setFieldsValue({
        session_timeout: Number(settings.auth.session_timeout) || 86400,
        idle_timeout: Number(settings.auth.idle_timeout) || 1800,
        max_login_attempts: settings.auth.max_login_attempts,
        lockout_duration: Number(settings.auth.lockout_duration) || 900,
        rate_limit: settings.auth.rate_limit,
        rate_interval: Number(settings.auth.rate_interval) || 60,
        login_rate_limit: settings.auth.login_rate_limit,
        login_rate_interval: Number(settings.auth.login_rate_interval) || 60,
        allow_multi_session: settings.auth.allow_multi_session,
        mobile_device_binding: settings.auth.mobile_device_binding,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateAuth(values);
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

  return (
    <Card title="认证安全配置">
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          session_timeout: 86400,
          idle_timeout: 1800,
          max_login_attempts: 5,
          lockout_duration: 900,
          rate_limit: 1000,
          rate_interval: 60,
          login_rate_limit: 10,
          login_rate_interval: 60,
          allow_multi_session: false,
          mobile_device_binding: true,
        }}
      >
        <Form.Item
          name="session_timeout"
          label="会话超时"
          extra="用户会话的有效持续时间（秒，默认 86400 秒 = 24 小时）"
          rules={[{ required: true, message: '请输入会话超时秒数' }]}
        >
          <InputNumber min={300} suffix="秒" style={{ width: 200 }} placeholder="86400" />
        </Form.Item>

        <Form.Item
          name="idle_timeout"
          label="空闲超时"
          extra="用户无操作后自动登出的时间（秒，默认 1800 秒 = 30 分钟）"
          rules={[{ required: true, message: '请输入空闲超时秒数' }]}
        >
          <InputNumber min={60} suffix="秒" style={{ width: 200 }} placeholder="1800" />
        </Form.Item>

        <Form.Item
          name="max_login_attempts"
          label="最大登录尝试次数"
          extra="登录失败多少次后锁定账户"
          rules={[{ required: true, message: '请输入最大次数' }]}
        >
          <InputNumber min={3} max={100} style={{ width: 200 }} />
        </Form.Item>

        <Form.Item
          name="lockout_duration"
          label="锁定时长"
          extra="账户锁定的持续时间（秒，默认 900 秒 = 15 分钟）"
          rules={[{ required: true, message: '请输入锁定时长秒数' }]}
        >
          <InputNumber min={60} max={86400} suffix="秒" style={{ width: 200 }} placeholder="900" />
        </Form.Item>

        <Form.Item
          name="rate_limit"
          label="速率限制"
          extra="每个时间窗口内允许的最大请求数"
          rules={[{ required: true, message: '请输入速率限制' }]}
        >
          <InputNumber min={10} style={{ width: 200 }} />
        </Form.Item>

        <Form.Item
          name="rate_interval"
          label="速率限制时间窗口"
          extra="通用 API 速率限制的时间窗口（秒，默认 60 秒 = 1 分钟）"
          rules={[{ required: true, message: '请输入时间窗口秒数' }]}
        >
          <InputNumber min={1} suffix="秒" style={{ width: 200 }} placeholder="60" />
        </Form.Item>

        <Form.Item
          name="login_rate_limit"
          label="登录速率限制"
          extra="每个时间窗口内登录接口允许的最大请求数"
          rules={[{ required: true, message: '请输入登录速率限制' }]}
        >
          <InputNumber min={1} max={100} style={{ width: 200 }} />
        </Form.Item>

        <Form.Item
          name="login_rate_interval"
          label="登录限流时间窗口"
          extra="登录速率限制的时间窗口（秒，默认 60 秒 = 1 分钟）"
          rules={[{ required: true, message: '请输入登录限流时间窗口秒数' }]}
        >
          <InputNumber min={1} max={3600} suffix="秒" style={{ width: 200 }} placeholder="60" />
        </Form.Item>

        <Form.Item
          name="allow_multi_session"
          label="允许多端同时登录"
          valuePropName="checked"
          extra="开启后新登录不会踢出其他设备会话（移动端与 Web 可同时在线）；关闭后新登录会使其他设备下线。扫码登录始终共存，不受此开关影响。"
        >
          <Switch />
        </Form.Item>

        <Form.Item
          shouldUpdate={(prev, cur) => prev.allow_multi_session !== cur.allow_multi_session}
          noStyle
        >
          {({ getFieldValue }) => {
            const multi = getFieldValue('allow_multi_session') as boolean;
            return (
              <Form.Item
                name="mobile_device_binding"
                label="移动端单设备绑定"
                valuePropName="checked"
                extra={
                  multi
                    ? '多端在线开启时生效：限制同类型移动设备（APP / 小程序）仅绑定一台常用设备，换设备登录将被拒绝，直至管理者手动解绑。Web 登录不受影响。'
                    : '全局单会话模式下本开关不生效（任何新登录均会替换原有会话）。'
                }
              >
                <Switch disabled={!multi} />
              </Form.Item>
            );
          }}
        </Form.Item>

        <Form.Item>
          <Button
            type="primary"
            onClick={handleSave}
            loading={saving}
          >
            保存配置
          </Button>
        </Form.Item>
      </Form>
    </Card>
  );
}
