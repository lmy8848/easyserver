import { useState, useEffect } from 'react';
import {
  Card, Alert, Form, Input, Switch, Button, Space, message,
} from 'antd';
import { settingsApi } from '../../services/settings';
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
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    } finally {
      setSaving(false);
    }
  };

  const [sending, setSending] = useState(false);
  const [notifyForm] = Form.useForm();

  const handleSendNotify = async () => {
    try {
      const values = await notifyForm.validateFields();
      setSending(true);
      const res = await settingsApi.sendNotify({ title: values.title, content: values.content });
      message.success(res.data.data?.message || '通知已发送，请检查 Webhook');
      notifyForm.setFieldsValue({ title: '', content: '' });
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '发送失败'));
    } finally {
      setSending(false);
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
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '测试失败'));
    }
  };

  return (
    <Card title="登录通知配置">
      <Form
        form={form}
        layout="vertical"
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

      <Card
        type="inner"
        title="发送自定义通知"
        style={{ marginTop: 16 }}
        extra={<Button type="primary" onClick={handleSendNotify} loading={sending} disabled={!settings?.notify?.webhook_url}>发送</Button>}
      >
        <Form form={notifyForm} layout="vertical">
          <Form.Item name="title" label="标题（可选）" extra="留空时使用默认标题 EasyServer 通知">
            <Input placeholder="例如：维护公告" maxLength={100} />
          </Form.Item>
          <Form.Item
            name="content"
            label="内容（必填）"
            rules={[{ required: true, message: '请输入通知内容' }]}
            extra="发送到上方配置的 Webhook，按渠道（钉钉/飞书/企微/通用）自动适配格式"
          >
            <Input.TextArea rows={4} placeholder="支持 Markdown 语法" maxLength={4000} showCount />
          </Form.Item>
        </Form>
      </Card>

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
