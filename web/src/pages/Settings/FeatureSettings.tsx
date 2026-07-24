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
        file_preview: settings.features.file_preview,
        login_guard: settings.features.login_guard,
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
        <Form.Item name="file_preview" label="文件预览增强" valuePropName="checked" extra="支持图片/音频/视频/PDF/文本/压缩文件预览。关闭后文件管理不显示预览按钮。">
          <Switch />
        </Form.Item>
        <Form.Item name="login_guard" label="登录防护" valuePropName="checked" extra="登录历史、暴力破解识别、IP 封禁/解封。关闭后侧边栏隐藏「登录防护」菜单。">
          <Switch />
        </Form.Item>
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
