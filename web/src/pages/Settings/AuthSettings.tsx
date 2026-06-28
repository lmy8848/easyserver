import { useState, useEffect } from 'react';
import {
  Card, Form, Input, Button, message, InputNumber,
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
        session_timeout: settings.auth.session_timeout,
        idle_timeout: settings.auth.idle_timeout,
        max_login_attempts: settings.auth.max_login_attempts,
        lockout_duration: settings.auth.lockout_duration,
        rate_limit: settings.auth.rate_limit,
        rate_interval: settings.auth.rate_interval,
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
          session_timeout: '24h',
          idle_timeout: '30m',
          max_login_attempts: 5,
          lockout_duration: '15m',
          rate_limit: 1000,
          rate_interval: '1m',
        }}
      >
        <Form.Item
          name="session_timeout"
          label="会话超时"
          extra="用户会话的有效期，如 24h、12h"
        >
          <Input placeholder="24h" />
        </Form.Item>

        <Form.Item
          name="idle_timeout"
          label="空闲超时"
          extra="用户无操作后自动登出的时间，如 30m、1h"
        >
          <Input placeholder="30m" />
        </Form.Item>

        <Form.Item
          name="max_login_attempts"
          label="最大登录尝试次数"
          extra="登录失败多少次后锁定账户"
        >
          <InputNumber min={1} max={100} style={{ width: '100%' }} />
        </Form.Item>

        <Form.Item
          name="lockout_duration"
          label="锁定时长"
          extra="账户锁定的持续时间，如 15m、30m"
        >
          <Input placeholder="15m" />
        </Form.Item>

        <Form.Item
          name="rate_limit"
          label="速率限制"
          extra="每个时间窗口内允许的最大请求数"
        >
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>

        <Form.Item
          name="rate_interval"
          label="速率限制时间窗口"
          extra="速率限制的时间窗口，如 1m、5m"
        >
          <Input placeholder="1m" />
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
