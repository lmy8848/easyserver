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

  // 实时日志
  const wsRef = useRef<WebSocket | null>(null);
  const logContainerRef = useRef<HTMLDivElement>(null);

  // 筛选/搜索变化时重置页码
  const [prevFilters, setPrevFilters] = useState({ statusFilter, searchText });
  if (prevFilters.statusFilter !== statusFilter || prevFilters.searchText !== searchText) {
    setPrevFilters({ statusFilter, searchText });
    setCurrentPage(1);
  }

  const fetchServices = useCallback(async () => {
    setLoading(true);
    try {
      const res = await serviceApi.list();
      setServices(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch services:', error);
    } finally {
      setLoading(false);
    }
  }, []);

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
    } catch (error: any) {
      message.error(error.message || '操作失败');
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
    } catch (error: any) {
      message.error(error.message || '批量操作失败');
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

  // 过滤服务列表
  const filteredServices = services.filter(s => {
    const matchSearch = !searchText ||
      s.name.toLowerCase().includes(searchText.toLowerCase()) ||
      s.description?.toLowerCase().includes(searchText.toLowerCase());
    const matchStatus = statusFilter === 'all' || s.state === statusFilter;
    return matchSearch && matchStatus;
  });

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
        stats={stats}
        loading={loading}
        canManageService={true}
        selectedRowKeys={selectedRowKeys}
        autoRefresh={autoRefresh}
        searchText={searchText}
        statusFilter={statusFilter}
        currentPage={currentPage}
        pageSize={pageSize}
        onRefresh={fetchServices}
        onAction={handleAction}
        onBatchAction={handleBatchAction}
        onToggleEnabled={handleToggleEnabled}
        onShowDetail={showDetail}
        onShowLogs={showLogs}
        onSearchChange={setSearchText}
        onStatusFilterChange={setStatusFilter}
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
