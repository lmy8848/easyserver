import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Tabs, Spin, Alert } from 'antd';
import {
  ReloadOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  SyncOutlined,
  PlusOutlined,
  DeleteOutlined,
  CameraOutlined,
} from '@ant-design/icons';
import { cloudApi } from '../services/api';
import type { CloudInstance, FirewallRule, Snapshot } from '../types';

export default function Cloud() {
  const [instances, setInstances] = useState<CloudInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedInstance, setSelectedInstance] = useState<string>('');
  const [firewallRules, setFirewallRules] = useState<FirewallRule[]>([]);
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [firewallModalVisible, setFirewallModalVisible] = useState(false);
  const [snapshotModalVisible, setSnapshotModalVisible] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchInstances();
  }, []);

  const fetchInstances = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await cloudApi.getInstances();
      setInstances(res.data.data?.instances || []);
    } catch (error: any) {
      console.error('Failed to fetch instances:', error);
      setError(error.message || '获取实例列表失败');
    } finally {
      setLoading(false);
    }
  };

  const fetchFirewall = async (instanceId: string) => {
    try {
      const res = await cloudApi.getFirewall(instanceId);
      setFirewallRules(res.data.data?.rules || []);
    } catch (error: any) {
      console.error('Failed to fetch firewall rules:', error);
      message.error(error.message || '获取防火墙规则失败');
    }
  };

  const fetchSnapshots = async () => {
    try {
      const res = await cloudApi.getSnapshots();
      setSnapshots(res.data.data?.snapshots || []);
    } catch (error: any) {
      console.error('Failed to fetch snapshots:', error);
      message.error(error.message || '获取快照列表失败');
    }
  };

  const handleInstanceAction = async (instanceId: string, action: string) => {
    try {
      switch (action) {
        case 'start':
          await cloudApi.startInstance(instanceId);
          message.success('实例已启动');
          break;
        case 'stop':
          await cloudApi.stopInstance(instanceId);
          message.success('实例已停止');
          break;
        case 'restart':
          await cloudApi.restartInstance(instanceId);
          message.success('实例已重启');
          break;
      }
      fetchInstances();
    } catch (error: any) {
      message.error(error.message || '操作失败');
    }
  };

  const handleAddFirewallRule = async () => {
    try {
      const values = await form.validateFields();
      await cloudApi.addFirewallRule(selectedInstance, values);
      message.success('规则已添加');
      setFirewallModalVisible(false);
      form.resetFields();
      fetchFirewall(selectedInstance);
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    }
  };

  const handleDeleteFirewallRule = async (ruleId: string) => {
    try {
      await cloudApi.deleteFirewallRule(selectedInstance, ruleId);
      message.success('规则已删除');
      fetchFirewall(selectedInstance);
    } catch (error: any) {
      message.error(error.message || '删除失败');
    }
  };

  const handleCreateSnapshot = async () => {
    try {
      const values = await form.validateFields();
      await cloudApi.createSnapshot(selectedInstance, values.name);
      message.success('快照已创建');
      setSnapshotModalVisible(false);
      form.resetFields();
      fetchSnapshots();
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    }
  };

  const instanceColumns = [
    {
      title: '实例 ID',
      dataIndex: 'instance_id',
      key: 'instance_id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      render: (state: string) => {
        const colorMap: Record<string, string> = {
          RUNNING: 'success',
          STOPPED: 'error',
          STARTING: 'processing',
          STOPPING: 'warning',
        };
        return <Tag color={colorMap[state] || 'default'}>{state}</Tag>;
      },
    },
    {
      title: '公网 IP',
      dataIndex: 'public_ip',
      key: 'public_ip',
    },
    {
      title: '配置',
      key: 'config',
      render: (_: any, record: CloudInstance) => (
        `${record.cpu}核 / ${record.memory_gb}GB / ${record.disk_gb}GB`
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: CloudInstance) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {record.state !== 'RUNNING' ? (
            <Button
              type="primary"
              size="small"
              icon={<PlayCircleOutlined />}
              onClick={() => handleInstanceAction(record.instance_id, 'start')}
            >
              启动
            </Button>
          ) : (
            <>
              <Button
                danger
                size="small"
                icon={<PauseCircleOutlined />}
                onClick={() => handleInstanceAction(record.instance_id, 'stop')}
              >
                停止
              </Button>
              <Button
                size="small"
                icon={<SyncOutlined />}
                onClick={() => handleInstanceAction(record.instance_id, 'restart')}
              >
                重启
              </Button>
            </>
          )}
          <Button
            size="small"
            onClick={() => {
              setSelectedInstance(record.instance_id);
              fetchFirewall(record.instance_id);
            }}
          >
            防火墙
          </Button>
        </div>
      ),
    },
  ];

  const firewallColumns = [
    {
      title: '协议',
      dataIndex: 'protocol',
      key: 'protocol',
    },
    {
      title: '端口',
      dataIndex: 'port',
      key: 'port',
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
    },
    {
      title: '策略',
      dataIndex: 'action',
      key: 'action',
      render: (action: string) => (
        <Tag color={action === 'ACCEPT' ? 'success' : 'error'}>{action}</Tag>
      ),
    },
    {
      title: '备注',
      dataIndex: 'remark',
      key: 'remark',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: FirewallRule) => (
        <Button
          type="link"
          size="small"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleDeleteFirewallRule(record.rule_id)}
        >
          删除
        </Button>
      ),
    },
  ];

  const snapshotColumns = [
    {
      title: '快照 ID',
      dataIndex: 'snapshot_id',
      key: 'snapshot_id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'NORMAL' ? 'success' : 'processing'}>{status}</Tag>
      ),
    },
    {
      title: '大小',
      dataIndex: 'disk_gb',
      key: 'disk_gb',
      render: (gb: number) => `${gb} GB`,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Snapshot) => (
        <Button
          type="link"
          size="small"
          onClick={async () => {
            try {
              await cloudApi.applySnapshot(record.snapshot_id);
              message.success('快照回滚中');
            } catch (error: any) {
              message.error(error.message || '回滚失败');
            }
          }}
        >
          回滚
        </Button>
      ),
    },
  ];

  const tabItems = [
    {
      key: 'instances',
      label: '实例管理',
      children: (
        <Table
          columns={instanceColumns}
          dataSource={instances}
          rowKey="instance_id"
          loading={loading}
          pagination={false}
        />
      ),
    },
    {
      key: 'firewall',
      label: '防火墙规则',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setFirewallModalVisible(true)}
              disabled={!selectedInstance}
            >
              添加规则
            </Button>
            {!selectedInstance && (
              <span style={{ marginLeft: 8, color: '#999' }}>请先在实例管理中选择一个实例</span>
            )}
          </div>
          <Table
            columns={firewallColumns}
            dataSource={firewallRules}
            rowKey="rule_id"
            pagination={false}
          />
        </div>
      ),
    },
    {
      key: 'snapshots',
      label: '快照管理',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <Space>
              <Button
                type="primary"
                icon={<CameraOutlined />}
                onClick={() => setSnapshotModalVisible(true)}
                disabled={!selectedInstance}
              >
                创建快照
              </Button>
              <Button icon={<ReloadOutlined />} onClick={fetchSnapshots}>
                刷新
              </Button>
            </Space>
          </div>
          <Table
            columns={snapshotColumns}
            dataSource={snapshots}
            rowKey="snapshot_id"
            pagination={false}
          />
        </div>
      ),
    },
  ];

  return (
    <div>
      {error && (
        <Alert
          message="错误"
          description={error}
          type="error"
          closable
          onClose={() => setError(null)}
          style={{ marginBottom: 16 }}
        />
      )}
      <Card
        title="腾讯云管理"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchInstances} loading={loading}>
            刷新
          </Button>
        }
      >
        <Spin spinning={loading}>
          <Tabs items={tabItems} />
        </Spin>
      </Card>

      {/* Add firewall rule modal */}
      <Modal
        title="添加防火墙规则"
        open={firewallModalVisible}
        onCancel={() => setFirewallModalVisible(false)}
        onOk={handleAddFirewallRule}
        okText="添加"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="protocol"
            label="协议"
            rules={[{ required: true, message: '请选择协议' }]}
          >
            <Select>
              <Select.Option value="TCP">TCP</Select.Option>
              <Select.Option value="UDP">UDP</Select.Option>
              <Select.Option value="ICMP">ICMP</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="port"
            label="端口"
            rules={[{ required: true, message: '请输入端口' }]}
          >
            <Input placeholder="如: 80, 443, 8000-9000" />
          </Form.Item>
          <Form.Item
            name="source"
            label="来源"
            rules={[{ required: true, message: '请输入来源' }]}
          >
            <Input placeholder="如: 0.0.0.0/0" />
          </Form.Item>
          <Form.Item
            name="action"
            label="策略"
            rules={[{ required: true, message: '请选择策略' }]}
          >
            <Select>
              <Select.Option value="ACCEPT">ACCEPT</Select.Option>
              <Select.Option value="DROP">DROP</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Create snapshot modal */}
      <Modal
        title="创建快照"
        open={snapshotModalVisible}
        onCancel={() => setSnapshotModalVisible(false)}
        onOk={handleCreateSnapshot}
        okText="创建"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="快照名称"
            rules={[{ required: true, message: '请输入快照名称' }]}
          >
            <Input placeholder="如: pre-update-backup" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
