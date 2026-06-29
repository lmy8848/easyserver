import { useState, useEffect } from 'react';
import {
  Card, Descriptions, Tag, Alert, Form, Input, Switch, Button, Space, message,
  InputNumber, Modal,
} from 'antd';
import { settingsApi } from '../../services/api';
import type { Settings, SystemInfo } from './types';

export interface ServerSettingsProps {
  settings: Settings;
  systemInfo: SystemInfo | null;
  onRefresh: () => void;
}

export default function ServerSettings({ settings, systemInfo, onRefresh }: ServerSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [requiresRestart, setRequiresRestart] = useState(false);

  useEffect(() => {
    if (settings?.server) {
      form.setFieldsValue({
        host: settings.server.host,
        port: settings.server.port,
        serve_frontend: settings.server.serve_frontend,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      const res = await settingsApi.updateServer(values);
      if (res.data?.data?.requires_restart) {
        setRequiresRestart(true);
        message.warning('服务器配置已保存，需要重启面板才能生效');
      } else {
        message.success('服务器配置已保存');
      }
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    } finally {
      setSaving(false);
    }
  };

  const handleRestart = () => {
    Modal.confirm({
      title: '确认重启',
      content: '重启面板将中断当前所有连接，确定要继续吗？',
      okText: '确认重启',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setRestarting(true);
        try {
          await settingsApi.restart();
          message.success('面板正在重启，请稍候...');
          setTimeout(() => {
            window.location.reload();
          }, 3000);
        } catch (error: unknown) {
          message.error((error instanceof Error ? error.message : '重启失败'));
          setRestarting(false);
        }
      },
    });
  };

  return (
    <div>
      <Card title="服务器配置">
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            host: '0.0.0.0',
            port: 8080,
            serve_frontend: false,
          }}
        >
          <Form.Item
            name="host"
            label="监听地址"
            extra="服务器监听的 IP 地址，0.0.0.0 表示所有地址"
          >
            <Input placeholder="0.0.0.0" />
          </Form.Item>

          <Form.Item
            name="port"
            label="监听端口"
            extra="服务器监听的端口号"
          >
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="serve_frontend"
            label="提供前端"
            extra="是否由后端直接提供前端静态文件服务"
            valuePropName="checked"
          >
            <Switch />
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
                type="primary"
                danger
                onClick={handleRestart}
                loading={restarting}
              >
                重启面板
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      {requiresRestart && (
        <Alert
          message="需要重启"
          description="服务器配置已修改，需要重启面板才能生效。"
          type="warning"
          showIcon
          style={{ marginTop: 16 }}
          action={
            <Button size="small" type="primary" onClick={handleRestart} loading={restarting}>
              立即重启
            </Button>
          }
        />
      )}

      <Card title="系统信息" style={{ marginTop: 16 }}>
        <Descriptions bordered column={{ xs: 1, sm: 2 }}>
          <Descriptions.Item label="系统版本">{systemInfo?.version || '-'}</Descriptions.Item>
          <Descriptions.Item label="TLS/HTTPS">
            {settings?.server.tls_enabled
              ? <Tag color="success">已启用</Tag>
              : <Tag color="default">未启用</Tag>}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}
