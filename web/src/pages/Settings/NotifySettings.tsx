import { useState, useEffect } from 'react';
import {
  Card, Alert, Form, Input, Switch, Button, Space, message,
} from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

export interface NotifySettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function NotifySettings({ settings, onRefresh }: NotifySettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.notify) {
      form.setFieldsValue({
        enabled: settings.notify.enabled,
        webhook_url: settings.notify.webhook_url,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateNotify(values);
      message.success('通知配置已保存');
      onRefresh();
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    } finally {
      setSaving(false);
    }
  };

  const handleTestWebhook = async () => {
    try {
      const values = await form.validateFields();
      if (!values.webhook_url) {
        message.error('请先填写 Webhook URL');
        return;
      }
      await settingsApi.testWebhook();
      message.success('测试消息已发送，请检查 Webhook');
    } catch (error: any) {
      message.error(error.message || '测试失败');
    }
  };

  return (
    <Card title="登录通知配置">
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          enabled: false,
          webhook_url: '',
        }}
      >
        <Form.Item
          name="enabled"
          label="启用登录通知"
          extra="登录成功或失败时发送通知到 Webhook"
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>

        <Form.Item
          name="webhook_url"
          label="Webhook URL"
          extra="支持钉钉、飞书、企微等 Webhook 地址"
        >
          <Input placeholder="https://oapi.dingtalk.com/robot/send?access_token=xxx" />
        </Form.Item>

        <Form.Item>
          <Space>
            <Button
              type="primary"
              onClick={handleSave}
              loading={saving}
            >
              保存配置
            </Button>
            <Button
              onClick={handleTestWebhook}
            >
              测试通知
            </Button>
          </Space>
        </Form.Item>
      </Form>

      <Alert
        title="支持的 Webhook 类型"
        description={
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            <li>钉钉机器人 - https://oapi.dingtalk.com/robot/send?access_token=xxx</li>
            <li>飞书机器人 - https://open.feishu.cn/open-apis/bot/v2/hook/xxx</li>
            <li>企业微信 - https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx</li>
            <li>通用 Webhook - Slack 兼容格式</li>
          </ul>
        }
        type="info"
        showIcon
        style={{ marginTop: 16 }}
      />
    </Card>
  );
}
