import { useState, useCallback, useEffect, useRef } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber, Select, Switch,
  message, Popconfirm, Table, Empty, Spin, Tooltip, Segmented,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, PlayCircleOutlined,
  DeleteOutlined, EditOutlined, HistoryOutlined,
  ClockCircleOutlined, EyeOutlined, QuestionCircleOutlined,
} from '@ant-design/icons';
import type { CronTask, Script, ScheduleForm } from '../../types';
import { cronApi } from '../../services/api';
import { STYLES } from './types';
import RuntimeVersionSelect from '../../components/RuntimeVersionSelect';

interface CronTasksProps {
  tasks: CronTask[];
  loading: boolean;
  operating: string;
  scripts: Script[];
  onRefresh: () => void;
  onDelete: (task: CronTask) => void;
  onToggle: (task: CronTask) => void;
  onRun: (task: CronTask) => void;
  onViewLogs: (task: CronTask) => void;
  onShowHelp?: () => void;
}

// 频率选项：value 对应后端 ScheduleForm.Frequency
const FREQUENCY_OPTIONS = [
  { value: 'minutely', label: '每 N 分钟' },
  { value: 'hourly', label: '每 N 小时' },
  { value: 'daily', label: '每天固定时间' },
  { value: 'weekly', label: '每周固定几天' },
  { value: 'monthly', label: '每月固定日' },
];

const WEEKDAY_OPTIONS = [
  { value: 'Mon', label: '周一' },
  { value: 'Tue', label: '周二' },
  { value: 'Wed', label: '周三' },
  { value: 'Thu', label: '周四' },
  { value: 'Fri', label: '周五' },
  { value: 'Sat', label: '周六' },
  { value: 'Sun', label: '周日' },
];

