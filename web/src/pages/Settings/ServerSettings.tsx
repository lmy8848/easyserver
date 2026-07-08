import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Card, Descriptions, Tag, Alert, Form, Input, Select, Switch, Button, Space, message,
  InputNumber, Modal, Divider, Typography,
} from 'antd';
import dayjs from 'dayjs';
import { settingsApi } from '../../services/api';
import type { Settings, SystemInfo, TLSCertInfo } from './types';

const { TextArea } = Input;
const { Text, Paragraph } = Typography;

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
  // Track original port/host to detect changes that need a force restart.
  // Use refs so closures always read the latest value.
  const originalPortRef = useRef<number | undefined>(undefined);
  const originalHostRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (settings?.server) {
      form.setFieldsValue({
        host: settings.server.host,
        port: settings.server.port,
        serve_frontend: settings.server.serve_frontend,
        domain: settings.server.domain,
        redirect_mode: settings.server.redirect_mode || 'off',
        www_handling: settings.server.www_handling || 'off',
        max_upload_size: settings.server.max_upload_size ? Math.round(settings.server.max_upload_size / 1024 / 1024) : 512,
        assets_rate_limit: settings.server.assets_rate_limit,
        assets_rate_interval: settings.server.assets_rate_interval,
      });
      // Only set on first load (don't overwrite when settings refresh after save)
      if (originalPortRef.current === undefined) {
        originalPortRef.current = settings.server.port;
      }
      if (originalHostRef.current === undefined) {
        originalHostRef.current = settings.server.host;
      }
    }
  }, [settings, form]); // eslint-disable-line react-hooks/exhaustive-deps

  // Ref to always read latest form values inside closures (avoids stale closure)
  const formRef = useRef(form);
  useEffect(() => { formRef.current = form; }, [form]);

  const checkPortOrHostChanged = useCallback(() => {
    const f = formRef.current;
    const op = originalPortRef.current;
    const oh = originalHostRef.current;
    if (op === undefined || oh === undefined) return false;
    return f.getFieldValue('port') !== op || f.getFieldValue('host') !== oh;
  }, []);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      // Convert MB to bytes for backend
      const payload = { ...values };
      if (payload.max_upload_size != null) {
        payload.max_upload_size = payload.max_upload_size * 1024 * 1024;
      }
      const res = await settingsApi.updateServer(payload);
      if (res.data?.data?.requires_restart) {
        setRequiresRestart(true);
        if (values.port !== originalPortRef.current || values.host !== originalHostRef.current) {
          message.warning('服务器配置已保存，端口/地址变更需要强制重启（会短暂中断连接）');
        } else {
          message.warning('服务器配置已保存，需要重启面板才能生效');
        }
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
    const changed = checkPortOrHostChanged();
    Modal.confirm({
      title: '确认重启',
      content: changed
        ? '端口或地址已变更，重启将短暂中断连接（约 1-2 秒），确定要继续吗？'
        : '重启面板将中断当前所有连接，确定要继续吗？',
      okText: '确认重启',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setRestarting(true);
        try {
          await settingsApi.restart(changed);
          message.success('面板正在重启，请稍候...');
          setTimeout(() => {
            window.location.reload();
          }, changed ? 5000 : 3000);
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
            assets_rate_limit: 5000,
            assets_rate_interval: '1m',
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

          <Form.Item
            name="domain"
            label="面板域名"
            extra="设置后可通过域名访问面板，留空则不限制"
          >
            <Input placeholder="例：panel.example.com" />
          </Form.Item>

          <Form.Item
            name="redirect_mode"
            label="域名跳转模式"
            extra="选择哪些访问地址会自动跳转到面板域名"
          >
            <Select
              options={[
                { label: '关闭', value: 'off' },
                { label: '仅 IP 访问时跳转', value: 'ip_only' },
                { label: '所有不匹配地址跳转', value: 'non_matching' },
              ]}
            />
          </Form.Item>

          <Form.Item
            name="www_handling"
            label="www 处理"
            extra="是否统一 www 前缀"
          >
            <Select
              options={[
                { label: '不处理', value: 'off' },
                { label: '强制添加 www', value: 'force_www' },
                { label: '强制去除 www', value: 'remove_www' },
              ]}
            />
          </Form.Item>

          <Form.Item
            name="max_upload_size"
            label="最大上传大小 (MB)"
            extra="单个文件最大上传大小（MB），保存后需重启生效"
          >
            <InputNumber min={1} max={4096} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="assets_rate_limit"
            label="静态资源速率限制"
            extra="每个时间窗口内静态资源（JS、CSS 等）允许的最大请求数"
          >
            <InputNumber min={100} max={100000} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="assets_rate_interval"
            label="静态资源限流时间窗口"
            extra="静态资源速率限制的时间窗口，如 1m、5m"
          >
            <Input placeholder="1m" />
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
            {settings?.server.tls.enabled
              ? <Tag color="success">已启用</Tag>
              : <Tag color="default">未启用</Tag>}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <TLSCard
        tls={settings?.server.tls ?? { enabled: false, cert_info: null }}
        onSaved={onRefresh}
        onRestart={handleRestart}
      />
    </div>
  );
}

// TLS certificate management card
interface TLSCardProps {
  tls: { enabled: boolean; cert_info: TLSCertInfo | null };
  onSaved: () => void;
  onRestart: () => void;
}

function TLSCard({ tls, onSaved, onRestart }: TLSCardProps) {
  const [enabled, setEnabled] = useState(tls.enabled);
  const [certContent, setCertContent] = useState('');
  const [keyContent, setKeyContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [showCertForm, setShowCertForm] = useState(false);

  useEffect(() => {
    setEnabled(tls.enabled);
  }, [tls.enabled]);

  const handleSave = async () => {
    if (enabled && !certContent && !keyContent && !tls.cert_info) {
      message.warning('请粘贴证书和私钥内容');
      return;
    }
    setSaving(true);
    try {
      const res = await settingsApi.updateTLS({
        enabled,
        cert_content: certContent || undefined,
        key_content: keyContent || undefined,
      });
      message.success(res.data?.message || 'TLS 配置已保存');
      setCertContent('');
      setKeyContent('');
      setShowCertForm(false);
      onSaved();
      if (res.data?.data?.requires_restart) {
        Modal.confirm({
          title: '需要重启',
          content: 'TLS 配置已保存，需要重启面板才能生效。重启期间连接会中断，确定立即重启吗？',
          okText: '立即重启',
          cancelText: '稍后手动重启',
          okButtonProps: { danger: true },
          onOk: onRestart,
        });
      }
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const handleFileUpload = (type: 'cert' | 'key') => (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const content = ev.target?.result as string;
      if (type === 'cert') setCertContent(content);
      else setKeyContent(content);
    };
    reader.readAsText(file);
    e.target.value = '';
  };

  const isExpiringSoon = tls.cert_info?.expires_at
    ? dayjs(tls.cert_info.expires_at).isBefore(dayjs().add(30, 'day'))
    : false;
  const isExpired = tls.cert_info?.expires_at
    ? dayjs(tls.cert_info.expires_at).isBefore(dayjs())
    : false;

  return (
    <Card
      title={
        <Space>
          <span>TLS/HTTPS 证书</span>
          {tls.enabled && (
            isExpired
              ? <Tag color="red">已过期</Tag>
              : isExpiringSoon
                ? <Tag color="orange">即将过期</Tag>
                : <Tag color="green">有效</Tag>
          )}
        </Space>
      }
      style={{ marginTop: 16 }}
      extra={
        <Space>
          {enabled && (
            <Button size="small" onClick={() => setShowCertForm(!showCertForm)}>
              {showCertForm ? '取消' : tls.cert_info ? '更新证书' : '上传证书'}
            </Button>
          )}
          <Switch
            checked={enabled}
            onChange={(checked) => {
              setEnabled(checked);
              if (checked && !tls.cert_info) {
                setShowCertForm(true);
              }
            }}
            checkedChildren="启用"
            unCheckedChildren="禁用"
          />
        </Space>
      }
    >
      {tls.cert_info && (
        <Descriptions bordered column={{ xs: 1, sm: 2 }} size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="域名">{tls.cert_info.domain}</Descriptions.Item>
          <Descriptions.Item label="颁发者">{tls.cert_info.issuer}</Descriptions.Item>
          <Descriptions.Item label="过期时间" span={2}>
            {dayjs(tls.cert_info.expires_at).format('YYYY-MM-DD HH:mm:ss')}
            {isExpired && <Text type="danger"> （已过期，请尽快更新）</Text>}
            {isExpiringSoon && !isExpired && <Text type="warning"> （将在30天内过期）</Text>}
          </Descriptions.Item>
        </Descriptions>
      )}

      {!enabled && !tls.cert_info && (
        <Text type="secondary">
          启用 HTTPS 可加密面板访问流量。支持 Cloudflare 源证书、Let's Encrypt 或自签名证书。
          证书文件将存储在服务器本地，不会上传到任何第三方服务。
        </Text>
      )}

      {showCertForm && enabled && (
        <>
          <Divider />
          <Paragraph strong>证书内容 (PEM 格式)</Paragraph>
          <Space direction="vertical" style={{ width: '100%' }} size={8}>
            <div>
              <Text type="secondary" style={{ fontSize: 12 }}>
                证书文件或粘贴内容（-----BEGIN CERTIFICATE----- 开头）
              </Text>
              <TextArea
                value={certContent}
                onChange={(e) => setCertContent(e.target.value)}
                placeholder={"-----BEGIN CERTIFICATE-----\nMIIFazCCA1OgAwIBAgIUE...\n-----END CERTIFICATE-----"}
                rows={5}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
              <input
                type="file"
                accept=".pem,.crt,.cer"
                onChange={handleFileUpload('cert')}
                style={{ marginTop: 4, fontSize: 12 }}
              />
            </div>

            <div>
              <Text type="secondary" style={{ fontSize: 12 }}>
                私钥文件或粘贴内容（-----BEGIN PRIVATE KEY----- 或 -----BEGIN RSA PRIVATE KEY----- 开头）
              </Text>
              <TextArea
                value={keyContent}
                onChange={(e) => setKeyContent(e.target.value)}
                placeholder={"-----BEGIN PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END PRIVATE KEY-----"}
                rows={5}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
              <input
                type="file"
                accept=".pem,.key"
                onChange={handleFileUpload('key')}
                style={{ marginTop: 4, fontSize: 12 }}
              />
            </div>

            <div style={{ marginTop: 8 }}>
              <Paragraph type="warning" style={{ fontSize: 12, marginBottom: 8 }}>
                ⚠️ 私钥内容仅用于写入服务器文件，不会被存储到数据库。
                如需更新，建议同时粘贴证书和私钥（确保配对）。
              </Paragraph>
              <Button type="primary" onClick={handleSave} loading={saving}>
                保存证书
              </Button>
            </div>
          </Space>
        </>
      )}
    </Card>
  );
}

