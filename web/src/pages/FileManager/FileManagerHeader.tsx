import { useState, useEffect, useRef, type ReactNode } from 'react';
import {
  Card, Breadcrumb, Button, Space, Upload, Dropdown, Input,
} from 'antd';
import {
  HomeOutlined, SearchOutlined, UploadOutlined,
  DeleteOutlined, ReloadOutlined, SortAscendingOutlined,
} from '@ant-design/icons';

interface FileManagerHeaderProps {
  basePath: string;
  currentPath: string;
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
  canManageFiles,
  selectedKeys,
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
  const [editing, setEditing] = useState(false);
  const [inputPath, setInputPath] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // 计算显示路径（以 basePath 为根）
  const displayPath = basePath && currentPath.startsWith(basePath)
    ? '/' + currentPath.slice(basePath.length).replace(/^\//, '')
    : currentPath;
  const pathParts = displayPath === '/' ? [] : displayPath.split('/').filter(Boolean);

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editing]);

  const handleBreadcrumbClick = () => {
    // 显示相对于 basePath 的路径
    setInputPath(displayPath);
    setEditing(true);
  };

  const handlePathSubmit = () => {
    const trimmed = inputPath.trim();
    if (!trimmed) {
      setEditing(false);
      return;
    }

    // 将显示路径转换为实际路径
    let realPath: string;
    if (trimmed === '/') {
      realPath = basePath;
    } else {
      realPath = basePath + trimmed;
    }

    if (realPath !== currentPath) {
      onNavigate(realPath);
    }
    setEditing(false);
  };

  const handleNavigate = (path: string) => {
    setEditing(false);
    onNavigate(path);
  };

  return (
    <Card
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginRight: 16 }}>
          <Button icon={<HomeOutlined />} onClick={() => handleNavigate(basePath)} />
          <div
            style={{
              flex: 1,
              minWidth: 300,
              cursor: 'text',
              padding: editing ? 0 : '4px 8px',
              borderRadius: 4,
              background: editing ? 'transparent' : 'rgba(0,0,0,0.04)',
              border: editing ? 'none' : '1px solid transparent',
            }}
            onClick={() => !editing && handleBreadcrumbClick()}
          >
            {editing ? (
              <Input
                ref={inputRef as any}
                value={inputPath}
                onChange={e => setInputPath(e.target.value)}
                onPressEnter={handlePathSubmit}
                onBlur={handlePathSubmit}
                style={{ width: '100%' }}
                placeholder="输入路径，回车跳转"
              />
            ) : (
              <Breadcrumb
                items={[
                  { title: '根目录', onClick: (e) => { e.stopPropagation(); handleNavigate(basePath); } },
                  ...pathParts.map((part, index) => ({
                    title: part,
                    onClick: (e: any) => {
                      e.stopPropagation();
                      const newPath = basePath + '/' + pathParts.slice(0, index + 1).join('/');
                      handleNavigate(newPath);
                    },
                  })),
                ]}
              />
            )}
          </div>
        </div>
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
