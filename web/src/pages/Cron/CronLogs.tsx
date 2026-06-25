import { Modal, Table, Tag, Button, Space, Empty, Tooltip } from 'antd';
import { HistoryOutlined } from '@ant-design/icons';
import type { CronTask, CronLog } from '../../types';

interface CronLogsProps {
  visible: boolean;
  task: CronTask | null;
  logs: CronLog[];
  loading: boolean;
  onClose: () => void;
}

export default function CronLogs({ visible, task, logs, loading, onClose }: CronLogsProps) {
  const logColumns = [
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status: string) => (
        <Tag color={status === 'success' ? 'success' : 'error'}>{status}</Tag>
      ),
    },
    {
      title: '耗时',
      dataIndex: 'duration',
      key: 'duration',
      width: 100,
      render: (ms: number) => `${ms}ms`,
    },
    {
      title: '执行时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
    },
    {
      title: '输出',
      dataIndex: 'output',
      key: 'output',
      ellipsis: true,
      render: (output: string) => (
        <Tooltip title={output} placement="left">
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {output?.substring(0, 100)}{output?.length > 100 ? '...' : ''}
          </span>
        </Tooltip>
      ),
    },
  ];

  return (
    <Modal
      title={<Space><HistoryOutlined /> {task?.name} - 执行日志</Space>}
      open={visible}
      onCancel={onClose}
      footer={<Button onClick={onClose}>关闭</Button>}
      width={900}
    >
      <Table
        columns={logColumns}
        dataSource={logs}
        rowKey="id"
        loading={loading}
        size="small"
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: <Empty description="暂无执行记录" /> }}
      />
    </Modal>
  );
}
