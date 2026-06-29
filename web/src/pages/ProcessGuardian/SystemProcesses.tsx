import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, Table, Tag, Space, Input, Select, Button, Tooltip, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { SystemProcess } from '../../types';
import { systemProcessApi } from '../../services/api';

const { Text } = Typography;
const { Search } = Input;

// --- Constants ---
const REFRESH_INTERVAL_MS = 15000;
const SEARCH_DEBOUNCE_MS = 500;
const PROCESS_PAGE_SIZES = [20, 50, 100, 200] as const;
const DEFAULT_PROCESS_PAGE_SIZE = 50;
const FETCH_PROCESS_LIMIT = 100;
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

export default function SystemProcesses() {
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
    const timer = setInterval(fetchProcesses, REFRESH_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [fetchProcesses]);

  const formatMemory = (mb: number): string => {
    if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
    return `${mb.toFixed(1)}MB`;
  };

  const totalPages = Math.ceil(allProcesses.length / pageSize);

  return (
    <Card size="small" extra={
      <Space size="small">
        <Search placeholder="搜索进程名/命令" allowClear value={searchText}
          onChange={(e) => handleSearchChange(e.target.value)} onSearch={handleSearchChange}
          style={{ width: SEARCH_WIDTH }} size="small" />
        <Select value={sortBy} onChange={setSortBy} style={{ width: SORT_WIDTH }} size="small">
          {SORT_OPTIONS.map(o => <Select.Option key={o.value} value={o.value}>{o.label}</Select.Option>)}
        </Select>
        <Button size="small" icon={<ReloadOutlined />} onClick={fetchProcesses}>刷新</Button>
      </Space>
    }>
      <Table dataSource={processes} rowKey="pid" loading={loading} size="small" pagination={false} columns={[
        { title: 'PID', dataIndex: 'pid', width: 70 },
        { title: 'PPID', dataIndex: 'ppid', width: 70 },
        { title: '名称', dataIndex: 'name', ellipsis: true },
        { title: '用户', dataIndex: 'user', width: 80 },
        { title: '状态', dataIndex: 'state', width: 70, render: (s: string) => { const c = STATE_MAP[s] || STATE_MAP['T']; return <Tag color={c?.color}>{c?.label}</Tag>; } },
        { title: 'CPU%', dataIndex: 'cpu_percent', width: 80, render: (v: number) => <Text type={v > 50 ? 'danger' : undefined}>{v.toFixed(1)}%</Text> },
        { title: '内存', dataIndex: 'memory_mb', width: 80, render: (v: number) => formatMemory(v) },
        { title: '线程', dataIndex: 'threads', width: 60 },
        { title: '启动时间', dataIndex: 'start_time', width: 100 },
        { title: '命令', dataIndex: 'command', ellipsis: true, render: (c: string) => <Tooltip title={c}><Text code style={{ fontSize: 12 }}>{c}</Text></Tooltip> },
      ]} />
      <div style={{ marginTop: 8, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space size="small">
          <Text type="secondary" style={{ fontSize: 12 }}>每页</Text>
          <Select value={pageSize} onChange={(s) => { setPageSize(s); setCurrentPage(1); }} size="small" style={{ width: PAGE_SIZE_WIDTH }}>
            {PROCESS_PAGE_SIZES.map(n => <Select.Option key={n} value={n}>{n}</Select.Option>)}
          </Select>
          <Text type="secondary" style={{ fontSize: 12 }}>条</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>共 {allProcesses.length} 条</Text>
        </Space>
        {totalPages > 1 && (
          <Space size="small">
            <Button size="small" disabled={currentPage <= 1} onClick={() => setCurrentPage(currentPage - 1)}>上一页</Button>
            <Text style={{ fontSize: 12 }}>{currentPage}/{totalPages}</Text>
            <Button size="small" disabled={currentPage >= totalPages} onClick={() => setCurrentPage(currentPage + 1)}>下一页</Button>
          </Space>
        )}
      </div>
    </Card>
  );
}
