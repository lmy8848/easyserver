import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, Table, Tag, Space, Input, Select, Button, Tooltip, Typography, Tabs, message, DatePicker, Row, Col } from 'antd';
import ReactECharts from 'echarts-for-react';
import { ReloadOutlined } from '@ant-design/icons';
import type { SystemProcess, MonitorSnapshot } from '../../types';
import { systemProcessApi, systemApi, monitorApi } from '../../services/api';
import { formatBytes } from '../../utils/format';
import dayjs from 'dayjs';

const { Text } = Typography;
const { Search } = Input;

// --- Constants ---
const PROCESS_REFRESH_MS = 5000;
const PORT_REFRESH_MS = 5000;
const SEARCH_DEBOUNCE_MS = 500;
const PROCESS_PAGE_SIZES = [20, 50, 100, 200] as const;
const DEFAULT_PROCESS_PAGE_SIZE = 50;
const FETCH_PROCESS_LIMIT = 100;
const ROW_SIZE_OPTIONS = [
  { label: '宽松', value: 'large' as const },
  { label: '标准', value: 'medium' as const },
  { label: '紧凑', value: 'small' as const },
];
const SORT_OPTIONS = [
  { value: 'cpu', label: 'CPU' },
  { value: 'memory', label: '内存' },
  { value: 'pid', label: 'PID' },
  { value: 'name', label: '名称' },
] as const;
const SEARCH_WIDTH = 200;
const SORT_WIDTH = 100;

const STATE_MAP: Record<string, { color: string; label: string }> = {
  R: { color: 'green', label: '运行' },
  S: { color: 'blue', label: '睡眠' },
  D: { color: 'orange', label: '等待' },
  Z: { color: 'red', label: '僵尸' },
  T: { color: 'default', label: '停止' },
};

const PROTOCOL_COLORS: Record<string, string> = {
  tcp: 'blue',
  tcp6: 'cyan',
  udp: 'green',
  udp6: 'lime',
};

interface PortInfo {
  protocol: string;
  port: number;
  local_addr: string;
  state: string;
  pid: number;
  process_name: string;
  user: string;
}

// ============================================================
// Process tab
// ============================================================

