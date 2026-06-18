import { useState, useEffect, useRef, useCallback } from 'react';
import { Row, Col, Card, Statistic, Spin, Descriptions, Table, Tag, Segmented } from 'antd';
import {
  DesktopOutlined,
  HddOutlined,
  CloudServerOutlined,
  WifiOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import { monitorApi } from '../services/api';
import type { MonitorSnapshot, HistoryPoint, ProcessInfo } from '../types';

const MAX_HISTORY_POINTS = 360;

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}天${hours}小时${minutes}分钟`;
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  return `${minutes}分钟`;
}

function getStatusColor(percent: number): string {
  if (percent >= 90) return '#cf1322';
  if (percent >= 70) return '#faad14';
  return '#3f8600';
}

export default function Dashboard() {
  const [stats, setStats] = useState<MonitorSnapshot | null>(null);
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<string>('1h');
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);

  const appendToHistory = useCallback((point: HistoryPoint) => {
    setHistory(prev => {
      const next = [...prev, point];
      if (next.length > MAX_HISTORY_POINTS) {
        return next.slice(next.length - MAX_HISTORY_POINTS);
      }
      return next;
    });
  }, []);

  // Fetch initial data
  useEffect(() => {
    const fetchData = async () => {
      try {
        const [statsRes, historyRes] = await Promise.all([
          monitorApi.getStats(),
          monitorApi.getHistory(),
        ]);
        setStats(statsRes.data.data);
        setHistory(historyRes.data.data.points || []);
      } catch (error) {
        console.error('Failed to fetch monitor data:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
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

  // WebSocket with reconnect
  const connectWebSocket = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/monitor?token=${token}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'stats' && msg.data) {
          setStats(msg.data);
          appendToHistory(msg.data);
        }
      } catch (e) {
        console.error('WebSocket message error:', e);
      }
    };

    ws.onclose = () => {
      reconnectTimerRef.current = window.setTimeout(connectWebSocket, 3000);
    };

    const pingInterval = setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping' }));
      }
    }, 30000);

    return () => clearInterval(pingInterval);
  }, [appendToHistory]);

  useEffect(() => {
    const cleanup = connectWebSocket();
    return () => {
      cleanup?.();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      wsRef.current?.close();
    };
  }, [connectWebSocket]);

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    );
  }

  const cpuChartOption = {
    title: { text: 'CPU 使用率', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis' as const },
    grid: { top: 40, right: 20, bottom: 30, left: 50 },
    xAxis: {
      type: 'category' as const,
      data: history.map(p => new Date(p.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
      axisLabel: { fontSize: 11, hideOverlap: true, rotate: 0 },
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
      data: history.map(p => p.cpu.usage_percent),
      smooth: true,
      areaStyle: { opacity: 0.3 },
      showSymbol: false,
    }],
  };

  const memChartOption = {
    title: { text: '内存使用率', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis' as const },
    grid: { top: 40, right: 20, bottom: 30, left: 50 },
    xAxis: {
      type: 'category' as const,
      data: history.map(p => new Date(p.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
      axisLabel: { fontSize: 11, hideOverlap: true, rotate: 0 },
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
      data: history.map(p => p.memory.usage_percent),
      smooth: true,
      areaStyle: { opacity: 0.3 },
      itemStyle: { color: '#52c41a' },
      showSymbol: false,
    }],
  };

  const netChartOption = {
    title: { text: '网络流量', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: any) => {
        const time = params[0]?.axisValue || '';
        let html = `<div>${time}</div>`;
        params.forEach((p: any) => {
          html += `<div>${p.marker} ${p.seriesName}: ${formatBytes(p.value)}/s</div>`;
        });
        return html;
      },
    },
    legend: { bottom: 0, data: ['上传', '下载'] },
    grid: { top: 40, right: 20, bottom: 40, left: 60 },
    xAxis: {
      type: 'category' as const,
      data: history.map(p => new Date(p.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
      axisLabel: { fontSize: 11, hideOverlap: true, rotate: 0 },
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
        data: history.map(p => p.network.bytes_sent),
        smooth: true,
        showSymbol: false,
        itemStyle: { color: '#faad14' },
        areaStyle: { opacity: 0.2 },
      },
      {
        name: '下载',
        type: 'line',
        data: history.map(p => p.network.bytes_recv),
        smooth: true,
        showSymbol: false,
        itemStyle: { color: '#1890ff' },
        areaStyle: { opacity: 0.2 },
      },
    ],
  };

  const processColumns = [
    { title: 'PID', dataIndex: 'pid', key: 'pid', width: 70 },
    { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
    { title: '用户', dataIndex: 'user', key: 'user', width: 80 },
    {
      title: '内存',
      dataIndex: 'mem_percent',
      key: 'mem_percent',
      width: 100,
      render: (v: number) => `${v.toFixed(1)}%`,
      sorter: (a: ProcessInfo, b: ProcessInfo) => a.mem_percent - b.mem_percent,
    },
    {
      title: '内存用量',
      dataIndex: 'mem_bytes',
      key: 'mem_bytes',
      width: 100,
      render: (v: number) => formatBytes(v),
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 70,
      render: (v: string) => {
        const stateMap: Record<string, { text: string; color: string }> = {
          R: { text: '运行', color: 'green' },
          S: { text: '睡眠', color: 'blue' },
          D: { text: '等待', color: 'orange' },
          Z: { text: '僵尸', color: 'red' },
          T: { text: '停止', color: 'default' },
        };
        const s = stateMap[v] || { text: v, color: 'default' };
        return <Tag color={s.color}>{s.text}</Tag>;
      },
    },
  ];

  const sys = stats?.system;
  const swap = stats?.swap;
  const partitions = stats?.partitions || [];
  const topProcesses = stats?.top_process || [];

  return (
    <div>
      {/* 指标卡片 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="CPU 使用率"
              value={stats?.cpu.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<DesktopOutlined />}
              valueStyle={{ color: getStatusColor(stats?.cpu.usage_percent || 0) }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              负载: {stats?.cpu.load_1m?.toFixed(2) || '-'} / {stats?.cpu.load_5m?.toFixed(2) || '-'} / {stats?.cpu.load_15m?.toFixed(2) || '-'}
            </div>
          </Card>
        </Col>

        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="内存使用率"
              value={stats?.memory.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<HddOutlined />}
              valueStyle={{ color: getStatusColor(stats?.memory.usage_percent || 0) }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {formatBytes(stats?.memory.used_bytes || 0)} / {formatBytes(stats?.memory.total_bytes || 0)}
            </div>
          </Card>
        </Col>

        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="磁盘使用率"
              value={stats?.disk?.[0]?.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<CloudServerOutlined />}
              valueStyle={{ color: getStatusColor(stats?.disk?.[0]?.usage_percent || 0) }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {formatBytes(stats?.disk?.[0]?.used_bytes || 0)} / {formatBytes(stats?.disk?.[0]?.total_bytes || 0)}
            </div>
          </Card>
        </Col>

        <Col xs={24} sm={12} lg={6}>
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
                    <span style={{ color: getStatusColor(v) }}>{v.toFixed(1)}%</span>
                  ),
                },
                {
                  title: '容量',
                  key: 'size',
                  width: 140,
                  render: (_: any, r: any) => `${formatBytes(r.used_bytes)} / ${formatBytes(r.total_bytes)}`,
                },
              ]}
            />
          </Card>
        </Col>
      </Row>

      {/* Swap + Top 进程 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={8}>
          <Card title="Swap 交换分区">
            <Statistic
              title="使用率"
              value={swap?.usage_percent || 0}
              precision={1}
              suffix="%"
              prefix={<SwapOutlined />}
              valueStyle={{ color: getStatusColor(swap?.usage_percent || 0) }}
            />
            <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
              {formatBytes(swap?.used_bytes || 0)} / {formatBytes(swap?.total_bytes || 0)}
            </div>
            {(!swap || swap.total_bytes === 0) && (
              <div style={{ marginTop: 8, color: '#999', fontSize: 12 }}>未配置 Swap</div>
            )}
          </Card>
        </Col>

        <Col xs={24} lg={16}>
          <Card title="Top 进程 (按内存排序)">
            <Table
              dataSource={topProcesses}
              columns={processColumns}
              rowKey="pid"
              size="small"
              pagination={false}
              scroll={{ y: 240 }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}
