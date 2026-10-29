import { useCallback, useEffect, useState } from 'react';
import {
  Alert, Button, Card, Descriptions, Form, Input, Popconfirm, Space, Switch,
  Table, Tag, Typography, message,
} from 'antd';
import { CopyOutlined, ReloadOutlined } from '@ant-design/icons';
import api from '../../services/client';
import { settingsApi, type MobileDirectInfo } from '../../services/settings';

interface DeviceRow {
  id: number;
  device_id: string;
  name: string;
  created_at: string;
  last_seen_at?: string;
  revoked: boolean;
}

// 移动端直连通道：独立端口 + 设备签名，绕过 Cloudflare。
// 没有域名的用户可直接用「IP:端口」连接，手机端首次连接时核对证书指纹并固化
// （自签证书 + pinning），因此不依赖域名或公网 CA。
export default function MobileDirectSettings() {
  const [info, setInfo] = useState<MobileDirectInfo | null>(null);
  const [devices, setDevices] = useState<DeviceRow[]>([]);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  const load = useCallback(async () => {
    try {
      const [res, devRes] = await Promise.all([
        settingsApi.getMobileDirect(),
        api.get('/auth/devices'),
      ]);
      const data = res.data.data;
      setInfo(data);
      form.setFieldsValue({
        enabled: data.enabled,
        listen: data.listen,
        hostname: data.hostname,
        clock_skew: data.clock_skew,
      });
      // 后端 /auth/devices 返回 data 为数组；兼容 _items 包裹形式
      const raw = devRes.data?.data as unknown;
      const list = Array.isArray(raw)
        ? (raw as DeviceRow[])
        : ((raw as { _items?: DeviceRow[] })?._items ?? []);
      setDevices(list);
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '加载直连配置失败');
    }
  }, [form]);

  useEffect(() => { load(); }, [load]);

  const save = async (regenerate = false) => {
    setSaving(true);
    try {
      const values = await form.validateFields();
      const res = await settingsApi.updateMobileDirect({
        enabled: values.enabled,
        listen: values.listen,
        hostname: values.hostname,
        clock_skew: values.clock_skew,
        regenerate_cert: regenerate,
      });
      setInfo(res.data.data);
      message.success(regenerate ? '已重新生成自签证书' : '已保存并应用');
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const revoke = async (id: number) => {
    try {
      await api.delete(`/auth/devices/${id}`);
      message.success('设备已吊销');
      load();
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '吊销失败');
    }
  };

  const directAddr = info?.listen || '-';

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="移动端直连通道（绕过 Cloudflare）"
        description={
          <span>
            默认经 Cloudflare 代理时，冷连接握手常需数秒。开启直连后，手机端通过独立端口
            带设备签名访问，只暴露 <code>/api</code> 业务接口，未签名请求一律 401。
            <b>没有域名也可以</b>：用「服务器IP:端口」连接，手机端首次连接核对下方证书指纹
            并固化（自签证书 + pinning）。
          </span>
        }
      />

      <Card title="通道配置" extra={<Tag color={info?.running ? 'green' : 'default'}>{info?.running ? '运行中' : '未运行'}</Tag>}>
        <Form form={form} layout="vertical" disabled={saving}>
          <Form.Item label="启用" name="enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            label="监听地址"
            name="listen"
            extra="0.0.0.0:8443 表示对所有网卡开放 8443 端口；也可绑定具体 IP"
            rules={[{ required: true, message: '请输入监听地址' }]}
          >
            <Input placeholder="0.0.0.0:8443" />
          </Form.Item>
          <Form.Item
            label="面板域名（反代 Host）"
            name="hostname"
            extra="直连请求转发给面板时使用的 Host 头，一般填面板自己的域名；用 IP 访问也不影响"
          >
            <Input placeholder="panel.example.com" />
          </Form.Item>
          <Form.Item label="签名时间偏差上限" name="clock_skew" extra="格式如 5m，范围 1m–30m">
            <Input placeholder="5m" />
          </Form.Item>
          <Space>
            <Button type="primary" loading={saving} onClick={() => save(false)}>保存并应用</Button>
            <Popconfirm
              title="重新生成自签证书？"
              description="已配对的手机需要重新核对指纹。"
              onConfirm={() => save(true)}
            >
              <Button icon={<ReloadOutlined />}>重新生成证书</Button>
            </Popconfirm>
          </Space>
        </Form>

        {info?.last_error && (
          <Alert style={{ marginTop: 16 }} type="error" showIcon message="最近一次启动失败" description={info.last_error} />
        )}
      </Card>

      <Card title="手机端连接信息">
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="连接地址">
            <Typography.Text copyable>{directAddr}</Typography.Text>
            <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
              局域网可直接用内网 IP，公网用服务器 IP
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label="证书指纹（SHA-256）">
            {info?.fingerprint ? (
              <Typography.Text code copyable={{ icon: <CopyOutlined /> }}>
                {info.fingerprint}
              </Typography.Text>
            ) : (
              <Typography.Text type="secondary">启用后自动生成</Typography.Text>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="证书文件">
            <Typography.Text type="secondary">{info?.cert_file || '-'}</Typography.Text>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="已登记设备">
        <Table<DeviceRow>
          size="small"
          rowKey="id"
          dataSource={devices}
          pagination={false}
          locale={{ emptyText: '还没有设备注册。手机 App 首次登录后会登记设备公钥。' }}
          columns={[
            { title: '设备', dataIndex: 'name', key: 'name', ellipsis: true },
            { title: '设备号', dataIndex: 'device_id', key: 'device_id', ellipsis: true },
            { title: '最近使用', dataIndex: 'last_seen_at', key: 'last_seen_at', width: 180 },
            {
              title: '状态', dataIndex: 'revoked', key: 'revoked', width: 90,
              render: (revoked: boolean) => revoked
                ? <Tag color="red">已吊销</Tag>
                : <Tag color="green">可用</Tag>,
            },
            {
              title: '操作', key: 'action', width: 100,
              render: (_: unknown, row: DeviceRow) => row.revoked ? null : (
                <Popconfirm title="吊销后该设备直连立即失效" onConfirm={() => revoke(row.id)}>
                  <Button size="small" danger>吊销</Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </Space>
  );
}
