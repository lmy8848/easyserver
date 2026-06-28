import { useState, useEffect } from 'react';
import {
  Card, Form, Input, Button, message,
} from 'antd';
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
      form.setFieldsValue({
        history_retention: settings.monitor.history_retention,
        collect_interval: settings.monitor.collect_interval,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateMonitor(values);
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
        initialValues={{
          history_retention: '24h',
          collect_interval: '1s',
        }}
      >
        <Form.Item
          name="history_retention"
          label="历史数据保留时间"
          extra="监控数据保留的时长，如 24h、168h（7天）"
        >
          <Input placeholder="24h" />
        </Form.Item>

        <Form.Item
          name="collect_interval"
          label="数据采集间隔"
          extra="监控数据采集的间隔，如 1s、5s"
        >
          <Input placeholder="1s" />
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
