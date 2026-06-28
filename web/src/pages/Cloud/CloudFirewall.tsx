import { useState } from 'react';
import { Table, Button, Tag, Modal, Form, Input, Select, message } from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import { cloudApi } from '../../services/api';
import type { CloudFirewallRule } from '../../types';

interface Props {
  firewallRules: CloudFirewallRule[];
  selectedInstance: string;
  onRefresh: () => void;
}

export default function CloudFirewall({ firewallRules, selectedInstance, onRefresh }: Props) {
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();

  const handleAddRule = async () => {
    try {
      const values = await form.validateFields();
      await cloudApi.addFirewallRule(selectedInstance, values);
      message.success('规则已添加');
      setModalVisible(false);
      form.resetFields();
      onRefresh();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    try {
      await cloudApi.deleteFirewallRule(selectedInstance, ruleId);
      message.success('规则已删除');
      onRefresh();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const columns = [
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
      render: (_: unknown, record: CloudFirewallRule) => (
        <Button
          type="link"
          size="small"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleDeleteRule(record.rule_id)}
        >
          删除
        </Button>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setModalVisible(true)}
          disabled={!selectedInstance}
        >
          添加规则
        </Button>
        {!selectedInstance && (
          <span style={{ marginLeft: 8, color: '#999' }}>请先在实例管理中选择一个实例</span>
        )}
      </div>
      <Table
        columns={columns}
        dataSource={firewallRules}
        rowKey="rule_id"
        pagination={false}
      />
      <Modal
        title="添加防火墙规则"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleAddRule}
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
    </div>
  );
}
