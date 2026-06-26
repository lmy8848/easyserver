import { useState, useEffect } from 'react';
import {
  Card, Table, Tag, Button, Space, message, Modal, Form, Input,
} from 'antd';
import {
  PlayCircleOutlined, StopOutlined, ReloadOutlined,
  InfoCircleOutlined, EditOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { ComposeProject } from './types';

export default function ComposeTab() {
  const [projects, setProjects] = useState<ComposeProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [logsVisible, setLogsVisible] = useState(false);
  const [configVisible, setConfigVisible] = useState(false);
  const [configDir, setConfigDir] = useState('');
  const [logs, setLogs] = useState('');
  const [configForm] = Form.useForm();

  const loadProjects = async () => {
    try {
      const res = await api.get('/compose/projects');
      setProjects(res.data?.data?.projects || []);
    } catch {
      // Compose might not be available
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadProjects(); }, []);

  const handleAction = async (action: string, dir: string) => {
    try {
      await api.post(`/compose/${action}`, { project_dir: dir });
      message.success(`compose ${action} 成功`);
      setLoading(true);
      loadProjects();
    } catch {
      message.error(`compose ${action} 失败`);
    }
  };

  const handleLogs = async (dir: string) => {
    try {
      const res = await api.get(`/compose/logs?dir=${encodeURIComponent(dir)}&tail=200`);
      setLogs(res.data?.data?.logs || '');
      setLogsVisible(true);
    } catch {
      message.error('获取日志失败');
    }
  };

  const handleGetConfig = async (dir: string) => {
    try {
      const res = await api.get(`/compose/config?dir=${encodeURIComponent(dir)}`);
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
      await api.put('/compose/config', { project_dir: configDir, content: values.content });
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
          <Button size="small" icon={<PlayCircleOutlined />} onClick={() => handleAction('up', record.config_file)}>启动</Button>
          <Button size="small" icon={<StopOutlined />} onClick={() => handleAction('down', record.config_file)}>停止</Button>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => handleAction('restart', record.config_file)}>重启</Button>
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

      <Modal title="编辑 Compose 配置" open={configVisible} onOk={handleSaveConfig} onCancel={() => setConfigVisible(false)} width={800}>
        <Form form={configForm} layout="vertical">
          <Form.Item name="content" rules={[{ required: true }]}>
            <Input.TextArea rows={20} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
