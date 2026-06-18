import { useState, useEffect } from 'react';
import {
  Table, Card, Breadcrumb, Button, Space, Modal, Input, message, Upload,
  Dropdown, Tag, Row, Col,
} from 'antd';
import {
  FolderOutlined, FileOutlined, EditOutlined, DeleteOutlined,
  DownloadOutlined, UploadOutlined, ReloadOutlined, HomeOutlined,
  CopyOutlined, FormOutlined, ScissorOutlined, SearchOutlined,
  ExpandOutlined, LockOutlined,
  FileImageOutlined, FileTextOutlined,
  SortAscendingOutlined,
} from '@ant-design/icons';
import { fileApi } from '../services/api';
import type { FileEntry } from '../types';
import { useAuthStore } from '../store/useAuthStore';
import { hasPermission, PERMISSIONS } from '../utils/permissions';

export default function FileManager() {
  const { user } = useAuthStore();
  const canManageFiles = hasPermission(user?.role, PERMISSIONS.FILE_MANAGE);
  const [basePath, setBasePath] = useState<string>('');
  const [currentPath, setCurrentPath] = useState<string>('');
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);

  // 编辑文件
  const [editVisible, setEditVisible] = useState(false);
  const [editPath, setEditPath] = useState('');
  const [editContent, setEditContent] = useState('');

  // 新建文件夹
  const [mkdirVisible, setMkdirVisible] = useState(false);
  const [newDirName, setNewDirName] = useState('');

  // 重命名
  const [renameVisible, setRenameVisible] = useState(false);
  const [renamePath, setRenamePath] = useState('');
  const [renameNewName, setRenameNewName] = useState('');

  // 复制/移动
  const [copyMoveVisible, setCopyMoveVisible] = useState(false);
  const [copyMoveMode, setCopyMoveMode] = useState<'copy' | 'move'>('copy');
  const [copyMoveSource, setCopyMoveSource] = useState('');
  const [copyMoveDest, setCopyMoveDest] = useState('');

  // 搜索
  const [searchVisible, setSearchVisible] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<any[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);

  // 权限修改
  const [chmodVisible, setChmodVisible] = useState(false);
  const [chmodPath, setChmodPath] = useState('');
  const [chmodMode, setChmodMode] = useState('644');

  // 文件详情
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailData, setDetailData] = useState<any>(null);

  // 预览
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewPath, setPreviewPath] = useState('');
  const [previewType, setPreviewType] = useState('');
  const [previewContent, setPreviewContent] = useState('');

  // 排序
  const [sortField, setSortField] = useState<string>('name');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');

  // 批量选择
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);

  useEffect(() => {
    const initPath = async () => {
      try {
        const res = await fileApi.getBasePath();
        const path = res.data.data?.base_path || '/';
        setBasePath(path);
        setCurrentPath(path);
      } catch (error) {
        console.error('Failed to get base path:', error);
        setBasePath('/');
        setCurrentPath('/');
      }
    };
    initPath();
  }, []);

  useEffect(() => {
    if (currentPath && basePath) {
      fetchFiles(currentPath);
    }
  }, [currentPath, basePath]);

  const fetchFiles = async (path: string) => {
    setLoading(true);
    try {
      const res = await fileApi.list(path);
      setFiles(res.data.data?.entries || []);
    } catch (error) {
      console.error('Failed to fetch files:', error);
      message.error('获取文件列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleClick = (file: FileEntry) => {
    if (file.is_dir) {
      setCurrentPath(file.path);
    } else {
      openFile(file.path);
    }
  };

  const openFile = async (path: string) => {
    try {
      const res = await fileApi.getContent(path);
      setEditPath(path);
      setEditContent(res.data.data?.content || '');
      setEditVisible(true);
    } catch (error) {
      message.error('无法打开文件');
    }
  };

  const handleSave = async () => {
    try {
      await fileApi.saveContent(editPath, editContent);
      message.success('保存成功');
      setEditVisible(false);
    } catch (error) {
      message.error('保存失败');
    }
  };

  const handleDelete = async (path: string, isDir: boolean) => {
    const title = isDir ? '确认删除文件夹' : '确认删除文件';
    const content = isDir
      ? `确定要删除文件夹 ${path} 及其所有内容吗？此操作不可恢复！`
      : `确定要删除文件 ${path} 吗？`;

    Modal.confirm({
      title,
      content,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await fileApi.delete(path, isDir);
          message.success('删除成功');
          fetchFiles(currentPath);
        } catch (error) {
          message.error('删除失败');
        }
      },
    });
  };

  const handleMkdir = async () => {
    if (!newDirName) return;
    const dirPath = currentPath === '/' ? `/${newDirName}` : `${currentPath}/${newDirName}`;
    try {
      await fileApi.mkdir(dirPath);
      message.success('创建成功');
      setMkdirVisible(false);
      setNewDirName('');
      fetchFiles(currentPath);
    } catch (error) {
      message.error('创建失败');
    }
  };

  const handleDownload = async (path: string) => {
    try {
      const res = await fileApi.download(path);
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.download = path.split('/').pop() || 'file';
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
    } catch (error) {
      message.error('下载失败');
    }
  };

  // 重命名
  const showRename = (path: string, name: string) => {
    setRenamePath(path);
    setRenameNewName(name);
    setRenameVisible(true);
  };

  const handleRename = async () => {
    if (!renameNewName) return;
    const parentDir = renamePath.substring(0, renamePath.lastIndexOf('/')) || '/';
    const newPath = parentDir === '/' ? `/${renameNewName}` : `${parentDir}/${renameNewName}`;
    try {
      await fileApi.rename(renamePath, newPath);
      message.success('重命名成功');
      setRenameVisible(false);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('重命名失败');
    }
  };

  // 复制/移动
  const showCopyMove = (mode: 'copy' | 'move', path: string) => {
    setCopyMoveMode(mode);
    setCopyMoveSource(path);
    setCopyMoveDest(currentPath);
    setCopyMoveVisible(true);
  };

  const handleCopyMove = async () => {
    if (!copyMoveDest) return;
    try {
      if (copyMoveMode === 'copy') {
        await fileApi.copy(copyMoveSource, copyMoveDest);
        message.success('复制成功');
      } else {
        await fileApi.move([copyMoveSource], copyMoveDest);
        message.success('移动成功');
      }
      setCopyMoveVisible(false);
      fetchFiles(currentPath);
    } catch (error) {
      message.error(copyMoveMode === 'copy' ? '复制失败' : '移动失败');
    }
  };

  // 搜索
  const handleSearch = async () => {
    if (!searchQuery) return;
    setSearchLoading(true);
    try {
      const res = await fileApi.search(currentPath, searchQuery);
      setSearchResults(res.data.data || []);
    } catch (error) {
      message.error('搜索失败');
    } finally {
      setSearchLoading(false);
    }
  };

  // 解压
  const handleExtract = async (path: string) => {
    const destPath = currentPath;
    try {
      await fileApi.extract(path, destPath);
      message.success('解压成功');
      fetchFiles(currentPath);
    } catch (error) {
      message.error('解压失败');
    }
  };

  // 权限修改
  const showChmod = (path: string) => {
    setChmodPath(path);
    setChmodVisible(true);
  };

  const handleChmod = async () => {
    try {
      await fileApi.chmod(chmodPath, chmodMode);
      message.success('权限修改成功');
      setChmodVisible(false);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('权限修改失败');
    }
  };

  // 文件详情
  const showDetails = async (path: string) => {
    try {
      const res = await fileApi.getDetails(path);
      setDetailData(res.data.data);
      setDetailVisible(true);
    } catch (error) {
      message.error('获取详情失败');
    }
  };

  // 预览
  const showPreview = async (path: string) => {
    const ext = path.split('.').pop()?.toLowerCase() || '';
    const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'];
    const textExts = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'conf', 'log', 'sh', 'py', 'go', 'js', 'ts', 'html', 'css'];

    if (imageExts.includes(ext)) {
      setPreviewType('image');
      setPreviewPath(path);
    } else if (ext === 'pdf') {
      setPreviewType('pdf');
      setPreviewPath(path);
    } else if (textExts.includes(ext)) {
      try {
        const res = await fileApi.getContent(path);
        setPreviewType('text');
        setPreviewContent(res.data.data?.content || '');
        setPreviewPath(path);
      } catch (error) {
        message.error('无法预览');
        return;
      }
    } else {
      message.info('不支持预览此文件类型');
      return;
    }
    setPreviewVisible(true);
  };

  // 批量删除
  const handleBatchDelete = () => {
    if (selectedKeys.length === 0) return;
    Modal.confirm({
      title: '确认批量删除',
      content: `确定要删除选中的 ${selectedKeys.length} 个文件/文件夹吗？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await Promise.all(selectedKeys.map(path => fileApi.delete(path, true)));
          message.success(`已删除 ${selectedKeys.length} 个文件`);
          setSelectedKeys([]);
          fetchFiles(currentPath);
        } catch (error) {
          message.error('批量删除失败');
        }
      },
    });
  };

  // 排序
  const sortedFiles = [...files].sort((a, b) => {
    // 文件夹优先
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;

    let cmp = 0;
    switch (sortField) {
      case 'name':
        cmp = a.name.localeCompare(b.name);
        break;
      case 'size':
        cmp = a.size_bytes - b.size_bytes;
        break;
      case 'modified':
        cmp = new Date(a.modified_at).getTime() - new Date(b.modified_at).getTime();
        break;
    }
    return sortOrder === 'asc' ? cmp : -cmp;
  });

  // Calculate path parts relative to base path
  const relativePath = basePath && currentPath.startsWith(basePath)
    ? currentPath.slice(basePath.length).replace(/^\//, '')
    : currentPath.replace(/^\//, '');
  const pathParts = relativePath.split('/').filter(Boolean);

  // 操作菜单
  const getActionMenu = (record: FileEntry) => ({
    items: [
      // 查看类操作 - 所有角色
      ...(!record.is_dir ? [{
        key: 'preview',
        icon: <FileImageOutlined />,
        label: '预览',
        onClick: () => showPreview(record.path),
      }] : []),
      ...(!record.is_dir ? [{
        key: 'download',
        icon: <DownloadOutlined />,
        label: '下载',
        onClick: () => handleDownload(record.path),
      }] : []),
      {
        key: 'details',
        icon: <FileTextOutlined />,
        label: '详情',
        onClick: () => showDetails(record.path),
      },
      // 管理类操作 - operator 以上
      ...(canManageFiles ? [
        { type: 'divider' as const },
        ...(!record.is_dir ? [{
          key: 'edit',
          icon: <EditOutlined />,
          label: '编辑',
          onClick: () => openFile(record.path),
        }] : []),
        {
          key: 'rename',
          icon: <FormOutlined />,
          label: '重命名',
          onClick: () => showRename(record.path, record.name),
        },
        {
          key: 'copy',
          icon: <CopyOutlined />,
          label: '复制到...',
          onClick: () => showCopyMove('copy', record.path),
        },
        {
          key: 'move',
          icon: <ScissorOutlined />,
          label: '移动到...',
          onClick: () => showCopyMove('move', record.path),
        },
        {
          key: 'chmod',
          icon: <LockOutlined />,
          label: '修改权限',
          onClick: () => showChmod(record.path),
        },
        ...((record.name.endsWith('.zip') || record.name.endsWith('.tar.gz') || record.name.endsWith('.tgz')) ? [{
          key: 'extract',
          icon: <ExpandOutlined />,
          label: '解压到当前',
          onClick: () => handleExtract(record.path),
        }] : []),
        {
          key: 'delete',
          icon: <DeleteOutlined />,
          label: '删除',
          danger: true,
          onClick: () => handleDelete(record.path, record.is_dir),
        },
      ] : []),
    ],
  });

  const columns = [
    {
      title: '名称',
      key: 'name',
      sorter: true,
      render: (_: any, record: FileEntry) => (
        <Space style={{ cursor: 'pointer' }} onClick={() => handleClick(record)}>
          {record.is_dir ? (
            <FolderOutlined style={{ color: '#faad14' }} />
          ) : (
            <FileOutlined style={{ color: '#1890ff' }} />
          )}
          <span style={{ color: record.is_dir ? '#1890ff' : undefined }}>
            {record.name}
          </span>
          {record.is_symlink && <span style={{ color: '#999' }}>→</span>}
        </Space>
      ),
    },
    {
      title: '大小',
      dataIndex: 'size_bytes',
      key: 'size',
      width: 100,
      sorter: true,
      render: (size: number, record: FileEntry) => {
        if (record.is_dir) return '-';
        if (size < 1024) return `${size} B`;
        if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
        if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
        return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`;
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
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: any, record: FileEntry) => (
        <Dropdown menu={getActionMenu(record)} trigger={['click']}>
          <Button type="link" size="small">操作</Button>
        </Dropdown>
      ),
    },
  ];

  return (
    <div>
      <Card
        title={
          <Space>
            <Button icon={<HomeOutlined />} onClick={() => setCurrentPath(basePath)} />
            <Breadcrumb
              items={[
                { title: '根目录', onClick: () => setCurrentPath(basePath) },
                ...pathParts.map((part, index) => ({
                  title: part,
                  onClick: () => setCurrentPath(basePath + '/' + pathParts.slice(0, index + 1).join('/')),
                })),
              ]}
            />
          </Space>
        }
        extra={
          <Space wrap>
            <Button icon={<SearchOutlined />} onClick={() => setSearchVisible(true)}>搜索</Button>
            {canManageFiles && (
              <>
                <Button onClick={() => setMkdirVisible(true)}>新建文件夹</Button>
                <Upload
                  showUploadList={false}
                  customRequest={async ({ file, onSuccess, onError }) => {
                    try {
                      await fileApi.upload(file as File, currentPath);
                      onSuccess?.({});
                      message.success('上传成功');
                      fetchFiles(currentPath);
                    } catch (error) {
                      onError?.(error as Error);
                      message.error('上传失败');
                }
              }}
            >
              <Button icon={<UploadOutlined />}>上传文件</Button>
            </Upload>
            {selectedKeys.length > 0 && (
              <Button danger icon={<DeleteOutlined />} onClick={handleBatchDelete}>
                删除选中 ({selectedKeys.length})
              </Button>
            )}
              </>
            )}
            <Dropdown
              menu={{
                items: [
                  { key: 'name', label: '按名称', onClick: () => setSortField('name') },
                  { key: 'size', label: '按大小', onClick: () => setSortField('size') },
                  { key: 'modified', label: '按时间', onClick: () => setSortField('modified') },
                  { type: 'divider' },
                  { key: 'asc', label: '升序', onClick: () => setSortOrder('asc') },
                  { key: 'desc', label: '降序', onClick: () => setSortOrder('desc') },
                ],
              }}
            >
              <Button icon={<SortAscendingOutlined />}>排序</Button>
            </Dropdown>
            <Button icon={<ReloadOutlined />} onClick={() => fetchFiles(currentPath)}>刷新</Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={sortedFiles}
          rowKey="path"
          loading={loading}
          pagination={false}
          size="small"
          rowSelection={{
            selectedRowKeys: selectedKeys,
            onChange: (keys) => setSelectedKeys(keys as string[]),
          }}
        />
      </Card>

      {/* 编辑文件 */}
      <Modal
        title={`编辑文件: ${editPath}`}
        open={editVisible}
        onCancel={() => setEditVisible(false)}
        onOk={handleSave}
        width="80%"
        okText="保存"
        cancelText="取消"
      >
        <Input.TextArea
          value={editContent}
          onChange={(e) => setEditContent(e.target.value)}
          rows={20}
          style={{ fontFamily: 'monospace' }}
        />
      </Modal>

      {/* 新建文件夹 */}
      <Modal
        title="新建文件夹"
        open={mkdirVisible}
        onCancel={() => setMkdirVisible(false)}
        onOk={handleMkdir}
        okText="创建"
        cancelText="取消"
      >
        <Input
          placeholder="文件夹名称"
          value={newDirName}
          onChange={(e) => setNewDirName(e.target.value)}
          onPressEnter={handleMkdir}
        />
      </Modal>

      {/* 重命名 */}
      <Modal
        title={`重命名: ${renamePath.split('/').pop()}`}
        open={renameVisible}
        onCancel={() => setRenameVisible(false)}
        onOk={handleRename}
        okText="确定"
        cancelText="取消"
      >
        <Input
          placeholder="新名称"
          value={renameNewName}
          onChange={(e) => setRenameNewName(e.target.value)}
          onPressEnter={handleRename}
        />
      </Modal>

      {/* 复制/移动 */}
      <Modal
        title={copyMoveMode === 'copy' ? '复制文件' : '移动文件'}
        open={copyMoveVisible}
        onCancel={() => setCopyMoveVisible(false)}
        onOk={handleCopyMove}
        okText="确定"
        cancelText="取消"
      >
        <div style={{ marginBottom: 8 }}><strong>源文件：</strong> {copyMoveSource}</div>
        <div style={{ marginBottom: 8 }}><strong>{copyMoveMode === 'copy' ? '复制到：' : '移动到：'}</strong></div>
        <Input
          placeholder="目标路径"
          value={copyMoveDest}
          onChange={(e) => setCopyMoveDest(e.target.value)}
        />
      </Modal>

      {/* 搜索 */}
      <Modal
        title="搜索文件"
        open={searchVisible}
        onCancel={() => { setSearchVisible(false); setSearchResults([]); }}
        footer={null}
        width={800}
      >
        <Space style={{ marginBottom: 16 }}>
          <Input
            placeholder="输入文件名关键词"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onPressEnter={handleSearch}
            style={{ width: 400 }}
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch} loading={searchLoading}>
            搜索
          </Button>
        </Space>
        <Table
          dataSource={searchResults}
          rowKey="path"
          size="small"
          pagination={{ pageSize: 20 }}
          columns={[
            {
              title: '名称', dataIndex: 'name',
              render: (name: string, record: any) => (
                <Space style={{ cursor: 'pointer' }} onClick={() => {
                  if (record.is_dir) {
                    setCurrentPath(record.path);
                    setSearchVisible(false);
                  } else {
                    openFile(record.path);
                    setSearchVisible(false);
                  }
                }}>
                  {record.is_dir ? <FolderOutlined style={{ color: '#faad14' }} /> : <FileOutlined style={{ color: '#1890ff' }} />}
                  {name}
                </Space>
              ),
            },
            { title: '路径', dataIndex: 'path', ellipsis: true },
            { title: '大小', dataIndex: 'size', width: 100, render: (s: number) => `${(s / 1024).toFixed(1)} KB` },
            { title: '匹配', dataIndex: 'match', width: 80, render: (m: string) => <Tag color="blue">{m}</Tag> },
          ]}
        />
      </Modal>

      {/* 权限修改 */}
      <Modal
        title={`修改权限: ${chmodPath.split('/').pop()}`}
        open={chmodVisible}
        onCancel={() => setChmodVisible(false)}
        onOk={handleChmod}
        okText="确定"
        cancelText="取消"
      >
        <Input
          placeholder="权限模式 (如 755, 644)"
          value={chmodMode}
          onChange={(e) => setChmodMode(e.target.value)}
          addonBefore="chmod"
        />
        <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
          常用权限：755 (rwxr-xr-x) | 644 (rw-r--r--) | 700 (rwx------)
        </div>
      </Modal>

      {/* 文件详情 */}
      <Modal
        title={`文件详情: ${detailData?.name || ''}`}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={500}
      >
        {detailData && (
          <div>
            <Row gutter={[16, 8]}>
              <Col span={8}><strong>名称：</strong></Col>
              <Col span={16}>{detailData.name}</Col>
              <Col span={8}><strong>路径：</strong></Col>
              <Col span={16} style={{ wordBreak: 'break-all' }}>{detailData.path}</Col>
              <Col span={8}><strong>类型：</strong></Col>
              <Col span={16}>{detailData.is_dir ? '文件夹' : '文件'}</Col>
              <Col span={8}><strong>大小：</strong></Col>
              <Col span={16}>{(detailData.size_bytes / 1024).toFixed(1)} KB</Col>
              <Col span={8}><strong>权限：</strong></Col>
              <Col span={16}>{detailData.mode} ({detailData.mode_octal})</Col>
              <Col span={8}><strong>属主：</strong></Col>
              <Col span={16}>UID: {detailData.uid}, GID: {detailData.gid}</Col>
              <Col span={8}><strong>修改时间：</strong></Col>
              <Col span={16}>{new Date(detailData.modified_at).toLocaleString()}</Col>
            </Row>
          </div>
        )}
      </Modal>

      {/* 预览 */}
      <Modal
        title={`预览: ${previewPath.split('/').pop()}`}
        open={previewVisible}
        onCancel={() => setPreviewVisible(false)}
        footer={null}
        width={previewType === 'image' ? 800 : 900}
      >
        {previewType === 'image' && (
          <img
            src={`/api/files/download?path=${encodeURIComponent(previewPath)}`}
            alt="preview"
            style={{ maxWidth: '100%', maxHeight: '70vh' }}
          />
        )}
        {previewType === 'pdf' && (
          <iframe
            src={`/api/files/download?path=${encodeURIComponent(previewPath)}`}
            style={{ width: '100%', height: '70vh', border: 'none' }}
          />
        )}
        {previewType === 'text' && (
          <pre style={{
            background: '#f5f5f5',
            padding: 16,
            borderRadius: 4,
            maxHeight: '70vh',
            overflow: 'auto',
            fontSize: 12,
            fontFamily: 'Consolas, Monaco, monospace',
          }}>
            {previewContent}
          </pre>
        )}
      </Modal>
    </div>
  );
}
