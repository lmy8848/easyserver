import {
  Card, Button, Space, Tag, Table, Empty, Tooltip, Popconfirm, Switch, Upload,
} from 'antd';
import type { Key } from 'antd/es/table/interface';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  SafetyOutlined, CheckCircleOutlined, CloseCircleOutlined,
  SnippetsOutlined, UploadOutlined, DownloadOutlined,
  ArrowUpOutlined, ArrowDownOutlined,
} from '@ant-design/icons';
import type { FirewallRule } from '../../types';
import { actionColor } from './types';

interface Props {
  rules: FirewallRule[];
  systemRules: FirewallRule[];
  loading: boolean;
  operating: string;
  selectedRowKeys: Key[];
  bulkOperating: boolean;
  exporting: boolean;
  importing: boolean;
  onCreate: () => void;
  onEdit: (rule: FirewallRule) => void;
  onDelete: (id: number) => void;
  onToggleRule: (rule: FirewallRule) => void;
  onMoveUp: (id: number) => void;
  onMoveDown: (id: number) => void;
  onBulkEnable: () => void;
  onBulkDisable: () => void;
  onBulkDelete: () => void;
  onExport: () => void;
  onImportFileChange: (info: any) => void;
  onOpenTemplates: () => void;
  onRefresh: () => void;
  onSelectedRowKeysChange: (keys: Key[]) => void;
}

