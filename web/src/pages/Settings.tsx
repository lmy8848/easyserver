import { useState, useEffect } from 'react';
import {
  Card, Descriptions, Tag, Spin, Tabs, Alert,
} from 'antd';
import {
  SettingOutlined, SafetyOutlined, DatabaseOutlined,
  CloudOutlined, MonitorOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import api from '../services/api';

interface Settings {
  server: {
    port: number;
    host: string;
    serve_frontend: boolean;
    tls_enabled: boolean;
  };
  auth: {
    session_timeout: string;
    idle_timeout: string;
    max_login_attempts: number;
    lockout_duration: string;
    rate_limit: number;
    rate_interval: string;
  };
  monitor: {
    history_retention: string;
    collect_interval: string;
  };
  database: {
    path: string;
  };
  audit: {
    enabled: boolean;
    log_path: string;
  };
  tencentcloud: {
    enabled: boolean;
    region: string;
    instance_id: string;
  };
}

interface SystemInfo {
  version: string;
  go_version: string;
  platform: string;
}

export default function Settings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchSettings();
  }, []);

  const fetchSettings = async () => {
    setLoading(true);
    try {
      const [settingsRes, infoRes] = await Promise.all([
        api.get<any>('/settings'),
        api.get<any>('/settings/system'),
      ]);
      setSettings(settingsRes.data?.data || null);
      setSystemInfo(infoRes.data?.data || null);
    } catch (error) {
      console.error('Failed to fetch settings:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <Alert
        message="面板设置"
        description="此处显示当前系统配置。如需修改配置，请直接编辑服务器上的 config.yaml 文件，然后重启服务。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Tabs
        items={[
          {
            key: 'server',
            label: <span><SettingOutlined /> 服务器</span>,
            children: (
              <Card>
                <Descriptions bordered column={{ xs: 1, sm: 2 }}>
                  <Descriptions.Item label="监听地址">{settings?.server.host}</Descriptions.Item>
                  <Descriptions.Item label="监听端口">{settings?.server.port}</Descriptions.Item>
                  <Descriptions.Item label="TLS/HTTPS">
                    {settings?.server.tls_enabled
                      ? <Tag color="success">已启用</Tag>
                      : <Tag color="default">未启用</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="提供前端">
                    {settings?.server.serve_frontend
                      ? <Tag color="success">是</Tag>
                      : <Tag color="default">否</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="系统版本">{systemInfo?.version || '-'}</Descriptions.Item>
                  <Descriptions.Item label="Go 版本">{systemInfo?.go_version || '-'}</Descriptions.Item>
                  <Descriptions.Item label="平台">{systemInfo?.platform || '-'}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'auth',
            label: <span><SafetyOutlined /> 认证安全</span>,
            children: (
              <Card>
                <Descriptions bordered column={{ xs: 1, sm: 2 }}>
                  <Descriptions.Item label="会话超时">{settings?.auth.session_timeout}</Descriptions.Item>
                  <Descriptions.Item label="空闲超时">{settings?.auth.idle_timeout}</Descriptions.Item>
                  <Descriptions.Item label="最大登录尝试">{settings?.auth.max_login_attempts} 次</Descriptions.Item>
                  <Descriptions.Item label="锁定时长">{settings?.auth.lockout_duration}</Descriptions.Item>
                  <Descriptions.Item label="速率限制">{settings?.auth.rate_limit} 次/{settings?.auth.rate_interval}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'monitor',
            label: <span><MonitorOutlined /> 系统监控</span>,
            children: (
              <Card>
                <Descriptions bordered column={{ xs: 1, sm: 2 }}>
                  <Descriptions.Item label="采集间隔">{settings?.monitor.collect_interval}</Descriptions.Item>
                  <Descriptions.Item label="历史保留">{settings?.monitor.history_retention}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'database',
            label: <span><DatabaseOutlined /> 数据库</span>,
            children: (
              <Card>
                <Descriptions bordered column={1}>
                  <Descriptions.Item label="数据库路径">{settings?.database.path}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'audit',
            label: <span><InfoCircleOutlined /> 审计日志</span>,
            children: (
              <Card>
                <Descriptions bordered column={{ xs: 1, sm: 2 }}>
                  <Descriptions.Item label="审计功能">
                    {settings?.audit.enabled
                      ? <Tag color="success">已启用</Tag>
                      : <Tag color="default">未启用</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="日志路径">{settings?.audit.log_path}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'cloud',
            label: <span><CloudOutlined /> 腾讯云</span>,
            children: (
              <Card>
                <Descriptions bordered column={{ xs: 1, sm: 2 }}>
                  <Descriptions.Item label="腾讯云集成">
                    {settings?.tencentcloud.enabled
                      ? <Tag color="success">已启用</Tag>
                      : <Tag color="default">未启用</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="地域">{settings?.tencentcloud.region || '-'}</Descriptions.Item>
                  <Descriptions.Item label="实例 ID">{settings?.tencentcloud.instance_id || '-'}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
        ]}
      />
    </div>
  );
}
