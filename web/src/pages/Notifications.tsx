import { useState, useEffect, useCallback } from 'react';
import {
  Card, Table, Tag, Button, Space, Select, message, Popconfirm, Empty, Tooltip,
} from 'antd';
import {
  BellOutlined, CheckOutlined, DeleteOutlined, ReloadOutlined,
  WarningOutlined, InfoCircleOutlined, CloseCircleOutlined,
} from '@ant-design/icons';
import type { Notification } from '../types';
import { notificationApi } from '../services/api';
import { useSSE } from '../hooks/useSSE';

const LEVEL_OPTIONS = [
  { label: '全部', value: '' },
  { label: '信息', value: 'info' },
  { label: '警告', value: 'warning' },
  { label: '错误', value: 'error' },
];

const TYPE_OPTIONS = [
  { label: '全部', value: '' },
  { label: '告警', value: 'alert' },
  { label: '安全', value: 'security' },
  { label: '部署', value: 'deploy' },
  { label: '定时任务', value: 'cron' },
  { label: '系统', value: 'system' },
];

const LEVEL_COLORS: Record<string, string> = {
  info: 'blue',
  warning: 'orange',
  error: 'red',
};

const LEVEL_ICONS: Record<string, React.ReactNode> = {
  info: <InfoCircleOutlined />,
  warning: <WarningOutlined />,
  error: <CloseCircleOutlined />,
};

export default function Notifications() {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [unreadCount, setUnreadCount] = useState(0);
  const [unreadFilter, setUnreadFilter] = useState(false);
  const [levelFilter, setLevelFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');

  const fetchUnreadCount = useCallback(async () => {
    try {
      const res = await notificationApi.unreadCount();
      setUnreadCount(res.data?.data?.count ?? 0);
    } catch {
      // ignore
    }
  }, []);

  const fetchNotifications = useCallback(async () => {
    setLoading(true);
    try {
      const [res] = await Promise.all([
        notificationApi.list(unreadFilter, page, pageSize, levelFilter, typeFilter),
        fetchUnreadCount(),
      ]);
      setNotifications(res.data?.data?.items || []);
      setTotal(res.data?.data?.total || 0);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取通知失败'));
    } finally {
      setLoading(false);
    }
  }, [unreadFilter, levelFilter, typeFilter, page, pageSize, fetchUnreadCount]);

  useEffect(() => {
    fetchNotifications();
  }, [fetchNotifications]);

  useSSE({
    path: '/api/notifications/stream',
    onOpen: fetchNotifications,
    events: {
      notification: (data: Notification) => {
        fetchUnreadCount();
        if (unreadFilter && data.is_read) return;
        if (levelFilter && data.level !== levelFilter) return;
        if (typeFilter && data.type !== typeFilter) return;

        setTotal(t => t + 1);
        if (page === 1) {
          setNotifications(prev => {
            const filtered = prev.filter(n => n.id !== data.id);
            return [data, ...filtered].slice(0, pageSize);
          });
        }
      },
      read: (data: { ids: number[]; all: boolean }) => {
        fetchUnreadCount();
        setNotifications(prev =>
          prev.map(n => {
            if (data.all || data.ids.includes(n.id)) {
              return { ...n, is_read: true };
            }
            return n;
          })
        );
      },
    },
  });

  const handleMarkRead = async (id: number) => {
    try {
      await notificationApi.markAsRead(id);
      setNotifications(prev =>
        prev.map(n => n.id === id ? { ...n, is_read: true } : n)
      );
      fetchUnreadCount();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '标记失败'));
    }
  };

  const handleMarkAllRead = async () => {
    try {
      await notificationApi.markAllAsRead();
      setNotifications(prev => prev.map(n => ({ ...n, is_read: true })));
      setUnreadCount(0);
      message.success('全部已读');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '标记失败'));
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await notificationApi.delete(id);
      setNotifications(prev => prev.filter(n => n.id !== id));
      setTotal(t => Math.max(0, t - 1));
      fetchUnreadCount();
      message.success('已删除');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const columns = [
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (level: string) => (
        <Tag color={LEVEL_COLORS[level] || 'default'} icon={LEVEL_ICONS[level]}>
          {level === 'info' ? '信息' : level === 'warning' ? '警告' : level === 'error' ? '错误' : level}
        </Tag>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => {
        const typeLabel: Record<string, string> = {
          alert: '告警', security: '安全', deploy: '部署',
          cron: '定时任务', system: '系统', update: '更新',
        };
        return <Tag>{typeLabel[type] || type}</Tag>;
      },
    },
    {
      title: '标题',
      dataIndex: 'title',
      key: 'title',
      width: 200,
      render: (title: string, record: Notification) => (
        <span style={{ fontWeight: record.is_read ? 400 : 600 }}>{title}</span>
      ),
    },
    {
      title: '内容',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
    },
    {
      title: '状态',
      key: 'status',
      width: 80,
      render: (_: unknown, record: Notification) => (
        record.is_read
          ? <Tag color="default">已读</Tag>
          : <Tag color="processing">未读</Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: unknown, record: Notification) => (
        <Space size="small">
          {!record.is_read && (
            <Tooltip title="标记已读">
              <Button
                type="link"
                size="small"
                icon={<CheckOutlined />}
                onClick={() => handleMarkRead(record.id)}
              />
            </Tooltip>
          )}
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record.id)}>
            <Tooltip title="删除">
              <Button type="link" size="small" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title={
          <Space>
            <BellOutlined />
            <span>通知中心</span>
            {unreadCount > 0 && <Tag color="processing">{unreadCount} 条未读</Tag>}
          </Space>
        }
        extra={
          <Space>
            <Select
              value={unreadFilter}
              onChange={(val) => { setUnreadFilter(val); setPage(1); }}
              style={{ width: 100 }}
              options={[
                { label: '全部', value: false },
                { label: '未读', value: true },
              ]}
            />
            <Select
              value={levelFilter}
              onChange={(val) => { setLevelFilter(val); setPage(1); }}
              style={{ width: 100 }}
              options={LEVEL_OPTIONS}
              placeholder="级别"
            />
            <Select
              value={typeFilter}
              onChange={(val) => { setTypeFilter(val); setPage(1); }}
              style={{ width: 100 }}
              options={TYPE_OPTIONS}
              placeholder="类型"
            />
            {unreadCount > 0 && (
              <Button icon={<CheckOutlined />} onClick={handleMarkAllRead}>
                全部已读
              </Button>
            )}
            <Button icon={<ReloadOutlined />} onClick={fetchNotifications}>
              刷新
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={notifications}
          rowKey="id"
          loading={loading}
          rowClassName={(record) => record.is_read ? '' : 'unread-row'}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
          locale={{ emptyText: <Empty description="暂无通知" /> }}
        />
      </Card>

      <style>{`
        .unread-row { background: #f0f5ff; }
        .unread-row:hover td { background: #e6edff !important; }
      `}</style>
    </div>
  );
}
