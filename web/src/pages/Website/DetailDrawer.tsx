import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Tabs, Table, Descriptions, Button, Space, Tag, message, Spin,
  Alert, Statistic, Row, Col, Typography, Empty,
} from 'antd';
import {
  ReloadOutlined, ThunderboltOutlined, SafetyOutlined, CloudServerOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { Website } from '../../types';

const { Paragraph } = Typography;

interface SSLCertInfo {
  enabled: boolean; subject: string; issuer: string; not_before: string; not_after: string;
  days_remaining: number; serial: string; dns_names: string[]; sig_algo: string; cert_path: string; key_path: string;
}
interface LogEntry { time: string; ip: string; method: string; path: string; status: string; bytes: string; ua: string; raw?: string; }
interface WebsiteStats { total_requests: number; total_bytes: number; status_2xx: number; status_3xx: number; status_4xx: number; status_5xx: number; top_ips: { key: string; value: number }[]; top_paths: { key: string; value: number }[]; window: string; }
interface HealthResult { ok: boolean; status_code: number; latency_ms: number; error?: string; checked_at: string; }
interface ProcessStatus { status: string; pid: number; uptime: number; cpu_percent: number; memory_mb: number; restarts: number; last_start: string; last_error: string; managed: boolean; }

interface Props { webServerId: number; website: Website | null; open: boolean; onClose: () => void; }

export default function DetailDrawer({ webServerId, website, open, onClose }: Props) {
  const [tab, setTab] = useState('overview');
  if (!website) return null;
  const wid = website.id;
  const base = `/web-servers/${webServerId}/websites/${wid}`;

  return (
    <Drawer
      width="70%"
      open={open}
      onClose={onClose}
      title={<Space><CloudServerOutlined /> {website.name} <Tag>{website.domain}</Tag>
        <Tag color={website.status === 'active' ? 'green' : 'red'}>{website.status === 'active' ? '启用' : '禁用'}</Tag></Space>}
    >
      <Tabs activeKey={tab} onChange={setTab} items={[
        { key: 'overview', label: '概览', children: <OverviewTab website={website} /> },
        { key: 'access', label: '访问日志', children: tab === 'access' && <LogsTab base={base} type="access" /> },
        { key: 'error', label: '错误日志', children: tab === 'error' && <LogsTab base={base} type="error" /> },
        { key: 'ssl', label: 'SSL 证书', children: tab === 'ssl' && <SSLTab base={base} /> },
        { key: 'process', label: '进程', children: tab === 'process' && <ProcessTab base={base} /> },
        { key: 'config', label: 'Nginx 配置', children: tab === 'config' && <ConfigTab base={base} /> },
        { key: 'stats', label: '访问统计', children: tab === 'stats' && <StatsTab base={base} /> },
        { key: 'health', label: '健康探活', children: tab === 'health' && <HealthTab base={base} /> },
      ]} />
    </Drawer>
  );
}

function OverviewTab({ website }: { website: Website }) {
  return (
    <Descriptions column={2} bordered size="small">
      <Descriptions.Item label="域名">{website.domain}</Descriptions.Item>
      <Descriptions.Item label="监听端口">{website.port}</Descriptions.Item>
      <Descriptions.Item label="应用端口">{website.app_port || '-'}</Descriptions.Item>
      <Descriptions.Item label="项目类型">{website.project_type || '静态'}</Descriptions.Item>
      <Descriptions.Item label="根目录">{website.root_path}</Descriptions.Item>
      <Descriptions.Item label="SSL">{website.ssl_enabled ? <Tag color="green">已启用</Tag> : <Tag>未启用</Tag>}</Descriptions.Item>
      <Descriptions.Item label="反代">{website.proxy_enabled ? <Tag color="blue">{website.proxy_pass}</Tag> : <Tag>关闭</Tag>}</Descriptions.Item>
      <Descriptions.Item label="状态">{website.status === 'active' ? <Tag color="green">启用</Tag> : <Tag color="red">禁用</Tag>}</Descriptions.Item>
      <Descriptions.Item label="构建命令" span={2}>{website.build_command || '-'}</Descriptions.Item>
      <Descriptions.Item label="启动命令" span={2}>{website.start_command || '-'}</Descriptions.Item>
      <Descriptions.Item label="访问日志" span={2}>{website.access_log || '-'}</Descriptions.Item>
      <Descriptions.Item label="错误日志" span={2}>{website.error_log || '-'}</Descriptions.Item>
      <Descriptions.Item label="创建时间">{website.created_at}</Descriptions.Item>
      <Descriptions.Item label="更新时间">{website.updated_at}</Descriptions.Item>
    </Descriptions>
  );
}

function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const reload = () => { setLoading(true); fn().then(d => { setData(d); setError(''); }).catch(e => setError(e.response?.data?.message || '加载失败')).finally(() => setLoading(false)); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(reload, deps);
  return { data, loading, error, reload };
}

function LogsTab({ base, type }: { base: string; type: string }) {
  const { data, loading } = useAsync(async () => {
    const res = await api.get(`${base}/logs/parse`, { params: { type, lines: 500 } });
    return (res.data.data?.entries || []) as LogEntry[];
  }, [type]);
  return (
    <Table size="small" loading={loading} dataSource={data || []} rowKey={(r, i) => `${r.ip}-${i}`} pagination={{ pageSize: 20 }}
      columns={[
        { title: '时间', dataIndex: 'time', key: 'time', width: 180, ellipsis: true },
        { title: 'IP', dataIndex: 'ip', key: 'ip', width: 130 },
        { title: '方法', dataIndex: 'method', key: 'method', width: 70 },
        { title: '路径', dataIndex: 'path', key: 'path', ellipsis: true },
        { title: '状态', dataIndex: 'status', key: 'status', width: 70, render: (s: string) => s ? <Tag color={s[0] === '2' ? 'green' : s[0] === '4' ? 'orange' : s[0] === '5' ? 'red' : 'default'}>{s}</Tag> : '-' },
        { title: '字节', dataIndex: 'bytes', key: 'bytes', width: 80 },
      ]}
      locale={{ emptyText: '暂无日志' }}
    />
  );
}

function SSLTab({ base }: { base: string }) {
  const { data, loading, reload } = useAsync(async () => {
    const res = await api.get(`${base}/ssl`);
    return res.data.data as SSLCertInfo;
  }, []);
  if (loading) return <Spin />;
  if (!data?.enabled) return <Alert message="该网站未启用 SSL" type="info" showIcon />;
  const dr = data.days_remaining;
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert message={`证书剩余 ${dr} 天`} type={dr < 0 ? 'error' : dr < 7 ? 'error' : dr < 30 ? 'warning' : 'success'} showIcon
        action={<Button size="small" icon={<ReloadOutlined />} onClick={reload}>刷新</Button>} />
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="Subject">{data.subject}</Descriptions.Item>
        <Descriptions.Item label="颁发者">{data.issuer}</Descriptions.Item>
        <Descriptions.Item label="有效期">{data.not_before} ~ {data.not_after}</Descriptions.Item>
        <Descriptions.Item label="序列号">{data.serial}</Descriptions.Item>
        <Descriptions.Item label="DNS Names">{(data.dns_names || []).join(', ') || '-'}</Descriptions.Item>
        <Descriptions.Item label="签名算法">{data.sig_algo}</Descriptions.Item>
        <Descriptions.Item label="证书路径">{data.cert_path}</Descriptions.Item>
        <Descriptions.Item label="私钥路径">{data.key_path}</Descriptions.Item>
      </Descriptions>
    </Space>
  );
}

