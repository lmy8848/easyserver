import { Card, Button, Space, Tag, Tooltip, Row, Col } from 'antd';
import {
  CloudServerOutlined, PlayCircleOutlined, StopOutlined,
  ReloadOutlined, DownloadOutlined,
} from '@ant-design/icons';
import type { WebServer } from '../../types';
import { getServiceStatusColor, ServiceStatusTag } from '../../utils/status';

interface WebServerListProps {
  servers: WebServer[];
  loading: boolean;
  operating: string;
  onEnterServer: (server: WebServer) => void;
  onInstall: (server: WebServer) => void;
  onStart: (server: WebServer) => void;
  onStop: (server: WebServer) => void;
  onRestart: (server: WebServer) => void;
  onRefresh: () => void;
}

function statusTag(status: string) {
  return <ServiceStatusTag status={status} />;
}

function statusColor(status: string) {
  const colorName = getServiceStatusColor(status);
  const colorMap: Record<string, string> = {
    success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
  };
  return colorMap[colorName] || '#999';
}

export default function WebServerList({
  servers, loading, operating, onEnterServer, onInstall, onStart, onStop, onRestart, onRefresh,
}: WebServerListProps) {
  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
          刷新
        </Button>
      </div>
      <Row gutter={[16, 16]}>
        {servers.map(server => (
          <Col xs={24} sm={12} lg={6} key={server.id}>
            <Card
              hoverable
              onClick={() => server.status !== 'not_installed' && onEnterServer(server)}
              style={{ borderColor: statusColor(server.status) }}
              actions={[
                server.status === 'not_installed' ? (
                  <Tooltip title="安装" key="install">
                    <Button type="link" icon={<DownloadOutlined />} loading={operating === `install-${server.id}`} onClick={(e) => { e.stopPropagation(); onInstall(server); }}>
                      安装
                    </Button>
                  </Tooltip>
                ) : server.status === 'running' ? (
                  <Tooltip title="停止" key="stop">
                    <Button type="link" danger icon={<StopOutlined />} loading={operating === `stop-${server.id}`} onClick={(e) => { e.stopPropagation(); onStop(server); }}>
                      停止
                    </Button>
                  </Tooltip>
                ) : (
                  <Tooltip title="启动" key="start">
                    <Button type="link" icon={<PlayCircleOutlined />} loading={operating === `start-${server.id}`} onClick={(e) => { e.stopPropagation(); onStart(server); }}>
                      启动
                    </Button>
                  </Tooltip>
                ),
                server.status !== 'not_installed' && (
                  <Tooltip title="重启" key="restart">
                    <Button type="link" icon={<ReloadOutlined />} loading={operating === `restart-${server.id}`} onClick={(e) => { e.stopPropagation(); onRestart(server); }}>
                      重启
                    </Button>
                  </Tooltip>
                ),
              ].filter(Boolean)}
            >
              <Card.Meta
                avatar={<CloudServerOutlined style={{ fontSize: 32, color: statusColor(server.status) }} />}
                title={
                  <Space>
                    {server.display_name}
                    {statusTag(server.status)}
                  </Space>
                }
                description={
                  <div>
                    <p style={{ margin: '8px 0', color: '#666' }}>{server.description}</p>
                    {server.version && <Tag color="blue">{server.version}</Tag>}
                  </div>
                }
              />
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
}
