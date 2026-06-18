import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Table, Card, Button, Space, Tag, Modal, message, Tooltip, Switch,
  Input, Select, Badge, Descriptions, Typography,
} from 'antd';
import {
  PlayCircleOutlined,
  PauseCircleOutlined,
  ReloadOutlined,
  FileTextOutlined,
  SearchOutlined,
  InfoCircleOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  PauseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { serviceApi } from '../services/api';
import type { Service } from '../types';
import { useAuthStore } from '../store/useAuthStore';
import { hasPermission, PERMISSIONS } from '../utils/permissions';

const { Text } = Typography;

interface LogEntry {
  time: string;
  message: string;
  priority: string;
}

export default function Services() {
  const { user } = useAuthStore();
  const canManageService = hasPermission(user?.role, PERMISSIONS.SERVICE_MANAGE);
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
  useEffect(() => {
    setCurrentPage(1);
  }, [statusFilter, searchText]);

  useEffect(() => {
    fetchServices();
    return () => {
      if (autoRefreshRef.current) clearInterval(autoRefreshRef.current);
      wsRef.current?.close();
    };
  }, []);

  // 自动刷新
  useEffect(() => {
    if (autoRefresh) {
      autoRefreshRef.current = window.setInterval(fetchServices, 5000);
    } else {
      if (autoRefreshRef.current) {
        clearInterval(autoRefreshRef.current);
        autoRefreshRef.current = null;
      }
    }
    return () => {
      if (autoRefreshRef.current) clearInterval(autoRefreshRef.current);
    };
  }, [autoRefresh]);

  const fetchServices = async () => {
    setLoading(true);
    try {
      const res = await serviceApi.list();
      setServices(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch services:', error);
    } finally {
      setLoading(false);
    }
  };

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
    wsRef.current?.close();
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/services/${serviceName}/logs?token=${token}`;
    const ws = new WebSocket(wsUrl);
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

  const getPriorityColor = (priority: string) => {
    const colors: Record<string, string> = {
      emerg: '#ff0000', alert: '#ff4444', crit: '#ff6666',
      err: '#ff4d4f', warn: '#faad14', notice: '#1890ff',
      info: '#d4d4d4', debug: '#888888',
    };
    return colors[priority] || '#d4d4d4';
  };

  const columns = [
    {
      title: '#',
      key: 'index',
      width: 60,
      render: (_: any, __: any, index: number) => (currentPage - 1) * pageSize + index + 1,
    },
    {
      title: '服务名称',
      dataIndex: 'name',
      key: 'name',
      width: 180,
      render: (text: string) => <strong>{text}</strong>,
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 150,
      render: (state: string, record: Service) => (
        <Space>
          <Tag color={state === 'active' ? 'success' : state === 'failed' ? 'error' : 'default'}>
            {state}
          </Tag>
          <span style={{ color: '#666', fontSize: 12 }}>{record.sub_state}</span>
        </Space>
      ),
    },
    {
      title: '开机自启',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 100,
      render: (enabled: boolean, record: Service) => (
        <Tooltip title={canManageService ? (enabled ? '点击禁用开机自启' : '点击启用开机自启') : '无权限操作'}>
          <Switch
            size="small"
            checked={enabled}
            onChange={(checked) => handleToggleEnabled(record, checked)}
            disabled={!canManageService}
          />
        </Tooltip>
      ),
    },
    {
      title: 'PID',
      dataIndex: 'pid',
      key: 'pid',
      width: 70,
      render: (pid: number) => pid || '-',
    },
    {
      title: '内存',
      dataIndex: 'memory_bytes',
      key: 'memory_bytes',
      width: 90,
      render: (bytes: number) => {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / 1048576).toFixed(1)} MB`;
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 260,
      render: (_: any, record: Service) => (
        <Space size="small">
          {canManageService && (
            <>
              {record.state !== 'active' ? (
                <Button
                  type="primary"
                  size="small"
                  icon={<PlayCircleOutlined />}
                  onClick={() => handleAction(record.name, 'start')}
                >
                  启动
                </Button>
              ) : (
                <>
                  <Button
                    danger
                    size="small"
                    icon={<PauseCircleOutlined />}
                    onClick={() => handleAction(record.name, 'stop')}
                  >
                    停止
                  </Button>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    onClick={() => handleAction(record.name, 'restart')}
                  >
                    重启
                  </Button>
                </>
              )}
            </>
          )}
          <Tooltip title="查看详情">
            <Button
              size="small"
              icon={<InfoCircleOutlined />}
              onClick={() => showDetail(record)}
            />
          </Tooltip>
          <Tooltip title="查看日志">
            <Button
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => showLogs(record)}
            />
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {/* 统计卡片 */}
      <Space style={{ marginBottom: 16 }} size={12}>
        <Badge count={stats.total} showZero color="#1890ff" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'all' ? 'blue' : undefined}
            onClick={() => setStatusFilter('all')}
          >全部</Tag>
        </Badge>
        <Badge count={stats.active} showZero color="#52c41a" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'active' ? 'green' : undefined}
            onClick={() => setStatusFilter('active')}
          >运行中</Tag>
        </Badge>
        <Badge count={stats.inactive} showZero color="#d9d9d9" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'inactive' ? 'default' : undefined}
            onClick={() => setStatusFilter('inactive')}
          >已停止</Tag>
        </Badge>
        <Badge count={stats.failed} showZero color="#ff4d4f" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'failed' ? 'red' : undefined}
            onClick={() => setStatusFilter('failed')}
          >异常</Tag>
        </Badge>
      </Space>

      <Card
        title="服务管理"
        extra={
          <Space>
            <Input
              placeholder="搜索服务名/描述"
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={e => setSearchText(e.target.value)}
              style={{ width: 200 }}
              allowClear
            />
            <Select
              value={statusFilter}
              onChange={(value) => setStatusFilter(value)}
              style={{ width: 120 }}
              popupMatchSelectWidth={false}
              showSearch={false}
            >
              <Select.Option value="all">全部状态</Select.Option>
              <Select.Option value="active">运行中</Select.Option>
              <Select.Option value="inactive">已停止</Select.Option>
              <Select.Option value="failed">异常</Select.Option>
            </Select>
            <Tooltip title={autoRefresh ? '关闭自动刷新' : '开启自动刷新（5秒）'}>
              <Button
                icon={<SyncOutlined spin={autoRefresh} />}
                type={autoRefresh ? 'primary' : 'default'}
                onClick={() => setAutoRefresh(!autoRefresh)}
              />
            </Tooltip>
            <Button onClick={fetchServices} loading={loading}>刷新</Button>
          </Space>
        }
      >
        {/* 批量操作栏 */}
        {selectedRowKeys.length > 0 && canManageService && (
          <div style={{ marginBottom: 16, padding: '8px 12px', background: '#f0f5ff', borderRadius: 4 }}>
            <Space>
              <Text>已选择 {selectedRowKeys.length} 个服务</Text>
              <Button size="small" icon={<PlayCircleOutlined />} onClick={() => handleBatchAction('start')}>
                批量启动
              </Button>
              <Button size="small" danger icon={<PauseOutlined />} onClick={() => handleBatchAction('stop')}>
                批量停止
              </Button>
              <Button size="small" icon={<ReloadOutlined />} onClick={() => handleBatchAction('restart')}>
                批量重启
              </Button>
              <Button size="small" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
            </Space>
          </div>
        )}

        <Table
          columns={columns}
          dataSource={filteredServices}
          rowKey="name"
          loading={loading}
          rowSelection={{
            selectedRowKeys,
            onChange: (keys) => setSelectedRowKeys(keys as string[]),
          }}
          pagination={{
            current: currentPage,
            pageSize: pageSize,
            defaultPageSize: 50,
            showTotal: (total) => `共 ${total} 个服务`,
            showSizeChanger: { showSearch: false },
            pageSizeOptions: ['20', '50', '100'],
            showQuickJumper: false,
            onChange: (page, size) => {
              setCurrentPage(page);
              setPageSize(size);
            },
          }}
          size="small"
        />
      </Card>

      {/* 服务详情弹窗 */}
      <Modal
        title={`服务详情 - ${detailService?.name}`}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={600}
      >
        {detailService && (
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="服务名称" span={2}>
              <strong>{detailService.name}</strong>
            </Descriptions.Item>
            <Descriptions.Item label="描述" span={2}>
              {detailService.description || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={detailService.state === 'active' ? 'success' : detailService.state === 'failed' ? 'error' : 'default'}>
                {detailService.state}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="子状态">{detailService.sub_state}</Descriptions.Item>
            <Descriptions.Item label="PID">{detailService.pid || '-'}</Descriptions.Item>
            <Descriptions.Item label="内存">
              {detailService.memory_bytes
                ? detailService.memory_bytes < 1048576
                  ? `${(detailService.memory_bytes / 1024).toFixed(1)} KB`
                  : `${(detailService.memory_bytes / 1048576).toFixed(1)} MB`
                : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="开机自启">
              {detailService.enabled
                ? <Tag icon={<CheckCircleOutlined />} color="success">已启用</Tag>
                : <Tag icon={<CloseCircleOutlined />} color="default">未启用</Tag>}
            </Descriptions.Item>
            <Descriptions.Item label="配置文件路径">
              <Text copyable>/etc/systemd/system/{detailService.name}.service</Text>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>

      {/* 日志弹窗 */}
      <Modal
        title={
          <Space>
            <span>{selectedService?.name} 日志</span>
            <Button
              size="small"
              type="primary"
              icon={<SyncOutlined />}
              onClick={() => {
                setLogs([]);
                if (selectedService) startLogStream(selectedService.name);
              }}
            >
              实时流
            </Button>
          </Space>
        }
        open={logsVisible}
        onCancel={() => {
          setLogsVisible(false);
          wsRef.current?.close();
        }}
        footer={null}
        width={900}
      >
        <div
          ref={logContainerRef}
          style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 4,
            maxHeight: 500,
            overflow: 'auto',
            fontFamily: 'Consolas, Monaco, monospace',
            fontSize: 12,
            lineHeight: 1.6,
          }}
        >
          {logsLoading ? (
            <div style={{ color: '#666', textAlign: 'center', padding: 20 }}>加载中...</div>
          ) : logs.length > 0 ? (
            logs.map((log, index) => (
              <div key={index} style={{ marginBottom: 2 }}>
                {log.time && (
                  <span style={{ color: '#6a9955', marginRight: 8 }}>{log.time}</span>
                )}
                {log.priority && log.priority !== 'info' && (
                  <span style={{
                    color: getPriorityColor(log.priority),
                    marginRight: 8,
                    fontWeight: 'bold',
                    textTransform: 'uppercase',
                    fontSize: 10,
                  }}>
                    [{log.priority}]
                  </span>
                )}
                <span>{log.message}</span>
              </div>
            ))
          ) : (
            <div style={{ color: '#666', textAlign: 'center', padding: 20 }}>暂无日志</div>
          )}
        </div>
      </Modal>
    </div>
  );
}
