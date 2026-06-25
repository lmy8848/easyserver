import { useState, useEffect } from 'react';
import {
  Card, Descriptions, Form, Switch, Button, message,
} from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

export interface AuditSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function AuditSettings({ settings, onRefresh }: AuditSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.audit) {
      form.setFieldsValue({
        enabled: settings.audit.enabled,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateAudit(values);
      message.success('审计配置已保存');
      onRefresh();
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card title="审计日志配置">
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          enabled: true,
        }}
      >
        <Form.Item
          name="enabled"
          label="启用审计功能"
          extra="记录所有用户操作到审计日志"
          valuePropName="checked"
        >
          <Switch />
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

      <Descriptions bordered column={1} style={{ marginTop: 16 }}>
        <Descriptions.Item label="日志路径">{settings?.audit.log_path}</Descriptions.Item>
      </Descriptions>
    </Card>
  );
}