function ProcessTab({ base }: { base: string }) {
  const { data, loading, reload } = useAsync(async () => {
    const res = await api.get(`${base}/process`);
    return res.data.data as ProcessStatus | null;
  }, []);
  if (loading) return <Spin />;
  if (!data) return <Alert message="未关联进程守护" type="info" showIcon />;
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Row gutter={16}>
        <Col span={6}><Statistic title="状态" value={data.status} valueStyle={{ color: data.status === 'running' ? '#52c41a' : '#ff4d4f' }} /></Col>
        <Col span={6}><Statistic title="PID" value={data.pid || '-'} /></Col>
        <Col span={6}><Statistic title="CPU %" value={data.cpu_percent?.toFixed(1) || 0} /></Col>
        <Col span={6}><Statistic title="内存 MB" value={data.memory_mb?.toFixed(1) || 0} /></Col>
      </Row>
      <Descriptions column={2} bordered size="small">
        <Descriptions.Item label="运行时长(秒)">{data.uptime}</Descriptions.Item>
        <Descriptions.Item label="重启次数">{data.restarts}</Descriptions.Item>
        <Descriptions.Item label="最后启动">{data.last_start || '-'}</Descriptions.Item>
        <Descriptions.Item label="是否托管">{data.managed ? '是' : '否'}</Descriptions.Item>
        <Descriptions.Item label="最后错误" span={2}>{data.last_error || '-'}</Descriptions.Item>
      </Descriptions>
      <Space>
        <Button icon={<ThunderboltOutlined />} onClick={() => api.post(`${base}/process/start`).then(() => { message.success('已启动'); reload(); }).catch(() => message.error('启动失败'))}>启动</Button>
        <Button danger onClick={() => api.post(`${base}/process/stop`).then(() => { message.success('已停止'); reload(); }).catch(() => message.error('停止失败'))}>停止</Button>
      </Space>
    </Space>
  );
}

