import { useState, useEffect } from 'react';
import { Card, Form, InputNumber, Button, message } from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

export interface MonitorSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function MonitorSettings({ settings, onRefresh }: MonitorSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.monitor) {
      // Backend returns history_retention in hours (e.g., 168 = 7 days)
      const days = settings.monitor.history_retention != null
        ? Math.round(settings.monitor.history_retention / 24)
        : undefined;

      form.setFieldsValue({
        history_retention: days,
        collect_interval: settings.monitor.collect_interval,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      // Backend API receives history_retention in hours (days * 24), collect_interval in seconds
      await settingsApi.updateMonitor({
        history_retention: values.history_retention != null ? values.history_retention * 24 : undefined,
        collect_interval: values.collect_interval,
      });
      message.success('监控配置已保存');
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
    <Card title="系统监控配置">
      <Form
        form={form}
        layout="vertical"
      >
        <Form.Item
          name="history_retention"
          label="历史数据保留时间"
          extra="历史监控记录在数据库中保留的天数（1 ~ 365 天，默认 7 天）"
          rules={[{ required: true, message: '请输入保留天数' }]}
        >
          <InputNumber
            min={1}
            max={365}
            suffix="天"
            style={{ width: 200 }}
            placeholder="7"
          />
        </Form.Item>

        <Form.Item
          name="collect_interval"
          label="数据采集间隔"
          extra="监控指标采集与推送到前端的时间间隔（1 ~ 300 秒，默认 3 秒）"
          rules={[{ required: true, message: '请输入采集间隔秒数' }]}
        >
          <InputNumber
            min={1}
            max={300}
            suffix="秒"
            style={{ width: 200 }}
            placeholder="3"
          />
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
