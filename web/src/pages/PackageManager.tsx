import { useState, useEffect, useCallback } from 'react';
import { Card, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Popconfirm, Typography } from 'antd';
import { PlusOutlined, DeleteOutlined, SyncOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons';
import api from '../services/api';

const { Text } = Typography;

interface Package {
  id: number;
  runtime_id: number;
  runtime_name: string;
  name: string;
  version: string;
  scope: string;
  source: string;
  installed_at: string;
}

interface Runtime {
  id: number;
  name: string;
  version: string;
  path: string;
  is_default: boolean;
  status: string;
}

export default function PackageManager() {
  const [packages, setPackages] = useState<Package[]>([]);
  const [runtimes, setRuntimes] = useState<Runtime[]>([]);
  const [selectedRuntime, setSelectedRuntime] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [scanLoading, setScanLoading] = useState(false);
  const [installVisible, setInstallVisible] = useState(false);
  const [form] = Form.useForm();

  const fetchRuntimes = useCallback(async () => {
    try {
      const res = await api.get('/runtime');
      const installedRuntimes = (res.data.data?.environments || []).filter(
        (r: Runtime) => r.status === 'installed'
      );
      setRuntimes(installedRuntimes);
      if (installedRuntimes.length > 0 && !selectedRuntime) {
        setSelectedRuntime(installedRuntimes[0].id);
      }
    } catch (error) {
      message.error('获取运行环境列表失败');
    }
  }, [selectedRuntime]);

  const fetchPackages = async (runtimeId: number) => {
    setLoading(true);
    try {
      const res = await api.get(`/packages?runtime_id=${runtimeId}`);
      setPackages(res.data.data?.packages || []);
    } catch (error) {
      message.error('获取包列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleScan = async () => {
    if (!selectedRuntime) return;

    setScanLoading(true);
    try {
      const res = await api.get(`/packages/scan/${selectedRuntime}`);
      const scannedPackages = res.data.data?.packages || [];
      setPackages(scannedPackages);
      message.success(`扫描完成，发现 ${scannedPackages.length} 个包`);
    } catch (error: any) {
      message.error(error.message || '扫描失败');
    } finally {
      setScanLoading(false);
    }
  };

  useEffect(() => {
    fetchRuntimes();
  }, [fetchRuntimes]);

  useEffect(() => {
    if (selectedRuntime) {
      fetchPackages(selectedRuntime);
    }
  }, [selectedRuntime]);

  const handleInstall = async (values: { name: string; version: string }) => {
    if (!selectedRuntime) return;

    try {
      await api.post('/packages/install', {
        runtime_id: selectedRuntime,
        name: values.name,
        version: values.version || '',
        scope: 'global',
      });
      message.success(`正在安装 ${values.name}...`);
      setInstallVisible(false);
      form.resetFields();
      // Refresh after a delay
      setTimeout(() => fetchPackages(selectedRuntime), 3000);
    } catch (error: any) {
      message.error(error.message || '安装失败');
    }
  };

  const handleUninstall = async (name: string) => {
    if (!selectedRuntime) return;

    try {
      await api.post('/packages/uninstall', {
        runtime_id: selectedRuntime,
        name: name,
      });
      message.success(`${name} 卸载成功`);
      fetchPackages(selectedRuntime);
    } catch (error: any) {
      message.error(error.message || '卸载失败');
    }
  };

  const handleUpdate = async (name: string) => {
    if (!selectedRuntime) return;

    try {
      await api.post('/packages/update', {
        runtime_id: selectedRuntime,
        name: name,
      });
      message.success(`${name} 更新成功`);
      fetchPackages(selectedRuntime);
    } catch (error: any) {
      message.error(error.message || '更新失败');
    }
  };

  const getRuntimeIcon = (name: string) => {
    const icons: Record<string, string> = {
      java: '☕',
      node: '🟢',
      go: '🔵',
      python: '🐍',
      php: '🐘',
    };
    return icons[name] || '📦';
  };

  const columns = [
    {
      title: '包名',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => <Text strong>{name}</Text>,
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      render: (version: string) => <Tag color="blue">{version}</Tag>,
    },
    {
      title: '范围',
      dataIndex: 'scope',
      key: 'scope',
      render: (scope: string) => (
        <Tag color={scope === 'global' ? 'green' : 'orange'}>{scope}</Tag>
      ),
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      render: (source: string) => <Tag>{source}</Tag>,
    },
    {
      title: '安装时间',
      dataIndex: 'installed_at',
      key: 'installed_at',
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_: any, record: Package) => (
        <Space>
          <Button
            type="link"
            size="small"
            icon={<SyncOutlined />}
            onClick={() => handleUpdate(record.name)}
          >
            更新
          </Button>
          <Popconfirm
            title={`确定要卸载 ${record.name} 吗？`}
            onConfirm={() => handleUninstall(record.name)}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              卸载
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title={
          <Space>
            <span>包管理</span>
            {selectedRuntime && (
              <Tag color="blue">
                {getRuntimeIcon(runtimes.find(r => r.id === selectedRuntime)?.name || '')}
                {runtimes.find(r => r.id === selectedRuntime)?.name} {runtimes.find(r => r.id === selectedRuntime)?.version}
              </Tag>
            )}
          </Space>
        }
        extra={
          <Space>
            <Select
              placeholder="选择运行环境"
              style={{ width: 200 }}
              value={selectedRuntime}
              onChange={(value) => setSelectedRuntime(value)}
            >
              {runtimes.map(runtime => (
                <Select.Option key={runtime.id} value={runtime.id}>
                  {getRuntimeIcon(runtime.name)} {runtime.name} {runtime.version}
                </Select.Option>
              ))}
            </Select>
            <Button
              icon={<SearchOutlined />}
              onClick={handleScan}
              loading={scanLoading}
              disabled={!selectedRuntime}
            >
              扫描
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => selectedRuntime && fetchPackages(selectedRuntime)}
              disabled={!selectedRuntime}
            >
              刷新
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setInstallVisible(true)}
              disabled={!selectedRuntime}
            >
              安装包
            </Button>
          </Space>
        }
      >
        {!selectedRuntime ? (
          <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
            请先选择一个运行环境
          </div>
        ) : packages.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
            暂无已安装的包，点击"扫描"按钮检测
          </div>
        ) : (
          <Table
            columns={columns}
            dataSource={packages}
            rowKey="id"
            loading={loading}
            pagination={{ pageSize: 20 }}
          />
        )}
      </Card>

      {/* 安装包弹窗 */}
      <Modal
        title="安装包"
        open={installVisible}
        onCancel={() => {
          setInstallVisible(false);
          form.resetFields();
        }}
        footer={null}
      >
        <Form form={form} onFinish={handleInstall} layout="vertical">
          <Form.Item
            name="name"
            label="包名"
            rules={[{ required: true, message: '请输入包名' }]}
          >
            <Input placeholder="例如：express、flask、requests" />
          </Form.Item>
          <Form.Item
            name="version"
            label="版本（可选）"
          >
            <Input placeholder="留空则安装最新版本" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>
              开始安装
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
