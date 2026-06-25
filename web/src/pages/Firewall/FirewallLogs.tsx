import { Card, Button, Space, Tag, Select, Switch, Table, Empty } from 'antd';
import { ReloadOutlined, FileTextOutlined } from '@ant-design/icons';
import type { FirewallLogEntry } from '../../types';
import { actionColor } from './types';

interface Props {
  logs: FirewallLogEntry[];
  loading: boolean;
  logLines: number;
  autoRefresh: boolean;
  onLogLinesChange: (lines: number) => void;
  onAutoRefreshChange: (auto: boolean) => void;
  onRefresh: () => void;
}

export default function FirewallLogs({
  logs,
  loading,
  logLines,
  autoRefresh,
  onLogLinesChange,
  onAutoRefreshChange,
  onRefresh,
}: Props) {
  return (
    <Card
      title={<Space><FileTextOutlined /> 防火墙日志</Space>}
      style={{ marginBottom: 16 }}
      extra={
        <Space>
          <Select
            value={logLines}
            onChange={onLogLinesChange}
            size="small"
            style={{ width: 100 }}
            options={[
              { label: '100 行', value: 100 },
              { label: '500 行', value: 500 },
              { label: '1000 行', value: 1000 },
            ]}
          />
          <Space>
            <span style={{ fontSize: 12 }}>自动刷新</span>
            <Switch
              size="small"
              checked={autoRefresh}
              onChange={onAutoRefreshChange}
            />
          </Space>
          <Button
            icon={<ReloadOutlined />}
            onClick={onRefresh}
            loading={loading}
            size="small"
          >
            刷新
          </Button>
        </Space>
      }
    >
      <Table
        dataSource={logs}
        rowKey={(_, index) => `log-${index}`}
        loading={loading}
        size="small"
        locale={{ emptyText: <Empty description="暂无防火墙日志" /> }}
        pagination={{ pageSize: 50, showSizeChanger: false }}
        columns={[
          {
            title: '时间',
            dataIndex: 'timestamp',
            key: 'timestamp',
            width: 160,
            ellipsis: true,
          },
          {
            title: '动作',
            dataIndex: 'action',
            key: 'action',
            width: 90,
            render: (action: string) => (
              <Tag color={actionColor(action)}>{action}</Tag>
            ),
          },
          {
            title: '协议',
            dataIndex: 'protocol',
            key: 'protocol',
            width: 70,
            render: (protocol: string) => protocol?.toUpperCase() || '-',
          },
          {
            title: '来源 IP',
            dataIndex: 'src_ip',
            key: 'src_ip',
            width: 140,
            render: (ip: string) => ip || '-',
          },
          {
            title: '目标 IP',
            dataIndex: 'dst_ip',
            key: 'dst_ip',
            width: 140,
            render: (ip: string) => ip || '-',
          },
          {
            title: '目标端口',
            dataIndex: 'dst_port',
            key: 'dst_port',
            width: 90,
            render: (port: number) => port || '-',
          },
          {
            title: '接口',
            dataIndex: 'interface',
            key: 'interface',
            width: 80,
            render: (iface: string) => iface || '-',
          },
        ]}
      />
    </Card>
  );
}
