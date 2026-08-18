import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Card, Descriptions, Tag, Alert, Form, Input, Switch, Button, Space, message,
  InputNumber, Modal, Divider, Typography, Upload, Select, Row, Col,
} from 'antd';
import { UploadOutlined, SyncOutlined, CloudDownloadOutlined } from '@ant-design/icons';
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
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<{
    current_version: string;
    latest_version: string;
    release_title: string;
    has_update: boolean;
    release_url: string;
    release_notes: string;
    published_at: string;
  } | null>(null);
  const [showUpdateModal, setShowUpdateModal] = useState(false);

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    try {
      const res = await settingsApi.checkUpdate();
      const data = res.data?.data;
      if (data) {
        setUpdateInfo(data);
        if (data.has_update) {
          setShowUpdateModal(true);
        } else {
          message.success(`当前已是最新版本 (${data.current_version})`);
        }
      }
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '检查更新失败，请稍后重试');
    } finally {
      setCheckingUpdate(false);
    }
  };

  // Track original port/host to detect changes that need a force restart.
  // Use refs so closures always read the latest value.
  const originalPortRef = useRef<number | undefined>(undefined);
  const originalHostRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (settings?.server) {
      form.setFieldsValue({
        host: settings.server.host,
        port: settings.server.port,
        domain: settings.server.domain,
        force_domain: settings.server.force_domain,
        max_upload_size: settings.server.max_upload_size != null
          ? Math.round(settings.server.max_upload_size / 1024 / 1024)
          : undefined,
        assets_rate_limit: settings.server.assets_rate_limit,
        assets_rate_interval: settings.server.assets_rate_interval,
        allowed_origins: settings.server.allowed_origins ?? [],
        turnstile_site_key: settings.server.turnstile?.site_key,
        turnstile_secret_key: settings.server.turnstile?.secret_key,
        turnstile_enable_login: settings.server.turnstile?.enable_login,
        turnstile_enable_qr_login: settings.server.turnstile?.enable_qr_login,
        turnstile_enable_public_share: settings.server.turnstile?.enable_public_share,
      });
      // Only set on first load (don't overwrite when settings refresh after save)
      if (originalPortRef.current === undefined) {
        originalPortRef.current = settings.server.port;
      }
      if (originalHostRef.current === undefined) {
        originalHostRef.current = settings.server.host;
      }
    }
  }, [settings, form]);  

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
      // Build payload from individual fields to avoid `any`/index-signature
      // friction with the form values object.
      const payload: Record<string, unknown> = {
        host: values.host,
        port: values.port,
        domain: values.domain,
        force_domain: values.force_domain,
        max_upload_size: values.max_upload_size != null ? values.max_upload_size * 1024 * 1024 : undefined,
        assets_rate_limit: values.assets_rate_limit,
        assets_rate_interval: values.assets_rate_interval,
        allowed_origins: (values.allowed_origins ?? []).map((o: string) => o.trim()).filter(Boolean),
        turnstile: {
          site_key: values.turnstile_site_key || '',
          secret_key: values.turnstile_secret_key || '',
          enable_login: !!values.turnstile_enable_login,
          enable_qr_login: !!values.turnstile_enable_qr_login,
          enable_public_share: !!values.turnstile_enable_public_share,
        },
      };
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
        >
          <Row gutter={[24, 0]}>
            <Col xs={24} sm={12}>
              <Form.Item
                name="host"
                label="监听地址"
                extra="服务器监听的 IP 地址，0.0.0.0 表示所有地址"
              >
                <Input placeholder="0.0.0.0" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="port"
                label="监听端口"
                extra="服务器监听的端口号"
              >
                <InputNumber min={1} max={65535} style={{ width: '100%' }} />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="domain"
                label="面板域名"
                extra="设置后可通过域名访问面板，留空则不限制"
              >
                <Input placeholder="例：panel.example.com" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="force_domain"
                label="强制域名访问"
                valuePropName="checked"
                extra="开启后，所有通过 IP 或其他非匹配主机名的访问将自动 301 重定向至该域名"
              >
                <Switch />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="max_upload_size"
                label="最大上传大小 (MB)"
                extra="单个文件最大上传大小（MB），0 表示不限制/使用默认（512MB），保存后需重启生效"
              >
                <InputNumber min={0} max={4096} style={{ width: '100%' }} placeholder="0" />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="assets_rate_limit"
                label="静态资源速率限制"
                extra="每个时间窗口内静态资源（JS、CSS 等）允许的最大请求数"
              >
                <InputNumber min={100} max={100000} style={{ width: '100%' }} />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="assets_rate_interval"
                label="静态资源限流时间窗口"
                extra="静态资源速率限制的时间窗口，如 1m、5m"
              >
                <Input placeholder="1m" />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item
                name="allowed_origins"
                label="允许的跨域来源 (CORS)"
                extra="允许跨域访问面板的 Origin 列表，每行一个（如 http://localhost:5173）。默认已含本机来源，一般无需修改。"
              >
                <Select
                  mode="tags"
                  open={false}
                  suffixIcon={null}
                  placeholder="每行一个 Origin，回车添加"
                  tokenSeparators={[',', '，']}
                />
              </Form.Item>
            </Col>
          </Row>

          <Divider />
          <Typography.Title level={5} style={{ margin: 0 }}>Cloudflare Turnstile</Typography.Title>
          <Paragraph style={{ color: '#8c8c8c', fontSize: 12, marginBottom: 8 }}>
            配置 Cloudflare Turnstile 人机验证，在登录/扫码/外链下载前要求用户完成验证。
            需在 <a href="https://dash.cloudflare.com/" target="_blank" rel="noreferrer">Cloudflare Dashboard</a> 创建 Turnstile 站点。
          </Paragraph>

          <Row gutter={[24, 0]}>
            <Col xs={24} sm={12}>
              <Form.Item
                name="turnstile_site_key"
                label="Site Key"
                extra="Turnstile 站点密钥(公开)"
              >
                <Input placeholder="0x4AAAAAA..." />
              </Form.Item>
            </Col>

            <Col xs={24} sm={12}>
              <Form.Item
                name="turnstile_secret_key"
                label="Secret Key"
                extra="Turnstile 密钥(敏感,请妥善保管)"
              >
                <Input.Password placeholder="0x4AAAAAA..." autoComplete="off" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={[24, 0]}>
            <Col xs={24} sm={8}>
              <Form.Item
                name="turnstile_enable_login"
                label="登录验证"
                extra="密码登录和 TOTP/备份码验证时要求 Turnstile"
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
            </Col>
            <Col xs={24} sm={8}>
              <Form.Item
                name="turnstile_enable_qr_login"
                label="扫码登录验证"
                extra="手机端扫码确认登录时要求 Turnstile"
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
            </Col>
            <Col xs={24} sm={8}>
              <Form.Item
                name="turnstile_enable_public_share"
                label="外链下载验证"
                extra="公开文件外链下载时要求 Turnstile"
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item style={{ marginTop: 16 }}>
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
          title="需要重启"
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

      <TLSCard
        tls={settings?.server.tls ?? { enabled: false, cert_file: '', key_file: '', cert_info: null }}
        onSaved={onRefresh}
        onRestart={handleRestart}
      />

      <Card
        title="版本信息"
        style={{ marginTop: 16 }}
        extra={
          <Button
            icon={<SyncOutlined spin={checkingUpdate} />}
            loading={checkingUpdate}
            onClick={handleCheckUpdate}
          >
            检查更新
          </Button>
        }
      >
        <Descriptions bordered column={1}>
          <Descriptions.Item label="系统版本">
            <Space>
              <span>{systemInfo?.version || '-'}</span>
              {updateInfo?.has_update && (
                <Tag color="processing" style={{ cursor: 'pointer' }} onClick={() => setShowUpdateModal(true)}>
                  有新版本 {updateInfo.latest_version} 可用
                </Tag>
              )}
            </Space>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Modal
        title={
          <Space>
            <CloudDownloadOutlined style={{ color: '#1890ff' }} />
            <span>发现新版本 {updateInfo?.latest_version}</span>
          </Space>
        }
        open={showUpdateModal}
        onCancel={() => setShowUpdateModal(false)}
        footer={[
          <Button key="close" onClick={() => setShowUpdateModal(false)}>
            关闭
          </Button>,
          <Button
            key="github"
            type="primary"
            href={updateInfo?.release_url}
            target="_blank"
            rel="noreferrer"
          >
            前往 GitHub 查看并下载
          </Button>,
        ]}
        width={560}
      >
        {updateInfo && (
          <div style={{ marginTop: 12 }}>
            <Descriptions bordered size="small" column={1} style={{ marginBottom: 16 }}>
              <Descriptions.Item label="当前版本">{updateInfo.current_version}</Descriptions.Item>
              <Descriptions.Item label="最新版本">{updateInfo.latest_version}</Descriptions.Item>
              {updateInfo.published_at && (
                <Descriptions.Item label="发布时间">
                  {dayjs(updateInfo.published_at).format('YYYY-MM-DD HH:mm:ss')}
                </Descriptions.Item>
              )}
            </Descriptions>

            {updateInfo.release_title && (
              <Paragraph strong style={{ fontSize: 14, marginBottom: 8 }}>
                {updateInfo.release_title}
              </Paragraph>
            )}

            {updateInfo.release_notes ? (
              <Card size="small" style={{ maxHeight: 240, overflowY: 'auto', background: '#fafafa' }}>
                <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'inherit', fontSize: 13 }}>
                  {updateInfo.release_notes}
                </pre>
              </Card>
            ) : (
              <Text type="secondary">暂无更新日志说明</Text>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}

// TLS certificate management card
interface TLSCardProps {
  tls: { enabled: boolean; cert_file: string; key_file: string; cert_info: TLSCertInfo | null };
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
          <Descriptions.Item label="证书文件">{tls.cert_file || '-'}</Descriptions.Item>
          <Descriptions.Item label="私钥文件">{tls.key_file || '-'}</Descriptions.Item>
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
          <Space orientation="vertical" style={{ width: '100%' }} size={8}>
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
              <Upload
                accept=".pem,.crt,.cer"
                showUploadList={false}
                beforeUpload={(file) => {
                  const reader = new FileReader();
                  reader.onload = (ev) => setCertContent(ev.target?.result as string);
                  reader.readAsText(file);
                  return false;
                }}
              >
                <Button size="small" icon={<UploadOutlined />} style={{ marginTop: 8 }}>选择证书文件</Button>
              </Upload>
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
              <Upload
                accept=".pem,.key"
                showUploadList={false}
                beforeUpload={(file) => {
                  const reader = new FileReader();
                  reader.onload = (ev) => setKeyContent(ev.target?.result as string);
                  reader.readAsText(file);
                  return false;
                }}
              >
                <Button size="small" icon={<UploadOutlined />} style={{ marginTop: 8 }}>选择私钥文件</Button>
              </Upload>
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

