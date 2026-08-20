import { useState, useCallback, useEffect, useRef } from 'react';
import {
  Card, Button, Space, Modal, Form, Input, InputNumber, Select, Switch,
  message, Popconfirm, Table, Empty, Spin, Tooltip, Segmented, Radio, Row, Col, ConfigProvider,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, PlayCircleOutlined,
  DeleteOutlined, EditOutlined, HistoryOutlined,
  ClockCircleOutlined, EyeOutlined, QuestionCircleOutlined,
} from '@ant-design/icons';
import type { CronTask, Script } from '../../types';
import { cronApi } from '../../services/cron';
import { buildOnCalendar, describeSchedule, computeNextRun, describeOnCalendar, type ScheduleForm } from './schedule';
import RuntimeVersionSelect from '../../components/RuntimeVersionSelect';

interface CronTasksProps {
  tasks: CronTask[];
  loading: boolean;
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number, pageSize: number) => void;
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
  { value: 'secondly', label: '每 N 秒' },
  { value: 'minutely', label: '每 N 分钟' },
  { value: 'hourly', label: '每 N 小时' },
  { value: 'every_n_days', label: '每 N 天' },
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

interface EnvEntry { key: string; value: string; }

// 后端 env_vars 为 "KEY=VALUE\n..." 每行一个，转为表单 KV 列表。
function parseEnvVars(raw: string): EnvEntry[] {
  return raw.split('\n')
    .map(s => s.trim())
    .filter(Boolean)
    .map(line => {
      const i = line.indexOf('=');
      return i === -1 ? { key: line, value: '' } : { key: line.slice(0, i), value: line.slice(i + 1) };
    });
}

// 表单 KV 列表 → 后端字符串，跳过空行。
function serializeEnvVars(entries: EnvEntry[] | undefined): string {
  return (entries || [])
    .filter(e => e.key.trim() || e.value.trim())
    .map(e => `${e.key.trim()}=${e.value}`)
    .join('\n');
}

