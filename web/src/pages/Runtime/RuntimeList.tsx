import { Table, Button, Space, Tag, Popconfirm } from 'antd';
import {
  SyncOutlined,
  DeleteOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import type { RuntimeEnvironment } from './types';
import { getRuntimeIcon } from './types';

interface RuntimeListProps {
  environments: RuntimeEnvironment[];
  loading: boolean;
  cleanupLoading: boolean;
  onDeleteRecord: (name: string, version: string) => void;
  onRetry: (name: string, version: string) => void;
  onViewLogs: (binding: string) => void;
  onViewCleanup: (binding: string) => void;
}

function getStatusTag(status: string) {
  switch (status) {
    case 'installed':
      return <Tag color="green">已安装</Tag>;
    case 'installing':
      return <Tag color="blue" icon={<SyncOutlined spin />}>安装中</Tag>;
    case 'uninstalling':
      return <Tag color="orange" icon={<SyncOutlined spin />}>卸载中</Tag>;
    case 'failed':
      return <Tag color="red">安装失败</Tag>;
    case 'uninstall_failed':
      return <Tag color="red">卸载失败</Tag>;
    default:
      return <Tag>{status}</Tag>;
  }
}

export default function RuntimeList({
  environments,
  loading,
  cleanupLoading,
  onDeleteRecord,
  onRetry,
  onViewLogs,
  onViewCleanup,
}: RuntimeListProps) {
  const columns = [
    {
      title: '运行环境',
      key: 'name',
      render: (_: unknown, record: RuntimeEnvironment) => (
        <Space>
          <span>{getRuntimeIcon(record.name)}</span>
          <span style={{ textTransform: 'capitalize' }}>{record.name}</span>
        </Space>
      ),
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      render: (version: string) => <span>{version}</span>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '安装路径',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: unknown, record: RuntimeEnvironment) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {record.status === 'installed' && (
            <>
              {/* 包管理按钮暂注释（后端保留）
              <Button
                type="link"
                size="small"
                icon={<AppstoreOutlined />}
                onClick={() => onOpenPackageManager(record)}
              >
                包管理
              </Button>
              */}
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => onViewCleanup(`${record.name}@${record.version}`)}
                loading={cleanupLoading}
              >
                卸载
              </Button>
            </>
          )}
          {(record.status === 'installing' || record.status === 'uninstalling') && (
            <Button
              type="link"
              size="small"
              onClick={() => onViewLogs(`${record.name}@${record.version}`)}
            >
              查看日志
            </Button>
          )}
          {record.status === 'uninstall_failed' && (
            <>
              <Button
                type="link"
                size="small"
                onClick={() => onViewLogs(`${record.name}@${record.version}`)}
              >
                查看日志
              </Button>
              <Popconfirm
                title="确定要删除此记录吗？"
                onConfirm={() => onDeleteRecord(record.name, record.version)}
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            </>
          )}
          {record.status === 'failed' && (
            <>
              <Button
                type="link"
                size="small"
                icon={<ReloadOutlined />}
                onClick={() => onRetry(record.name, record.version)}
              >
                重试
              </Button>
              <Button
                type="link"
                size="small"
                onClick={() => onViewLogs(`${record.name}@${record.version}`)}
              >
                查看日志
              </Button>
              <Popconfirm
                title="确定要删除此记录吗？"
                onConfirm={() => onDeleteRecord(record.name, record.version)}
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            </>
          )}
        </div>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={environments}
      rowKey={(r) => `${r.name}@${r.version}`}
      loading={loading}
      pagination={false}
    />
  );
}
