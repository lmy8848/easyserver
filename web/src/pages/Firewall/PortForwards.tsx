import { useCallback, useEffect, useState } from 'react';
import {
  Card, Button, Table, Tag, Space, message, Form, Input, Select, Modal, Popconfirm, Switch, Tooltip, Alert,
} from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { firewallApi } from '../../services/firewall';
import type { PortForwardRule } from '../../types';

const PROTOCOL_OPTIONS = [
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP' },
];

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

// 端口转发（nftables DNAT）：外部端口 -> 内网/本机目标端口
export default function PortForwards() {
  const [rules, setRules] = useState<PortForwardRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<PortForwardRule | null>(null);
  const [operating, setOperating] = useState('');
  const [form] = Form.useForm();

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const res = await firewallApi.listPortForwards();
      setRules(res.data.data?.items || []);
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '获取端口转发规则失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void fetchRules(); }, [fetchRules]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ protocol: 'tcp' });
    setModalOpen(true);
  };

  const openEdit = (rule: PortForwardRule) => {
    setEditing(rule);
    form.setFieldsValue({
      protocol: rule.protocol,
      listen_port: rule.listen_port,
      dest_ip: rule.dest_ip,
      dest_port: rule.dest_port,
      source_cidr: rule.source_cidr,
      interface: rule.interface,
      remark: rule.remark,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const values = await form.validateFields();
    setOperating('save');
    try {
      if (editing) {
        await firewallApi.updatePortForward(editing.id, values);
        message.success('端口转发已更新');
      } else {
        await firewallApi.createPortForward(values);
        message.success('端口转发已创建并生效');
      }
      setModalOpen(false);
      await fetchRules();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '保存失败');
    } finally {
      setOperating('');
    }
  };

  const handleToggle = async (rule: PortForwardRule) => {
    setOperating(`toggle-${rule.id}`);
    try {
      if (rule.enabled) {
        await firewallApi.disablePortForward(rule.id);
        message.success('已禁用');
      } else {
        await firewallApi.enablePortForward(rule.id);
        message.success('已启用');
      }
      await fetchRules();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '操作失败');
    } finally {
      setOperating('');
    }
  };

  const handleDelete = async (rule: PortForwardRule) => {
    setOperating(`del-${rule.id}`);
    try {
      await firewallApi.deletePortForward(rule.id);
      message.success('已删除');
      await fetchRules();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '删除失败');
    } finally {
      setOperating('');
    }
  };

  return (
    <Card
      title="端口转发"
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={fetchRules} loading={loading}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建转发</Button>
        </Space>
      }
    >
      <Alert
        message="将服务器公网端口的流量转发到内网/本机的其他端口（如游戏服务器）。基于 nftables nat 实现，支持 TCP/UDP，规则在面板重启后自动重放。"
        type="info" showIcon style={{ marginBottom: 16 }}
      />
      <Table
        size="small"
        loading={loading}
        dataSource={rules}
        rowKey="id"
        pagination={false}
        locale={{ emptyText: '暂无转发规则，点击右上角"新建转发"创建' }}
        columns={[
          {
            title: '协议', dataIndex: 'protocol', width: 70,
            render: (p: string) => <Tag color="blue" style={{ margin: 0 }}>{p.toUpperCase()}</Tag>,
          },
          { title: '监听端口', dataIndex: 'listen_port', width: 110 },
          {
            title: '目标', key: 'dest',
            render: (_: unknown, r: PortForwardRule) => `${r.dest_ip}:${r.dest_port || r.listen_port}`,
          },
          { title: '来源限制', dataIndex: 'source_cidr', width: 130, render: (v: string) => v || '任意' },
          { title: '网卡', dataIndex: 'interface', width: 100, render: (v: string) => v || '全部' },
          {
            title: '流量', key: 'traffic', width: 150,
            render: (_: unknown, r: PortForwardRule) => (r.packets > 0
              ? <span style={{ fontVariantNumeric: 'tabular-nums' }}>{r.packets} 包 / {formatBytes(r.bytes)}</span>
              : <span style={{ opacity: 0.5 }}>-</span>),
          },
          {
            title: '备注', dataIndex: 'remark', ellipsis: true, render: (v: string) => v || '-',
          },
          {
            title: '状态', key: 'enabled', width: 90,
            render: (_: unknown, r: PortForwardRule) => (
              <Switch
                size="small"
                checked={r.enabled}
                loading={operating === `toggle-${r.id}`}
                onChange={() => handleToggle(r)}
              />
            ),
          },
          {
            title: '操作', key: 'action', width: 110,
            render: (_: unknown, r: PortForwardRule) => (
              <Space size="small">
                <Button type="link" size="small" onClick={() => openEdit(r)}>编辑</Button>
                <Popconfirm title="删除该转发规则？" onConfirm={() => handleDelete(r)}>
                  <Button type="link" size="small" danger loading={operating === `del-${r.id}`}>删除</Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={editing ? '编辑端口转发' : '新建端口转发'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSave}
        confirmLoading={operating === 'save'}
        destroyOnHidden
      >
        <Alert
          message="流量进入服务器的监听端口后，经内核 DNAT 转发到目标地址。目标是内网地址时自动配置回程伪装（hairpin）。"
          type="info" showIcon style={{ marginBottom: 16 }}
        />
        <Form form={form} layout="vertical">
          <Space size="middle" style={{ display: 'flex' }}>
            <Form.Item name="protocol" label="协议" rules={[{ required: true }]} style={{ width: 120 }}>
              <Select options={PROTOCOL_OPTIONS} />
            </Form.Item>
            <Form.Item
              name="listen_port" label="监听端口"
              rules={[{ required: true, message: '请输入监听端口' }]}
              extra="单端口（25565）或范围（1000-2000）"
              style={{ width: 180 }}
            >
              <Input placeholder="25565" />
            </Form.Item>
          </Space>
          <Space size="middle" style={{ display: 'flex' }}>
            <Form.Item
              name="dest_ip" label="目标 IP"
              rules={[{ required: true, message: '请输入目标 IPv4 地址' }]}
              extra="本机填 127.0.0.1 或内网 IP"
              style={{ width: 200 }}
            >
              <Input placeholder="192.168.1.100" />
            </Form.Item>
            <Form.Item name="dest_port" label="目标端口" extra="留空 = 同监听端口" style={{ width: 160 }}>
              <Input placeholder="同监听端口" />
            </Form.Item>
          </Space>
          <Space size="middle" style={{ display: 'flex' }}>
            <Form.Item name="source_cidr" label="来源限制（可选）" extra="仅放行指定来源，如 1.2.3.4/32" style={{ width: 200 }}>
              <Input placeholder="任意来源" />
            </Form.Item>
            <Form.Item name="interface" label="入站网卡（可选）" extra="如 eth0，留空 = 全部" style={{ width: 180 }}>
              <Input placeholder="全部网卡" />
            </Form.Item>
          </Space>
          <Form.Item name="remark" label="备注（可选）">
            <Input placeholder="例如：MC 服务器" maxLength={100} />
          </Form.Item>
          <div style={{ fontSize: 12, opacity: 0.65 }}>
            <Tooltip title="目标是内网地址时，面板会自动添加 postrouting masquerade 规则解决回程路由">
              目标为内网地址时自动处理 hairpin 回程；目标的日志里看到的来源将是服务器内网 IP。
            </Tooltip>
          </div>
        </Form>
      </Modal>
    </Card>
  );
}
