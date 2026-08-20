import { useState, useEffect, useCallback, type UIEvent } from 'react';
import { Modal, Tag, Button, Space, Empty, List, Segmented, DatePicker, Spin, theme, message } from 'antd';
import { HistoryOutlined, ReloadOutlined } from '@ant-design/icons';
import type { CronTask, CronRun } from '../../types';
import { cronApi } from '../../services/cron';
import { LogViewer } from '../../components/LogViewer';

interface CronLogsProps {
  visible: boolean;
  task: CronTask | null;
  onClose: () => void;
}

function statusTag(status: string) {
  switch (status) {
    case 'success': return <Tag color="success">成功</Tag>;
    case 'failed': return <Tag color="error">失败</Tag>;
    default: return <Tag color="processing">执行中</Tag>;
  }
}

const PAGE_SIZE = 20;

export default function CronLogs({
  visible, task, onClose,
}: CronLogsProps) {
  const { token } = theme.useToken();
  const [runs, setRuns] = useState<CronRun[]>([]);
  const [page, setPage] = useState<number>(1);
  const [hasMore, setHasMore] = useState<boolean>(true);
  const [loading, setLoading] = useState<boolean>(false);
  const [loadingMore, setLoadingMore] = useState<boolean>(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [filter, setFilter] = useState<'all' | 'success' | 'failed'>('all');
  const [since, setSince] = useState<string | undefined>(undefined);
  const [until, setUntil] = useState<string | undefined>(undefined);

  // 加载数据（初始加载或滚动翻页）
  const loadRuns = useCallback(async (
    targetPage: number,
    isInitial: boolean,
    targetSince = since,
    targetUntil = until
  ) => {
    if (!task?.name) return;
    if (isInitial) {
      setLoading(true);
    } else {
      setLoadingMore(true);
    }

    try {
      const res = await cronApi.getRuns(task.name, targetPage, PAGE_SIZE, targetSince, targetUntil);
      const items = res.data?.data?.items ?? [];
      const total = res.data?.data?.total ?? 0;

      if (isInitial) {
        setRuns(items);
        setSelected(items[0]?.invocation_id ?? null);
      } else {
        setRuns((prev) => [...prev, ...items]);
      }

      setHasMore(targetPage * PAGE_SIZE < total);
      setPage(targetPage);
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '获取执行记录失败');
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, [task, since, until]);

  // 打开弹窗或切换任务时重置并初始加载
  useEffect(() => {
    if (visible && task?.name) {
      setSelected(null);
      setFilter('all');
      setSince(undefined);
      setUntil(undefined);
      loadRuns(1, true, undefined, undefined);
    }
  }, [visible, task, loadRuns]);

  // 滚动触底分页监听
  const handleScroll = (e: UIEvent<HTMLDivElement>) => {
    const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
    if (scrollHeight - scrollTop - clientHeight < 40 && hasMore && !loading && !loadingMore) {
      loadRuns(page + 1, false);
    }
  };

  const handleDateRangeChange = (_dates: unknown, dateStrings: [string, string]) => {
    const s = dateStrings[0] ? `${dateStrings[0]} 00:00:00` : undefined;
    const u = dateStrings[1] ? `${dateStrings[1]} 23:59:59` : undefined;
    setSince(s);
    setUntil(u);
    loadRuns(1, true, s, u);
  };

  const handleRefresh = () => {
    loadRuns(1, true);
  };

  const filteredRuns = runs.filter((r) => {
    if (filter !== 'all' && r.status !== filter) return false;
    return true;
  });

  const selectedRun = runs.find((r) => r.invocation_id === selected) || null;

  return (
    <Modal
      title={<Space><HistoryOutlined /> {task?.name} - 执行记录</Space>}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={1300}
      destroyOnHidden
      styles={{ body: { padding: 0 } }}
    >
      <div style={{ display: 'flex', height: 600 }}>
        {/* 左侧：执行列表（无限滚动触底分页） */}
        <div
          style={{
            width: 440,
            borderRight: `1px solid ${token.colorBorderSecondary}`,
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <div
            style={{
              padding: 12,
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
              display: 'flex',
              gap: 8,
              alignItems: 'center',
            }}
          >
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
            <Button icon={<ReloadOutlined />} onClick={handleRefresh} />
          </div>

          <div
            onScroll={handleScroll}
            style={{
              flex: 1,
              overflowY: 'auto',
              padding: 12,
            }}
          >
            <List
              size="small"
              loading={loading}
              dataSource={filteredRuns}
              pagination={false}
              locale={{ emptyText: <Empty description="暂无执行记录" /> }}
              renderItem={(r: CronRun) => (
                <List.Item
                  onClick={() => setSelected(r.invocation_id)}
                  style={{
                    cursor: 'pointer',
                    borderRadius: token.borderRadiusSM,
                    padding: '8px 12px',
                    backgroundColor: selected === r.invocation_id ? token.controlItemBgActive : 'transparent',
                    color: selected === r.invocation_id ? token.colorPrimary : undefined,
                    transition: 'background-color 0.2s ease, color 0.2s ease',
                  }}
                >
                  <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                    <span style={{ fontSize: 12, fontWeight: selected === r.invocation_id ? 500 : 400 }}>
                      {r.started_at}
                    </span>
                    {statusTag(r.status)}
                  </Space>
                </List.Item>
              )}
            />

            {/* 滚动触底加载状态指示 */}
            {loadingMore && (
              <div style={{ textAlign: 'center', padding: '12px 0' }}>
                <Space>
                  <Spin size="small" />
                  <span style={{ fontSize: 12, color: token.colorTextTertiary }}>加载更多记录...</span>
                </Space>
              </div>
            )}
            {!hasMore && runs.length > 0 && (
              <div style={{ textAlign: 'center', padding: '12px 0', fontSize: 12, color: token.colorTextTertiary }}>
                已加载全部记录（共 {runs.length} 条）
              </div>
            )}
          </div>
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
