import { useState, useEffect } from 'react';
import { Modal, Table, Tag, Button, Space, Empty, Tooltip, List, DatePicker, Segmented } from 'antd';
import { HistoryOutlined, ReloadOutlined } from '@ant-design/icons';
import type { CronTask, CronRun } from '../../types';

interface CronLogsProps {
  visible: boolean;
  task: CronTask | null;
  runs: CronRun[];
  loading: boolean;
  onClose: () => void;
  onRefresh: (task: CronTask) => void;
}

const priorityColor: Record<string, string> = {
  err: 'error', crit: 'error', emerg: 'error', alert: 'error',
  warn: 'warning', info: 'default', notice: 'blue', debug: 'default',
};

function statusTag(status: string) {
  switch (status) {
    case 'success': return <Tag color="success">成功</Tag>;
    case 'failed': return <Tag color="error">失败</Tag>;
    default: return <Tag color="processing">执行中</Tag>;
  }
}

export default function CronLogs({ visible, task, runs, loading, onClose, onRefresh }: CronLogsProps) {
  const [selected, setSelected] = useState<string | null>(null);
  const [date, setDate] = useState<string | null>(null);
  const [filter, setFilter] = useState<'all' | 'success' | 'failed'>('all');

  // 切换任务时重置选中与筛选
  useEffect(() => {
    setSelected(null);
    setDate(null);
    setFilter('all');
  }, [task?.name]);

  const filtered = runs.filter(r => {
    if (date && !r.started_at.startsWith(date)) return false;
    if (filter !== 'all' && r.status !== filter) return false;
    return true;
  });

  const selectedRun = runs.find(r => r.invocation_id === selected) || null;

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
    { title: '时间', dataIndex: 'time', key: 'time', width: 180 },
    {
      title: '日志内容',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
      render: (message: string) => (
        <Tooltip title={message} placement="left">
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{message}</span>
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
      width={1000}
      styles={{ body: { padding: 0 } }}
    >
      <div style={{ display: 'flex', height: 480 }}>
        {/* 左侧：执行列表 */}
        <div style={{ width: 360, borderRight: '1px solid #f0f0f0', display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 12, borderBottom: '1px solid #f0f0f0', display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <DatePicker
              size="small"
              placeholder="按日期筛选"
              onChange={(_d, dStr) => setDate(dStr || null)}
              style={{ width: 130 }}
            />
            <Segmented
              size="small"
              value={filter}
              onChange={(v) => setFilter(v as typeof filter)}
              options={[
                { label: '全部', value: 'all' },
                { label: '成功', value: 'success' },
                { label: '失败', value: 'failed' },
              ]}
            />
            <Button size="small" icon={<ReloadOutlined />} onClick={() => task && onRefresh(task)} />
          </div>
          <List
            size="small"
            loading={loading}
            dataSource={filtered}
            style={{ flex: 1, overflowY: 'auto' }}
            locale={{ emptyText: <Empty description="暂无执行记录" /> }}
            renderItem={(r: CronRun) => (
              <List.Item
                onClick={() => setSelected(r.invocation_id)}
                style={{
                  cursor: 'pointer',
                  padding: '8px 12px',
                  background: selected === r.invocation_id ? '#e6f4ff' : undefined,
                }}
              >
                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <span style={{ fontSize: 12 }}>{r.started_at}</span>
                  {statusTag(r.status)}
                </Space>
              </List.Item>
            )}
          />
        </div>
        {/* 右侧：选中执行的日志 */}
        <div style={{ flex: 1, padding: 12, overflowY: 'auto' }}>
          {selectedRun ? (
            <Table
              columns={logColumns}
              dataSource={selectedRun.logs}
              rowKey={(l, i) => `${l.time}-${i}`}
              size="small"
              pagination={false}
              locale={{ emptyText: <Empty description="该次执行无日志输出" /> }}
            />
          ) : (
            <Empty description="选择左侧某次执行查看日志" style={{ marginTop: 150 }} />
          )}
        </div>
      </div>
    </Modal>
  );
}