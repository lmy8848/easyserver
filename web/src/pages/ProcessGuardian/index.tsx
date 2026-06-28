import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber,
  Select, Switch, message, Popconfirm, Table, Empty, Tooltip, Tabs,
  Typography, Badge, Row, Col, Statistic,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  CaretRightOutlined, PauseOutlined, RedoOutlined,
  FileTextOutlined, ExportOutlined, ImportOutlined,
  ThunderboltOutlined, AppstoreOutlined, ClusterOutlined,
  DashboardOutlined,
} from '@ant-design/icons';
import type { ProcessWithStatus, ProcessLog, ProcessGroup } from '../../types';
import { processApi } from '../../services/api';
import SystemMonitor from './SystemMonitor';

const { Text } = Typography;
const { TextArea } = Input;

const MODAL_TOP_OFFSET = 40; // px from viewport top

const STATUS_CONFIG: Record<string, { color: string; label: string }> = {
  running: { color: 'green', label: '运行中' },
  stopped: { color: 'default', label: '已停止' },
  error: { color: 'red', label: '错误' },
  starting: { color: 'processing', label: '启动中' },
  stopping: { color: 'warning', label: '停止中' },
};

export default function ProcessGuardian() {
  const [activeTab, setActiveTab] = useState<'managed' | 'system'>('managed');
  const [processes, setProcesses] = useState<ProcessWithStatus[]>([]);
  const [loading, setLoading] = useState(false);
  const [operating, setOperating] = useState<string>('');

  // Modal state
  const [modalVisible, setModalVisible] = useState(false);
  const [editingProcess, setEditingProcess] = useState<ProcessWithStatus | null>(null);
  const [form] = Form.useForm();

  // Logs state
  const [logsVisible, setLogsVisible] = useState(false);
  const [logsProcess, setLogsProcess] = useState<ProcessWithStatus | null>(null);
  const [logs, setLogs] = useState<ProcessLog[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);

  // Groups state
  const [groups, setGroups] = useState<ProcessGroup[]>([]);
  const [selectedRowKeys, setSelectedRowKeys] = useState<number[]>([]);

  const fetchProcesses = useCallback(async () => {
    setLoading(true);
    try {
      const res = await processApi.list();
      setProcesses(res.data?.data || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取进程列表失败'));
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchGroups = useCallback(async () => {
    try {
      const res = await processApi.listGroups();
      setGroups(res.data?.data || []);
    } catch { /* silent */ }
  }, []);

  useEffect(() => { fetchProcesses(); fetchGroups(); }, [fetchProcesses, fetchGroups]);

  // Auto-refresh every 5s
  useEffect(() => {
    const timer = setInterval(fetchProcesses, 5000);
    return () => clearInterval(timer);
  }, [fetchProcesses]);

  const handleCreate = () => {
    setEditingProcess(null);
    form.resetFields();
    form.setFieldsValue({ auto_restart: true, max_restarts: 10, restart_delay: 5, stop_timeout: 10, startup_timeout: 30 });
    setModalVisible(true);
  };

  const handleEdit = (p: ProcessWithStatus) => {
    setEditingProcess(p);
    form.setFieldsValue({
      name: p.name,
      command: p.command,
      args: p.args,
      dir: p.dir,
      env: p.env,
      auto_restart: p.auto_restart,
      max_restarts: p.max_restarts,
      restart_delay: p.restart_delay,
      stop_timeout: p.stop_timeout,
      startup_timeout: p.startup_timeout,
      auto_start: p.auto_start,
      log_file: p.log_file,
      group_id: p.group_id || undefined,
    });
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingProcess) {
        await processApi.update(editingProcess.id, values);
        message.success('更新成功');
      } else {
        await processApi.create(values);
        message.success('创建成功');
      }
      setModalVisible(false);
      fetchProcesses();
    } catch (error: unknown) {
      const formErr = error as { errorFields?: unknown };
      if (formErr.errorFields) return; // form validation error
      message.error((error instanceof Error ? error.message : '操作失败'));
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await processApi.delete(id);
      message.success('已删除');
      fetchProcesses();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleAction = async (id: number, action: 'start' | 'stop' | 'restart') => {
    const key = `${action}-${id}`;
    setOperating(key);
    try {
      await processApi[action](id);
      message.success(`${action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'}成功`);
      fetchProcesses();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    } finally {
      setOperating('');
    }
  };

  const handleBatchAction = async (action: 'start' | 'stop' | 'restart') => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择进程');
      return;
    }
    try {
      const fn = action === 'start' ? processApi.batchStart :
        action === 'stop' ? processApi.batchStop : processApi.batchRestart;
      await fn(selectedRowKeys);
      message.success(`批量${action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'}完成`);
      setSelectedRowKeys([]);
      fetchProcesses();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    }
  };

  const handleViewLogs = async (p: ProcessWithStatus) => {
    setLogsProcess(p);
    setLogsVisible(true);
    setLogsLoading(true);
    try {
      const res = await processApi.getLogs(p.id, 100);
      const data = res.data?.data;
      setLogs(Array.isArray(data) ? data : data?.items || []);
    } catch {
      message.error('获取日志失败');
    } finally {
      setLogsLoading(false);
    }
  };

  const handleExport = async () => {
    try {
      const res = await processApi.export();
      const data = JSON.stringify(res.data?.data || [], null, 2);
      const blob = new Blob([data], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'processes-export.json';
      a.click();
      URL.revokeObjectURL(url);
      message.success('导出成功');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '导出失败'));
    }
  };

  const handleImport = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async (e: any) => {
      const file = e.target.files?.[0];
      if (!file) return;
      try {
        const text = await file.text();
        const processes = JSON.parse(text);
        await processApi.import(processes);
        message.success('导入成功');
        fetchProcesses();
      } catch (error: unknown) {
        message.error((error instanceof Error ? error.message : '导入失败'));
      }
    };
    input.click();
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
      render: (text: string, record: ProcessWithStatus) => (
        <Tooltip title={`${text} ${record.args || ''}`}>
          <Text code style={{ fontSize: 12 }}>{text}</Text>
        </Tooltip>
      ),
    },
    {
      title: '状态',
      dataIndex: ['status', 'status'],
      key: 'status',
      width: 100,
      render: (_: unknown, record: ProcessWithStatus) => {
        const st = record.status?.status || 'stopped';
        const cfg = STATUS_CONFIG[st] || STATUS_CONFIG['stopped']!;
        return <Badge status={cfg.color as any} text={cfg.label} />;
      },
    },
    {
      title: 'PID',
      dataIndex: ['status', 'pid'],
      key: 'pid',
      width: 70,
      render: (pid: number) => pid > 0 ? pid : '-',
    },
    {
      title: 'CPU',
      dataIndex: ['status', 'cpu_percent'],
      key: 'cpu',
      width: 80,
      render: (v: number) => v > 0 ? `${v.toFixed(1)}%` : '-',
    },
    {
      title: '内存',
      dataIndex: ['status', 'memory_mb'],
      key: 'memory',
      width: 80,
      render: (v: number) => v > 0 ? `${v.toFixed(1)}MB` : '-',
    },
    {
      title: '重启',
      dataIndex: ['status', 'restarts'],
      key: 'restarts',
      width: 60,
      render: (v: number) => v || 0,
    },
    {
      title: '操作',
      key: 'action',
      width: 240,
      render: (_: unknown, record: ProcessWithStatus) => {
        const isRunning = record.status?.status === 'running';
        const isBusy = record.status?.status === 'starting' || record.status?.status === 'stopping';
        const opKey = `start-${record.id}`;
        const spKey = `stop-${record.id}`;
        const rsKey = `restart-${record.id}`;
        return (
          <Space size="small">
            {!isRunning && (
              <Tooltip title="启动">
                <Button type="link" size="small" icon={<CaretRightOutlined />}
                  loading={operating === opKey} disabled={isBusy}
                  onClick={() => handleAction(record.id, 'start')} />
              </Tooltip>
            )}
            {isRunning && (
              <Tooltip title="停止">
                <Button type="link" size="small" icon={<PauseOutlined />}
                  loading={operating === spKey} disabled={isBusy}
                  onClick={() => handleAction(record.id, 'stop')} />
              </Tooltip>
            )}
            <Tooltip title="重启">
              <Button type="link" size="small" icon={<RedoOutlined />}
                loading={operating === rsKey} disabled={isBusy}
                onClick={() => handleAction(record.id, 'restart')} />
            </Tooltip>
            <Tooltip title="日志">
              <Button type="link" size="small" icon={<FileTextOutlined />}
                onClick={() => handleViewLogs(record)} />
            </Tooltip>
            <Tooltip title="编辑">
              <Button type="link" size="small" icon={<EditOutlined />}
                onClick={() => handleEdit(record)} />
            </Tooltip>
            <Popconfirm title="确定删除?" onConfirm={() => handleDelete(record.id)}>
              <Tooltip title="删除">
                <Button type="link" size="small" danger icon={<DeleteOutlined />}
                  disabled={isRunning} />
              </Tooltip>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  const runningCount = processes.filter(p => p.status?.status === 'running').length;
  const stoppedCount = processes.filter(p => p.status?.status === 'stopped').length;
  const errorCount = processes.filter(p => p.status?.status === 'error').length;

  return (
    <div>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'managed' | 'system')}
        items={[
          {
            key: 'managed',
            label: (
              <span>
                <ClusterOutlined />
                托管进程
              </span>
            ),
          },
          {
            key: 'system',
            label: (
              <span>
                <DashboardOutlined />
                系统监控
              </span>
            ),
          },
        ]}
        style={{ marginBottom: 16 }}
      />

      {activeTab === 'managed' && (
        <>
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col span={6}>
              <Card size="small">
                <Statistic title="进程总数" value={processes.length} prefix={<AppstoreOutlined />} />
              </Card>
            </Col>
            <Col span={6}>
              <Card size="small">
                <Statistic title="运行中" value={runningCount} styles={{ content: { color: '#3f8600' } }}
                  prefix={<CaretRightOutlined />} />
              </Card>
            </Col>
            <Col span={6}>
              <Card size="small">
                <Statistic title="已停止" value={stoppedCount} prefix={<PauseOutlined />} />
              </Card>
            </Col>
            <Col span={6}>
              <Card size="small">
                <Statistic title="异常" value={errorCount} styles={{ content: { color: '#cf1322' } }}
                  prefix={<ThunderboltOutlined />} />
              </Card>
            </Col>
          </Row>

      <Card
        title={<Space><ClusterOutlined />进程守护</Space>}
        extra={
          <Space>
            <Button size="small" icon={<CaretRightOutlined />}
              onClick={() => handleBatchAction('start')} disabled={selectedRowKeys.length === 0}>
              批量启动
            </Button>
            <Button size="small" icon={<PauseOutlined />}
              onClick={() => handleBatchAction('stop')} disabled={selectedRowKeys.length === 0}>
              批量停止
            </Button>
            <Button size="small" icon={<RedoOutlined />}
              onClick={() => handleBatchAction('restart')} disabled={selectedRowKeys.length === 0}>
              批量重启
            </Button>
            <Button size="small" icon={<ExportOutlined />} onClick={handleExport}>导出</Button>
            <Button size="small" icon={<ImportOutlined />} onClick={handleImport}>导入</Button>
            <Button size="small" icon={<ReloadOutlined />} onClick={fetchProcesses}>刷新</Button>
            <Button size="small" type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
              添加进程
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={processes}
          loading={loading}
          size="small"
          rowSelection={{
            selectedRowKeys,
            onChange: (keys) => setSelectedRowKeys(keys as number[]),
          }}
          pagination={false}
          locale={{ emptyText: <Empty description={'暂无进程配置，点击「添加进程」开始'} /> }}
        />
      </Card>

      {/* Add/Edit Modal */}
      <Modal
        title={editingProcess ? '编辑进程' : '添加进程'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={600}
        destroyOnHidden
        style={{ top: MODAL_TOP_OFFSET }}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="进程名称" rules={[{ required: true, message: '请输入进程名称' }]}>
            <Input placeholder="例如: my-app" />
          </Form.Item>
          <Form.Item name="command" label="启动命令" rules={[{ required: true, message: '请输入启动命令' }]}>
            <Input placeholder="例如: node /app/server.js" />
          </Form.Item>
          <Form.Item name="args" label="参数">
            <Input placeholder="命令行参数" />
          </Form.Item>
          <Form.Item name="dir" label="工作目录">
            <Input placeholder="例如: /app" />
          </Form.Item>
          <Form.Item name="env" label="环境变量 (JSON)">
            <TextArea rows={3} placeholder='例如: {"NODE_ENV": "production", "PORT": "3000"}' />
          </Form.Item>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item name="auto_restart" label="自动重启" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="max_restarts" label="最大重启次数">
                <InputNumber min={0} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="restart_delay" label="重启延迟(秒)">
                <InputNumber min={1} max={300} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="stop_timeout" label="停止超时(秒)">
                <InputNumber min={1} max={60} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="startup_timeout" label="启动超时(秒)">
                <InputNumber min={5} max={300} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="auto_start" label="开机自启" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="group_id" label="分组">
                <Select allowClear placeholder="无分组">
                  {groups.map(g => (
                    <Select.Option key={g.id} value={g.id}>{g.name}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="log_file" label="日志文件路径">
            <Input placeholder="留空则自动捕获 stdout/stderr" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Logs Modal */}
      <Modal
        title={`${logsProcess?.name || ''} - 日志`}
        open={logsVisible}
        onCancel={() => setLogsVisible(false)}
        footer={null}
        width={800}
      >
        <div style={{ maxHeight: 500, overflow: 'auto', background: '#1e1e1e', borderRadius: 6, padding: 12 }}>
          {logsLoading ? (
            <Text style={{ color: '#888' }}>加载中...</Text>
          ) : logs.length === 0 ? (
            <Text style={{ color: '#888' }}>暂无日志</Text>
          ) : (
            logs.map((log, i) => (
              <div key={i} style={{ marginBottom: 4, fontFamily: 'monospace', fontSize: 12 }}>
                <Text style={{ color: '#666' }}>{log.created_at} </Text>
                <Tag color={log.type === 'stderr' ? 'red' : log.type === 'system' ? 'blue' : 'default'}
                  style={{ fontSize: 10, lineHeight: '16px', marginRight: 4 }}>
                  {log.type}
                </Tag>
                <Text style={{ color: log.type === 'stderr' ? '#ff6b6b' : '#d4d4d4' }}>
                  {log.content}
                </Text>
              </div>
            ))
          )}
        </div>
      </Modal>

      </>
      )}

      {activeTab === 'system' && <SystemMonitor />}
    </div>
  );
}