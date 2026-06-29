import { useState, useEffect, useRef, useCallback } from 'react';
import { message } from 'antd';
import { serviceApi } from '../../services/api';
import type { Service } from '../../types';
import type { LogEntry } from './types';
import ServiceList from './ServiceList';
import ServiceDetail from './ServiceDetail';
import ServiceLogs from './ServiceLogs';

export default function Services() {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [detailsLoading, setDetailsLoading] = useState(false);
  const [selectedService, setSelectedService] = useState<Service | null>(null);
  const [logsVisible, setLogsVisible] = useState(false);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailService, setDetailService] = useState<Service | null>(null);

  // 搜索和筛选
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');

  // 分页
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);

  // 自动刷新
  const [autoRefresh, setAutoRefresh] = useState(false);
  const autoRefreshRef = useRef<number | null>(null);

  // 批量选择
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);

  // 行级操作 loading（互斥：同一行只能有一个操作在进行）
  const [actingService, setActingService] = useState<string | null>(null);

  // 实时日志
  const wsRef = useRef<WebSocket | null>(null);
  const logContainerRef = useRef<HTMLDivElement>(null);

  // 获取当前页服务的详细信息（PID、内存、开机自启）
  const fetchPageDetails = useCallback(async (names: string[]) => {
    if (names.length === 0) return;
    setDetailsLoading(true);
    try {
      const res = await serviceApi.getDetails(names);
      const details: Service[] = res.data.data || [];
      setServices(prev => prev.map(s => {
        const detail = details.find(d => d.name === s.name);
        return detail ? { ...s, ...detail } : s;
      }));
    } catch {
      // ignore detail fetch errors
    } finally {
      setDetailsLoading(false);
    }
  }, []);

  const fetchServices = useCallback(async () => {
    setLoading(true);
    try {
      const res = await serviceApi.list();
      const data = res.data.data || [];
      setServices(data);
    } catch (error) {
      console.error('Failed to fetch services:', error);
    } finally {
      setLoading(false);
    }
  }, []);

  // 过滤后的服务列表（用于渲染和计算当前页名称）
  const filteredServices = services.filter(s => {
    const matchSearch = !searchText ||
      s.name.toLowerCase().includes(searchText.toLowerCase()) ||
      s.description?.toLowerCase().includes(searchText.toLowerCase());
    const matchStatus = statusFilter === 'all' || s.state === statusFilter;
    return matchSearch && matchStatus;
  });

  // 当前页的服务名称和数据
  const pageStart = (currentPage - 1) * pageSize;
  const currentPageNames = filteredServices.slice(pageStart, pageStart + pageSize).map(s => s.name);
  const paginatedServices = filteredServices.slice(pageStart, pageStart + pageSize);

  // 页码/筛选变化时重新获取详情
  const currentPageNamesRef = useRef<string[]>([]);
  useEffect(() => {
    const names = currentPageNames;
    if (names.length > 0 && JSON.stringify(names) !== JSON.stringify(currentPageNamesRef.current)) {
      currentPageNamesRef.current = names;
      fetchPageDetails(names);
    }
  }, [currentPageNames, fetchPageDetails]);

  // 列表加载后获取当前页详情
  useEffect(() => {
    if (services.length > 0 && currentPageNames.length > 0) {
      fetchPageDetails(currentPageNames);
    }
  }, [services.length]);

  useEffect(() => {
    fetchServices();
    return () => {
      if (autoRefreshRef.current) clearInterval(autoRefreshRef.current);
      wsRef.current?.close();
    };
  }, [fetchServices]);

  // 自动刷新
  useEffect(() => {
    if (autoRefresh) {
      autoRefreshRef.current = window.setInterval(fetchServices, 5000);
    }
    return () => {
      if (autoRefreshRef.current) {
        clearInterval(autoRefreshRef.current);
        autoRefreshRef.current = null;
      }
    };
  }, [autoRefresh, fetchServices]);

  const handleAction = async (name: string, action: string) => {
    setActingService(name);
    try {
      switch (action) {
        case 'start':
          await serviceApi.start(name);
          message.success(`服务 ${name} 已启动`);
          break;
        case 'stop':
          await serviceApi.stop(name);
          message.success(`服务 ${name} 已停止`);
          break;
        case 'restart':
          await serviceApi.restart(name);
          message.success(`服务 ${name} 已重启`);
          break;
        case 'enable':
          await serviceApi.enable(name);
          message.success(`服务 ${name} 已设置开机自启`);
          break;
        case 'disable':
          await serviceApi.disable(name);
          message.success(`服务 ${name} 已取消开机自启`);
          break;
      }
      fetchServices();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    } finally {
      setActingService(null);
    }
  };

  // 批量操作
  const handleBatchAction = async (action: string) => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择服务');
      return;
    }
    const actionNames: Record<string, string> = {
      start: '启动', stop: '停止', restart: '重启',
    };
    try {
      await Promise.all(selectedRowKeys.map(name => {
        switch (action) {
          case 'start': return serviceApi.start(name);
          case 'stop': return serviceApi.stop(name);
          case 'restart': return serviceApi.restart(name);
          default: return Promise.resolve();
        }
      }));
      message.success(`已批量${actionNames[action]} ${selectedRowKeys.length} 个服务`);
      setSelectedRowKeys([]);
      fetchServices();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '批量操作失败'));
    }
  };

  const handleToggleEnabled = async (record: Service, checked: boolean) => {
    await handleAction(record.name, checked ? 'enable' : 'disable');
  };

  // 服务详情
  const showDetail = (service: Service) => {
    setDetailService(service);
    setDetailVisible(true);
  };

  // 日志（REST）
  const showLogs = async (service: Service) => {
    setSelectedService(service);
    setLogsVisible(true);
    setLogsLoading(true);
    setLogs([]);
    // 关闭之前的 WebSocket
    wsRef.current?.close();
    try {
      const res = await serviceApi.getLogs(service.name, 100);
      setLogs(res.data.data?.lines || []);
    } catch (error) {
      console.error('Failed to fetch logs:', error);
      setLogs([]);
    } finally {
      setLogsLoading(false);
    }
  };

  // 实时日志流
  const startLogStream = useCallback((serviceName: string) => {
    setLogs([]);
    wsRef.current?.close();
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/services/${serviceName}/logs`;
    // Pass token via Sec-WebSocket-Protocol header instead of URL parameter
    const ws = new WebSocket(wsUrl, ['token', token]);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'log' && msg.data) {
          const entry: LogEntry = typeof msg.data === 'string'
            ? { time: new Date().toLocaleTimeString(), message: msg.data, priority: 'info' }
            : msg.data;
          setLogs(prev => [...prev.slice(-500), entry]);
          // 自动滚动到底部
          setTimeout(() => {
            logContainerRef.current?.scrollTo(0, logContainerRef.current.scrollHeight);
          }, 50);
        }
      } catch (e) {
        // ignore
      }
    };

    ws.onerror = () => {
      message.error('日志流连接失败');
    };

    ws.onclose = () => {
      console.log('Log stream closed');
    };
  }, []);

  const handleCloseLogs = () => {
    setLogsVisible(false);
    wsRef.current?.close();
  };

  // 统计
  const stats = {
    total: services.length,
    active: services.filter(s => s.state === 'active').length,
    inactive: services.filter(s => s.state === 'inactive').length,
    failed: services.filter(s => s.state === 'failed').length,
  };

  return (
    <div>
      <ServiceList
        services={services}
        filteredServices={filteredServices}
        paginatedServices={paginatedServices}
        stats={stats}
        loading={loading || detailsLoading}
        canManageService={true}
        selectedRowKeys={selectedRowKeys}
        autoRefresh={autoRefresh}
        searchText={searchText}
        statusFilter={statusFilter}
        currentPage={currentPage}
        pageSize={pageSize}
        actingService={actingService}
        onRefresh={fetchServices}
        onAction={handleAction}
        onBatchAction={handleBatchAction}
        onToggleEnabled={handleToggleEnabled}
        onShowDetail={showDetail}
        onShowLogs={showLogs}
        onSearchChange={(value) => {
          setSearchText(value);
          setCurrentPage(1);
        }}
        onStatusFilterChange={(value) => {
          setStatusFilter(value);
          setCurrentPage(1);
        }}
        onAutoRefreshChange={setAutoRefresh}
        onSelectedRowKeysChange={setSelectedRowKeys}
        onPageChange={(page, size) => {
          setCurrentPage(page);
          setPageSize(size);
        }}
      />

      <ServiceDetail
        visible={detailVisible}
        service={detailService}
        onClose={() => setDetailVisible(false)}
      />

      <ServiceLogs
        visible={logsVisible}
        service={selectedService}
        logs={logs}
        loading={logsLoading}
        onClose={handleCloseLogs}
        onStartStream={startLogStream}
        logContainerRef={logContainerRef}
      />
    </div>
  );
}
