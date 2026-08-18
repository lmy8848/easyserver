import { useState } from 'react';
import {
  Modal, Input, Table, Button, Space, Tag, Row, Col, Image,
} from 'antd';
import {
  SearchOutlined,
} from '@ant-design/icons';
import { formatBytes, formatDateTime } from '../../utils/format';
import { getFileIcon } from '../../utils/fileType';

// ==================== Mkdir Modal ====================
interface MkdirModalProps {
  visible: boolean;
  dirName: string;
  onClose: () => void;
  onOk: () => void;
  onDirNameChange: (name: string) => void;
}

export function MkdirModal({ visible, dirName, onClose, onOk, onDirNameChange }: MkdirModalProps) {
  return (
    <Modal
      title="新建文件夹"
      open={visible}
      onCancel={onClose}
      onOk={onOk}
      okText="创建"
      cancelText="取消"
    >
      <Input
        placeholder="文件夹名称"
        value={dirName}
        onChange={(e) => onDirNameChange(e.target.value)}
        onPressEnter={onOk}
      />
    </Modal>
  );
}

// ==================== Rename Modal ====================
interface RenameModalProps {
  visible: boolean;
  path: string;
  newName: string;
  onClose: () => void;
  onOk: () => void;
  onNewNameChange: (name: string) => void;
}

export function RenameModal({ visible, path, newName, onClose, onOk, onNewNameChange }: RenameModalProps) {
  return (
    <Modal
      title={`重命名: ${path.split('/').pop()}`}
      open={visible}
      onCancel={onClose}
      onOk={onOk}
      okText="确定"
      cancelText="取消"
    >
      <Input
        placeholder="新名称"
        value={newName}
        onChange={(e) => onNewNameChange(e.target.value)}
        onPressEnter={onOk}
      />
    </Modal>
  );
}

// ==================== CopyMove Modal ====================
interface CopyMoveModalProps {
  visible: boolean;
  mode: 'copy' | 'move';
  source: string;
  dest: string;
  onClose: () => void;
  onOk: () => void;
  onDestChange: (dest: string) => void;
}

export function CopyMoveModal({ visible, mode, source, dest, onClose, onOk, onDestChange }: CopyMoveModalProps) {
  return (
    <Modal
      title={mode === 'copy' ? '复制文件' : '移动文件'}
      open={visible}
      onCancel={onClose}
      onOk={onOk}
      okText="确定"
      cancelText="取消"
    >
      <div style={{ marginBottom: 8 }}><strong>源文件：</strong> {source}</div>
      <div style={{ marginBottom: 8 }}><strong>{mode === 'copy' ? '复制到：' : '移动到：'}</strong></div>
      <Input
        placeholder="目标路径"
        value={dest}
        onChange={(e) => onDestChange(e.target.value)}
      />
    </Modal>
  );
}

// ==================== Search Modal ====================
interface SearchModalProps {
  visible: boolean;
  query: string;
  results: any[];
  searchLoading: boolean;
  onClose: () => void;
  onSearch: () => void;
  onQueryChange: (query: string) => void;
  onItemClick: (record: any) => void;
}

export function SearchModal({
  visible, query, results, searchLoading, onClose, onSearch, onQueryChange, onItemClick,
}: SearchModalProps) {
  return (
    <Modal
      title="搜索文件"
      open={visible}
      onCancel={onClose}
      footer={null}
      width={800}
    >
      <Space style={{ marginBottom: 16 }}>
        <Input
          placeholder="输入文件名关键词"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onPressEnter={onSearch}
          style={{ width: 400 }}
        />
        <Button type="primary" icon={<SearchOutlined />} onClick={onSearch} loading={searchLoading}>
          搜索
        </Button>
      </Space>
      <Table
        dataSource={results}
        rowKey="path"
        size="small"
        pagination={{ pageSize: 20 }}
        columns={[
          {
            title: '名称', dataIndex: 'name',
            render: (name: string, record: any) => (
              <Space style={{ cursor: 'pointer' }} onClick={() => onItemClick(record)}>
                {getFileIcon(record.name, record.is_dir)}
                {name}
              </Space>
            ),
          },
          { title: '路径', dataIndex: 'path', ellipsis: true },
          { title: '大小', dataIndex: 'size', width: 100, render: (s: number) => formatBytes(s) },
          { title: '匹配', dataIndex: 'match', width: 80, render: (m: string) => <Tag color="blue">{m}</Tag> },
        ]}
      />
    </Modal>
  );
}

// ==================== Chmod Modal ====================
interface ChmodModalProps {
  visible: boolean;
  path: string;
  mode: string;
  onClose: () => void;
  onOk: () => void;
  onModeChange: (mode: string) => void;
}

export function ChmodModal({ visible, path, mode, onClose, onOk, onModeChange }: ChmodModalProps) {
  return (
    <Modal
      title={`修改权限: ${path.split('/').pop()}`}
      open={visible}
      onCancel={onClose}
      onOk={onOk}
      okText="确定"
      cancelText="取消"
    >
      <Space.Compact style={{ width: '100%' }}>
        <Button disabled style={{ cursor: 'default', color: 'rgba(0, 0, 0, 0.88)', backgroundColor: '#fafafa' }}>
          chmod
        </Button>
        <Input
          placeholder="权限模式 (如 755, 644)"
          value={mode}
          onChange={(e) => onModeChange(e.target.value)}
        />
      </Space.Compact>
      <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
        常用权限：755 (rwxr-xr-x) | 644 (rw-r--r--) | 700 (rwx------)
      </div>
    </Modal>
  );
}

