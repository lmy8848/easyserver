import { useState, useEffect, useCallback } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Modal, Form, Input,
} from 'antd';
import {
  PlayCircleOutlined, StopOutlined, ReloadOutlined,
  InfoCircleOutlined, EditOutlined,
} from '@ant-design/icons';
import { containerApi } from '../../services/container';
import { useAsyncRun } from '../../hooks/useAsyncRun';
import type { ComposeProject } from './types';

export default function ComposeTab({ engine }: { engine: string }) {
  const [projects, setProjects] = useState<ComposeProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [logsVisible, setLogsVisible] = useState(false);
  const [configVisible, setConfigVisible] = useState(false);
  const [configDir, setConfigDir] = useState('');
  const [logs, setLogs] = useState('');
  const [configForm] = Form.useForm();
  const [actionLoading, setActionLoading] = useState<string>('');
  const [saveLoading, runSaveConfig] = useAsyncRun();

  const loadProjects = useCallback(async () => {
    try {
      const res = await containerApi.listComposeProjects(engine);
      setProjects(res.data?.data?.items ?? []);
    } catch {
      // Compose might not be available
    } finally {
      setLoading(false);
    }
  }, [engine]);

  useEffect(() => { loadProjects(); }, [loadProjects]);

  const handleAction = async (action: string, dir: string) => {
    setActionLoading(`${dir}:${action}`);
    try {
      await containerApi.composeAction(action, dir, engine);
      message.success(`compose ${action} 成功`);
      setLoading(true);
      loadProjects();
    } catch {
      message.error(`compose ${action} 失败`);
    } finally {
      setActionLoading('');
    }
  };

  const handleLogs = async (dir: string) => {
    try {
      const res = await containerApi.getComposeLogs(dir, 200, engine);
      setLogs(res.data?.data?.logs || '');
      setLogsVisible(true);
    } catch {
      message.error('获取日志失败');
    }
  };

  const handleGetConfig = async (dir: string) => {
    try {
      const res = await containerApi.getComposeConfig(dir, engine);
      const content = res.data?.data?.content || '';
      setConfigDir(dir);
      configForm.setFieldsValue({ content });
      setConfigVisible(true);
    } catch {
      message.error('获取配置失败');
    }
  };

  const handleSaveConfig = async () => {
    try {
      const values = await configForm.validateFields();
      await runSaveConfig(() => containerApi.saveComposeConfig(configDir, values.content, engine));
      message.success('配置已保存');
      setConfigVisible(false);
    } catch {
      message.error('保存失败');
    }
  };

  const columns = [
    { title: '项目名', dataIndex: 'name', key: 'name' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <Tag color={status.includes('running') ? 'green' : 'default'}>{status}</Tag>,
    },
    {
      title: '服务',
      dataIndex: 'services',
      key: 'services',
      render: (services: string[]) => (
        <Space wrap>
          {services?.map((s, i) => <Tag key={i}>{s}</Tag>)}
        </Space>
      ),
    },
    { title: '配置文件', dataIndex: 'config_file', key: 'config_file', ellipsis: true },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: ComposeProject) => (
        <Space>
          <Button size="small" icon={<PlayCircleOutlined />} loading={actionLoading === `${record.config_file}:up`} disabled={!!actionLoading} onClick={() => handleAction('up', record.config_file)}>启动</Button>
          <Button size="small" icon={<StopOutlined />} loading={actionLoading === `${record.config_file}:down`} disabled={!!actionLoading} onClick={() => handleAction('down', record.config_file)}>停止</Button>
          <Button size="small" icon={<ReloadOutlined />} loading={actionLoading === `${record.config_file}:restart`} disabled={!!actionLoading} onClick={() => handleAction('restart', record.config_file)}>重启</Button>
          <Button size="small" icon={<InfoCircleOutlined />} onClick={() => handleLogs(record.config_file)}>日志</Button>
          <Button size="small" icon={<EditOutlined />} onClick={() => handleGetConfig(record.config_file)}>配置</Button>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Card extra={<Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); loadProjects(); }}>刷新</Button>}>
        <Table columns={columns} dataSource={projects} rowKey="name" loading={loading} locale={{ emptyText: '暂无 Compose 项目' }} />
      </Card>

      <Modal title="Compose 日志" open={logsVisible} onCancel={() => setLogsVisible(false)} footer={null} width={800}>
        <pre style={{ maxHeight: 500, overflow: 'auto', background: '#f5f5f5', padding: 16, fontSize: 12, whiteSpace: 'pre-wrap' }}>
          {logs || '暂无日志'}
        </pre>
      </Modal>

      <Modal title="编辑 Compose 配置" open={configVisible} onOk={handleSaveConfig} onCancel={() => setConfigVisible(false)} width={800} confirmLoading={saveLoading}>
        <Form form={configForm} layout="vertical">
          <Form.Item name="content" rules={[{ required: true }]}>
            <Input.TextArea rows={20} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
