import { useState, useEffect } from 'react';
import {
  Card, Descriptions, Tag, Alert, Form, Input, Select, Switch, Button, Space, message,
  Divider,
} from 'antd';
import { CheckCircleOutlined, LoadingOutlined } from '@ant-design/icons';
import { settingsApi } from '../../services/api';
import type { Settings } from './types';
import { REGION_OPTIONS } from './types';

export interface CloudSettingsProps {
  settings: Settings;
  onRefresh: () => void;
}

export default function CloudSettings({ settings, onRefresh }: CloudSettingsProps) {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);

  useEffect(() => {
    if (settings?.tencentcloud) {
      form.setFieldsValue({
        enabled: settings.tencentcloud.enabled,
        region: settings.tencentcloud.region || 'ap-guangzhou',
        instance_id: settings.tencentcloud.instance_id,
      });
    }
  }, [settings, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await settingsApi.updateCloud(values);
      message.success('腾讯云配置已保存');
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    } finally {
      setSaving(false);
    }
  };

  const handleTestConnection = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const values = await form.validateFields();
      await settingsApi.updateCloud(values);

      const res = await settingsApi.testCloud();
      setTestResult({
        success: true,
        message: `连接成功！发现 ${res.data?.data?.instance_count || 0} 个实例`,
      });
      onRefresh();
    } catch (error: unknown) {
      setTestResult({
        success: false,
        message: (error instanceof Error ? error.message : '连接失败'),
      });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div>
      <Alert
        title="腾讯云轻量应用服务器管理"
        description={
          <div>
            <p>配置腾讯云 API 凭据后，可以在「腾讯云」页面管理您的轻量应用服务器实例。</p>
            <Divider style={{ margin: '8px 0' }} />
            <p><strong>配置步骤：</strong></p>
            <ol style={{ paddingLeft: 20, margin: '8px 0' }}>
              <li>登录 <a href="https://console.cloud.tencent.com/cam/capi" target="_blank" rel="noopener">腾讯云控制台 - API密钥管理</a></li>
              <li>获取 <strong>SecretID</strong> 和 <strong>SecretKey</strong></li>
              <li>在下方填入密钥信息和实例 ID</li>
              <li>点击「测试连接」验证配置是否正确</li>
              <li>测试成功后点击「保存配置」</li>
            </ol>
            <p style={{ color: '#666', fontSize: 12 }}>提示：实例 ID 可在轻量应用服务器控制台的实例详情页获取</p>
          </div>
        }
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Card title="API 凭据配置">
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            enabled: false,
            region: 'ap-guangzhou',
          }}
        >
          <Form.Item
            name="enabled"
            label="启用腾讯云集成"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            name="secret_id"
            label="SecretID"
            extra="从腾讯云 API 密钥管理页面获取"
          >
            <Input.Password placeholder="请输入 SecretID" />
          </Form.Item>

          <Form.Item
            name="secret_key"
            label="SecretKey"
            extra="从腾讯云 API 密钥管理页面获取"
          >
            <Input.Password placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" />
          </Form.Item>

          <Form.Item
            name="region"
            label="地域"
            extra="选择您的轻量应用服务器所在的地域"
          >
            <Select options={REGION_OPTIONS} />
          </Form.Item>

          <Form.Item
            name="instance_id"
            label="实例 ID"
            extra="轻量应用服务器的实例 ID，可在控制台实例详情页获取"
          >
            <Input placeholder="lhins-xxxxxxxx" />
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
                onClick={handleTestConnection}
                loading={testing}
                icon={testing ? <LoadingOutlined /> : <CheckCircleOutlined />}
              >
                测试连接
              </Button>
            </Space>
          </Form.Item>

          {testResult && (
            <Alert
              title={testResult.success ? '连接成功' : '连接失败'}
              description={testResult.message}
              type={testResult.success ? 'success' : 'error'}
              showIcon
              closable
              onClose={() => setTestResult(null)}
            />
          )}
        </Form>
      </Card>

      <Card title="当前状态" style={{ marginTop: 16 }}>
        <Descriptions bordered column={{ xs: 1, sm: 2 }}>
          <Descriptions.Item label="腾讯云集成">
            {settings?.tencentcloud.enabled
              ? <Tag color="success">已启用</Tag>
              : <Tag color="default">未启用</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="API 凭据">
            {settings?.tencentcloud.has_secret
              ? <Tag color="success">已配置</Tag>
              : <Tag color="warning">未配置</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="地域">{settings?.tencentcloud.region || '-'}</Descriptions.Item>
          <Descriptions.Item label="实例 ID">{settings?.tencentcloud.instance_id || '-'}</Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}
