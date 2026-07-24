import { useState, useEffect, useCallback, useRef } from 'react';
import { Modal, message, Input, Form, InputNumber, Progress, Spin, Button } from 'antd';
import { fileApi, fileShareApi } from '../../services/api';
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

  // Drag-and-drop upload state
  const [dragOver, setDragOver] = useState(false);
  const [uploadQueue, setUploadQueue] = useState<{ name: string; progress: number; status: 'pending' | 'uploading' | 'done' | 'error'; error?: string }[]>([]);
  const uploadQueueRef = useRef(uploadQueue);
  useEffect(() => { uploadQueueRef.current = uploadQueue; }, [uploadQueue]);

  // Overwrite confirmation state
  const [overwriteVisible, setOverwriteVisible] = useState(false);
  const [overwriteConflicts, setOverwriteConflicts] = useState<string[]>([]);
  const overwriteResolveRef = useRef<((v: boolean) => void) | null>(null);

  // Show overwrite confirmation, returns Promise<boolean> (true = overwrite all)
  const confirmOverwrite = useCallback((conflicts: string[]): Promise<boolean> => {
    return new Promise((resolve) => {
      overwriteResolveRef.current = resolve;
      setOverwriteConflicts(conflicts);
      setOverwriteVisible(true);
    });
  }, []);

  const handleOverwriteDecision = useCallback((overwrite: boolean) => {
    setOverwriteVisible(false);
    overwriteResolveRef.current?.(overwrite);
    overwriteResolveRef.current = null;
  }, []);

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

  // 文件外链
  const [shareConfigVisible, setShareConfigVisible] = useState(false);
  const [shareConfigPath, setShareConfigPath] = useState('');
  const [shareConfigLoading, setShareConfigLoading] = useState(false);
  const [shareForm] = Form.useForm();
  const [shareResultVisible, setShareResultVisible] = useState(false);
  const [shareLink, setShareLink] = useState('');

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

  // 清理路径中的 . 和 ..
  const cleanPath = (path: string): string => {
    const parts = path.split('/');
    const stack: string[] = [];
    for (const part of parts) {
      if (part === '' || part === '.') {
        continue;
      }
      if (part === '..') {
        if (stack.length > 0) {
          stack.pop();
        }
      } else {
        stack.push(part);
      }
    }
    return '/' + stack.join('/');
  };

  const handleNavigate = async (path: string) => {
    const cleanedPath = cleanPath(path);
    if (!isValidPath(cleanedPath)) {
      message.error('路径不合法');
      return;
    }
    try {
      const relativePath = toRelativePath(cleanedPath);
      const res = await fileApi.list(relativePath);
      setFiles(res.data.data?.entries || []);
      setCurrentPath(cleanedPath);
    } catch (error) {
      console.error('Failed to navigate:', error);
      message.error('无法访问该路径');
    }
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

  const handleShare = (path: string) => {
    setShareConfigPath(path);
    shareForm.resetFields();
    setShareConfigVisible(true);
  };

  const handleShareCreate = async () => {
    try {
      const values = await shareForm.validateFields();
      setShareConfigLoading(true);
      const password = values.password || '';
      const res = await fileShareApi.create({
        file_path: shareConfigPath,
        password: password,
        expires_at: values.expires_at || '',
        max_downloads: values.max_downloads || 0,
      });
      const share = res.data.data;
      const base = `${window.location.origin}/share/${share.token}`;
      setShareLink(password ? `${base}?password=${encodeURIComponent(password)}` : base);
      setShareConfigVisible(false);
      setShareResultVisible(true);
    } catch (error: unknown) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return;
      }
      const errMsg = (error instanceof Error ? error.message : null)
        || (error && typeof error === 'object' && 'response' in error
          ? String((error as any).response?.data?.message || '')
          : '')
        || '生成外链失败';
      message.error(errMsg);
    } finally {
      setShareConfigLoading(false);
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
    const lowerPath = path.toLowerCase();
    const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico'];
    const audioExts = ['mp3', 'wav', 'ogg', 'flac', 'm4a', 'aac'];
    const videoExts = ['mp4', 'webm', 'mkv', 'mov', 'avi', 'm4v'];
    const textExts = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'conf', 'log', 'sh', 'py', 'go', 'js', 'ts', 'html', 'css', 'sql', 'env', 'csv', 'bat', 'ps1'];
    const isArchive = lowerPath.endsWith('.zip') || lowerPath.endsWith('.tar') || lowerPath.endsWith('.tar.gz') || lowerPath.endsWith('.tgz') || lowerPath.endsWith('.gz');

    if (imageExts.includes(ext)) {
      setPreviewType('image');
      setPreviewPath(path);
    } else if (audioExts.includes(ext)) {
      setPreviewType('audio');
      setPreviewPath(path);
    } else if (videoExts.includes(ext)) {
      setPreviewType('video');
      setPreviewPath(path);
    } else if (ext === 'pdf') {
      setPreviewType('pdf');
      setPreviewPath(path);
    } else if (isArchive) {
      try {
        const res = await fileApi.archiveList(path);
        setPreviewType('archive');
        setPreviewContent(JSON.stringify(res.data.data?.entries || []));
        setPreviewPath(path);
      } catch {
        message.error('无法读取压缩文件');
        return;
      }
    } else if (textExts.includes(ext)) {
      try {
        const res = await fileApi.getContent(path);
        setPreviewType('text');
        setPreviewContent(res.data.data?.content || '');
        setPreviewPath(path);
      } catch {
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
    // Check for conflict
    const existingNames = new Set(files.map(f => f.name));
    if (existingNames.has(file.name)) {
      const overwrite = await confirmOverwrite([file.name]);
      if (!overwrite) {
        message.info('已跳过重复文件');
        return;
      }
    }
    await fileApi.upload(file, toRelativePath(currentPath));
    message.success('上传成功');
    setLoading(true);
    fetchFiles(currentPath);
  };

  // Drag-and-drop upload
  const uploadFiles = useCallback(async (fileList: FileList | File[]) => {
    let fileArray = Array.from(fileList);
    if (fileArray.length === 0) return;

    console.log('[Upload] Files to upload:', fileArray.length, fileArray.map(f => ({ name: f.name, size: f.size, type: f.type })));

    // Check for conflicts with existing files (use state files, not param)
    const existingNames = new Set(files.map(f => f.name));
    const conflicts = fileArray.filter(f => existingNames.has(f.name)).map(f => f.name);
    if (conflicts.length > 0) {
      const overwrite = await confirmOverwrite(conflicts);
      if (!overwrite) {
        // Skip conflicting files
        const conflictSet = new Set(conflicts);
        fileArray = fileArray.filter(f => !conflictSet.has(f.name));
        if (fileArray.length === 0) {
          message.info('已跳过所有重复文件');
          return;
        }
      }
    }

    // Initialize queue
    const queue = fileArray.map(f => ({ name: f.name, progress: 0, status: 'pending' as const }));
    setUploadQueue(queue);

    const relPath = toRelativePath(currentPath);
    let successCount = 0;

    for (let i = 0; i < fileArray.length; i++) {
      // Update status to uploading
      setUploadQueue(prev => prev.map((item, idx) =>
        idx === i ? { ...item, status: 'uploading' as const } : item
      ));

      try {
        const f = fileArray[i];
        if (!f) continue;
        await fileApi.upload(f, relPath, (percent) => {
          setUploadQueue(prev => prev.map((item, idx) =>
            idx === i ? { ...item, progress: percent } : item
          ));
        });
        // Update progress to 100%
        setUploadQueue(prev => prev.map((item, idx) =>
          idx === i ? { ...item, progress: 100, status: 'done' as const } : item
        ));
        successCount++;
      } catch (error: any) {
        const errMsg = error?.response?.data?.message || error?.message || '上传失败';
        setUploadQueue(prev => prev.map((item, idx) =>
          idx === i ? { ...item, status: 'error' as const, error: errMsg } : item
        ));
      }
    }

    if (successCount > 0) {
      message.success(`成功上传 ${successCount} 个文件`);
      setLoading(true);
      fetchFiles(currentPath);
    }

    // Clear queue after 3 seconds
    setTimeout(() => setUploadQueue([]), 3000);
  }, [currentPath, files, confirmOverwrite]);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer.types.includes('Files')) {
      setDragOver(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);

    console.log('[Drop] DataTransfer types:', Array.from(e.dataTransfer.types));
    console.log('[Drop] Files count:', e.dataTransfer.files?.length ?? 0);
    if (e.dataTransfer.files?.length) {
      const firstFile = e.dataTransfer.files[0]!;
      console.log('[Drop] First file:', {
        name: firstFile.name,
        size: firstFile.size,
        type: firstFile.type,
        constructor: firstFile.constructor.name,
        isFile: firstFile instanceof File,
      });
      uploadFiles(e.dataTransfer.files);
    } else {
      console.log('[Drop] No files in DataTransfer');
      // Log all items for debugging
      for (let i = 0; i < (e.dataTransfer.items?.length || 0); i++) {
        const item = e.dataTransfer.items[i];
        if (!item) continue;
        console.log(`[Drop] Item ${i}:`, { kind: item.kind, type: item.type });
      }
    }
  }, [uploadFiles]);

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

  // Upload queue summary for display
  const uploadingCount = uploadQueue.filter(u => u.status === 'uploading').length;
  const doneCount = uploadQueue.filter(u => u.status === 'done').length;
  const errorCount = uploadQueue.filter(u => u.status === 'error').length;
  const showQueue = uploadQueue.length > 0 && (uploadingCount > 0 || doneCount > 0 || errorCount > 0);

  return (
    <div
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      style={{ position: 'relative', minHeight: '200px' }}
    >
      {/* Drag overlay */}
      {dragOver && (
        <div style={{
          position: 'absolute',
          inset: 0,
          zIndex: 1000,
          background: 'rgba(99, 102, 241, 0.08)',
          border: '2px dashed #6366f1',
          borderRadius: 8,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          pointerEvents: 'none',
        }}>
          <div style={{
            fontSize: 24,
            fontWeight: 600,
            color: '#6366f1',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 8,
          }}>
            <span style={{ fontSize: 48 }}>📁</span>
            <span>拖放文件到此处上传</span>
          </div>
        </div>
      )}

      {/* Upload progress */}
      {showQueue && (
        <div style={{
          position: 'fixed',
          bottom: 24,
          right: 24,
          zIndex: 1010,
          background: '#fff',
          borderRadius: 8,
          boxShadow: '0 4px 16px rgba(0,0,0,0.15)',
          padding: '16px 20px',
          minWidth: 280,
          maxHeight: 360,
          overflowY: 'auto',
        }}>
          <div style={{ fontWeight: 600, marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8 }}>
            {uploadingCount > 0 && <Spin size="small" />}
            <span>
              {uploadingCount > 0
                ? `正在上传 ${uploadingCount} 个文件...`
                : `上传完成: ${doneCount} 成功${errorCount > 0 ? `, ${errorCount} 失败` : ''}`
              }
            </span>
          </div>
          {uploadQueue.slice(0, 8).map((item, idx) => (
            <div key={idx} style={{ marginBottom: 4 }}>
              <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                fontSize: 12,
                color: item.status === 'error' ? '#ff4d4f' : '#666',
                marginBottom: 2,
              }}>
                <span style={{
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  maxWidth: 180,
                }}>{item.name}</span>
                <span style={{ flexShrink: 0, marginLeft: 8 }}>
                  {item.status === 'done' ? '✓' : item.status === 'error' ? '✗' : `${item.progress}%`}
                </span>
              </div>
              {item.status === 'uploading' && (
                <Progress percent={item.progress} size="small" showInfo={false} style={{ marginBottom: 0 }} />
              )}
            </div>
          ))}
          {uploadQueue.length > 8 && (
            <div style={{ fontSize: 12, color: '#999', marginTop: 4 }}>
              ...还有 {uploadQueue.length - 8} 个文件
            </div>
          )}
        </div>
      )}

      <FileManagerHeader
        basePath={basePath}
        currentPath={currentPath}
        canManageFiles={true}
        selectedKeys={selectedKeys}
        sortField={sortField}
        sortOrder={sortOrder}
        onNavigate={handleNavigate}
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
          onShare={handleShare}
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

      <Modal
        title="生成文件外链"
        open={shareConfigVisible}
        onCancel={() => setShareConfigVisible(false)}
        onOk={handleShareCreate}
        okText="生成"
        confirmLoading={shareConfigLoading}
        cancelText="取消"
        width={480}
      >
        <Form form={shareForm} layout="vertical">
          <Form.Item label="文件路径">
            <Input value={shareConfigPath} disabled />
          </Form.Item>
          <Form.Item name="expires_at" label="过期时间"
            extra="留空为永久有效。支持：1h, 1d, 7d 或具体时间 2026-07-01 12:00:00"
          >
            <Input placeholder="留空、1h、7d 或 2026-07-01 12:00:00" />
          </Form.Item>
          <Form.Item name="max_downloads" label="最大下载次数" extra="0 表示不限制">
            <InputNumber min={0} max={100000} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="password" label="访问密码（可选）">
            <Input.Password placeholder="留空表示不需要密码" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="外链生成成功"
        open={shareResultVisible}
        onCancel={() => setShareResultVisible(false)}
        footer={null}
        width={500}
      >
        <p>文件分享链接已生成，复制链接发送给他人即可下载：</p>
        <Input
          value={shareLink}
          readOnly
          style={{ fontFamily: 'monospace', marginBottom: 12 }}
          addonAfter={
            <span
              style={{ cursor: 'pointer', color: '#1890ff' }}
              onClick={() => {
                navigator.clipboard.writeText(shareLink).then(() => {
                  message.success('已复制');
                }).catch(() => {
                  const ta = document.createElement('textarea');
                  ta.value = shareLink;
                  document.body.appendChild(ta);
                  ta.select();
                  document.execCommand('copy');
                  document.body.removeChild(ta);
                  message.success('已复制');
                });
              }}
            >
              复制
            </span>
          }
        />
        {shareLink.includes('?password=') && (
          <p style={{ color: '#faad14', fontSize: 13, marginTop: 8 }}>
            ⚠ 该外链设置了密码，分享时请将完整链接（含 ?password=xxx）发送给对方
          </p>
        )}
      </Modal>

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

      <Modal
        title="文件已存在"
        open={overwriteVisible}
        onCancel={() => handleOverwriteDecision(false)}
        footer={[
          <Button key="skip" onClick={() => handleOverwriteDecision(false)}>
            跳过重复文件
          </Button>,
          <Button key="overwrite" type="primary" danger onClick={() => handleOverwriteDecision(true)}>
            覆盖已有文件
          </Button>,
        ]}
      >
        <p style={{ marginBottom: 8, color: '#666' }}>
          以下 {overwriteConflicts.length} 个文件已存在，是否覆盖？
        </p>
        <div style={{
          maxHeight: 200,
          overflowY: 'auto',
          background: '#fafafa',
          borderRadius: 4,
          padding: '8px 12px',
          fontSize: 13,
        }}>
          {overwriteConflicts.map((name, i) => (
            <div key={i} style={{ padding: '2px 0', color: '#cf1322' }}>⚠ {name}</div>
          ))}
        </div>
      </Modal>
    </div>
  );
}