export default function CronTasks({
  tasks, loading, page, pageSize, total, onPageChange, operating, scripts,
  onRefresh, onDelete, onToggle, onRun, onViewLogs, onShowHelp,
}: CronTasksProps) {
  const [modalVisible, setModalVisible] = useState(false);
  const [editingTask, setEditingTask] = useState<CronTask | null>(null);
  const [form] = Form.useForm();
  const [mode, setMode] = useState<'preset' | 'manual'>('preset');
  const [useType, setUseType] = useState<'command' | 'script'>('command');
  const [frequency, setFrequency] = useState<string>('daily');
  const [formDesc, setFormDesc] = useState('');
  const [descLoading, setDescLoading] = useState(false);
  const [nextRun, setNextRun] = useState('');
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
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
      const { on_calendar, description } = describeSchedule(formData);
      setFormDesc(`${description}（${on_calendar}）`);
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
        onCalendar = buildOnCalendar(formData);
      }
      if (!onCalendar) {
        setNextRun('请先填写调度表达式');
        return;
      }
      setNextRun(computeNextRun(onCalendar, new Date()) || '无法解析调度');
    } finally {
      setPreviewLoading(false);
    }
  }, [form, mode]);

  const handleCreate = () => {
    setEditingTask(null);
    form.resetFields();
    form.setFieldsValue({ frequency: 'daily', every_n: 5, time: '03:00', weekdays: ['Mon'], day_of_month: 1, envs: [] });
    setMode('preset');
    setUseType('command');
    setFrequency('daily');
    setFormDesc('');
    setNextRun('');
    setModalVisible(true);
  };

  const handleEdit = (task: CronTask) => {
    setEditingTask(task);
    // 后端只存 OnCalendar 表达式，编辑时一律以表达式回显（手动模式）。
    setMode('manual');
    // 通过脚本路径匹配判断任务是否关联脚本（请求统一为 command）。
    const script = scripts.find(s => s.path === task.command);
    setUseType(script ? 'script' : 'command');
    form.setFieldsValue({
      name: task.name,
      command: task.command,
      schedule: task.schedule,
      persistent: task.persistent,
      description: task.description,
      script_exec: script ? script.path : undefined,
      timeout: task.timeout || 0,
      max_retry: task.max_retry || 0,
      envs: parseEnvVars(task.env_vars || ''),
      work_dir: task.work_dir || '',
      runtime: task.runtime || undefined,
    });
    setFormDesc('');
    setNextRun('');
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      const values = await form.validateFields();
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
        schedule = buildOnCalendar(formData);
      }
      if (!schedule) {
        message.error('请填写调度表达式');
        return;
      }
      const payload = {
        command: useType === 'command' ? (values.command || '') : (values.script_exec || ''),
        schedule,
        persistent: !!values.persistent,
        description: values.description || '',
        script_id: 0,
        timeout: values.timeout || 0,
        max_retry: values.max_retry || 0,
        env_vars: serializeEnvVars(values.envs),
        work_dir: values.work_dir || '',
        runtime: values.runtime,
      };
      if (editingTask) {
        await cronApi.update(editingTask.name, { ...payload, name: values.name });
        message.success('任务已更新');
      } else {
        await cronApi.create({ ...payload, name: values.name, runtime: values.runtime });
        message.success('任务已创建');
      }
      setModalVisible(false);
      onRefresh();
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : String(error);
      if (msg) message.error(msg);
    } finally {
      setSubmitting(false);
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
      title: '执行周期',
      dataIndex: 'schedule',
      key: 'schedule',
      width: 170,
      render: (schedule: string) => (
        <Tooltip title={schedule}>
          <span>{describeOnCalendar(schedule)}</span>
        </Tooltip>
      ),
    },
    {
      title: '工作目录',
      dataIndex: 'work_dir',
      key: 'work_dir',
      width: 140,
      ellipsis: true,
      render: (workDir: string) => workDir || '-',
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (description: string) => description || '-',
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
          checkedChildren="启用"
          unCheckedChildren="禁用"
        />
      ),
    },
    {
      title: '上次执行时间',
      dataIndex: 'last_run',
      key: 'last_run',
      width: 200,
      render: (lastRun: string) => lastRun || '-',
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
          <Tooltip title="执行记录">
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
            description="删除后将无法恢复，任务及其执行记录会被一并移除"
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
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 个任务`,
            onChange: onPageChange,
          }}
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
        confirmLoading={submitting}
        cancelText="取消"
        style={{ top: 40 }}
        destroyOnHidden
        zIndex={1000}
        styles={{ body: { maxHeight: 'calc(100vh - 220px)', overflowY: 'auto', paddingRight: 8 } }}
      >
        <ConfigProvider theme={{ components: { Form: { itemMarginBottom: 16 } } }}>
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
              <RowForm frequency={frequency} setFrequency={(f) => { setFrequency(f); form.setFieldsValue({ frequency: f }); }} onShowHelp={onShowHelp} />
              <div style={{ color: '#8c8c8c', fontSize: 12, marginBottom: 16, minHeight: 18 }}>
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
          <Form.Item
            name="name"
            label="任务名称"
            rules={[
              { required: true, message: '请输入任务名称' },
              { pattern: /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/, message: '名称只能包含小写字母、数字、连字符，且不能以连字符开头/结尾' },
            ]}
          >
            <Input placeholder="例：daily-backup（小写字母/数字/连字符）" />
          </Form.Item>
          <Form.Item label="执行内容">
            <Radio.Group value={useType} onChange={(e) => setUseType(e.target.value)}>
              <Radio.Button value="command">执行命令</Radio.Button>
              <Radio.Button value="script">关联脚本</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item
            name={useType === 'command' ? 'command' : 'script_exec'}
            label={useType === 'command' ? '执行命令' : '关联脚本'}
            rules={[{ required: true, message: useType === 'command' ? '请填写执行命令' : '请选择脚本' }]}
            extra={useType === 'command' ? '任务将在所选运行时环境中执行，支持管道、脚本调用等 Shell 用法' : undefined}
          >
            {useType === 'command' ? (
              <Input.TextArea rows={2} placeholder="例：/opt/scripts/backup.sh" />
            ) : (
              <Select
                placeholder="选择脚本"
                options={scripts.map(s => ({ label: s.name, value: s.path }))}
              />
            )}
          </Form.Item>
          <Form.Item name="runtime" label="运行时版本" extra="选择已安装的运行时版本（lang@exact），留空则使用系统 PATH">
            <RuntimeVersionSelect />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="任务描述（可选）" />
          </Form.Item>
          <Form.Item name="work_dir" label="工作目录">
            <Input placeholder="例：/opt/app（可选）" />
          </Form.Item>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="timeout" label="超时时间（秒）" extra="0 = 不超时">
                <InputNumber style={{ width: '100%' }} placeholder="0" min={0} max={86400} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="max_retry" label="失败重试次数" extra="0 = 不重试">
                <InputNumber style={{ width: '100%' }} placeholder="0" min={0} max={10} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="persistent" label="持久化执行" valuePropName="checked" extra="系统关机或休眠期间错过的执行计划，将在下次开机时自动补齐执行">
            <Switch />
          </Form.Item>
          <Form.Item label="环境变量">
            <Form.List name="envs">
              {(fields, { add, remove }) => (
                <>
                  {fields.map((field) => (
                    <div key={field.key} style={{ display: 'flex', gap: 8, marginBottom: 8 }} className="cron-env-row">
                      <Form.Item {...field} name={[field.name, 'key']} rules={[{ required: true, whitespace: true, message: '请填写变量名' }]} noStyle>
                        <Input placeholder="KEY" style={{ fontFamily: 'monospace', flex: 1, minWidth: 0 }} />
                      </Form.Item>
                      <span style={{ color: '#999', lineHeight: '32px' }}>=</span>
                      <Form.Item {...field} name={[field.name, 'value']} noStyle>
                        <Input placeholder="VALUE" style={{ fontFamily: 'monospace', flex: 1, minWidth: 0 }} />
                      </Form.Item>
                      <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(field.name)} />
                    </div>
                  ))}
                  <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                    添加环境变量
                  </Button>
                </>
              )}
            </Form.List>
          </Form.Item>
        </Form>
        </ConfigProvider>
      </Modal>

      {/* Next Run Preview Modal */}
      <Modal
        title={<Space><EyeOutlined /> 下次执行时间</Space>}
        open={previewVisible}
        onCancel={() => { setPreviewVisible(false); setNextRun(''); }}
        footer={<Button onClick={() => { setPreviewVisible(false); setNextRun(''); }}>关闭</Button>}
        width={400}
        zIndex={1200}
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
function RowForm({ frequency, setFrequency, onShowHelp }: { frequency: string; setFrequency: (f: string) => void; onShowHelp?: () => void }) {
  return (
    <>
      <Form.Item
        name="frequency"
        label={
          <Space>
            <span>调度频率</span>
            {onShowHelp && (
              <Tooltip title="查看使用手册">
                <Button type="link" size="small" icon={<QuestionCircleOutlined />} onClick={onShowHelp} />
              </Tooltip>
            )}
          </Space>
        }
      >
        <Select options={FREQUENCY_OPTIONS} onChange={setFrequency} />
      </Form.Item>
      {(frequency === 'secondly' || frequency === 'minutely' || frequency === 'hourly') && (
        <Form.Item name="every_n" label={frequency === 'secondly' ? '每 N 秒' : frequency === 'minutely' ? '每 N 分钟' : '每 N 小时'}>
          <InputNumber min={1} max={frequency === 'secondly' ? 3600 : 60} />
        </Form.Item>
      )}
      {(frequency === 'every_n_days' || frequency === 'daily' || frequency === 'weekly') && (
        <Form.Item name="time" label="触发时间（HH:MM）">
          <Input placeholder="例：03:00" />
        </Form.Item>
      )}
      {frequency === 'every_n_days' && (
        <Form.Item name="every_n" label="每 N 天">
          <InputNumber min={1} max={30} />
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
