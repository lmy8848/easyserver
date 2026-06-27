import { useState, useEffect } from 'react';
import { Modal, message } from 'antd';
import { fileApi } from '../../services/api';
import type { FileEntry } from '../../types';
import { isValidPath } from './types';
import FileManagerHeader from './FileManagerHeader';
import FileManagerTable from './FileManagerTable';
import FileManagerEditor from './FileManagerEditor';
import {
  MkdirModal, RenameModal, CopyMoveModal, SearchModal,
  ChmodModal, DetailsModal, PreviewModal,
} from './FileManagerModals';

export default function FileManager() {
  const [basePath, setBasePath] = useState<string>('');
  const [currentPath, setCurrentPath] = useState<string>('');
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);

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
    // 直接使用 "/" 作为根目录，不请求服务器
    setBasePath('/');
    setCurrentPath('/');
  }, []);

  // Convert absolute path to relative path (strip basePath prefix)
  const toRelativePath = (absolutePath: string): string => {
    if (basePath && absolutePath === basePath) {
      return ''; // root - backend will use BasePath()
    }
    if (basePath && absolutePath.startsWith(basePath + '/')) {
      return absolutePath.slice(basePath.length + 1);
    }
    return absolutePath;
  };

  const fetchFiles = async (path: string) => {
    if (!isValidPath(path)) {
      message.error('路径不合法');
      return;
    }
    try {
      const relativePath = toRelativePath(path);
      const res = await fileApi.list(relativePath);
      setFiles(res.data.data?.entries || []);
    } catch (error) {
      console.error('Failed to fetch files:', error);
      message.error('获取文件列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!currentPath || !basePath) return;
    if (!isValidPath(currentPath)) {
      message.error('路径不合法');
      return;
    }
    const relativePath = toRelativePath(currentPath);
    fileApi.list(relativePath).then(
      res => setFiles(res.data.data?.entries || []),
      error => {
        console.error('Failed to fetch files:', error);
        message.error('获取文件列表失败');
      },
    ).finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- toRelativePath only depends on basePath
  }, [currentPath, basePath]);

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
      await fileApi.saveContent(toRelativePath(editPath), editContent);
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
          await fileApi.delete(toRelativePath(path), isDir);
          message.success('删除成功');
          setLoading(true);
          fetchFiles(currentPath);
        } catch (error) {
          message.error('删除失败');
        }
      },
    });
  };

  const handleMkdir = async () => {
    if (!newDirName) return;
    if (newDirName.includes('/') || newDirName.includes('\\') || newDirName.includes('\x00') || newDirName === '..') {
      message.error('目录名不合法');
      return;
    }
    const dirPath = currentPath === '/' ? `/${newDirName}` : `${currentPath}/${newDirName}`;
    try {
      await fileApi.mkdir(toRelativePath(dirPath));
      message.success('创建成功');
      setMkdirVisible(false);
      setNewDirName('');
      setLoading(true);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('创建失败');
    }
  };

  const handleDownload = async (path: string) => {
    try {
      const res = await fileApi.download(toRelativePath(path));
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

  const showRename = (path: string, name: string) => {
    setRenamePath(path);
    setRenameNewName(name);
    setRenameVisible(true);
  };

  const handleRename = async () => {
    if (!renameNewName) return;
    if (renameNewName.includes('/') || renameNewName.includes('\\') || renameNewName.includes('\x00') || renameNewName === '..') {
      message.error('文件名不合法');
      return;
    }
    const parentDir = renamePath.substring(0, renamePath.lastIndexOf('/')) || '/';
    const newPath = parentDir === '/' ? `/${renameNewName}` : `${parentDir}/${renameNewName}`;
    try {
      await fileApi.rename(toRelativePath(renamePath), toRelativePath(newPath));
      message.success('重命名成功');
      setRenameVisible(false);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('重命名失败');
    }
  };

  const showCopyMove = (mode: 'copy' | 'move', path: string) => {
    setCopyMoveMode(mode);
    setCopyMoveSource(path);
    setCopyMoveDest(currentPath);
    setCopyMoveVisible(true);
  };

  const handleCopyMove = async () => {
    if (!copyMoveDest) return;
    if (!isValidPath(copyMoveDest)) {
      message.error('目标路径不合法');
      return;
    }
    try {
      if (copyMoveMode === 'copy') {
        await fileApi.copy(toRelativePath(copyMoveSource), toRelativePath(copyMoveDest));
        message.success('复制成功');
      } else {
        await fileApi.move([toRelativePath(copyMoveSource)], toRelativePath(copyMoveDest));
        message.success('移动成功');
      }
      setCopyMoveVisible(false);
      setLoading(true);
      fetchFiles(currentPath);
    } catch (error) {
      message.error(copyMoveMode === 'copy' ? '复制失败' : '移动失败');
    }
  };

  const handleSearch = async () => {
    if (!searchQuery) return;
    setSearchLoading(true);
    try {
      const res = await fileApi.search(toRelativePath(currentPath), searchQuery);
      setSearchResults(res.data.data || []);
    } catch (error) {
      message.error('搜索失败');
    } finally {
      setSearchLoading(false);
    }
  };

  const handleExtract = async (path: string) => {
    const destPath = currentPath;
    try {
      await fileApi.extract(toRelativePath(path), toRelativePath(destPath));
      message.success('解压成功');
      setLoading(true);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('解压失败');
    }
  };

  const showChmod = (path: string) => {
    setChmodPath(path);
    setChmodVisible(true);
  };

  const handleChmod = async () => {
    try {
      await fileApi.chmod(toRelativePath(chmodPath), chmodMode);
      message.success('权限修改成功');
      setChmodVisible(false);
      setLoading(true);
      fetchFiles(currentPath);
    } catch (error) {
      message.error('权限修改失败');
    }
  };

  const showDetails = async (path: string) => {
    try {
      const res = await fileApi.getDetails(toRelativePath(path));
      setDetailData(res.data.data);
      setDetailVisible(true);
    } catch (error) {
      message.error('获取详情失败');
    }
  };

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
          await Promise.all(selectedKeys.map(path => fileApi.delete(toRelativePath(path), true)));
          message.success(`已删除 ${selectedKeys.length} 个文件`);
          setSelectedKeys([]);
          setLoading(true);
          fetchFiles(currentPath);
        } catch (error) {
          message.error('批量删除失败');
        }
      },
    });
  };

  const handleUpload = async (file: File) => {
    await fileApi.upload(file, toRelativePath(currentPath));
    message.success('上传成功');
    setLoading(true);
    fetchFiles(currentPath);
  };

  const handleSearchItemClick = (record: any) => {
    if (record.is_dir) {
      setCurrentPath(record.path);
      setSearchVisible(false);
    } else {
      openFile(record.path);
      setSearchVisible(false);
    }
  };

  // 排序
  const sortedFiles = [...files].sort((a, b) => {
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

  return (
    <div>
      <FileManagerHeader
        basePath={basePath}
        currentPath={currentPath}
        pathParts={pathParts}
        canManageFiles={true}
        selectedKeys={selectedKeys}
        sortField={sortField}
        sortOrder={sortOrder}
        onNavigate={setCurrentPath}
        onSearch={() => setSearchVisible(true)}
        onMkdir={() => setMkdirVisible(true)}
        onUpload={handleUpload}
        onBatchDelete={handleBatchDelete}
        onSortFieldChange={setSortField}
        onSortOrderChange={setSortOrder}
        onRefresh={() => { setLoading(true); fetchFiles(currentPath); }}
      >
        <FileManagerTable
          files={sortedFiles}
          loading={loading}
          selectedKeys={selectedKeys}
          canManageFiles={true}
          onClick={handleClick}
          onEdit={openFile}
          onRename={showRename}
          onCopyMove={showCopyMove}
          onDelete={handleDelete}
          onChmod={showChmod}
          onDetails={showDetails}
          onPreview={showPreview}
          onDownload={handleDownload}
          onExtract={handleExtract}
          onSelectedKeysChange={setSelectedKeys}
        />
      </FileManagerHeader>

      <FileManagerEditor
        visible={editVisible}
        path={editPath}
        content={editContent}
        onClose={() => setEditVisible(false)}
        onSave={handleSave}
        onContentChange={setEditContent}
      />

      <MkdirModal
        visible={mkdirVisible}
        dirName={newDirName}
        onClose={() => setMkdirVisible(false)}
        onOk={handleMkdir}
        onDirNameChange={setNewDirName}
      />

      <RenameModal
        visible={renameVisible}
        path={renamePath}
        newName={renameNewName}
        onClose={() => setRenameVisible(false)}
        onOk={handleRename}
        onNewNameChange={setRenameNewName}
      />

      <CopyMoveModal
        visible={copyMoveVisible}
        mode={copyMoveMode}
        source={copyMoveSource}
        dest={copyMoveDest}
        onClose={() => setCopyMoveVisible(false)}
        onOk={handleCopyMove}
        onDestChange={setCopyMoveDest}
      />

      <SearchModal
        visible={searchVisible}
        query={searchQuery}
        results={searchResults}
        searchLoading={searchLoading}
        onClose={() => { setSearchVisible(false); setSearchResults([]); }}
        onSearch={handleSearch}
        onQueryChange={setSearchQuery}
        onItemClick={handleSearchItemClick}
      />

      <ChmodModal
        visible={chmodVisible}
        path={chmodPath}
        mode={chmodMode}
        onClose={() => setChmodVisible(false)}
        onOk={handleChmod}
        onModeChange={setChmodMode}
      />

      <DetailsModal
        visible={detailVisible}
        data={detailData}
        onClose={() => setDetailVisible(false)}
      />

      <PreviewModal
        visible={previewVisible}
        path={previewPath}
        type={previewType}
        content={previewContent}
        onClose={() => setPreviewVisible(false)}
      />
    </div>
  );
}
