import { useState, useEffect, useCallback } from 'react';
import { message } from 'antd';
import type { CronTask, Script } from '../../types';
import { cronApi } from '../../services/cron';
import CronTasks from './CronTasks';
import CronLogs from './CronLogs';
import CronDocs from './CronDocs';

export default function CronPage() {
  const [tasks, setTasks] = useState<CronTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [operating, setOperating] = useState('');
  const [scripts, setScripts] = useState<Script[]>([]);

  // Logs modal state
  const [logsVisible, setLogsVisible] = useState(false);
  const [logsTask, setLogsTask] = useState<CronTask | null>(null);

  // Docs drawer state
  const [helpVisible, setHelpVisible] = useState(false);

  const fetchTasks = useCallback(async () => {
    setLoading(true);
    try {
      const res = await cronApi.list(page, pageSize);
      setTasks(res.data?.data?.items ?? []);
      setTotal(res.data?.data?.total ?? 0);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取任务列表失败'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => { fetchTasks(); }, [fetchTasks]);

  // Fetch scripts on mount
  useEffect(() => {
    cronApi.listScripts(1, 1000).then(res => {
      setScripts(res.data?.data?.items ?? []);
    }).catch(() => {});
  }, []);

  const handleDelete = async (task: CronTask) => {
    try {
      await cronApi.delete(task.name);
      message.success('任务已删除');
      fetchTasks();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleToggle = async (task: CronTask) => {
    setOperating(`toggle-${task.name}`);
    try {
      if (task.enabled) {
        await cronApi.disable(task.name);
        message.success('任务已禁用');
      } else {
        await cronApi.enable(task.name);
        message.success('任务已启用');
      }
      fetchTasks();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    } finally {
      setOperating('');
    }
  };

  const handleRun = async (task: CronTask) => {
    setOperating(`run-${task.name}`);
    try {
      await cronApi.run(task.name);
      message.success('任务已执行');
      fetchTasks();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '执行失败'));
    } finally {
      setOperating('');
    }
  };

  const handleViewLogs = (task: CronTask) => {
    setLogsTask(task);
    setLogsVisible(true);
  };

  const handleShowHelp = useCallback(() => {
    setHelpVisible(true);
  }, []);

  return (
    <div>
      <CronTasks
        tasks={tasks}
        loading={loading}
        page={page}
        pageSize={pageSize}
        total={total}
        onPageChange={(p, ps) => { setPage(p); setPageSize(ps); }}
        operating={operating}
        scripts={scripts}
        onRefresh={fetchTasks}
        onDelete={handleDelete}
        onToggle={handleToggle}
        onRun={handleRun}
        onViewLogs={handleViewLogs}
        onShowHelp={handleShowHelp}
      />
      <CronLogs
        visible={logsVisible}
        task={logsTask}
        onClose={() => setLogsVisible(false)}
      />
      <CronDocs
        visible={helpVisible}
        onClose={() => setHelpVisible(false)}
      />
    </div>
  );
}