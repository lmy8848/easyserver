import { useState, useEffect } from 'react';
import {
  Card, Form, InputNumber, Button, message,
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
        retention_days: settings.audit.retention_days,
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
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { message?: string } } })?.response?.data?.message || (error as Error)?.message;
      if (msg) message.error(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card title="审计日志配置">
      <Form
        form={form}
        layout="vertical"
      >
        <Form.Item
          name="retention_days"
          label="日志保留天数"
          extra="审计日志全量记录于 SQLite 数据库中。超过保留天数的历史日志将由后台定时任务自动清理。"
          rules={[
            { required: true, message: '请输入保留天数' },
            { type: 'number', min: 1, max: 3650, message: '保留天数需在 1 到 3650 之间' },
          ]}
        >
          <InputNumber min={1} max={3650} addonAfter="天" style={{ width: 160 }} />
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