// ==================== Details Modal ====================
interface DetailsModalProps {
  visible: boolean;
  data: any;
  onClose: () => void;
}

export function DetailsModal({ visible, data, onClose }: DetailsModalProps) {
  return (
    <Modal
      title={`文件详情: ${data?.name || ''}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={500}
    >
      {data && (
        <div>
          <Row gutter={[16, 8]}>
            <Col span={8}><strong>名称：</strong></Col>
            <Col span={16}>{data.name}</Col>
            <Col span={8}><strong>路径：</strong></Col>
            <Col span={16} style={{ wordBreak: 'break-all' }}>{data.path}</Col>
            <Col span={8}><strong>类型：</strong></Col>
            <Col span={16}>{data.is_dir ? '文件夹' : '文件'}</Col>
            <Col span={8}><strong>大小：</strong></Col>
            <Col span={16}>{formatBytes(data.size_bytes)}</Col>
            <Col span={8}><strong>权限：</strong></Col>
            <Col span={16}>{data.mode} ({data.mode_octal})</Col>
            <Col span={8}><strong>属主：</strong></Col>
            <Col span={16}>UID: {data.uid}, GID: {data.gid}</Col>
            <Col span={8}><strong>修改时间：</strong></Col>
            <Col span={16}>{formatDateTime(data.modified_at)}</Col>
          </Row>
        </div>
      )}
    </Modal>
  );
}

interface PreviewModalProps {
  visible: boolean;
  path: string;
  type: string;
  content: string;
  onClose: () => void;
}

export function PreviewModal({ visible, path, type, content, onClose }: PreviewModalProps) {
  // Build a download URL that carries the JWT via the access_token query param.
  // <audio>/<video>/<img>/<iframe> tags cannot send the Authorization header,
  // so the cookie must be carried. The backend serves these with
  // http.ServeContent (Range/206), so playback is streamed/buffered rather
  // than requiring a full download first. 登录态走 HttpOnly cookie 同源自动携带。
  const downloadUrl = `/api/files/download?path=${encodeURIComponent(path)}`;

  // 视频原始分辨率，加载 metadata 后用于按比例调整弹窗大小
  const [videoMeta, setVideoMeta] = useState<{ w: number; h: number } | null>(null);

  // 图片用 antd Image 内置预览层（全屏放大/缩放/旋转），不套外层 Modal。
  if (type === 'image') {
    return (
      <Image.PreviewGroup
        preview={{
          open: visible,
          onOpenChange: (v) => { if (!v) onClose(); },
        }}
        items={path ? [downloadUrl] : []}
      />
    );
  }

  // Parse archive entries from content (JSON string)
  let archiveEntries: Array<{ name: string; size: number; is_dir: boolean }> = [];
  if (type === 'archive' && content) {
    try { archiveEntries = JSON.parse(content); } catch { archiveEntries = []; }
  }

  // 视频按分辨率计算弹窗宽度：高度上限 70vh、宽度上限 90vw，保持宽高比。
  // 未加载时用默认 800，metadata 加载后跟随分辨率（+48 容纳 body padding）。
  let modalWidth = type === 'video' ? 800 : 900;
  if (type === 'video' && videoMeta) {
    const maxH = window.innerHeight * 0.7;
    const maxW = window.innerWidth * 0.9;
    const aspect = videoMeta.w / videoMeta.h;
    // 按高度上限算宽，再约束宽度上限
    const w = Math.min(Math.min(videoMeta.h, maxH) * aspect, maxW);
    modalWidth = Math.ceil(w) + 48;
  }

  return (
    <Modal
      title={`预览: ${path.split('/').pop()}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={modalWidth}
    >
      {type === 'audio' && (
        <audio controls src={downloadUrl} style={{ width: '100%' }} />
      )}
      {type === 'video' && (
        <video
          controls
          src={downloadUrl}
          onLoadedMetadata={(e) => {
            const v = e.currentTarget;
            if (v.videoWidth && v.videoHeight) setVideoMeta({ w: v.videoWidth, h: v.videoHeight });
          }}
          style={{ width: '100%', maxHeight: '70vh', display: 'block' }}
        />
      )}
      {type === 'pdf' && (
        <iframe
          src={downloadUrl}
          style={{ width: '100%', height: '70vh', border: 'none' }}
        />
      )}
      {type === 'archive' && (
        <Table
          size="small"
          dataSource={archiveEntries}
          rowKey="name"
          pagination={{ pageSize: 50 }}
          style={{ maxHeight: '70vh', overflow: 'auto' }}
          locale={{ emptyText: '压缩文件为空或无法读取' }}
          columns={[
            { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true,
              render: (name: string, r: { is_dir: boolean }) => r.is_dir ? `📁 ${name}` : name },
            { title: '大小', dataIndex: 'size', key: 'size', width: 100,
              render: (s: number, r: { is_dir: boolean }) => r.is_dir ? '-' : formatBytes(s) },
            { title: '类型', dataIndex: 'is_dir', key: 'is_dir', width: 80,
              render: (d: boolean) => d ? <Tag>目录</Tag> : <Tag color="blue">文件</Tag> },
          ]}
        />
      )}
    </Modal>
  );
}
