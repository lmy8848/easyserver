import { useState, useCallback, useEffect, useRef } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber, Select, Switch,
  message, Popconfirm, Table, Empty, Spin, Tooltip, List, Row, Col, Collapse,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, PlayCircleOutlined,
  DeleteOutlined, EditOutlined, HistoryOutlined,
  ClockCircleOutlined, CheckCircleOutlined, CloseCircleOutlined,
  LoadingOutlined, EyeOutlined, QuestionCircleOutlined,
} from '@ant-design/icons';
import type { CronTask, Script } from '../../types';
import { cronApi } from '../../services/api';
import { STYLES, type Preset } from './types';

interface CronTasksProps {
  tasks: CronTask[];
  loading: boolean;
  operating: string;
  presets: Preset[];
  scripts: Script[];
  onRefresh: () => void;
  onDelete: (id: number) => void;
  onToggle: (task: CronTask) => void;
  onRun: (task: CronTask) => void;
  onViewLogs: (task: CronTask) => void;
  onShowHelp?: () => void;
}

export default function CronTasks({
  tasks, loading, operating, presets, scripts,
  onRefresh, onDelete, onToggle, onRun, onViewLogs, onShowHelp,
}: CronTasksProps) {
  const [modalVisible, setModalVisible] = useState(false);
  const [editingTask, setEditingTask] = useState<CronTask | null>(null);
  const [form] = Form.useForm();
  const [scheduleDesc, setScheduleDesc] = useState('');
  const [descLoading, setDescLoading] = useState(false);
  const [previewVisible, setPreviewVisible] = useState(false);
  const [nextRuns, setNextRuns] = useState<string[]>([]);
  const [previewLoading, setPreviewLoading] = useState(false);
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cleanup debounce timer on unmount
  useEffect(() => {
    return () => {
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
      }
    };
  }, []);

  // Auto-describe schedule when value changes
  const handleScheduleChange = useCallback((value: string) => {
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current);
    }
    if (!value || !value.trim()) {
      setScheduleDesc('');
      return;
    }
    debounceTimer.current = setTimeout(() => {
      setDescLoading(true);
      cronApi.describeSchedule(value.trim())
        .then(res => { setScheduleDesc(res.data?.data?.description || ''); })
        .catch(() => { setScheduleDesc(''); })
        .finally(() => { setDescLoading(false); });
    }, 500);
  }, []);

  const handlePresetSelect = useCallback((presetValue: string) => {
    form.setFieldsValue({ schedule: presetValue });
    handleScheduleChange(presetValue);
  }, [form, handleScheduleChange]);

  const handlePreview = useCallback(async () => {
    const schedule = form.getFieldValue('schedule');
    if (!schedule || !schedule.trim()) {
      message.warning('请先输入 Cron 表达式');
      return;
    }
    setPreviewLoading(true);
    setPreviewVisible(true);
    try {
      const res = await cronApi.getNextRuns(schedule.trim());
      setNextRuns(res.data?.data?.next_runs || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '解析 Cron 表达式失败'));
      setNextRuns([]);
    } finally {
      setPreviewLoading(false);
    }
  }, [form]);

  const handleCreate = () => {
    setEditingTask(null);
    form.resetFields();
    setScheduleDesc('');
    setModalVisible(true);
  };

  const handleEdit = (task: CronTask) => {
    setEditingTask(task);
    form.setFieldsValue({
      name: task.name,
      command: task.command,
      schedule: task.schedule,
      description: task.description,
      script_id: task.script_id || undefined,
      timeout: task.timeout || 0,
      max_retry: task.max_retry || 0,
      env_vars: task.env_vars || '',
      work_dir: task.work_dir || '',
    });
    handleScheduleChange(task.schedule);
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (!values.command && !values.script_id) {
        message.error('请填写执行命令或选择脚本');
        return;
      }
      if (editingTask) {
        await cronApi.update(editingTask.id, values);
        message.success('任务已更新');
      } else {
        await cronApi.create(values);
        message.success('任务已创建');
      }
      setModalVisible(false);
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const statusIcon = (status: string) => {
    switch (status) {
      case 'success': return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
      case 'failed': return <CloseCircleOutlined style={{ color: '#ff4d4f' }} />;
      case 'running': return <LoadingOutlined style={{ color: '#1890ff' }} />;
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
      title: 'Cron 表达式',
      dataIndex: 'schedule',
      key: 'schedule',
      width: 150,
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
      render: (_: any, record: CronTask) => (
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
          loading={operating === `toggle-${record.id}`}
          size="small"
        />
      ),
    },
    {
      title: '上次执行',
      dataIndex: 'last_run',
      key: 'last_run',
      width: 160,
      render: (lastRun: string) => lastRun || '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_: any, record: CronTask) => (
        <Space>
          <Tooltip title="立即执行">
            <Button
              type="link"
              icon={<PlayCircleOutlined />}
              onClick={() => onRun(record)}
              loading={operating === `run-${record.id}`}
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
            description="删除后无法恢复，执行日志也将被清除"
            onConfirm={() => onDelete(record.id)}
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
          <div style={STYLES.header}>
            <Space>
              <Button icon={<ReloadOutlined />} onClick={onRefresh} loading={loading}>刷新</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>创建任务</Button>
            </Space>
          </div>
        }
      >
        <Table
          columns={columns}
          dataSource={tasks}
          rowKey="id"
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
      >
        <Form form={form} layout="vertical">
          <Form.Item label="常用预设">
            <Select
              placeholder="选择常用预设（可选）"
              allowClear
              style={STYLES.presetSelect}
              onChange={handlePresetSelect}
              options={presets.map(p => ({ label: `${p.label} — ${p.value}`, value: p.value }))}
            />
          </Form.Item>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="name" label="任务名称" rules={[{ required: true, message: '请输入任务名称' }]}>
                <Input placeholder="例：数据库备份" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item
                name="schedule"
                label={
                  <Space>
                    <span>Cron 表达式</span>
                    <Tooltip title="查看表达式手册">
                      <Button type="link" size="small" icon={<QuestionCircleOutlined />} onClick={onShowHelp} />
                    </Tooltip>
                  </Space>
                }
                rules={[{ required: true, message: '请输入 Cron 表达式' }]}
                extra={
                  <div style={STYLES.description}>
                    {descLoading ? <Spin size="small" /> : scheduleDesc}
                  </div>
                }
              >
                <Input
                  placeholder="例：0 2 * * *"
                  style={{ fontFamily: 'monospace' }}
                  onChange={e => handleScheduleChange(e.target.value)}
                  suffix={
                    <Tooltip title="预览执行时间">
                      <Button
                        type="text"
                        size="small"
                        icon={<EyeOutlined />}
                        onClick={handlePreview}
                        loading={previewLoading}
                      />
                    </Tooltip>
                  }
                />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} md={16}>
              <Form.Item name="command" label="执行命令">
                <Input.TextArea rows={2} placeholder="例：/opt/scripts/backup.sh（与脚本二选一）" />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="script_id" label="关联脚本">
                <Select
                  placeholder="选择脚本"
                  allowClear
                  options={scripts.map(s => ({ label: `${s.name} (${s.language})`, value: s.id }))}
                />
              </Form.Item>
            </Col>
          </Row>
          <Collapse
            ghost
            items={[{
              key: 'advanced',
              label: '高级选项',
              children: (
                <Row gutter={16}>
                  <Col xs={24} sm={12}>
                    <Form.Item name="work_dir" label="工作目录">
                      <Input placeholder="例：/opt/app" />
                    </Form.Item>
                    <Form.Item name="timeout" label="超时时间（秒）">
                      <InputNumber style={{ width: '100%' }} placeholder="0 = 不限制" min={0} max={86400} />
                    </Form.Item>
                    <Form.Item name="max_retry" label="失败重试">
                      <InputNumber style={{ width: '100%' }} placeholder="0 = 不重试" min={0} max={10} />
                    </Form.Item>
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Item name="env_vars" label="环境变量">
                      <Input.TextArea rows={5} placeholder="每行一个&#10;KEY=VALUE" style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                  </Col>
                </Row>
              ),
            }]}
          />
          <Form.Item name="description" label="描述" style={{ marginTop: 16 }}>
            <Input.TextArea rows={2} placeholder="任务描述（可选）" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Next Runs Preview Modal */}
      <Modal
        title={<Space><EyeOutlined /> 下 5 次执行时间</Space>}
        open={previewVisible}
        onCancel={() => setPreviewVisible(false)}
        footer={<Button onClick={() => setPreviewVisible(false)}>关闭</Button>}
        width={400}
      >
        {previewLoading ? (
          <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
        ) : nextRuns.length > 0 ? (
          <List
            size="small"
            dataSource={nextRuns}
            renderItem={(item, index) => (
              <List.Item>
                <Space>
                  <Tag color="blue">{index + 1}</Tag>
                  <span style={STYLES.nextRunItem}>{item}</span>
                </Space>
              </List.Item>
            )}
          />
        ) : (
          <Empty description="无法解析表达式" />
        )}
      </Modal>
    </>
  );
}
