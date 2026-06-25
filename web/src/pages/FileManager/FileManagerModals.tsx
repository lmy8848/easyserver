import {
  Modal, Input, Table, Button, Space, Tag, Row, Col,
} from 'antd';
import {
  FolderOutlined, FileOutlined, SearchOutlined,
} from '@ant-design/icons';

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
      <Input
        placeholder="权限模式 (如 755, 644)"
        value={mode}
        onChange={(e) => onModeChange(e.target.value)}
        addonBefore="chmod"
      />
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
            <Col span={16}>{(data.size_bytes / 1024).toFixed(1)} KB</Col>
            <Col span={8}><strong>权限：</strong></Col>
            <Col span={16}>{data.mode} ({data.mode_octal})</Col>
            <Col span={8}><strong>属主：</strong></Col>
            <Col span={16}>UID: {data.uid}, GID: {data.gid}</Col>
            <Col span={8}><strong>修改时间：</strong></Col>
            <Col span={16}>{new Date(data.modified_at).toLocaleString()}</Col>
          </Row>
        </div>
      )}
    </Modal>
  );
}

// ==================== Preview Modal ====================
interface PreviewModalProps {
  visible: boolean;
  path: string;
  type: string;
  content: string;
  onClose: () => void;
}

export function PreviewModal({ visible, path, type, content, onClose }: PreviewModalProps) {
  return (
    <Modal
      title={`预览: ${path.split('/').pop()}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={type === 'image' ? 800 : 900}
    >
      {type === 'image' && (
        <img
          src={`/api/files/download?path=${encodeURIComponent(path)}`}
          alt="preview"
          style={{ maxWidth: '100%', maxHeight: '70vh' }}
        />
      )}
      {type === 'pdf' && (
        <iframe
          src={`/api/files/download?path=${encodeURIComponent(path)}`}
          style={{ width: '100%', height: '70vh', border: 'none' }}
        />
      )}
      {type === 'text' && (
        <pre style={{
          background: '#f5f5f5',
          padding: 16,
          borderRadius: 4,
          maxHeight: '70vh',
          overflow: 'auto',
          fontSize: 12,
          fontFamily: 'Consolas, Monaco, monospace',
        }}>
          {content}
        </pre>
      )}
    </Modal>
  );
}