function ProcessTab() {
  const [allProcesses, setAllProcesses] = useState<SystemProcess[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageSize, setPageSize] = useState(DEFAULT_PROCESS_PAGE_SIZE);
  const [currentPage, setCurrentPage] = useState(1);
  const [searchText, setSearchText] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearchChange = useCallback((value: string) => {
    setSearchText(value);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => setDebouncedSearch(value), SEARCH_DEBOUNCE_MS);
  }, []);

  const [sortBy, setSortBy] = useState(SORT_OPTIONS[0].value);

  const fetchProcesses = useCallback(async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    try {
      const res = await systemProcessApi.listProcesses({
        sort_by: sortBy,
        order: 'desc',
        search: debouncedSearch,
        limit: FETCH_PROCESS_LIMIT,
      });
      setAllProcesses(res.data?.data || []);
    } catch { /* silent */ }
    if (!isBackground) setLoading(false);
  }, [sortBy, debouncedSearch]);

  const [prevFilters, setPrevFilters] = useState({ debouncedSearch, sortBy });
  if (prevFilters.debouncedSearch !== debouncedSearch || prevFilters.sortBy !== sortBy) {
    setPrevFilters({ debouncedSearch, sortBy });
    setCurrentPage(1);
  }

  useEffect(() => {
    fetchProcesses(false);
    const timer = setInterval(() => fetchProcesses(true), PROCESS_REFRESH_MS);
    return () => clearInterval(timer);
  }, [fetchProcesses]);

  const formatMemory = (mb: number): string => {
    if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
    return `${mb.toFixed(1)}MB`;
  };



  const [rowSize, setRowSize] = useState<'large' | 'medium' | 'small'>('small');

  return (
    <Card extra={
      <Space>
        <Search placeholder="搜索进程名/命令" allowClear value={searchText}
          onChange={(e) => handleSearchChange(e.target.value)} onSearch={handleSearchChange}
          style={{ width: SEARCH_WIDTH }} />
        <Select value={sortBy} onChange={setSortBy} style={{ width: SORT_WIDTH }}>
          {SORT_OPTIONS.map(o => <Select.Option key={o.value} value={o.value}>{o.label}</Select.Option>)}
        </Select>
        <Tooltip title="行高">
          <Select value={rowSize} onChange={(v) => setRowSize(v as 'large' | 'medium' | 'small')} style={{ width: 80 }}>
            {ROW_SIZE_OPTIONS.map(o => <Select.Option key={o.value} value={o.value}>{o.label}</Select.Option>)}
          </Select>
        </Tooltip>
        <Button icon={<ReloadOutlined />} onClick={() => fetchProcesses(false)}>刷新</Button>
      </Space>
    }>
      <Table dataSource={allProcesses} rowKey="pid" loading={loading} size={rowSize} pagination={{
        current: currentPage,
        pageSize,
        showSizeChanger: true,
        pageSizeOptions: PROCESS_PAGE_SIZES.map(String),
        showTotal: (t) => `共 ${t} 条`,
        onChange: (p, ps) => { setCurrentPage(p); setPageSize(ps); },
      }} columns={[
        { title: 'PID', dataIndex: 'pid', width: 70 },
        { title: 'PPID', dataIndex: 'ppid', width: 70 },
        { title: '名称', dataIndex: 'name', ellipsis: true },
        { title: '用户', dataIndex: 'user', width: 80 },
        { title: '状态', dataIndex: 'state', width: 70, render: (s: string) => { const c = STATE_MAP[s] || STATE_MAP['T']; return <Tag color={c?.color}>{c?.label}</Tag>; } },
        { title: 'CPU%', dataIndex: 'cpu_percent', width: 80, render: (v: number) => <Text type={v > 50 ? 'danger' : undefined}>{v.toFixed(1)}%</Text> },
        { title: '内存', dataIndex: 'memory_mb', width: 80, render: (v: number) => formatMemory(v) },
        { title: '线程', dataIndex: 'threads', width: 60 },
        { title: '启动时间', dataIndex: 'start_time', width: 100 },
        { title: '命令', dataIndex: 'command', ellipsis: true, render: (c: string) => <Tooltip title={c}><Text code>{c}</Text></Tooltip> },
      ]} />
    </Card>
  );
}

// ============================================================
// Port tab
// ============================================================

function PortTab() {
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState('');

  const fetchPorts = useCallback(async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    try {
      const res = await systemApi.getListeningPorts();
      setPorts(res.data?.data?.ports || []);
    } catch {
      if (!isBackground) message.error('获取端口信息失败');
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchPorts(false);
    const timer = setInterval(() => fetchPorts(true), PORT_REFRESH_MS);
    return () => clearInterval(timer);
  }, [fetchPorts]);

  const filtered = useMemo(() => {
    const list = filter
      ? ports.filter(p =>
          String(p.port).includes(filter) ||
          p.process_name.toLowerCase().includes(filter.toLowerCase()) ||
          p.user.toLowerCase().includes(filter.toLowerCase()) ||
          p.protocol.toLowerCase().includes(filter.toLowerCase())
        )
      : ports;
    return list.map((item, index) => ({ ...item, _uid: `${item.protocol}-${item.port}-${item.local_addr}-${item.pid}-${index}` }));
  }, [ports, filter]);

  const [rowSize, setRowSize] = useState<'large' | 'medium' | 'small'>('small');

  return (
    <Card
      title="端口占用列表"
      extra={
        <Space>
          <Search
            placeholder="搜索端口/进程/用户..."
            allowClear
            onSearch={setFilter}
            onChange={e => setFilter(e.target.value)}
            style={{ width: 220 }}
          />
          <Tooltip title="行高">
            <Select value={rowSize} onChange={(v) => setRowSize(v as 'large' | 'medium' | 'small')} style={{ width: 80 }}>
              {ROW_SIZE_OPTIONS.map(o => <Select.Option key={o.value} value={o.value}>{o.label}</Select.Option>)}
            </Select>
          </Tooltip>
          <Button icon={<ReloadOutlined />} onClick={() => fetchPorts(false)}>刷新</Button>
        </Space>
      }
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
        共 {filtered.length} 个监听端口（每 {PORT_REFRESH_MS / 1000} 秒自动刷新）
      </Text>
      <Table
        dataSource={filtered}
        rowKey="_uid"
        loading={loading}
        size={rowSize}
        pagination={{ pageSize: 50, showTotal: (t) => `共 ${t} 条` }}
        columns={[
          {
            title: '协议', dataIndex: 'protocol', width: 80,
            render: (proto: string) => <Tag color={PROTOCOL_COLORS[proto] || 'default'}>{proto.toUpperCase()}</Tag>,
          },
          { title: '端口', dataIndex: 'port', width: 100, sorter: (a: PortInfo, b: PortInfo) => a.port - b.port },
          { title: '本地地址', dataIndex: 'local_addr' },
          { title: '状态', dataIndex: 'state', width: 100, render: (state: string) => <Tag color="success">{state}</Tag> },
          { title: 'PID', dataIndex: 'pid', width: 80, render: (pid: number) => pid > 0 ? pid : '-' },
          { title: '进程', dataIndex: 'process_name', render: (name: string) => name || <Text type="secondary">-</Text> },
          {
            title: '用户', dataIndex: 'user',
            render: (user: string) => <Text type={user === 'root' ? 'danger' : undefined}>{user || '-'}</Text>,
          },
        ]}
      />
    </Card>
  );
}

