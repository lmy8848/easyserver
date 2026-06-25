import {
  Card, Button, Space, Tag, Modal, Form, Select, InputNumber,
  Row, Col, Empty,
} from 'antd';
import {
  DatabaseOutlined, ReloadOutlined,
} from '@ant-design/icons';
import type { ServerListProps } from './types';
import { getServiceStatusColor, ServiceStatusTag } from '../../utils/status';

export default function ServerList({
  servers, loading, onEnterServer, onRefresh,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, onInstallVersion,
  portCheck, onCheckPort,
}: ServerListProps) {
  const statusColor = (status: string) => {
    const colorName = getServiceStatusColor(status);
    const colorMap: Record<string, string> = {
      success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
    };
    return colorMap[colorName] || '#999';
  };

  const statusTag = (status: string) => <ServiceStatusTag status={status} />;

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>刷新</Button>
      </div>
      {servers.length === 0 && !loading ? (
        <Empty description="暂无数据库服务器" />
      ) : (
        <Row gutter={[16, 16]}>
          {servers.map(server => (
            <Col xs={24} sm={12} lg={8} key={server.id}>
              <Card hoverable onClick={() => onEnterServer(server)} style={{ borderColor: statusColor(server.status) }}>
                <Card.Meta
                  avatar={<DatabaseOutlined style={{ fontSize: 32, color: statusColor(server.status) }} />}
                  title={<Space>{server.display_name}{statusTag(server.status)}</Space>}
                  description={<div>
                    <p style={{ margin: '8px 0', color: '#666' }}>{server.description}</p>
                    {server.version && <Tag color="blue">已安装: {server.version}</Tag>}
                    <Tag>默认端口: {server.default_port}</Tag>
                  </div>} />
              </Card>
            </Col>
          ))}
        </Row>
      )}

      {/* Install Version Modal */}
      <Modal title="安装数据库版本" open={installVersionVisible} onCancel={() => onInstallVersionVisibleChange(false)}
        onOk={onInstallVersion} okText="安装" cancelText="取消">
        <Form form={installVersionForm} layout="vertical">
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select placeholder="选择要安装的版本">
              {versionTemplates.map(t => (
                <Select.Option key={t.version} value={t.version}>
                  <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="port" label="端口（留空使用默认）"
            extra={portCheck && (
              portCheck.available
                ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
            )}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }}
              onChange={(val) => val && onCheckPort(val as number)} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
