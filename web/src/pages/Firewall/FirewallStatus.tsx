import {
  Card, Button, Space, Tag, Select, Spin, Empty, Descriptions, Modal, Tooltip,
} from 'antd';
import {
  SafetyOutlined, CheckCircleOutlined, CloseCircleOutlined,
  LockOutlined, UnlockOutlined, ExclamationCircleOutlined,
} from '@ant-design/icons';
import type { FirewallStatus } from '../../types';

interface Props {
  status: FirewallStatus | null;
  statusLoading: boolean;
  operating: string;
  policyChanging: string;
  onToggleFirewall: () => void;
  onChangePolicy: (chain: 'INPUT' | 'OUTPUT', policy: string) => void;
}

export default function FirewallStatusCard({
  status,
  statusLoading,
  operating,
  policyChanging,
  onToggleFirewall,
  onChangePolicy,
}: Props) {
  return (
    <Card
      title={<Space><SafetyOutlined /> 防火墙状态</Space>}
      extra={
        <Button
          icon={status?.enabled ? <LockOutlined /> : <UnlockOutlined />}
          onClick={onToggleFirewall}
          loading={operating === 'firewall'}
          type={status?.enabled ? 'primary' : 'default'}
          danger={status?.enabled}
        >
          {status?.enabled ? '禁用防火墙' : '启用防火墙'}
        </Button>
      }
      style={{ marginBottom: 16 }}
    >
      {statusLoading ? (
        <Spin />
      ) : status ? (
        <Descriptions size="small" column={4}>
          <Descriptions.Item label="状态">
            {status.enabled ? (
              <Tag color="success" icon={<CheckCircleOutlined />}>已启用</Tag>
            ) : (
              <Tag color="error" icon={<CloseCircleOutlined />}>已禁用</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="工具">{status.tool}</Descriptions.Item>
          <Descriptions.Item label="规则数">
            <Tooltip title={`系统规则 ${status.rule_count - status.custom_rule_count} 条，自定义规则 ${status.custom_rule_count} 条`}>
              <span>{status.rule_count} <span style={{ color: '#94a3b8', fontSize: 12 }}>(自定义 {status.custom_rule_count})</span></span>
            </Tooltip>
          </Descriptions.Item>
        </Descriptions>
      ) : (
        <Empty description="无法获取防火墙状态" />
      )}
      {status && (
        <div style={{ marginTop: 12, display: 'flex', gap: 24 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontWeight: 500 }}>默认入站策略:</span>
            <Select
              value={status.default_in}
              onChange={(val) => onChangePolicy('INPUT', val)}
              loading={policyChanging === 'INPUT'}
              disabled={policyChanging !== ''}
              size="small"
              style={{ width: 120 }}
              options={[
                { label: '允许(ACCEPT)', value: 'ACCEPT' },
                { label: '拒绝(DROP)', value: 'DROP' },
              ]}
            />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontWeight: 500 }}>默认出站策略:</span>
            <Select
              value={status.default_out}
              onChange={(val) => onChangePolicy('OUTPUT', val)}
              loading={policyChanging === 'OUTPUT'}
              disabled={policyChanging !== ''}
              size="small"
              style={{ width: 120 }}
              options={[
                { label: '允许(ACCEPT)', value: 'ACCEPT' },
                { label: '拒绝(DROP)', value: 'DROP' },
              ]}
            />
          </div>
        </div>
      )}
    </Card>
  );
}