export default function CronTasks({
  tasks, loading, operating, scripts,
  onRefresh, onDelete, onToggle, onRun, onViewLogs, onShowHelp,
}: CronTasksProps) {
  const [modalVisible, setModalVisible] = useState(false);
  const [editingTask, setEditingTask] = useState<CronTask | null>(null);
  const [form] = Form.useForm();
  const [mode, setMode] = useState<'preset' | 'manual'>('preset');
  const [frequency, setFrequency] = useState<string>('daily');
  const [formDesc, setFormDesc] = useState('');
  const [descLoading, setDescLoading] = useState(false);
  const [nextRun, setNextRun] = useState('');
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (previewTimer.current) clearTimeout(previewTimer.current);
    };
  }, []);

  // 调度表单变化 → 请求后端转 OnCalendar + 描述（仅预设模式）
  const refreshFormDesc = useCallback(async () => {
    if (mode === 'manual') return;
    const v = form.getFieldsValue();
    const formData: ScheduleForm = {
      frequency: v.frequency,
      every_n: v.every_n || 5,
      time: v.time || '03:00',
      weekdays: v.weekdays?.length ? v.weekdays : ['Mon'],
      day_of_month: v.day_of_month || 1,
    };
    setDescLoading(true);
    try {
      const res = await cronApi.describeSchedule(formData);
      setFormDesc(`${res.data?.data?.description || ''}（${res.data?.data?.on_calendar || ''}）`);
    } catch {
      setFormDesc('调度表单无效');
    } finally {
      setDescLoading(false);
    }
  }, [form, mode]);

  const scheduleChanged = useCallback(() => {
    if (previewTimer.current) clearTimeout(previewTimer.current);
    previewTimer.current = setTimeout(() => { refreshFormDesc(); }, 300);
  }, [refreshFormDesc]);

  const handlePreview = useCallback(async () => {
    setPreviewLoading(true);
    setPreviewVisible(true);
    try {
      let onCalendar = '';
      if (mode === 'manual') {
        onCalendar = form.getFieldValue('schedule')?.trim() || '';
      } else {
        const v = form.getFieldsValue();
        const formData: ScheduleForm = {
          frequency: v.frequency,
          every_n: v.every_n || 5,
          time: v.time || '03:00',
          weekdays: v.weekdays?.length ? v.weekdays : ['Mon'],
          day_of_month: v.day_of_month || 1,
        };
        const res = await cronApi.describeSchedule(formData);
        onCalendar = res.data?.data?.on_calendar || '';
      }
      if (!onCalendar) {
        setNextRun('请先填写调度表达式');
        return;
      }
      const nres = await cronApi.getNextRun(onCalendar);
      setNextRun(nres.data?.data?.next_run || '');
    } catch {
      setNextRun('无法解析调度');
    } finally {
      setPreviewLoading(false);
    }
  }, [form, mode]);

  const handleCreate = () => {
    setEditingTask(null);
    form.resetFields();
    form.setFieldsValue({ frequency: 'daily', every_n: 5, time: '03:00', weekdays: ['Mon'], day_of_month: 1 });
    setMode('preset');
    setFrequency('daily');
    setFormDesc('');
    setNextRun('');
    setModalVisible(true);
  };

  const handleEdit = (task: CronTask) => {
    setEditingTask(task);
    // 后端只存 OnCalendar 表达式，编辑时一律以表达式回显（手动模式）。
    setMode('manual');
    form.setFieldsValue({
      name: task.name,
      command: task.command,
      schedule: task.schedule,
      persistent: task.persistent,
      description: task.description,
      script_id: task.script_id || undefined,
      timeout: task.timeout || 0,
      max_retry: task.max_retry || 0,
      env_vars: task.env_vars || '',
      work_dir: task.work_dir || '',
      runtime_version_id: task.runtime_version_id || undefined,
    });
    setFormDesc('');
    setNextRun('');
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (!values.command && !values.script_id) {
        message.error('请填写执行命令或选择脚本');
        return;
      }
      // 预设频率 → 先转 OnCalendar 表达式，再提交（后端只收表达式）
      let schedule = values.schedule?.trim() || '';
      if (mode === 'preset') {
        const formData: ScheduleForm = {
          frequency: values.frequency,
          every_n: values.every_n || 5,
          time: values.time || '03:00',
          weekdays: values.weekdays?.length ? values.weekdays : ['Mon'],
          day_of_month: values.day_of_month || 1,
        };
        const res = await cronApi.describeSchedule(formData);
        schedule = res.data?.data?.on_calendar || '';
      }
      if (!schedule) {
        message.error('请填写调度表达式');
        return;
      }
      const payload = {
        command: values.command || '',
        schedule,
        persistent: !!values.persistent,
        description: values.description || '',
        script_id: values.script_id || 0,
        timeout: values.timeout || 0,
        max_retry: values.max_retry || 0,
        env_vars: values.env_vars || '',
        work_dir: values.work_dir || '',
        runtime_version_id: values.runtime_version_id,
      };
      if (editingTask) {
        await cronApi.update(editingTask.name, { ...payload, name: values.name });
        message.success('任务已更新');
      } else {
        await cronApi.create({ ...payload, name: values.name, runtime_version_id: values.runtime_version_id });
        message.success('任务已创建');
      }
      setModalVisible(false);
      onRefresh();
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : String(error);
      if (msg) message.error(msg);
    }
  };

  // systemd 状态图标：active / inactive / failed
  const statusIcon = (status: string) => {
    switch (status) {
      case 'active': return <PlayCircleOutlined style={{ color: '#52c41a' }} />;
      case 'failed': return <ClockCircleOutlined style={{ color: '#ff4d4f' }} />;
      default: return <ClockCircleOutlined style={{ color: '#8c8c8c' }} />;
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 150,
      render: (name: string, record: CronTask) => (
        <Space>
          {statusIcon(record.status)}
          <span>{name}</span>
        </Space>
      ),
    },
    {
      title: '调度',
      dataIndex: 'schedule',
      key: 'schedule',
      width: 170,
      render: (schedule: string) => (
        <Tag style={STYLES.scheduleTag}>{schedule}</Tag>
      ),
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
      render: (command: string, record: CronTask) => {
        if (record.script_id > 0) {
          return <Tag color="blue">脚本 #{record.script_id}</Tag>;
        }
        return command;
      },
    },
    {
      title: '超时/重试',
      key: 'config',
      width: 120,
      render: (_: unknown, record: CronTask) => (
        <Space size={4}>
          {record.timeout > 0 && <Tag>{record.timeout}s</Tag>}
          {record.max_retry > 0 && <Tag color="orange">重试{record.max_retry}</Tag>}
          {record.timeout === 0 && record.max_retry === 0 && <span style={{ color: '#8c8c8c' }}>-</span>}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled: boolean, record: CronTask) => (
        <Switch
          checked={enabled}
          onChange={() => onToggle(record)}
          loading={operating === `toggle-${record.name}`}
          size="small"
        />
      ),
    },
    {
      title: '下次执行',
      dataIndex: 'next_run',
      key: 'next_run',
      width: 160,
      render: (nextRun: string) => nextRun || '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_: unknown, record: CronTask) => (
        <Space>
          <Tooltip title="立即执行">
            <Button
              type="link"
              icon={<PlayCircleOutlined />}
              onClick={() => onRun(record)}
              loading={operating === `run-${record.name}`}
              disabled={!record.enabled}
            />
          </Tooltip>
          <Tooltip title="执行日志">
            <Button
              type="link"
              icon={<HistoryOutlined />}
              onClick={() => onViewLogs(record)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Popconfirm
            title="确定删除此任务？"
            description="删除后将无法恢复，任务及其执行日志会被一并移除"
            onConfirm={() => onDelete(record)}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Tooltip title="删除">
              <Button type="link" icon={<DeleteOutlined />} danger />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Card
        title={<Space><ClockCircleOutlined /> 计划任务</Space>}
        extra={
          <Space>
            {onShowHelp && (
              <Button icon={<QuestionCircleOutlined />} onClick={onShowHelp}>使用手册</Button>
            )}
            <Button icon={<ReloadOutlined />} onClick={onRefresh} loading={loading}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>创建任务</Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={tasks}
          rowKey="name"
          loading={loading}
          size="small"
          locale={{ emptyText: <Empty description="暂无计划任务" /> }}
        />
      </Card>

      {/* Create/Edit Modal */}
      <Modal
        title={editingTask ? '编辑任务' : '创建任务'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmit}
        okText={editingTask ? '保存' : '创建'}
        cancelText="取消"
        style={STYLES.modal}
        destroyOnHidden
      >
        <Form
          form={form}
          layout="vertical"
          onValuesChange={scheduleChanged}
          initialValues={{ frequency: 'daily', every_n: 5, time: '03:00', weekdays: ['Mon'], day_of_month: 1 }}
        >
          <Form.Item label="调度方式">
            <Segmented
              block
              value={mode}
              onChange={(v) => setMode(v as 'preset' | 'manual')}
              options={[
                { label: '预设频率', value: 'preset' },
                { label: '自定义表达式', value: 'manual' },
              ]}
            />
          </Form.Item>
          {mode === 'preset' ? (
            <>
              <RowForm frequency={frequency} setFrequency={(f) => { setFrequency(f); form.setFieldsValue({ frequency: f }); }} />
              <div style={STYLES.description}>
                {descLoading ? <Spin size="small" /> : (formDesc || <span style={{ color: '#8c8c8c' }}>选择调度频率后，将自动生成对应的执行计划</span>)}
                <Button type="link" size="small" icon={<EyeOutlined />} onClick={handlePreview} loading={previewLoading}>预览下次执行</Button>
              </div>
            </>
          ) : (
            <Form.Item
              name="schedule"
              label={
                <Space>
                  <span>调度表达式</span>
                  {onShowHelp && (
                    <Tooltip title="查看使用手册">
                      <Button type="link" size="small" icon={<QuestionCircleOutlined />} onClick={onShowHelp} />
                    </Tooltip>
                  )}
                </Space>
              }
              rules={[{ required: true, message: '请输入调度表达式' }]}
              extra="例：*-*-* 03:00:00（每天 3 点）、*:00/5（每 5 分钟）、Mon..Fri *-*-* 09:00:00（工作日 9 点）"
            >
              <Input
                placeholder="例：*-*-* 03:00:00"
                style={{ fontFamily: 'monospace' }}
                suffix={
                  <Tooltip title="预览下次执行">
                    <Button type="text" size="small" icon={<EyeOutlined />} onClick={handlePreview} loading={previewLoading} />
                  </Tooltip>
                }
              />
            </Form.Item>
          )}
          <Form.Item name="name" label="任务名称" rules={[{ required: true, message: '请输入任务名称' }]}>
            <Input placeholder="例：daily-backup（小写字母/数字/连字符）" />
          </Form.Item>
          <Form.Item name="command" label="执行命令" extra="任务将在所选运行时环境中执行，支持管道、脚本调用等 Shell 用法">
            <Input.TextArea rows={2} placeholder="例：/opt/scripts/backup.sh（与脚本二选一）" />
          </Form.Item>
          <Form.Item name="script_id" label="关联脚本">
            <Select
              placeholder="选择脚本（与命令二选一）"
              allowClear
              options={scripts.map(s => ({ label: `${s.name} (${s.language})`, value: s.id }))}
            />
          </Form.Item>
          <Form.Item
            name="runtime_version_id"
            label="运行时版本"
            rules={[{ required: true, message: '请选择已安装的运行时版本' }]}
            getValueFromEvent={(v?: { id: number }) => v?.id}
            getValueProps={(v: number) => ({ value: v ? { id: v } : undefined })}
          >
            <RuntimeVersionSelect />
          </Form.Item>
          <Form.Item name="persistent" label="持久化执行" valuePropName="checked" extra="系统关机或休眠期间错过的执行计划，将在下次开机时自动补齐执行">
            <Switch />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="任务描述（可选）" />
          </Form.Item>
          <Form.Item name="work_dir" label="工作目录" extra="可选">
            <Input placeholder="例：/opt/app" />
          </Form.Item>
          <Form.Item name="timeout" label="超时时间（秒）" extra="0 = 不超时">
            <InputNumber style={{ width: '100%' }} placeholder="0" min={0} max={86400} />
          </Form.Item>
          <Form.Item name="max_retry" label="失败重试次数" extra="0 = 不重试">
            <InputNumber style={{ width: '100%' }} placeholder="0" min={0} max={10} />
          </Form.Item>
          <Form.Item name="env_vars" label="环境变量">
            <Input.TextArea rows={4} placeholder={'每行一个\nKEY=VALUE'} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Next Run Preview Modal */}
      <Modal
        title={<Space><EyeOutlined /> 下次执行时间</Space>}
        open={previewVisible}
        onCancel={() => { setPreviewVisible(false); setNextRun(''); }}
        footer={<Button onClick={() => { setPreviewVisible(false); setNextRun(''); }}>关闭</Button>}
        width={400}
      >
        {previewLoading ? (
          <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
        ) : (
          <div style={{ textAlign: 'center', padding: 16 }}>{nextRun}</div>
        )}
      </Modal>
    </>
  );
}

// 调度表单：频率 + 条件子字段
function RowForm({ frequency, setFrequency }: { frequency: string; setFrequency: (f: string) => void }) {
  return (
    <>
      <Form.Item name="frequency" label={<Space>调度频率 <Tooltip title="查看使用手册"><Button type="link" size="small" icon={<QuestionCircleOutlined />} /></Tooltip></Space>}>
        <Select options={FREQUENCY_OPTIONS} onChange={setFrequency} />
      </Form.Item>
      {(frequency === 'minutely' || frequency === 'hourly') && (
        <Form.Item name="every_n" label={frequency === 'minutely' ? '每 N 分钟' : '每 N 小时'}>
          <InputNumber min={1} max={60} />
        </Form.Item>
      )}
      {(frequency === 'daily' || frequency === 'weekly') && (
        <Form.Item name="time" label="触发时间（HH:MM）">
          <Input placeholder="例：03:00" />
        </Form.Item>
      )}
      {frequency === 'weekly' && (
        <Form.Item name="weekdays" label="星期">
          <Select mode="multiple" options={WEEKDAY_OPTIONS} placeholder="选择星期" />
        </Form.Item>
      )}
      {frequency === 'monthly' && (
        <>
          <Form.Item name="day_of_month" label="每月第几号">
            <InputNumber min={1} max={31} />
          </Form.Item>
          <Form.Item name="time" label="触发时间（HH:MM）">
            <Input placeholder="例：03:00" />
          </Form.Item>
        </>
      )}
    </>
  );
}