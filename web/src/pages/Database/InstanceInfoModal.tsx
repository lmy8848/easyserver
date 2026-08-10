import { Modal, Table, Tag, Button, Empty } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { EngineInfo, DBInstance } from './types';

interface InstanceInfoModalProps {
  server: EngineInfo;
  version: DBInstance | null;
  visible: boolean;
  loading: boolean;
  onClose: () => void;
  statusTag: (status: string) => React.ReactNode;
}

// Instance info modal — one row per field of the selected instance.
export default function InstanceInfoModal({ server, version, visible, loading, onClose, statusTag }: InstanceInfoModalProps) {
  const columns: ColumnsType<{ key: string; label: string; value: React.ReactNode }> = [
    { title: '属性', dataIndex: 'label', width: 140 },
    { title: '值', dataIndex: 'value' },
  ];
  const rows: Array<{ key: string; label: string; value: React.ReactNode }> = version ? [
    { key: 'version', label: '版本', value: <strong>{version.version}</strong> },
    { key: 'status', label: '状态', value: statusTag(version.status) },
    { key: 'port', label: '端口', value: version.port },
    { key: 'container_engine', label: '容器引擎', value: version.container_engine },
    { key: 'image', label: '镜像', value: <Tag>{version.image}</Tag> },
    { key: 'container_id', label: '容器ID', value: <Tag>{version.container_id}</Tag> },
    { key: 'volume_name', label: '数据卷', value: version.volume_name },
    { key: 'bind_address', label: '监听地址', value: version.bind_address },
    { key: 'created_at', label: '创建时间', value: version.created_at },
  ] : [];

  return (
    <Modal
      title={`${server.display_name} ${version?.version || ''} - 实例信息`}
      open={visible}
      onCancel={onClose}
      footer={<Button size="small" onClick={onClose}>关闭</Button>}
    >
      <Table
        rowKey="key"
        columns={columns}
        dataSource={rows}
        pagination={false}
        showHeader={false}
        loading={loading}
        locale={{ emptyText: <Empty description="暂无实例信息" /> }}
      />
    </Modal>
  );
}
