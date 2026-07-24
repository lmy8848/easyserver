import {
  Table, Button, Space, Dropdown,
} from 'antd';
import {
  FolderOutlined, FileOutlined, EditOutlined, DeleteOutlined,
  DownloadOutlined, CopyOutlined, FormOutlined, ScissorOutlined,
  ExpandOutlined, LockOutlined, FileImageOutlined, FileTextOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import type { FileEntry } from '../../types';
import { formatFileSize } from './types';

interface FileManagerTableProps {
  files: FileEntry[];
  loading: boolean;
  selectedKeys: string[];
  canManageFiles: boolean;
  onClick: (file: FileEntry) => void;
  onEdit: (path: string) => void;
  onRename: (path: string, name: string) => void;
  onCopyMove: (mode: 'copy' | 'move', path: string) => void;
  onDelete: (path: string, isDir: boolean) => void;
  onChmod: (path: string) => void;
  onDetails: (path: string) => void;
  onPreview: (path: string) => void;
  onDownload: (path: string) => void;
  onExtract: (path: string) => void;
  onShare: (path: string) => void;
  onSelectedKeysChange: (keys: string[]) => void;
  sortField: string;
  sortOrder: 'asc' | 'desc' | '';
  onSortChange: (field: string, order: 'asc' | 'desc' | '') => void;
}

export default function FileManagerTable({
  files,
  loading,
  selectedKeys,
  canManageFiles,
  onClick,
  onEdit,
  onRename,
  onCopyMove,
  onDelete,
  onChmod,
  onDetails,
  onPreview,
  onDownload,
  onExtract,
  onShare,
  onSelectedKeysChange,
  sortField,
  sortOrder,
  onSortChange,
}: FileManagerTableProps) {
  const getActionMenu = (record: FileEntry) => ({
    items: [
      ...(!record.is_dir ? [{
        key: 'preview',
        icon: <FileImageOutlined />,
        label: '预览',
        onClick: () => onPreview(record.path),
      }] : []),
      ...(!record.is_dir ? [{
        key: 'download',
        icon: <DownloadOutlined />,
        label: '下载',
        onClick: () => onDownload(record.path),
      }] : []),
      ...(!record.is_dir ? [{
        key: 'share',
        icon: <LinkOutlined />,
        label: '生成外链',
        onClick: () => onShare(record.path),
      }] : []),
      {
        key: 'details',
        icon: <FileTextOutlined />,
        label: '详情',
        onClick: () => onDetails(record.path),
      },
      ...(canManageFiles ? [
        { type: 'divider' as const },
        ...(!record.is_dir ? [{
          key: 'edit',
          icon: <EditOutlined />,
          label: '编辑',
          onClick: () => onEdit(record.path),
        }] : []),
        {
          key: 'rename',
          icon: <FormOutlined />,
          label: '重命名',
          onClick: () => onRename(record.path, record.name),
        },
        {
          key: 'copy',
          icon: <CopyOutlined />,
          label: '复制到...',
          onClick: () => onCopyMove('copy', record.path),
        },
        {
          key: 'move',
          icon: <ScissorOutlined />,
          label: '移动到...',
          onClick: () => onCopyMove('move', record.path),
        },
        {
          key: 'chmod',
          icon: <LockOutlined />,
          label: '修改权限',
          onClick: () => onChmod(record.path),
        },
        ...((record.name.endsWith('.zip') || record.name.endsWith('.tar.gz') || record.name.endsWith('.tgz')) ? [{
          key: 'extract',
          icon: <ExpandOutlined />,
          label: '解压到当前',
          onClick: () => onExtract(record.path),
        }] : []),
        {
          key: 'delete',
          icon: <DeleteOutlined />,
          label: '删除',
          danger: true,
          onClick: () => onDelete(record.path, record.is_dir),
        },
      ] : []),
    ],
  });

  const columns = [
    {
      title: '名称',
      key: 'name',
      sorter: true,
      sortOrder: (sortField === 'name' && sortOrder ? (sortOrder === 'asc' ? 'ascend' : 'descend') : undefined) as 'ascend' | 'descend' | undefined,
      render: (_: unknown, record: FileEntry) => (
        <Space style={{ cursor: 'pointer' }} onClick={() => onClick(record)}>
          {record.is_dir ? (
            <FolderOutlined style={{ color: '#faad14' }} />
          ) : (
            <FileOutlined style={{ color: '#1890ff' }} />
          )}
          <span style={{ color: record.is_dir ? '#1890ff' : undefined }}>
            {record.name}
          </span>
          {record.is_symlink && <span style={{ color: '#999' }}>&rarr;</span>}
        </Space>
      ),
    },
    {
      title: '大小',
      dataIndex: 'size_bytes',
      key: 'size',
      width: 100,
      sorter: true,
      sortOrder: (sortField === 'size' && sortOrder ? (sortOrder === 'asc' ? 'ascend' : 'descend') : undefined) as 'ascend' | 'descend' | undefined,
      render: (size: number, record: FileEntry) => {
        if (record.is_dir) return '-';
        return formatFileSize(size);
      },
    },
    {
      title: '权限',
      dataIndex: 'mode',
      key: 'mode',
      width: 100,
    },
    {
      title: '修改时间',
      dataIndex: 'modified_at',
      key: 'modified_at',
      width: 180,
      sorter: true,
      sortOrder: (sortField === 'modified' && sortOrder ? (sortOrder === 'asc' ? 'ascend' : 'descend') : undefined) as 'ascend' | 'descend' | undefined,
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: unknown, record: FileEntry) => (
        <Dropdown menu={getActionMenu(record)} trigger={['click']}>
          <Button type="link" size="small">操作</Button>
        </Dropdown>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={files}
      rowKey="path"
      loading={loading}
      pagination={false}
      size="small"
      rowSelection={{
        selectedRowKeys: selectedKeys,
        onChange: (keys) => onSelectedKeysChange(keys as string[]),
      }}
      onChange={(_pagination, _filters, sorter: any) => {
        if (!sorter || !sorter.order) {
          onSortChange('', '');
        } else if (sorter.columnKey || sorter.field) {
          const key = sorter.columnKey || sorter.field;
          const field = key === 'modified_at' ? 'modified' : key;
          const order = sorter.order === 'descend' ? 'desc' : 'asc';
          onSortChange(field, order);
        }
      }}
    />
  );
}
