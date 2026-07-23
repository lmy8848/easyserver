import { useState, useEffect, useCallback } from 'react';
import { Card, Button, Table, Tag, Space, message, Input, Popconfirm, Alert } from 'antd';
import { ReloadOutlined, StopOutlined, CheckCircleOutlined } from '@ant-design/icons';
import api from '../../services/api';

interface LoginEvent { time: string; ip: string; username: string; action: string; user_agent: string; anomaly?: string; }
interface Anomaly { ip: string; failed_count: number; last_attempt: string; reason: string; }
interface BannedIP { id: number; ip: string; remark: string; created_at: string; }

export default function LoginGuard() {
  const [events, setEvents] = useState<LoginEvent[]>([]);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [banned, setBanned] = useState<BannedIP[]>([]);
  const [loading, setLoading] = useState(false);
  const [banIP, setBanIP] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [h, a, b] = await Promise.all([
        api.get('/security/login/history', { params: { limit: 200 } }),
        api.get('/security/login/anomalies'),
        api.get('/security/login/banned'),
      ]);
      setEvents(h.data.data?.events || []);
      setAnomalies(a.data.data?.anomalies || []);
      setBanned(b.data.data?.banned || []);
    } catch { message.error('加载失败'); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const doBan = async (ip: string, reason: string) => {
    try {
      await api.post('/security/login/ban', { ip, reason });
      message.success(`${ip} 已封禁`);
      load();
    } catch (e: unknown) { message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '封禁失败'); }
  };

  const doUnban = async (ip: string) => {
    try {
      await api.post('/security/login/unban', { ip });
      message.success(`${ip} 已解封`);
      load();
    } catch (e: unknown) { message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '解封失败'); }
  };

  const actionColor = (a: string) => a.includes('SUCCESS') ? 'green' : a.includes('FAILED') ? 'red' : 'orange';

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card title="异常 IP（疑似暴力破解，5 分钟内失败 ≥10 次）" extra={<Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>}>
        {anomalies.length === 0 ? <Alert message="暂无异常" type="success" showIcon /> : (
          <Table size="small" dataSource={anomalies} rowKey="ip" pagination={false}
            columns={[
              { title: 'IP', dataIndex: 'ip', key: 'ip' },
              { title: '失败次数', dataIndex: 'failed_count', key: 'failed_count', width: 90, render: (n: number) => <Tag color="red">{n}</Tag> },
              { title: '最后尝试', dataIndex: 'last_attempt', key: 'last_attempt', width: 180 },
              { title: '原因', dataIndex: 'reason', key: 'reason' },
              { title: '操作', key: 'action', width: 80, render: (_: unknown, r: Anomaly) => (
                <Popconfirm title={`封禁 ${r.ip}？`} onConfirm={() => doBan(r.ip, r.reason)}>
                  <Button size="small" type="link" danger icon={<StopOutlined />}>封禁</Button>
                </Popconfirm>
              ) },
            ]}
          />
        )}
      </Card>

      <Card title="已封禁 IP">
        <Table size="small" dataSource={banned} rowKey="id" pagination={false} locale={{ emptyText: '暂无封禁' }}
          columns={[
            { title: 'IP', dataIndex: 'ip', key: 'ip' },
            { title: '备注', dataIndex: 'remark', key: 'remark' },
            { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
            { title: '操作', key: 'action', width: 80, render: (_: unknown, r: BannedIP) => (
              <Popconfirm title={`解封 ${r.ip}？`} onConfirm={() => doUnban(r.ip)}>
                <Button size="small" type="link" icon={<CheckCircleOutlined />}>解封</Button>
              </Popconfirm>
            ) },
          ]}
        />
        <Space style={{ marginTop: 16 }}>
          <Input placeholder="手动封禁 IP" value={banIP} onChange={(e) => setBanIP(e.target.value)} style={{ width: 200 }} />
          <Popconfirm title={`封禁 ${banIP}？`} onConfirm={() => { doBan(banIP, '手动封禁'); setBanIP(''); }} disabled={!banIP}>
            <Button danger icon={<StopOutlined />} disabled={!banIP}>封禁</Button>
          </Popconfirm>
        </Space>
      </Card>

      <Card title="登录历史" extra={<Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>}>
        <Table size="small" dataSource={events} rowKey={(r, i) => `${r.time}-${i}`} loading={loading} pagination={{ pageSize: 20 }}
          columns={[
            { title: '时间', dataIndex: 'time', key: 'time', width: 160 },
            { title: 'IP', dataIndex: 'ip', key: 'ip', width: 130 },
            { title: '用户', dataIndex: 'username', key: 'username', width: 100 },
            { title: '动作', dataIndex: 'action', key: 'action', width: 150, render: (a: string) => <Tag color={actionColor(a)}>{a}</Tag> },
            { title: '异常', dataIndex: 'anomaly', key: 'anomaly', width: 100, render: (a?: string) => a ? <Tag color="orange">{a}</Tag> : '-' },
            { title: 'UA', dataIndex: 'user_agent', key: 'user_agent', ellipsis: true },
          ]}
        />
      </Card>
    </Space>
  );
}
