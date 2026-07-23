import { useState, useEffect, useCallback, useMemo } from 'react';
import { Row, Col, Card, Statistic, Spin, Descriptions, Table, Segmented } from 'antd';
import {
  DesktopOutlined,
  HddOutlined,
  CloudServerOutlined,
  WifiOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import { monitorApi } from '../services/api';
import type { MonitorSnapshot, HistoryPoint } from '../types';
import { formatBytes, formatUptime } from '../utils/format';
import { getPercentColor } from '../utils/status';
import { useWebSocket } from '../hooks/useWebSocket';

const MAX_HISTORY_POINTS = 360;

export default function Dashboard() {
  const [stats, setStats] = useState<MonitorSnapshot | null>(null);
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<string>('1h');

  const appendToHistory = useCallback((point: HistoryPoint) => {
    setHistory(prev => {
      const next = [...prev, point];
      if (next.length > MAX_HISTORY_POINTS) {
        return next.slice(next.length - MAX_HISTORY_POINTS);
      }
      return next;
    });
  }, []);

  // Fetch initial data — stats first (non-blocking), then history.
  // History (monitor/history) scans a large table and can take hundreds of
  // ms on a high-latency link; rendering immediately from stats + live
  // WebSocket keeps the post-login Dashboard feeling instant.
  useEffect(() => {
    // Stats: fast, blocks initial render so cards show real data immediately.
    monitorApi.getStats()
      .then(res => {
        setStats(res.data.data);
        setLoading(false);
      })
      .catch(() => setLoading(false));

    // History: async, does not block the loading state.
    monitorApi.getHistory()
      .then(res => setHistory(res.data.data.points || []))
      .catch(() => {});
  }, []);

  // Fetch history when time range changes
  useEffect(() => {
    const hours: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24 };
    const h = hours[timeRange] || 1;
    const end = new Date();
    const start = new Date(end.getTime() - h * 3600 * 1000);

    monitorApi.getHistory(start.toISOString(), end.toISOString())
      .then(res => setHistory(res.data.data.points || []))
      .catch(console.error);
  }, [timeRange]);

  // WebSocket via shared hook (token passed via Sec-WebSocket-Protocol header)
  useWebSocket({
    path: '/ws/monitor',
    onMessage: (msg) => {
      if (msg.type === 'stats' && msg.data) {
        setStats(msg.data);
        appendToHistory(msg.data);
      }
    },
    onClose: (event) => {
      if (event.code === 4001 || event.code === 4003 || event.code === 1006) {
        const currentToken = localStorage.getItem('token');
        if (currentToken) {
          fetch('/api/auth/me', {
            headers: { 'Authorization': `Bearer ${currentToken}` },
          }).then(res => {
            if (res.status === 401) {
              localStorage.removeItem('token');
              localStorage.removeItem('user');
              window.location.href = '/login';
            }
          }).catch(() => {});
        }
        return true; // prevent auto-reconnect on auth failure
      }
      return;
    },
  });

  /** Sanitize text for use inside ECharts HTML tooltip */
  const sanitizeTooltipText = (text: string): string =>
    text.replace(/[<>&"']/g, (ch) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;', '"': '&quot;', "'": '&#39;' }[ch] || ch));

  // 时间轴标签格式：1h/6h 显示 HH:mm；24h 跨日，显示 MM-DD HH:mm 避免同时段重复。
  // time 类型 xAxis 的 axisLabel.formatter 传入毫秒时间戳(number)，tooltip 复用同一函数。
  const formatChartTime = useCallback((ts: number | string): string => {
    const d = new Date(ts);
    if (timeRange === '24h') {
      return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    }
    return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
  }, [timeRange]);

  // 检测数据点间空隙（超过 10 分钟视为采集中断），插入 null 断点避免线段跨越无数据时段。
  // 阈值需大于降采样后的最大间隔（24h/360≈4min），否则降采样点之间也会被误判为断点。
  const MAX_MONITOR_GAP_MS = 10 * 60 * 1000;
  const insertNullGaps = (data: any[]) => {
    if (data.length < 2) return data;
    const result: (string | number | null)[][] = [data[0]];
    for (let i = 1; i < data.length; i++) {
      const prevTime = new Date(data[i-1][0]).getTime();
      const currTime = new Date(data[i][0]).getTime();
      if (currTime - prevTime > MAX_MONITOR_GAP_MS) {
        result.push([new Date(prevTime + 1000).toISOString(), null]);
      }
      result.push(data[i]);
    }
    return result;
  };

  const cpuChartOption = useMemo(() => ({
    animation: false,
    title: { text: 'CPU 使用率', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: Array<{axisValue?: string; seriesName?: string; marker?: string; value?: any}>) => {
        const ts = params[0]?.axisValue || (Array.isArray(params[0]?.value) ? params[0].value[0] : '');
        const time = sanitizeTooltipText(ts ? formatChartTime(ts) : '');
        let html = `<div>${time}</div>`;
        params.forEach((p: any) => {
          const name = sanitizeTooltipText(p.seriesName || '');
          const v = Array.isArray(p.value) ? p.value[1] : p.value;
          html += `<div>${p.marker} ${name}: ${v}%</div>`;
        });
        return html;
      },
    },
    grid: { top: 40, right: 20, bottom: 30, left: 50 },
    xAxis: {
      type: 'time' as const,
      axisLabel: { fontSize: 11, hideOverlap: true, formatter: (v: number) => formatChartTime(v) },
    },
    yAxis: {
      type: 'value' as const,
      min: 0,
      max: 100,
      axisLabel: { formatter: '{value}%' },
    },
    series: [{
      name: 'CPU',
      type: 'line',
      data: insertNullGaps(history.map(p => [p.timestamp, p.cpu.usage_percent])),
      smooth: true,
      areaStyle: { opacity: 0.3 },
      showSymbol: false,
      connectNulls: false,
    }],
  }), [history, formatChartTime]);

  const memChartOption = useMemo(() => ({
    animation: false,
    title: { text: '内存使用率', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: Array<{axisValue?: string; seriesName?: string; marker?: string; value?: any}>) => {
        const ts = params[0]?.axisValue || (Array.isArray(params[0]?.value) ? params[0].value[0] : '');
        const time = sanitizeTooltipText(ts ? formatChartTime(ts) : '');
        let html = `<div>${time}</div>`;
        params.forEach((p: any) => {
          const name = sanitizeTooltipText(p.seriesName || '');
          const v = Array.isArray(p.value) ? p.value[1] : p.value;
          html += `<div>${p.marker} ${name}: ${v}%</div>`;
        });
        return html;
      },
    },
    grid: { top: 40, right: 20, bottom: 30, left: 50 },
    xAxis: {
      type: 'time' as const,
      axisLabel: { fontSize: 11, hideOverlap: true, formatter: (v: number) => formatChartTime(v) },
    },
    yAxis: {
      type: 'value' as const,
      min: 0,
      max: 100,
      axisLabel: { formatter: '{value}%' },
    },
    series: [{
      name: '内存',
      type: 'line',
      data: insertNullGaps(history.map(p => [p.timestamp, p.memory.usage_percent])),
      smooth: true,
      areaStyle: { opacity: 0.3 },
      itemStyle: { color: '#52c41a' },
      showSymbol: false,
      connectNulls: false,
    }],
  }), [history, formatChartTime]);

  const netChartOption = useMemo(() => ({
    animation: false,
    title: { text: '网络流量', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: Array<{axisValue?: string; seriesName?: string; marker?: string; value?: any}>) => {
        const ts = params[0]?.axisValue || (Array.isArray(params[0]?.value) ? params[0].value[0] : '');
        const time = sanitizeTooltipText(ts ? formatChartTime(ts) : '');
        let html = `<div>${time}</div>`;
        params.forEach((p: any) => {
          const name = sanitizeTooltipText(p.seriesName || '');
          const v = Array.isArray(p.value) ? p.value[1] : p.value;
          html += `<div>${p.marker} ${name}: ${formatBytes(v)}/s</div>`;
        });
        return html;
      },
    },
    legend: { bottom: 0, data: ['上传', '下载'] },
    grid: { top: 40, right: 20, bottom: 40, left: 60 },
    xAxis: {
      type: 'time' as const,
      axisLabel: { fontSize: 11, hideOverlap: true, formatter: (v: number) => formatChartTime(v) },
    },
    yAxis: {
      type: 'value' as const,
      axisLabel: {
        formatter: (v: number) => formatBytes(v) + '/s',
      },
    },
    series: [
      {
        name: '上传',
        type: 'line',
        data: insertNullGaps(history.map(p => [p.timestamp, p.network.bytes_sent])),
        smooth: true,
        showSymbol: false,
        connectNulls: false,
        itemStyle: { color: '#faad14' },
        areaStyle: { opacity: 0.2 },
      },
      {
        name: '下载',
        type: 'line',
        data: insertNullGaps(history.map(p => [p.timestamp, p.network.bytes_recv])),
        smooth: true,
        showSymbol: false,
        itemStyle: { color: '#1890ff' },
        areaStyle: { opacity: 0.2 },
      },
    ],
  }), [history, formatChartTime]);

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    );
  }

  const sys = stats?.system;
  const swap = stats?.swap;
  const partitions = stats?.partitions || [];

  return (
    <div>
      {/* 指标卡片 */}
      <Row gutter={[16, 16]}>
        <Col style={{ flex: '1 1 180px', minWidth: 0 }}>
          <Card>
            <Statistic
              title="CPU 使用率"
              value={stats?.cpu.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<DesktopOutlined />}
              styles={{ content: { color: getPercentColor(stats?.cpu.usage_percent || 0) } }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              负载: {stats?.cpu.load_1m?.toFixed(2) || '-'} / {stats?.cpu.load_5m?.toFixed(2) || '-'} / {stats?.cpu.load_15m?.toFixed(2) || '-'}
            </div>
          </Card>
        </Col>

        <Col style={{ flex: '1 1 180px', minWidth: 0 }}>
          <Card>
            <Statistic
              title="内存使用率"
              value={stats?.memory.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<HddOutlined />}
              styles={{ content: { color: getPercentColor(stats?.memory.usage_percent || 0) } }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {formatBytes(stats?.memory.used_bytes || 0)} / {formatBytes(stats?.memory.total_bytes || 0)}
            </div>
          </Card>
        </Col>

        <Col style={{ flex: '1 1 180px', minWidth: 0 }}>
          <Card>
            <Statistic
              title="Swap 使用率"
              value={swap?.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<SwapOutlined />}
              styles={{ content: { color: getPercentColor(swap?.usage_percent || 0) } }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {(!swap || swap.total_bytes === 0)
                ? '未配置 Swap'
                : `${formatBytes(swap.used_bytes || 0)} / ${formatBytes(swap.total_bytes || 0)}`}
            </div>
          </Card>
        </Col>

        <Col style={{ flex: '1 1 180px', minWidth: 0 }}>
          <Card>
            <Statistic
              title="磁盘使用率"
              value={stats?.disk?.[0]?.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<CloudServerOutlined />}
              styles={{ content: { color: getPercentColor(stats?.disk?.[0]?.usage_percent || 0) } }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {formatBytes(stats?.disk?.[0]?.used_bytes || 0)} / {formatBytes(stats?.disk?.[0]?.total_bytes || 0)}
            </div>
          </Card>
        </Col>

        <Col style={{ flex: '1 1 180px', minWidth: 0 }}>
          <Card>
            <Statistic
              title="网络流量"
              value={formatBytes((stats?.network.bytes_sent || 0) + (stats?.network.bytes_recv || 0))}
              prefix={<WifiOutlined />}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              ↑ {formatBytes(stats?.network.bytes_sent || 0)} / ↓ {formatBytes(stats?.network.bytes_recv || 0)}
            </div>
          </Card>
        </Col>
      </Row>

      {/* 图表 + 时间范围选择 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card
            title="监控图表"
            extra={
              <Segmented
                options={[
                  { label: '1小时', value: '1h' },
                  { label: '6小时', value: '6h' },
                  { label: '24小时', value: '24h' },
                ]}
                value={timeRange}
                onChange={(v) => setTimeRange(v as string)}
              />
            }
          >
            <Row gutter={[16, 16]}>
              <Col xs={24} lg={8}>
                <ReactECharts option={cpuChartOption} style={{ height: 280 }} />
              </Col>
              <Col xs={24} lg={8}>
                <ReactECharts option={memChartOption} style={{ height: 280 }} />
              </Col>
              <Col xs={24} lg={8}>
                <ReactECharts option={netChartOption} style={{ height: 280 }} />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>

      {/* 系统信息 + Swap + 磁盘分区 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card title="系统信息">
            <Descriptions column={2} size="small">
              <Descriptions.Item label="主机名">{sys?.hostname || '-'}</Descriptions.Item>
              <Descriptions.Item label="操作系统">{sys?.os || '-'}</Descriptions.Item>
              <Descriptions.Item label="内核版本">{sys?.kernel || '-'}</Descriptions.Item>
              <Descriptions.Item label="系统架构">{sys?.arch || '-'}</Descriptions.Item>
              <Descriptions.Item label="CPU 核数">{sys?.cpu_cores || '-'} 核</Descriptions.Item>
              <Descriptions.Item label="运行时间">{formatUptime(sys?.uptime_seconds || 0)}</Descriptions.Item>
              <Descriptions.Item label="最后更新" span={2}>
                {stats?.timestamp ? new Date(stats.timestamp).toLocaleString() : '-'}
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card title="磁盘分区">
            <Table
              dataSource={partitions}
              rowKey="mount_point"
              size="small"
              pagination={false}
              columns={[
                { title: '挂载点', dataIndex: 'mount_point', key: 'mount_point' },
                { title: '设备', dataIndex: 'device', key: 'device', ellipsis: true },
                { title: '类型', dataIndex: 'fs_type', key: 'fs_type', width: 70 },
                {
                  title: '使用率',
                  dataIndex: 'usage_percent',
                  key: 'usage_percent',
                  width: 100,
                  render: (v: number) => (
                    <span style={{ color: getPercentColor(v) }}>{v.toFixed(1)}%</span>
                  ),
                },
                {
                  title: '容量',
                  key: 'size',
                  width: 140,
                  render: (_: unknown, r: { used_bytes: number; total_bytes: number }) => `${formatBytes(r.used_bytes)} / ${formatBytes(r.total_bytes)}`,
                },
              ]}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}
