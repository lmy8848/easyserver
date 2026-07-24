import { useState, useEffect, useCallback } from 'react';
import {
  Card, Switch, InputNumber, Button, Table, Tag, Space, message, Input, Popconfirm, Form, Alert,
} from 'antd';
import { SafetyOutlined, StopOutlined, CheckCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import { websiteSecurityApi } from '../../services/api';

interface SecurityConfig {
  website_id: number;
  rate_limit_enabled: boolean;
  rate_limit_rate: number;
  rate_limit_burst: number;
  limit_conn: number;
  auto_ban_enabled: boolean;
  auto_ban_threshold: number;
  auto_ban_404_threshold: number;
  auto_ban_duration: number;
}

interface BannedIP {
  id: number;
  ip: string;
  reason: string;
  source: string;
  expires_at: string | null;
  created_at: string;
}

// Default config returned by the backend before any customization.
const defaultConfig: SecurityConfig = {
  website_id: 0,
  rate_limit_enabled: false,
  rate_limit_rate: 10,
  rate_limit_burst: 20,
  limit_conn: 100,
  auto_ban_enabled: false,
  auto_ban_threshold: 100,
  auto_ban_404_threshold: 50,
  auto_ban_duration: 3600,
};

export default function SecurityTab({ websiteId }: { websiteId: number }) {
  const [cfg, setCfg] = useState<SecurityConfig>(defaultConfig);
  const [cfgLoading, setCfgLoading] = useState(false);
  const [banned, setBanned] = useState<BannedIP[]>([]);
  const [banLoading, setBanLoading] = useState(false);
  const [banIP, setBanIP] = useState('');
  const [banDuration, setBanDuration] = useState(3600);

  const loadConfig = useCallback(async () => {
    setCfgLoading(true);
    try {
      const res = await websiteSecurityApi.getConfig(websiteId);
      setCfg({ ...defaultConfig, ...res.data.data } as SecurityConfig);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载配置失败';
      message.error(msg);
    } finally {
      setCfgLoading(false);
    }
  }, [websiteId]);

  const loadBanned = useCallback(async () => {
    setBanLoading(true);
    try {
      const res = await websiteSecurityApi.listBanned(websiteId);
      setBanned((res.data.data || []) as unknown as BannedIP[]);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载封禁列表失败';
      message.error(msg);
    } finally {
      setBanLoading(false);
    }
  }, [websiteId]);

  useEffect(() => { loadConfig(); loadBanned(); }, [loadConfig, loadBanned]);

  const saveConfig = async () => {
    try {
      const res = await websiteSecurityApi.updateConfig(websiteId, cfg as unknown as Record<string, unknown>);
      setCfg({ ...defaultConfig, ...res.data.data } as SecurityConfig);
      message.success('配置已保存（重启/重载 Nginx 后生效）');
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '保存失败';
      message.error(msg);
    }
  };

  const doBan = async () => {
    if (!banIP) { message.warning('请输入 IP'); return; }
    try {
      await websiteSecurityApi.ban(websiteId, banIP, '手动封禁', banDuration);
      message.success(`${banIP} 已封禁`);
      setBanIP('');
      loadBanned();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '封禁失败';
      message.error(msg);
    }
  };

  const doUnban = async (id: number) => {
    try {
      await websiteSecurityApi.unban(websiteId, id);
      message.success('已解封');
      loadBanned();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '解封失败';
      message.error(msg);
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card title={<Space><SafetyOutlined /> 限流配置</Space>} loading={cfgLoading} extra={<Button type="primary" onClick={saveConfig}>保存</Button>}>
        <Form layout="vertical">
          <Space direction="vertical" style={{ width: '100%' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span>启用限流</span>
              <Switch checked={cfg.rate_limit_enabled} onChange={v => setCfg(c => ({ ...c, rate_limit_enabled: v }))} />
            </div>
            {cfg.rate_limit_enabled && (
              <>
                <Space wrap>
                  <span>请求速率（次/秒）</span>
                  <InputNumber min={1} max={1000} value={cfg.rate_limit_rate} onChange={v => setCfg(c => ({ ...c, rate_limit_rate: v || 10 }))} />
                  <span>突发缓冲</span>
                  <InputNumber min={1} max={500} value={cfg.rate_limit_burst} onChange={v => setCfg(c => ({ ...c, rate_limit_burst: v || 20 }))} />
                  <span>单 IP 最大并发</span>
                  <InputNumber min={1} max={10000} value={cfg.limit_conn} onChange={v => setCfg(c => ({ ...c, limit_conn: v || 100 }))} />
                </Space>
                <Alert type="info" showIcon message="基于 IP 的请求频率限制，超出限制的请求将返回 503。" />
              </>
            )}
          </Space>
        </Form>
      </Card>

      <Card title={<Space><SafetyOutlined /> 自动封禁</Space>}>
        <Form layout="vertical">
          <Space direction="vertical" style={{ width: '100%' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span>启用自动封禁</span>
              <Switch checked={cfg.auto_ban_enabled} onChange={v => setCfg(c => ({ ...c, auto_ban_enabled: v }))} />
            </div>
            {cfg.auto_ban_enabled && (
              <>
                <Space wrap>
                  <span>每分钟请求阈值</span>
                  <InputNumber min={10} max={10000} value={cfg.auto_ban_threshold} onChange={v => setCfg(c => ({ ...c, auto_ban_threshold: v || 100 }))} />
                  <span>每分钟 404 阈值</span>
                  <InputNumber min={10} max={10000} value={cfg.auto_ban_404_threshold} onChange={v => setCfg(c => ({ ...c, auto_ban_404_threshold: v || 50 }))} />
                  <span>封禁时长（秒，0=永久）</span>
                  <InputNumber min={0} max={86400} value={cfg.auto_ban_duration} onChange={v => setCfg(c => ({ ...c, auto_ban_duration: v || 0 }))} />
                </Space>
                <Alert type="warning" showIcon message="超出阈值的 IP 将被自动封禁（iptables DROP + Nginx deny），可在下方手动解封。" />
              </>
            )}
          </Space>
        </Form>
      </Card>

      <Card title={<Space><StopOutlined /> IP 封禁管理</Space>} extra={<Button icon={<ReloadOutlined />} onClick={loadBanned}>刷新</Button>}>
        <Space style={{ marginBottom: 16 }}>
          <Input placeholder="输入 IP 地址" value={banIP} onChange={e => setBanIP(e.target.value)} style={{ width: 200 }} />
          <InputNumber placeholder="时长(秒)" min={0} value={banDuration} onChange={v => setBanDuration(v || 0)} style={{ width: 120 }} />
          <Popconfirm title={`确定封禁 ${banIP}？`} onConfirm={doBan} disabled={!banIP}>
            <Button danger icon={<StopOutlined />} disabled={!banIP}>封禁</Button>
          </Popconfirm>
        </Space>
        <Table size="small" dataSource={banned} rowKey="id" loading={banLoading} pagination={{ pageSize: 10 }}
          locale={{ emptyText: '暂无封禁记录' }}
          columns={[
            { title: 'IP', dataIndex: 'ip', key: 'ip', width: 140 },
            { title: '原因', dataIndex: 'reason', key: 'reason' },
            { title: '来源', dataIndex: 'source', key: 'source', width: 80, render: (s: string) => s === 'auto' ? <Tag color="orange">自动</Tag> : <Tag color="blue">手动</Tag> },
            { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 180, render: (t: string | null) => t || '永久' },
            { title: '封禁时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
            { title: '操作', key: 'action', width: 80, render: (_: unknown, r: BannedIP) => (
              <Popconfirm title={`解封 ${r.ip}？`} onConfirm={() => doUnban(r.id)}>
                <Button size="small" type="link" icon={<CheckCircleOutlined />}>解封</Button>
              </Popconfirm>
            ) },
          ]}
        />
      </Card>
    </Space>
  );
}
