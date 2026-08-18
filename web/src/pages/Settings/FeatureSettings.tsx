import { useState, useEffect } from 'react';
import { Card, Form, Switch, Button, message } from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

export interface FeatureSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function FeatureSettings({ settings, onRefresh }: FeatureSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.features) {
      form.setFieldsValue({
        fim: settings.features.fim,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateFeatures(values);
      message.success('功能开关已保存');
      onRefresh();
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { message?: string } } })?.response?.data?.message || (error as Error)?.message;
      if (msg) message.error(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card title="功能开关">
      <Form form={form} layout="vertical">
        <Form.Item name="fim" label="文件完整性监控" valuePropName="checked" extra="关键文件 sha256 基线与变更检测。关闭后侧边栏隐藏「文件完整性」菜单。">
          <Switch />
        </Form.Item>
        <Form.Item>
          <Button type="primary" onClick={handleSave} loading={saving}>保存</Button>
        </Form.Item>
      </Form>
    </Card>
  );
}