// ============================================================
// History tab
// ============================================================

const { RangePicker } = DatePicker;

const formatChartTime = (ts: string | number) => {
  return dayjs(ts).format('MM-DD HH:mm');
};

const baseChartOption = {
  animation: false,
  tooltip: { trigger: 'axis' as const },
  grid: { top: 40, right: 20, bottom: 30, left: 50 },
  xAxis: {
    type: 'time' as const,
    axisLabel: { fontSize: 11, hideOverlap: true, formatter: (v: number) => formatChartTime(v) },
  },
  dataZoom: [{ type: 'inside' as const }],
};

function HistoryTab() {
  const [history, setHistory] = useState<MonitorSnapshot[]>([]);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>([dayjs().subtract(1, 'hour'), dayjs()]);
  const [cpuCores, setCpuCores] = useState<number>(1);

  useEffect(() => {
    monitorApi.getStats()
      .then(res => setCpuCores(res.data?.data?.system?.cpu_cores || 1))
      .catch(() => {});
  }, []);

  const rangePresets: { label: string; value: [dayjs.Dayjs, dayjs.Dayjs] }[] = [
    { label: '最近 1 小时', value: [dayjs().subtract(1, 'hour'), dayjs()] },
    { label: '最近 1 天', value: [dayjs().subtract(1, 'day'), dayjs()] },
    { label: '最近 7 天', value: [dayjs().subtract(7, 'day'), dayjs()] },
    { label: '最近 30 天', value: [dayjs().subtract(30, 'day'), dayjs()] },
  ];

  const fetchHistory = useCallback(async () => {
    try {
      const start = dateRange?.[0] ? dateRange[0].toISOString() : undefined;
      const end = dateRange?.[1] ? dateRange[1].toISOString() : undefined;
      const res = await monitorApi.getHistory(start, end);
      setHistory(res.data?.data?.points || []);
    } catch {
      message.error('获取历史监控数据失败');
    }
  }, [dateRange]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  const cpuChartOption = useMemo(() => ({
    ...baseChartOption,
    title: { text: 'CPU 使用率 (%)', left: 'center', textStyle: { fontSize: 14 } },
    yAxis: { type: 'value' as const, min: 0, max: 100, axisLabel: { formatter: '{value}%' } },
    series: [{
      name: 'CPU', type: 'line', smooth: true, areaStyle: { opacity: 0.3 },
      showSymbol: false, sampling: 'lttb' as const,
      data: history.map(p => [p.timestamp, p.cpu.usage_percent]),
    }]
  }), [history]);

  const memChartOption = useMemo(() => ({
    ...baseChartOption,
    title: { text: '内存使用率 (%)', left: 'center', textStyle: { fontSize: 14 } },
    yAxis: { type: 'value' as const, min: 0, max: 100, axisLabel: { formatter: '{value}%' } },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: any[]) => {
        const p = params[0];
        if (!p) return '';
        const point = history[p.dataIndex];
        if (!point) return '';
        const time = formatChartTime(point.timestamp);
        const used = formatBytes(point.memory.used_bytes);
        const total = formatBytes(point.memory.total_bytes);
        const percent = point.memory.usage_percent.toFixed(1);
        return `<div>${time}</div>
                <div>${p.marker} 内存使用率: ${percent}%</div>
                <div style="font-size:12px;color:#888;margin-top:4px;">已用: ${used} / 总量: ${total}</div>`;
      }
    },
    series: [{
      name: '内存', type: 'line', smooth: true, areaStyle: { opacity: 0.3 }, itemStyle: { color: '#52c41a' },
      showSymbol: false, sampling: 'lttb' as const,
      data: history.map(p => [p.timestamp, p.memory.usage_percent]),
    }]
  }), [history]);

  const diskChartOption = useMemo(() => ({
    ...baseChartOption,
    title: { text: '磁盘使用率 (%)', left: 'center', textStyle: { fontSize: 14 } },
    yAxis: { type: 'value' as const, min: 0, max: 100, axisLabel: { formatter: '{value}%' } },
    series: [{
      name: '磁盘', type: 'line', smooth: true, areaStyle: { opacity: 0.3 }, itemStyle: { color: '#fa8c16' },
      showSymbol: false, sampling: 'lttb' as const,
      data: history.map(p => [p.timestamp, p.disk.usage_percent]),
    }]
  }), [history]);

  const loadChartOption = useMemo(() => ({
    ...baseChartOption,
    title: { text: '系统负载', left: 'center', textStyle: { fontSize: 14 } },
    yAxis: { type: 'value' as const },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: any[]) => {
        const p = params[0];
        if (!p) return '';
        const point = history[p.dataIndex];
        if (!point) return '';
        const time = formatChartTime(point.timestamp);
        
        let html = `<div>${time}</div>`;
        params.forEach(param => {
          html += `<div>${param.marker} ${param.seriesName}: ${param.value[1].toFixed(2)}</div>`;
        });
        html += `<div style="font-size:12px;color:#888;margin-top:4px;">物理核心数: ${cpuCores} (负载 > 核心数即为高负载)</div>`;
        
        return html;
      }
    },
    series: [
      { name: '1分钟', type: 'line', smooth: true, showSymbol: false, sampling: 'lttb' as const, data: history.map(p => [p.timestamp, p.cpu.load_1m]) },
      { name: '5分钟', type: 'line', smooth: true, showSymbol: false, sampling: 'lttb' as const, data: history.map(p => [p.timestamp, p.cpu.load_5m]) },
      { name: '15分钟', type: 'line', smooth: true, showSymbol: false, sampling: 'lttb' as const, data: history.map(p => [p.timestamp, p.cpu.load_15m]) },
    ]
  }), [history, cpuCores]);

  return (
    <Card
      title="监控数据"
      extra={
        <Space>
          <RangePicker
            showTime
            presets={rangePresets}
            value={dateRange}
            onChange={(dates) => setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs] | null)}
            onOk={fetchHistory}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchHistory}>刷新</Button>
        </Space>
      }
    >
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <ReactECharts option={cpuChartOption} style={{ height: 300 }} />
        </Col>
        <Col xs={24} lg={12}>
          <ReactECharts option={memChartOption} style={{ height: 300 }} />
        </Col>
        <Col xs={24} lg={12}>
          <ReactECharts option={diskChartOption} style={{ height: 300 }} />
        </Col>
        <Col xs={24} lg={12}>
          <ReactECharts option={loadChartOption} style={{ height: 300 }} />
        </Col>
      </Row>
    </Card>
  );
}

// ============================================================
// SystemMonitor - unified monitoring page (processes + ports + history)
// ============================================================

export default function SystemMonitor() {
  return (
    <Tabs
      defaultActiveKey="history"
      items={[
        { key: 'history', label: '监控数据', children: <HistoryTab /> },
        { key: 'processes', label: '系统进程', children: <ProcessTab /> },
        { key: 'ports', label: '端口占用', children: <PortTab /> },
      ]}
    />
  );
}
