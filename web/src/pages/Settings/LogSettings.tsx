import { useState, useEffect } from 'react';
import { Card, Form, Select, InputNumber, Button, Input, Alert, message } from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';

const LEVEL_OPTIONS = [
  { label: 'Debug（调试）', value: 'debug' },
  { label: 'Info（信息）', value: 'info' },
  { label: 'Warn（警告）', value: 'warn' },
  { label: 'Error（错误）', value: 'error' },
];

const FORMAT_OPTIONS = [
  { label: '文本（text）', value: 'text' },
  { label: 'JSON（json）', value: 'json' },
];

export interface LogSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

/**
 * 运行日志设置：全局日志等级（运行时立即生效）、格式与轮转参数。
 * 日志文件持久化在应用根目录，可在面板查看实际路径便于 SSH 排查。
 */
export default function LogSettings({ settings, onRefresh }: LogSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings?.logs) {
      form.setFieldsValue({
        level: settings.logs.level || 'info',
        format: settings.logs.format || 'text',
        max_size_mb: settings.logs.max_size_mb || 10,
        max_files: settings.logs.max_files || 3,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateLogs(values);
      message.success('日志配置已保存，等级已生效');
      onRefresh();
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { message?: string } } })?.response?.data?.message || (error as Error)?.message;
      if (msg) message.error(msg);
    } finally {
      setSaving(false);
    }
  };

  const logPath = settings?.logs?.path || '应用根目录 easyserver.log';

  return (
    <Card title="运行日志">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="全局运行日志持久化到应用根目录，用于排查面板自身的问题。调整等级立即生效并写入配置，重启后保持。"
      />
      <Form form={form} layout="vertical">
        <Form.Item name="level" label="日志等级" extra="高于所选等级的日志将被写入文件；等级调低可排查更细的问题。">
          <Select options={LEVEL_OPTIONS} style={{ maxWidth: 280 }} />
        </Form.Item>
        <Form.Item name="format" label="日志格式" extra="JSON 便于日志采集与聚合工具解析。">
          <Select options={FORMAT_OPTIONS} style={{ maxWidth: 280 }} />
        </Form.Item>
        <Form.Item name="max_size_mb" label="单文件轮转阈值 (MB)" extra="超过后自动轮转为 .log.1，并保留 max_files 份。">
          <InputNumber min={1} max={1024} style={{ maxWidth: 280 }} />
        </Form.Item>
        <Form.Item name="max_files" label="保留轮转文件数" extra="轮转文件最多保留份数，防止日志占满磁盘。">
          <InputNumber min={1} max={10} style={{ maxWidth: 280 }} />
        </Form.Item>
        <Form.Item label="当前日志文件路径">
          <Input value={logPath} readOnly addonAfter="此路径仅供查看，不可在面板修改" />
        </Form.Item>
        <Form.Item>
          <Button type="primary" onClick={handleSave} loading={saving}>保存</Button>
        </Form.Item>
      </Form>
    </Card>
  );
}