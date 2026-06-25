import type { ReactNode } from 'react';
import {
  Card, Breadcrumb, Button, Space, Upload, Dropdown,
} from 'antd';
import {
  HomeOutlined, SearchOutlined, UploadOutlined,
  DeleteOutlined, ReloadOutlined, SortAscendingOutlined,
} from '@ant-design/icons';

interface FileManagerHeaderProps {
  basePath: string;
  currentPath: string;
  pathParts: string[];
  canManageFiles: boolean;
  selectedKeys: string[];
  sortField: string;
  sortOrder: 'asc' | 'desc';
  onNavigate: (path: string) => void;
  onSearch: () => void;
  onMkdir: () => void;
  onUpload: (file: File) => Promise<void>;
  onBatchDelete: () => void;
  onSortFieldChange: (field: string) => void;
  onSortOrderChange: (order: 'asc' | 'desc') => void;
  onRefresh: () => void;
  children?: ReactNode;
}

export default function FileManagerHeader({
  basePath,
  currentPath,
  pathParts,
  canManageFiles,
  selectedKeys,
  sortField,
  sortOrder,
  onNavigate,
  onSearch,
  onMkdir,
  onUpload,
  onBatchDelete,
  onSortFieldChange,
  onSortOrderChange,
  onRefresh,
  children,
}: FileManagerHeaderProps) {
  return (
    <Card
      title={
        <Space>
          <Button icon={<HomeOutlined />} onClick={() => onNavigate(basePath)} />
          <Breadcrumb
            items={[
              { title: '根目录', onClick: () => onNavigate(basePath) },
              ...pathParts.map((part, index) => ({
                title: part,
                onClick: () => onNavigate(basePath + '/' + pathParts.slice(0, index + 1).join('/')),
              })),
            ]}
          />
        </Space>
      }
      extra={
        <Space wrap>
          <Button icon={<SearchOutlined />} onClick={onSearch}>搜索</Button>
          {canManageFiles && (
            <>
              <Button onClick={onMkdir}>新建文件夹</Button>
              <Upload
                showUploadList={false}
                customRequest={async ({ file, onSuccess, onError }) => {
                  try {
                    await onUpload(file as File);
                    onSuccess?.({});
                  } catch (error) {
                    onError?.(error as Error);
                  }
                }}
              >
                <Button icon={<UploadOutlined />}>上传文件</Button>
              </Upload>
              {selectedKeys.length > 0 && (
                <Button danger icon={<DeleteOutlined />} onClick={onBatchDelete}>
                  删除选中 ({selectedKeys.length})
                </Button>
              )}
            </>
          )}
          <Dropdown
            menu={{
              items: [
                { key: 'name', label: '按名称', onClick: () => onSortFieldChange('name') },
                { key: 'size', label: '按大小', onClick: () => onSortFieldChange('size') },
                { key: 'modified', label: '按时间', onClick: () => onSortFieldChange('modified') },
                { type: 'divider' as const },
                { key: 'asc', label: '升序', onClick: () => onSortOrderChange('asc') },
                { key: 'desc', label: '降序', onClick: () => onSortOrderChange('desc') },
              ],
            }}
          >
            <Button icon={<SortAscendingOutlined />}>排序</Button>
          </Dropdown>
          <Button icon={<ReloadOutlined />} onClick={onRefresh}>刷新</Button>
        </Space>
      }
    >
      {children}
    </Card>
  );
}
