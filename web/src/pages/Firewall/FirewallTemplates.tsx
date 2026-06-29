import { Modal, Table, Tag, Button } from 'antd';
import type { FirewallRuleTemplate } from '../../types';
import { actionColor } from './types';

interface Props {
  visible: boolean;
  templates: FirewallRuleTemplate[];
  loading: boolean;
  onClose: () => void;
  onApply: (template: FirewallRuleTemplate) => void;
}

export default function FirewallTemplates({
  visible,
  templates,
  loading,
  onClose,
  onApply,
}: Props) {
  return (
    <Modal
      title="模板规则"
      open={visible}
      onCancel={onClose}
      footer={null}
      width={600}
      style={{ top: 20 }}
    >
      <p style={{ color: '#8c8c8c', marginBottom: 16 }}>选择一个预设模板快速创建防火墙规则（默认应用于 INPUT 链）</p>
      <Table
        dataSource={templates}
        rowKey="name"
        loading={loading}
        size="small"
        pagination={false}
        columns={[
          {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            width: 140,
          },
          {
            title: '协议',
            dataIndex: 'protocol',
            key: 'protocol',
            width: 70,
            render: (p: string) => p.toUpperCase(),
          },
          {
            title: '端口',
            dataIndex: 'port',
            key: 'port',
            width: 80,
          },
          {
            title: '动作',
            dataIndex: 'action',
            key: 'action',
            width: 90,
            render: (action: string) => <Tag color={actionColor(action)}>{action}</Tag>,
          },
          {
            title: '说明',
            dataIndex: 'remark',
            key: 'remark',
          },
          {
            title: '操作',
            key: 'op',
            width: 80,
            render: (_: unknown, record: FirewallRuleTemplate) => (
              <Button type="link" size="small" onClick={() => onApply(record)}>
                应用
              </Button>
            ),
          },
        ]}
      />
    </Modal>
  );
}
