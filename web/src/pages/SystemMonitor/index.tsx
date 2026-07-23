import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, Table, Tag, Space, Input, Select, Button, Tooltip, Typography, Tabs, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { SystemProcess } from '../../types';
import { systemProcessApi, systemApi } from '../../services/api';

const { Text } = Typography;
const { Search } = Input;

// --- Constants ---
const PROCESS_REFRESH_MS = 15000;
const PORT_REFRESH_MS = 10000;
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
const PAGE_SIZE_WIDTH = 80;

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

  const fetchProcesses = useCallback(async () => {
    setLoading(true);
    try {
      const res = await systemProcessApi.listProcesses({
        sort_by: sortBy,
        order: 'desc',
        search: debouncedSearch,
        limit: FETCH_PROCESS_LIMIT,
      });
      setAllProcesses(res.data?.data || []);
    } catch { /* silent */ }
    setLoading(false);
  }, [sortBy, debouncedSearch]);

  const processes = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return allProcesses.slice(start, start + pageSize);
  }, [allProcesses, currentPage, pageSize]);

  const [prevFilters, setPrevFilters] = useState({ debouncedSearch, sortBy });
  if (prevFilters.debouncedSearch !== debouncedSearch || prevFilters.sortBy !== sortBy) {
    setPrevFilters({ debouncedSearch, sortBy });
    setCurrentPage(1);
  }

  useEffect(() => {
    fetchProcesses();
    const timer = setInterval(fetchProcesses, PROCESS_REFRESH_MS);
    return () => clearInterval(timer);
  }, [fetchProcesses]);

  const formatMemory = (mb: number): string => {
    if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
    return `${mb.toFixed(1)}MB`;
  };

  const totalPages = Math.ceil(allProcesses.length / pageSize);

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
        <Button icon={<ReloadOutlined />} onClick={fetchProcesses}>刷新</Button>
      </Space>
    }>
      <Table dataSource={processes} rowKey="pid" loading={loading} size={rowSize} pagination={false} columns={[
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
      <div style={{ marginTop: 8, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Text type="secondary">每页</Text>
          <Select value={pageSize} onChange={(s) => { setPageSize(s); setCurrentPage(1); }} style={{ width: PAGE_SIZE_WIDTH }}>
            {PROCESS_PAGE_SIZES.map(n => <Select.Option key={n} value={n}>{n}</Select.Option>)}
          </Select>
          <Text type="secondary">条</Text>
          <Text type="secondary">共 {allProcesses.length} 条</Text>
        </Space>
        {totalPages > 1 && (
          <Space>
            <Button disabled={currentPage <= 1} onClick={() => setCurrentPage(currentPage - 1)}>上一页</Button>
            <Text>{currentPage}/{totalPages}</Text>
            <Button disabled={currentPage >= totalPages} onClick={() => setCurrentPage(currentPage + 1)}>下一页</Button>
          </Space>
        )}
      </div>
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

  const fetchPorts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await systemApi.getListeningPorts();
      setPorts(res.data?.data?.ports || []);
    } catch {
      message.error('获取端口信息失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchPorts();
    const timer = setInterval(fetchPorts, PORT_REFRESH_MS);
    return () => clearInterval(timer);
  }, [fetchPorts]);

  const filtered = filter
    ? ports.filter(p =>
        String(p.port).includes(filter) ||
        p.process_name.toLowerCase().includes(filter.toLowerCase()) ||
        p.user.toLowerCase().includes(filter.toLowerCase()) ||
        p.protocol.toLowerCase().includes(filter.toLowerCase())
      )
    : ports;

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
          <Button icon={<ReloadOutlined />} onClick={fetchPorts}>刷新</Button>
        </Space>
      }
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
        共 {filtered.length} 个监听端口（每 {PORT_REFRESH_MS / 1000} 秒自动刷新）
      </Text>
      <Table
        dataSource={filtered}
        rowKey={(r) => `${r.protocol}-${r.port}-${r.local_addr}`}
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
// SystemMonitor - unified monitoring page (processes + ports)
// ============================================================

export default function SystemMonitor() {
  return (
    <Tabs
      defaultActiveKey="processes"
      items={[
        { key: 'processes', label: '进程', children: <ProcessTab /> },
        { key: 'ports', label: '端口', children: <PortTab /> },
      ]}
    />
  );
}
