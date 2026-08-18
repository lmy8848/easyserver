import type { ReactNode } from 'react';
import mime from 'mime/lite';
import {
  FileOutlined, FileImageOutlined, FilePdfOutlined, FileZipOutlined,
  FileTextOutlined, VideoCameraOutlined, AudioOutlined, FolderOutlined,
} from '@ant-design/icons';

export type PreviewType = 'image' | 'audio' | 'video' | 'pdf' | 'archive';
export type FileCategory = PreviewType | 'text' | 'other';

/**
 * 判断是否为压缩包文件（ZIP、TAR、GZ、TGZ 等）
 */
export function isArchivePath(path: string): boolean {
  const p = path.toLowerCase();
  return (
    p.endsWith('.zip') ||
    p.endsWith('.tar') ||
    p.endsWith('.tar.gz') ||
    p.endsWith('.tgz') ||
    p.endsWith('.gz') ||
    p.endsWith('.bz2') ||
    p.endsWith('.xz') ||
    p.endsWith('.7z') ||
    p.endsWith('.rar')
  );
}

/**
 * 获取文件的标准 MIME 类型（基于 mime/lite）
 */
export function getMimeType(path: string): string {
  return mime.getType(path) || '';
}

/**
 * 获取文件的大类分类
 */
export function getFileCategory(path: string): FileCategory {
  if (isArchivePath(path)) return 'archive';
  const mimeType = getMimeType(path);
  if (mimeType.startsWith('image/')) return 'image';
  if (mimeType.startsWith('audio/')) return 'audio';
  if (mimeType.startsWith('video/')) return 'video';
  if (mimeType === 'application/pdf') return 'pdf';
  if (
    mimeType.startsWith('text/') ||
    mimeType === 'application/json' ||
    mimeType === 'application/xml' ||
    mimeType === 'application/javascript' ||
    mimeType === 'application/x-sh' ||
    mimeType === 'application/x-yaml' ||
    mimeType === 'application/toml'
  ) {
    return 'text';
  }
  return 'other';
}

/**
 * 判断文件是否为多媒体/压缩包等需要使用 PreviewModal 预览的类型
 * 若返回 null，则说明为代码/文本/未知文件，默认直接进入代码编辑器
 */
export function getPreviewType(path: string): PreviewType | null {
  if (isArchivePath(path)) return 'archive';
  const mimeType = getMimeType(path);
  if (mimeType.startsWith('image/')) return 'image';
  if (mimeType.startsWith('audio/')) return 'audio';
  if (mimeType.startsWith('video/')) return 'video';
  if (mimeType === 'application/pdf') return 'pdf';
  return null;
}

/**
 * 根据文件名或 MIME 类型返回对应的 Ant Design 图标
 */
export function getFileIcon(name: string, isDir?: boolean): ReactNode {
  if (isDir) {
    return <FolderOutlined style={{ color: '#faad14' }} />;
  }

  const category = getFileCategory(name);
  switch (category) {
    case 'image':
      return <FileImageOutlined style={{ color: '#1890ff' }} />;
    case 'audio':
      return <AudioOutlined style={{ color: '#13c2c2' }} />;
    case 'video':
      return <VideoCameraOutlined style={{ color: '#722ed1' }} />;
    case 'pdf':
      return <FilePdfOutlined style={{ color: '#ff4d4f' }} />;
    case 'archive':
      return <FileZipOutlined style={{ color: '#faad14' }} />;
    case 'text':
      return <FileTextOutlined style={{ color: '#52c41a' }} />;
    default:
      return <FileOutlined style={{ color: '#1890ff' }} />;
  }
}