export default function FirewallRules({
  rules,
  systemRules,
  loading,
  operating,
  selectedRowKeys,
  bulkOperating,
  exporting,
  importing,
  onCreate,
  onEdit,
  onDelete,
  onToggleRule,
  onMoveUp,
  onMoveDown,
  onBulkEnable,
  onBulkDisable,
  onBulkDelete,
  onExport,
  onImportFileChange,
  onOpenTemplates,
  onRefresh,
  onSelectedRowKeysChange,
}: Props) {
  const columns = [
    {
      title: '链',
      dataIndex: 'chain',
      key: 'chain',
      width: 80,
      render: (chain: string) => <Tag>{chain}</Tag>,
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      key: 'protocol',
      width: 80,
      render: (protocol: string) => protocol.toUpperCase(),
    },
    {
      title: 'IP 版本',
      dataIndex: 'ip_version',
      key: 'ip_version',
      width: 80,
      render: (ipVersion: string) => {
        const label = ipVersion === 'both' ? '双栈' : (ipVersion || 'IPv4').toUpperCase();
        const color = ipVersion === 'ipv6' ? 'purple' : ipVersion === 'both' ? 'blue' : 'default';
        return <Tag color={color}>{label}</Tag>;
      },
    },
    {
      title: '端口',
      dataIndex: 'port',
      key: 'port',
      width: 100,
      render: (port: string) => port || <span style={{ color: '#8c8c8c' }}>所有</span>,
    },
    {
      title: '动作',
      dataIndex: 'action',
      key: 'action',
      width: 100,
      render: (action: string) => <Tag color={actionColor(action)}>{action}</Tag>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      render: (source: string) => source || <span style={{ color: '#8c8c8c' }}>所有</span>,
    },
    {
      title: '备注',
      dataIndex: 'remark',
      key: 'remark',
      ellipsis: true,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled: boolean, record: FirewallRule) => (
        <Switch
          checked={enabled}
          onChange={() => onToggleRule(record)}
          loading={operating === `rule-${record.id}`}
          size="small"
        />
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, record: FirewallRule, index: number) => (
        <Space>
          <Tooltip title="上移">
            <Button
              type="link"
              icon={<ArrowUpOutlined />}
              disabled={index === 0}
              loading={operating === `move-${record.id}`}
              onClick={() => onMoveUp(record.id)}
            />
          </Tooltip>
          <Tooltip title="下移">
            <Button
              type="link"
              icon={<ArrowDownOutlined />}
              disabled={index === rules.length - 1}
              loading={operating === `move-${record.id}`}
              onClick={() => onMoveDown(record.id)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => onEdit(record)} />
          </Tooltip>
          <Popconfirm
            title="确定删除此规则？"
            description="删除后将从系统中移除"
            onConfirm={() => onDelete(record.id)}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Tooltip title="删除">
              <Button type="link" icon={<DeleteOutlined />} danger />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const rowSelection = {
    selectedRowKeys,
    onChange: (keys: Key[]) => onSelectedRowKeysChange(keys),
  };

  return (
    <Card
      title={<Space><SafetyOutlined /> 防火墙规则</Space>}
      extra={
        <Space>
          <Button icon={<DownloadOutlined />} onClick={onExport} loading={exporting}>导出</Button>
          <Upload
            accept=".json"
            showUploadList={false}
            beforeUpload={() => false}
            onChange={onImportFileChange}
          >
            <Button icon={<UploadOutlined />} loading={importing}>导入</Button>
          </Upload>
          <Button icon={<ReloadOutlined />} onClick={onRefresh} loading={loading}>刷新</Button>
          <Button icon={<SnippetsOutlined />} onClick={onOpenTemplates}>模板规则</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={onCreate}>添加规则</Button>
        </Space>
      }
    >
      {/* Database Rules */}
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <h4 style={{ margin: 0 }}>自定义规则</h4>
          {selectedRowKeys.length > 0 && (
            <Space>
              <span style={{ color: '#1890ff' }}>已选 {selectedRowKeys.length} 项</span>
              <Button
                size="small"
                icon={<CheckCircleOutlined />}
                onClick={onBulkEnable}
                loading={bulkOperating}
              >
                批量启用
              </Button>
              <Button
                size="small"
                icon={<CloseCircleOutlined />}
                onClick={onBulkDisable}
                loading={bulkOperating}
              >
                批量禁用
              </Button>
              <Popconfirm
                title="确定批量删除选中的规则？"
                description="删除后将从系统中移除"
                onConfirm={onBulkDelete}
                okText="删除"
                cancelText="取消"
                okButtonProps={{ danger: true }}
              >
                <Button
                  size="small"
                  icon={<DeleteOutlined />}
                  danger
                  loading={bulkOperating}
                >
                  批量删除
                </Button>
              </Popconfirm>
            </Space>
          )}
        </div>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey="id"
          loading={loading}
          size="small"
          locale={{ emptyText: <Empty description="暂无自定义规则" /> }}
          pagination={false}
          rowSelection={rowSelection}
          rowClassName={(record) => record.enabled ? '' : 'firewall-rule-disabled'}
        />
      </div>

      {/* System Rules - Read-only view */}
      {systemRules.length > 0 && (
        <div>
          <h4 style={{ marginBottom: 8 }}>
            系统规则
            <Tooltip title="系统规则是防火墙中实际生效的规则，通过左侧「自定义规则」管理">
              <Tag color="blue" style={{ marginLeft: 8 }}>只读</Tag>
            </Tooltip>
          </h4>
          <Table
            columns={[
              { title: '链', dataIndex: 'chain', key: 'chain', width: 80, render: (chain: string) => <Tag>{chain}</Tag> },
              { title: '协议', dataIndex: 'protocol', key: 'protocol', width: 80, render: (p: string) => p?.toUpperCase() || '-' },
              { title: '端口', dataIndex: 'port', key: 'port', width: 100, render: (port: string) => port || <span style={{ color: '#8c8c8c' }}>所有</span> },
              { title: '动作', dataIndex: 'action', key: 'action', width: 100, render: (action: string) => <Tag color={actionColor(action)}>{action}</Tag> },
              { title: '来源', dataIndex: 'source', key: 'source', render: (source: string) => source || <span style={{ color: '#8c8c8c' }}>所有</span> },
            ]}
            dataSource={systemRules}
            rowKey={(r) => `${r.chain}-${r.protocol}-${r.port}-${r.action}-${r.source}`}
            size="small"
            pagination={false}
          />
        </div>
      )}
    </Card>
  );
}