function ConfigTab({ base }: { base: string }) {
  const { data, loading } = useAsync(async () => {
    const res = await api.get(`${base}/config`);
    return res.data.data?.config as string;
  }, []);
  if (loading) return <Spin />;
  return <Paragraph><pre style={{ background: '#f5f5f5', padding: 12, borderRadius: 4, maxHeight: 500, overflow: 'auto', fontSize: 12 }}>{data || '（配置文件不存在）'}</pre></Paragraph>;
}

function StatsTab({ base }: { base: string }) {
  const { data, loading } = useAsync(async () => {
    const res = await api.get(`${base}/stats`);
    return res.data.data as WebsiteStats;
  }, []);
  if (loading) return <Spin />;
  if (!data) return <Empty />;
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert message={`统计窗口：${data.window}`} type="info" showIcon />
      <Row gutter={16}>
        <Col span={6}><Statistic title="总请求" value={data.total_requests} /></Col>
        <Col span={6}><Statistic title="总流量(MB)" value={(data.total_bytes / 1048576).toFixed(2)} /></Col>
        <Col span={3}><Statistic title="2xx" value={data.status_2xx} valueStyle={{ color: '#52c41a' }} /></Col>
        <Col span={3}><Statistic title="3xx" value={data.status_3xx} valueStyle={{ color: '#1677ff' }} /></Col>
        <Col span={3}><Statistic title="4xx" value={data.status_4xx} valueStyle={{ color: '#faad14' }} /></Col>
        <Col span={3}><Statistic title="5xx" value={data.status_5xx} valueStyle={{ color: '#ff4d4f' }} /></Col>
      </Row>
      <Row gutter={16}>
        <Col span={12}>
          <Table size="small" title={() => 'Top IP'} dataSource={data.top_ips || []} rowKey="key" pagination={false}
            columns={[{ title: 'IP', dataIndex: 'key', key: 'key' }, { title: '次数', dataIndex: 'value', key: 'value', width: 80 }]} />
        </Col>
        <Col span={12}>
          <Table size="small" title={() => 'Top 路径'} dataSource={data.top_paths || []} rowKey="key" pagination={false}
            columns={[{ title: '路径', dataIndex: 'key', key: 'key', ellipsis: true }, { title: '次数', dataIndex: 'value', key: 'value', width: 80 }]} />
        </Col>
      </Row>
    </Space>
  );
}

function HealthTab({ base }: { base: string }) {
  const [res, setRes] = useState<HealthResult | null>(null);
  const [loading, setLoading] = useState(false);
  const probe = useCallback(() => {
    setLoading(true);
    api.post(`${base}/health/probe`).then(r => setRes(r.data.data)).catch(e => message.error(e.response?.data?.message || '探活失败')).finally(() => setLoading(false));
  }, [base]);
  useEffect(() => { probe(); }, [probe]);
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Button type="primary" icon={<SafetyOutlined />} onClick={probe} loading={loading}>立即探活</Button>
      {res && (
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="结果">{res.ok ? <Tag color="green">正常</Tag> : <Tag color="red">异常</Tag>}</Descriptions.Item>
          <Descriptions.Item label="状态码">{res.status_code || '-'}</Descriptions.Item>
          <Descriptions.Item label="耗时(ms)">{res.latency_ms}</Descriptions.Item>
          <Descriptions.Item label="检查时间">{res.checked_at}</Descriptions.Item>
          {res.error && <Descriptions.Item label="错误" span={2}>{res.error}</Descriptions.Item>}
        </Descriptions>
      )}
    </Space>
  );
}
