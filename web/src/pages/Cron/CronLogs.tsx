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

const priorityColor: Record<string, string> = {
  err: 'error', crit: 'error', emerg: 'error', alert: 'error',
  warn: 'warning', info: 'default', notice: 'blue', debug: 'default',
};

export default function CronLogs({ visible, task, logs, loading, onClose }: CronLogsProps) {
  const logColumns = [
    {
      title: '级别',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
      render: (priority: string) => (
        <Tag color={priorityColor[priority] || 'default'}>{priority || 'info'}</Tag>
      ),
    },
    {
      title: '时间',
      dataIndex: 'time',
      key: 'time',
      width: 180,
    },
    {
      title: '日志内容',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
      render: (message: string) => (
        <Tooltip title={message} placement="left">
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {message}
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
        rowKey={(r, i) => `${r.time}-${i}`}
        loading={loading}
        size="small"
        pagination={{ pageSize: 20 }}
        locale={{ emptyText: <Empty description="暂无执行记录" /> }}
      />
    </Modal>
  );
}