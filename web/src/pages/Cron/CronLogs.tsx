import { useState, useEffect } from 'react';
import { Modal, Tag, Button, Space, Empty, List, Segmented, DatePicker } from 'antd';
import { HistoryOutlined, ReloadOutlined } from '@ant-design/icons';
import type { CronTask, CronRun } from '../../types';
import { LogViewer } from '../../components/LogViewer';

interface CronLogsProps {
  visible: boolean;
  task: CronTask | null;
  runs: CronRun[];
  loading: boolean;
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number, pageSize: number) => void;
  onDateRangeChange: (since?: string, until?: string) => void;
  onClose: () => void;
  onRefresh: (task: CronTask) => void;
}

function statusTag(status: string) {
  switch (status) {
    case 'success': return <Tag color="success">成功</Tag>;
    case 'failed': return <Tag color="error">失败</Tag>;
    default: return <Tag color="processing">执行中</Tag>;
  }
}

export default function CronLogs({
  visible, task, runs, loading, page, pageSize, total, onPageChange, onDateRangeChange, onClose, onRefresh,
}: CronLogsProps) {
  const [selected, setSelected] = useState<string | null>(null);
  const [filter, setFilter] = useState<'all' | 'success' | 'failed'>('all');

  // 切换任务时重置选中与筛选
  useEffect(() => {
    setSelected(null);
    setFilter('all');
  }, [task?.name]);

  const filtered = runs.filter(r => {
    if (filter !== 'all' && r.status !== filter) return false;
    return true;
  });

  const selectedRun = runs.find(r => r.invocation_id === selected) || null;

  const handleDateRangeChange = (_dates: unknown, dateStrings: [string, string]) => {
    const since = dateStrings[0] ? `${dateStrings[0]} 00:00:00` : undefined;
    const until = dateStrings[1] ? `${dateStrings[1]} 23:59:59` : undefined;
    onDateRangeChange(since, until);
  };

  return (
    <Modal
      title={<Space><HistoryOutlined /> {task?.name} - 执行记录</Space>}
      open={visible}
      onCancel={onClose}
      footer={<Button onClick={onClose}>关闭</Button>}
      width={1300}
      styles={{ body: { padding: 0 } }}
    >
      <div style={{ display: 'flex', height: 600 }}>
        {/* 左侧：执行列表 */}
        <div style={{ width: 440, borderRight: '1px solid #f0f0f0', display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 12, borderBottom: '1px solid #f0f0f0', display: 'flex', gap: 8, alignItems: 'center' }}>
            <DatePicker.RangePicker
              placeholder={['开始日期', '结束日期']}
              onChange={handleDateRangeChange}
              style={{ width: 220 }}
              allowClear
            />
            <Segmented
              value={filter}
              onChange={(v) => setFilter(v as typeof filter)}
              options={[
                { label: '全部', value: 'all' },
                { label: '成功', value: 'success' },
                { label: '失败', value: 'failed' },
              ]}
            />
            <Button icon={<ReloadOutlined />} onClick={() => task && onRefresh(task)} />
          </div>
          <List
            size="small"
            loading={loading}
            dataSource={filtered}
            pagination={{
              current: page,
              pageSize,
              total,
              size: 'small',
              showSizeChanger: true,
              onChange: onPageChange,
            }}
            style={{ flex: 1, overflowY: 'auto', padding: 12 }}
            locale={{ emptyText: <Empty description="暂无执行记录" /> }}
            renderItem={(r: CronRun) => (
              <List.Item
                onClick={() => setSelected(r.invocation_id)}
                style={{
                  cursor: 'pointer',
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
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          {selectedRun ? (
            <LogViewer
              entries={selectedRun.logs.map((l) => ({
                text: l.message,
                time: l.time,
                level: l.priority,
              }))}
              downloadFileName={`cron_${task?.name}_${selectedRun.started_at}`}
              emptyText="该次执行无日志输出"
              style={{ flex: 1, border: 'none', borderRadius: 0, height: '100%' }}
            />
          ) : (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Empty description="选择左侧某次执行查看日志" />
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}
