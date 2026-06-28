import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import {
  Card, Table, Tag, Space, Input, Select, Button, Statistic, Row, Col,
  Progress, message, Badge, Tooltip, Typography, Empty, Modal,
} from 'antd';
import {
  ReloadOutlined, CloudServerOutlined,
  DashboardOutlined, SettingOutlined, SafetyCertificateOutlined,
  PlayCircleOutlined, PauseCircleOutlined, SyncOutlined,
  CheckCircleOutlined, CloseCircleOutlined, FileTextOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import type { SystemProcess, SystemOverview, SystemService, ServiceWhitelistEntry } from '../../types';
import { systemProcessApi } from '../../services/api';

const { Text } = Typography;
const { Search } = Input;

// --- Constants ---
const REFRESH_INTERVAL_MS = 15000;
const SEARCH_DEBOUNCE_MS = 500;
const PROCESS_PAGE_SIZES = [20, 50, 100, 200] as const;
const SERVICE_PAGE_SIZES = [10, 20, 50] as const;
const DEFAULT_PROCESS_PAGE_SIZE = 50;
const DEFAULT_SERVICE_PAGE_SIZE = 20;
const FETCH_PROCESS_LIMIT = 100;
const SERVICE_LOG_LINES = 200;
const SORT_OPTIONS = [
  { value: 'cpu', label: 'CPU' },
  { value: 'memory', label: '内存' },
  { value: 'pid', label: 'PID' },
  { value: 'name', label: '名称' },
] as const;
const SEARCH_WIDTH = 200;
const SORT_WIDTH = 100;
const PAGE_SIZE_WIDTH = 80;
const OVERVIEW_COL_SPAN = 6;
const TOP_TABLE_SPAN = 12;

const STATE_MAP: Record<string, { color: string; label: string }> = {
  R: { color: 'green', label: '运行' },
  S: { color: 'blue', label: '睡眠' },
  D: { color: 'orange', label: '等待' },
  Z: { color: 'red', label: '僵尸' },
  T: { color: 'default', label: '停止' },
};

const SERVICE_STATE_MAP: Record<string, { color: string; label: string }> = {
  active: { color: 'green', label: '运行中' },
  inactive: { color: 'default', label: '已停止' },
  failed: { color: 'red', label: '失败' },
  activating: { color: 'processing', label: '启动中' },
  deactivating: { color: 'warning', label: '停止中' },
};

export default function SystemMonitor() {
  const [overview, setOverview] = useState<SystemOverview | null>(null);

  const [allProcesses, setAllProcesses] = useState<SystemProcess[]>([]);
  const [services, setServices] = useState<SystemService[]>([]);
  const [whitelist, setWhitelist] = useState<ServiceWhitelistEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageSize, setPageSize] = useState(DEFAULT_PROCESS_PAGE_SIZE);
  const [currentPage, setCurrentPage] = useState(1);
  const [svcPageSize, setSvcPageSize] = useState(DEFAULT_SERVICE_PAGE_SIZE);
  const [svcPage, setSvcPage] = useState(1);
  const [searchText, setSearchText] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearchChange = useCallback((value: string) => {
    setSearchText(value);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => setDebouncedSearch(value), SEARCH_DEBOUNCE_MS);
  }, []);

  const [sortBy, setSortBy] = useState(SORT_OPTIONS[0].value);
  const [activeTab, setActiveTab] = useState<'overview' | 'processes' | 'services'>('overview');

  const [logsVisible, setLogsVisible] = useState(false);
  const [logsService, setLogsService] = useState('');
  const [logsContent, setLogsContent] = useState('');
  const [logsLoading, setLogsLoading] = useState(false);

  const [protectedConfirm, setProtectedConfirm] = useState<{
    visible: boolean;
    name: string;
    action: string;
    reason: string;
  }>({ visible: false, name: '', action: '', reason: '' });

  const fetchOverview = useCallback(async () => {
    try {
      const res = await systemProcessApi.getOverview();
      setOverview(res.data?.data || null);
    } catch { /* silent */ }
  }, []);

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

  const fetchServices = useCallback(async () => {
    try {
      const [svcRes, wlRes] = await Promise.all([
        systemProcessApi.listServices(),
        systemProcessApi.listWhitelist(),
      ]);
      setServices(svcRes.data?.data || []);
      setWhitelist(wlRes.data?.data || []);
    } catch { /* silent */ }
  }, []);

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
    fetchOverview();
    fetchProcesses();
    fetchServices();
    const timer = setInterval(() => {
      fetchOverview();
      if (activeTab === 'processes') fetchProcesses();
      if (activeTab === 'services') fetchServices();
    }, REFRESH_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [fetchOverview, fetchProcesses, fetchServices, activeTab]);

  const handleServiceAction = async (name: string, action: string, force = false) => {
    try {
      await systemProcessApi.serviceAction(name, action, force);
      message.success(`${action} ${name} 成功`);
      fetchServices();
    } catch (error: unknown) {
      const axiosErr = error as { response?: { data?: { protected?: boolean; service?: string; reason?: string } }; message?: string };
      if (axiosErr.response?.data?.protected) {
        setProtectedConfirm({
          visible: true,
          name: axiosErr.response.data.service || name,
          action,
          reason: axiosErr.response.data.reason || '受保护的服务',
        });
        return;
      }
      message.error(axiosErr.message || `${action} 失败`);
    }
  };

  const handleForceAction = async () => {
    const { name, action } = protectedConfirm;
    setProtectedConfirm({ ...protectedConfirm, visible: false });
    await handleServiceAction(name, action, true);
  };

  const handleViewLogs = async (name: string) => {
    setLogsService(name);
    setLogsVisible(true);
    setLogsLoading(true);
    try {
      const res = await systemProcessApi.getServiceLogs(name, SERVICE_LOG_LINES);
      setLogsContent(res.data?.data?.logs || '暂无日志');
    } catch (error: unknown) {
      setLogsContent(`获取日志失败: ${(error instanceof Error ? error.message : '未知错误')}`);
    }
    setLogsLoading(false);
  };

  const handleAddToWhitelist = async (name: string) => {
    try {
      await systemProcessApi.addToWhitelist(name);
      message.success(`已添加 ${name} 到白名单`);
      fetchServices();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '添加失败'));
    }
  };

  const handleRemoveFromWhitelist = async (name: string) => {
    try {
      await systemProcessApi.removeFromWhitelist(name);
      message.success(`已移除 ${name}`);
      fetchServices();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '移除失败'));
    }
  };

  const formatUptime = (seconds: number): string => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}天${hours}小时`;
    if (hours > 0) return `${hours}小时${mins}分`;
    return `${mins}分`;
  };

  const formatMemory = (mb: number): string => {
    if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
    return `${mb.toFixed(1)}MB`;
  };

  // --- Pagination helpers ---
  const renderPaginationFooter = (total: number, page: number, size: number, options: readonly number[], onSizeChange: (s: number) => void) => {
    const totalPages = Math.ceil(total / size);
    return (
      <div style={{ marginTop: 8, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space size="small">
          <Text type="secondary" style={{ fontSize: 12 }}>每页</Text>
          <Select value={size} onChange={onSizeChange} size="small" style={{ width: PAGE_SIZE_WIDTH }}>
            {options.map(n => <Select.Option key={n} value={n}>{n}</Select.Option>)}
          </Select>
          <Text type="secondary" style={{ fontSize: 12 }}>条</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>共 {total} 条</Text>
        </Space>
        {totalPages > 1 && (
          <Space size="small">
            <Button size="small" disabled={page <= 1} onClick={() => (activeTab === 'processes' ? setCurrentPage : setSvcPage)(page - 1)}>上一页</Button>
            <Text style={{ fontSize: 12 }}>{page}/{totalPages}</Text>
            <Button size="small" disabled={page >= totalPages} onClick={() => (activeTab === 'processes' ? setCurrentPage : setSvcPage)(page + 1)}>下一页</Button>
          </Space>
        )}
      </div>
    );
  };

  // --- Overview tab ---
  const renderOverview = () => {
    if (!overview) return <Empty description="加载中..." />;
    return (
      <div>
        <Row gutter={16} style={{ marginBottom: 16 }}>
          <Col span={OVERVIEW_COL_SPAN}>
            <Card size="small">
              <Statistic title="CPU 使用率" value={overview.cpu_usage} precision={1} suffix="%"
                styles={{ content: { color: overview.cpu_usage > 80 ? '#cf1322' : '#3f8600' } }}
                prefix={<DashboardOutlined />} />
              <Progress percent={overview.cpu_usage} showInfo={false}
                status={overview.cpu_usage > 80 ? 'exception' : 'normal'} size="small" />
            </Card>
          </Col>
          <Col span={OVERVIEW_COL_SPAN}>
            <Card size="small">
              <Statistic title="内存使用" value={overview.memory_used}
                suffix={`/ ${overview.memory_total} MB`}
                styles={{ content: { color: overview.memory_usage > 80 ? '#cf1322' : '#3f8600' } }}
                prefix={<CloudServerOutlined />} />
              <Progress percent={overview.memory_usage} showInfo={false}
                status={overview.memory_usage > 80 ? 'exception' : 'normal'} size="small" />
            </Card>
          </Col>
          <Col span={OVERVIEW_COL_SPAN}>
            <Card size="small">
              <Statistic title="系统负载" value={overview.load_avg[0]} precision={2}
                suffix={`/ ${overview.load_avg[1]} / ${overview.load_avg[2]}`}
                prefix={<DashboardOutlined />} />
              <Text type="secondary" style={{ fontSize: 12 }}>1分 / 5分 / 15分</Text>
            </Card>
          </Col>
          <Col span={OVERVIEW_COL_SPAN}>
            <Card size="small">
              <Statistic title="运行时间" value={formatUptime(overview.uptime)} prefix={<InfoCircleOutlined />} />
              <Text type="secondary" style={{ fontSize: 12 }}>
                进程: {overview.running_procs}运行 / {overview.total_procs}总数
              </Text>
            </Card>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={TOP_TABLE_SPAN}>
            <Card title="CPU TOP 5" size="small">
              <Table dataSource={overview.top_cpu} rowKey="pid" size="small" pagination={false} columns={[
                { title: 'PID', dataIndex: 'pid', width: 70 },
                { title: '名称', dataIndex: 'name', ellipsis: true },
                { title: '用户', dataIndex: 'user', width: 80 },
                { title: 'CPU%', dataIndex: 'cpu_percent', width: 80, render: (v: number) => <Text type={v > 50 ? 'danger' : undefined}>{v.toFixed(1)}%</Text> },
                { title: '内存', dataIndex: 'memory_mb', width: 80, render: (v: number) => formatMemory(v) },
              ]} />
            </Card>
          </Col>
          <Col span={TOP_TABLE_SPAN}>
            <Card title="内存 TOP 5" size="small">
              <Table dataSource={overview.top_mem} rowKey="pid" size="small" pagination={false} columns={[
                { title: 'PID', dataIndex: 'pid', width: 70 },
                { title: '名称', dataIndex: 'name', ellipsis: true },
                { title: '用户', dataIndex: 'user', width: 80 },
                { title: 'CPU%', dataIndex: 'cpu_percent', width: 80, render: (v: number) => `${v.toFixed(1)}%` },
                { title: '内存', dataIndex: 'memory_mb', width: 80, render: (v: number) => <Text type={v > 500 ? 'danger' : undefined}>{formatMemory(v)}</Text> },
              ]} />
            </Card>
          </Col>
        </Row>
      </div>
    );
  };

  // --- Processes tab ---
  const renderProcesses = () => (
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
      {renderPaginationFooter(allProcesses.length, currentPage, pageSize, PROCESS_PAGE_SIZES, (s) => { setPageSize(s); setCurrentPage(1); })}
    </Card>
  );

  // --- Services tab ---
  const renderServices = () => {
    const isWhitelistEnabled = whitelist.length > 0;
    const svcTotal = services.length;
    const svcStart = (svcPage - 1) * svcPageSize;
    const pagedServices = services.slice(svcStart, svcStart + svcPageSize);

    return (
      <div>
        {isWhitelistEnabled && (
          <Card size="small" style={{ marginBottom: 16 }}>
            <Space>
              <SafetyCertificateOutlined />
              <Text>服务白名单已启用，仅管理白名单中的服务</Text>
              {whitelist.map(w => (
                <Tag key={w.id} closable onClose={() => handleRemoveFromWhitelist(w.name)}>{w.name}</Tag>
              ))}
            </Space>
          </Card>
        )}
        <Card size="small" extra={
          <Space size="small">
            <Button size="small" icon={<ReloadOutlined />} onClick={fetchServices}>刷新</Button>
          </Space>
        }>
          <Table dataSource={pagedServices} rowKey="name" size="small" pagination={false} columns={[
            { title: '服务名', dataIndex: 'name', ellipsis: true },
            { title: '描述', dataIndex: 'description', ellipsis: true },
            { title: '状态', dataIndex: 'active_state', width: 100, render: (s: string) => { const c = SERVICE_STATE_MAP[s] || SERVICE_STATE_MAP['inactive']; return <Badge status={c?.color as any} text={c?.label} />; } },
            { title: '子状态', dataIndex: 'sub_state', width: 80, render: (v: string) => <Tag>{v}</Tag> },
            { title: 'PID', dataIndex: 'pid', width: 70, render: (pid: number) => pid > 0 ? pid : '-' },
            { title: '开机启动', dataIndex: 'enabled', width: 80, render: (e: boolean) => e ? <Tag color="green">已启用</Tag> : <Tag>未启用</Tag> },
            { title: '操作', width: 240, render: (_: unknown, r: SystemService) => {
              const isActive = r.active_state === 'active';
              const inWl = !isWhitelistEnabled || whitelist.some(w => w.name === r.name);
              return (
                <Space size="small">
                  {inWl && (<>
                    {!isActive && <Tooltip title="启动"><Button type="link" size="small" icon={<PlayCircleOutlined />} onClick={() => handleServiceAction(r.name, 'start')} /></Tooltip>}
                    {isActive && <Tooltip title="停止"><Button type="link" size="small" danger icon={<PauseCircleOutlined />} onClick={() => handleServiceAction(r.name, 'stop')} /></Tooltip>}
                    <Tooltip title="重启"><Button type="link" size="small" icon={<SyncOutlined />} onClick={() => handleServiceAction(r.name, 'restart')} /></Tooltip>
                    {!r.enabled && <Tooltip title="启用开机启动"><Button type="link" size="small" icon={<CheckCircleOutlined />} onClick={() => handleServiceAction(r.name, 'enable')} /></Tooltip>}
                    {r.enabled && <Tooltip title="禁用开机启动"><Button type="link" size="small" icon={<CloseCircleOutlined />} onClick={() => handleServiceAction(r.name, 'disable')} /></Tooltip>}
                    <Tooltip title="日志"><Button type="link" size="small" icon={<FileTextOutlined />} onClick={() => handleViewLogs(r.name)} /></Tooltip>
                  </>)}
                  {!inWl && <Tooltip title="添加到白名单"><Button type="link" size="small" onClick={() => handleAddToWhitelist(r.name)}>加入白名单</Button></Tooltip>}
                </Space>
              );
            }},
          ]} />
          {renderPaginationFooter(svcTotal, svcPage, svcPageSize, SERVICE_PAGE_SIZES, (s) => { setSvcPageSize(s); setSvcPage(1); })}
        </Card>
      </div>
    );
  };

  return (
    <div>
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space>
          <Button type={activeTab === 'overview' ? 'primary' : 'default'} icon={<DashboardOutlined />} onClick={() => setActiveTab('overview')}>系统概览</Button>
          <Button type={activeTab === 'processes' ? 'primary' : 'default'} icon={<CloudServerOutlined />} onClick={() => setActiveTab('processes')}>系统进程</Button>
          <Button type={activeTab === 'services' ? 'primary' : 'default'} icon={<SettingOutlined />} onClick={() => setActiveTab('services')}>系统服务</Button>
        </Space>
      </Card>

      {activeTab === 'overview' && renderOverview()}
      {activeTab === 'processes' && renderProcesses()}
      {activeTab === 'services' && renderServices()}

      <Modal title={`${logsService} - 服务日志`} open={logsVisible} onCancel={() => setLogsVisible(false)} footer={null} width={800}>
        <div style={{ maxHeight: 500, overflow: 'auto', background: '#1e1e1e', borderRadius: 6, padding: 12 }}>
          {logsLoading ? <Text style={{ color: '#888' }}>加载中...</Text> : (
            <pre style={{ margin: 0, fontFamily: 'monospace', fontSize: 12, color: '#d4d4d4', whiteSpace: 'pre-wrap' }}>{logsContent}</pre>
          )}
        </div>
      </Modal>

      <Modal title="受保护的服务" open={protectedConfirm.visible}
        onCancel={() => setProtectedConfirm({ ...protectedConfirm, visible: false })}
        onOk={handleForceAction} okText="强制执行" cancelText="取消" okButtonProps={{ danger: true }}>
        <div>
          <p><strong>服务名称:</strong> {protectedConfirm.name}</p>
          <p><strong>操作:</strong> {protectedConfirm.action}</p>
          <p><strong>保护原因:</strong> {protectedConfirm.reason}</p>
          <p style={{ color: '#ff4d4f' }}>⚠️ 强制停止此服务可能导致系统不可用，请确认你了解风险！</p>
        </div>
      </Modal>
    </div>
  );
}
