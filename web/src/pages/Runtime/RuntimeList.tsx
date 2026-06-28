import { Table, Button, Space, Tag, Popconfirm, Progress } from 'antd';
import {
  CheckCircleOutlined,
  SyncOutlined,
  DeleteOutlined,
  ReloadOutlined,
  AppstoreOutlined,
} from '@ant-design/icons';
import type { RuntimeEnvironment } from './types';
import { getRuntimeIcon } from './types';

interface RuntimeListProps {
  environments: RuntimeEnvironment[];
  loading: boolean;
  logsLoading: boolean;
  cleanupLoading: boolean;
  onSetDefault: (name: string, version: string) => void;
  onDeleteRecord: (name: string, version: string) => void;
  onRetry: (name: string, version: string) => void;
  onViewLogs: (id: number) => void;
  onViewCleanup: (id: number) => void;
  onOpenPackageManager: (runtime: RuntimeEnvironment) => void;
}

function getStatusTag(status: string, record: RuntimeEnvironment) {
  switch (status) {
    case 'installed':
      return <Tag color="green">已安装</Tag>;
    case 'installing':
      return (
        <Space direction="vertical" size={0}>
          <Tag color="blue" icon={<SyncOutlined spin />}>安装中</Tag>
          {record.progress > 0 && (
            <Progress percent={record.progress} size="small" style={{ width: 100 }} />
          )}
          {record.progress_step && <span style={{ fontSize: 12, color: '#999' }}>{record.progress_step}</span>}
        </Space>
      );
    case 'uninstalling':
      return (
        <Space direction="vertical" size={0}>
          <Tag color="orange" icon={<SyncOutlined spin />}>卸载中</Tag>
          {record.progress_step && <span style={{ fontSize: 12, color: '#999' }}>{record.progress_step}</span>}
        </Space>
      );
    case 'failed':
      return (
        <Space direction="vertical" size={0}>
          <Tag color="red">安装失败</Tag>
          {record.error_message && <span style={{ fontSize: 12, color: '#ff4d4f' }}>{record.error_message}</span>}
        </Space>
      );
    case 'uninstall_failed':
      // 不内联 error_message —— "卸载失败 (exit -1)，详见日志" 在表格里只是噪音，
      // 用户想看真实原因点击 查看日志 即可。
      return <Tag color="red">卸载失败</Tag>;
    default:
      return <Tag>{status}</Tag>;
  }
}

export default function RuntimeList({
  environments,
  loading,
  logsLoading,
  cleanupLoading,
  onSetDefault,
  onDeleteRecord,
  onRetry,
  onViewLogs,
  onViewCleanup,
  onOpenPackageManager,
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
      render: (version: string, record: RuntimeEnvironment) => (
        <Space>
          <span>{version}</span>
          {record.is_default && <Tag color="blue">默认</Tag>}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string, record: RuntimeEnvironment) => getStatusTag(status, record),
    },
    {
      title: '安装路径',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
    },
    {
      title: '安装时间',
      dataIndex: 'installed_at',
      key: 'installed_at',
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: unknown, record: RuntimeEnvironment) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {record.status === 'installed' && (
            <>
              {!record.is_default ? (
                <Button
                  type="link"
                  size="small"
                  icon={<CheckCircleOutlined />}
                  onClick={() => onSetDefault(record.name, record.version)}
                >
                  设为默认
                </Button>
              ) : (
                <Tag color="green">当前默认</Tag>
              )}
              <Button
                type="link"
                size="small"
                icon={<AppstoreOutlined />}
                onClick={() => onOpenPackageManager(record)}
              >
                包管理
              </Button>
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => onViewCleanup(record.id)}
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
              onClick={() => onViewLogs(record.id)}
              loading={logsLoading}
            >
              查看日志
            </Button>
          )}
          {record.status === 'uninstall_failed' && (
            <>
              <Button
                type="link"
                size="small"
                onClick={() => onViewLogs(record.id)}
                loading={logsLoading}
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
                onClick={() => onViewLogs(record.id)}
                loading={logsLoading}
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
      rowKey="id"
      loading={loading}
      pagination={false}
    />
  );
}
