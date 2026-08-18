import { Card, Button, Space, Tag, Tooltip, Row, Col } from 'antd';
import {
  PlayCircleOutlined, StopOutlined,
  ReloadOutlined, DownloadOutlined,
} from '@ant-design/icons';
import { SiNginx, SiApache, SiApachetomcat, SiCaddy } from '@icons-pack/react-simple-icons';
import type { WebServer } from '../../types';
import { getStatusColor, StatusTag } from '../../utils/status';

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

const SERVER_DESCRIPTIONS: Record<string, string> = {
  nginx: '高性能 HTTP 与反向代理服务器，采用异步事件驱动架构，具备高并发吞吐与负载均衡能力',
  apache: '历史悠久且功能完备的开源 HTTP 服务器，支持多处理模块（MPM）与动态模块加载',
  tomcat: '开源 Java Servlet 与 JSP 容器，完整实现 Jakarta EE 核心规范，专门用于运行 Java 应用',
  caddy: 'Go 语言编写的现代 Web 服务器，内置自动 HTTPS 证书申请、HTTP/3 支持与极简配置',
};

function renderServerIcon(name: string, size = 36) {
  const s = (name || '').toLowerCase();
  switch (s) {
    case 'nginx':
      return <SiNginx size={size} color="#009639" style={{ flexShrink: 0, verticalAlign: 'middle' }} />;
    case 'apache':
    case 'apache2':
      return <SiApache size={size} color="#D22128" style={{ flexShrink: 0, verticalAlign: 'middle' }} />;
    case 'tomcat':
    case 'tomcat9':
      return <SiApachetomcat size={size} color="#F89820" style={{ flexShrink: 0, verticalAlign: 'middle' }} />;
    case 'caddy':
      return <SiCaddy size={size} color="#00D7B2" style={{ flexShrink: 0, verticalAlign: 'middle' }} />;
    default:
      return <SiNginx size={size} color="#1890FF" style={{ flexShrink: 0, verticalAlign: 'middle' }} />;
  }
}

function statusTag(status: string) {
  return <StatusTag status={status} />;
}

function statusColor(status: string) {
  return getStatusColor(status);
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
        {servers.map(server => {
          const desc = SERVER_DESCRIPTIONS[(server.name || '').toLowerCase()] || server.description;
          return (
            <Col xs={24} sm={12} lg={6} key={server.id} style={{ display: 'flex' }}>
              <Card
                hoverable
                onClick={() => server.status !== 'not_installed' && onEnterServer(server)}
                style={{
                  borderColor: statusColor(server.status),
                  width: '100%',
                  display: 'flex',
                  flexDirection: 'column',
                }}
                styles={{
                  body: {
                    flex: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                  },
                }}
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
                <div>
                  <Card.Meta
                    avatar={renderServerIcon(server.name, 36)}
                    title={
                      <Space wrap>
                        {server.display_name}
                        {statusTag(server.status)}
                      </Space>
                    }
                    description={
                      <p style={{ margin: '8px 0', color: '#666', lineHeight: 1.5 }}>{desc}</p>
                    }
                  />
                </div>
                {server.version && (
                  <div style={{ marginTop: 'auto', paddingTop: 8 }}>
                    <Tag color="blue">{server.version}</Tag>
                  </div>
                )}
              </Card>
            </Col>
          );
        })}
      </Row>
    </div>
  );
}
