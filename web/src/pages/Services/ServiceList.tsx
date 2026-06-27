import {
  Table, Card, Button, Space, Tag, Tooltip, Switch,
  Input, Select, Badge, Typography,
} from 'antd';
import {
  PlayCircleOutlined,
  PauseCircleOutlined,
  ReloadOutlined,
  FileTextOutlined,
  SearchOutlined,
  InfoCircleOutlined,
  PauseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import type { Service } from '../../types';

const { Text } = Typography;

interface Stats {
  total: number;
  active: number;
  inactive: number;
  failed: number;
}

interface ServiceListProps {
  services: Service[];
  filteredServices: Service[];
  stats: Stats;
  loading: boolean;
  canManageService: boolean;
  selectedRowKeys: string[];
  autoRefresh: boolean;
  searchText: string;
  statusFilter: string;
  currentPage: number;
  pageSize: number;
  actingService: string | null;
  onRefresh: () => void;
  onAction: (name: string, action: string) => void;
  onBatchAction: (action: string) => void;
  onToggleEnabled: (record: Service, checked: boolean) => void;
  onShowDetail: (service: Service) => void;
  onShowLogs: (service: Service) => void;
  onSearchChange: (value: string) => void;
  onStatusFilterChange: (value: string) => void;
  onAutoRefreshChange: (value: boolean) => void;
  onSelectedRowKeysChange: (keys: string[]) => void;
  onPageChange: (page: number, size: number) => void;
}

export default function ServiceList({
  filteredServices,
  stats,
  loading,
  canManageService,
  selectedRowKeys,
  autoRefresh,
  searchText,
  statusFilter,
  currentPage,
  pageSize,
  actingService,
  onRefresh,
  onAction,
  onBatchAction,
  onToggleEnabled,
  onShowDetail,
  onShowLogs,
  onSearchChange,
  onStatusFilterChange,
  onAutoRefreshChange,
  onSelectedRowKeysChange,
  onPageChange,
}: ServiceListProps) {
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
            loading={actingService === record.name}
            onChange={(checked) => onToggleEnabled(record, checked)}
            disabled={!canManageService || actingService === record.name}
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
      render: (_: any, record: Service) => {
        const isActing = actingService === record.name;
        return (
        <Space size="small">
          {canManageService && (
            <>
              {record.state !== 'active' ? (
                <Button
                  type="primary"
                  size="small"
                  icon={<PlayCircleOutlined />}
                  loading={isActing}
                  disabled={isActing}
                  onClick={() => onAction(record.name, 'start')}
                >
                  启动
                </Button>
              ) : (
                <>
                  <Button
                    danger
                    size="small"
                    icon={<PauseCircleOutlined />}
                    loading={isActing}
                    disabled={isActing}
                    onClick={() => onAction(record.name, 'stop')}
                  >
                    停止
                  </Button>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    loading={isActing}
                    disabled={isActing}
                    onClick={() => onAction(record.name, 'restart')}
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
              onClick={() => onShowDetail(record)}
            />
          </Tooltip>
          <Tooltip title="查看日志">
            <Button
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => onShowLogs(record)}
            />
          </Tooltip>
        </Space>
        );
      },
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
            onClick={() => onStatusFilterChange('all')}
          >全部</Tag>
        </Badge>
        <Badge count={stats.active} showZero color="#52c41a" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'active' ? 'green' : undefined}
            onClick={() => onStatusFilterChange('active')}
          >运行中</Tag>
        </Badge>
        <Badge count={stats.inactive} showZero color="#d9d9d9" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'inactive' ? 'default' : undefined}
            onClick={() => onStatusFilterChange('inactive')}
          >已停止</Tag>
        </Badge>
        <Badge count={stats.failed} showZero color="#ff4d4f" overflowCount={999}>
          <Tag
            style={{ padding: '4px 12px', cursor: 'pointer' }}
            color={statusFilter === 'failed' ? 'red' : undefined}
            onClick={() => onStatusFilterChange('failed')}
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
              onChange={e => onSearchChange(e.target.value)}
              style={{ width: 200 }}
              allowClear
            />
            <Select
              value={statusFilter}
              onChange={(value) => onStatusFilterChange(value)}
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
                onClick={() => onAutoRefreshChange(!autoRefresh)}
              />
            </Tooltip>
            <Button onClick={onRefresh} loading={loading}>刷新</Button>
          </Space>
        }
      >
        {/* 批量操作栏 */}
        {selectedRowKeys.length > 0 && canManageService && (
          <div style={{ marginBottom: 16, padding: '8px 12px', background: '#f0f5ff', borderRadius: 4 }}>
            <Space>
              <Text>已选择 {selectedRowKeys.length} 个服务</Text>
              <Button size="small" icon={<PlayCircleOutlined />} onClick={() => onBatchAction('start')}>
                批量启动
              </Button>
              <Button size="small" danger icon={<PauseOutlined />} onClick={() => onBatchAction('stop')}>
                批量停止
              </Button>
              <Button size="small" icon={<ReloadOutlined />} onClick={() => onBatchAction('restart')}>
                批量重启
              </Button>
              <Button size="small" onClick={() => onSelectedRowKeysChange([])}>取消选择</Button>
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
            onChange: (keys) => onSelectedRowKeysChange(keys as string[]),
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
              onPageChange(page, size);
            },
          }}
          size="small"
        />
      </Card>
    </div>
  );
}
