import { Table, Button, Tag } from 'antd';
import {
  PlayCircleOutlined,
  PauseCircleOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import type { CloudInstance } from '../../types';

interface Props {
  instances: CloudInstance[];
  loading: boolean;
  selectedInstance: string;
  onSelect: (instanceId: string) => void;
  onAction: (instanceId: string, action: string) => void;
}

export default function CloudInstances({ instances, loading, selectedInstance, onSelect, onAction }: Props) {
  const columns = [
    {
      title: '实例 ID',
      dataIndex: 'instance_id',
      key: 'instance_id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      render: (state: string) => {
        const colorMap: Record<string, string> = {
          RUNNING: 'success',
          STOPPED: 'error',
          STARTING: 'processing',
          STOPPING: 'warning',
        };
        return <Tag color={colorMap[state] || 'default'}>{state}</Tag>;
      },
    },
    {
      title: '公网 IP',
      dataIndex: 'public_ip',
      key: 'public_ip',
    },
    {
      title: '配置',
      key: 'config',
      render: (_: unknown, record: CloudInstance) => (
        `${record.cpu}核 / ${record.memory_gb}GB / ${record.disk_gb}GB`
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: CloudInstance) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Button
            type={selectedInstance === record.instance_id ? 'primary' : 'default'}
            size="small"
            onClick={() => onSelect(record.instance_id)}
          >
            {selectedInstance === record.instance_id ? '已选择' : '选择'}
          </Button>
          {record.state !== 'RUNNING' ? (
            <Button
              type="primary"
              size="small"
              icon={<PlayCircleOutlined />}
              onClick={() => onAction(record.instance_id, 'start')}
            >
              启动
            </Button>
          ) : (
            <>
              <Button
                danger
                size="small"
                icon={<PauseCircleOutlined />}
                onClick={() => onAction(record.instance_id, 'stop')}
              >
                停止
              </Button>
              <Button
                size="small"
                icon={<SyncOutlined />}
                onClick={() => onAction(record.instance_id, 'restart')}
              >
                重启
              </Button>
            </>
          )}
        </div>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={instances}
      rowKey="instance_id"
      loading={loading}
      pagination={false}
    />
  );
}
