import { useState, useEffect, useCallback } from 'react';
import {
  Card, Spin, Tabs, Alert,
} from 'antd';
import {
  SettingOutlined, SafetyOutlined,
  CloudOutlined, MonitorOutlined, InfoCircleOutlined,
  AlertOutlined,
} from '@ant-design/icons';
import { settingsApi } from '../../services/api';
import type { Settings, SystemInfo } from './types';
import ServerSettings from './ServerSettings';
import AuthSettings from './AuthSettings';
import MonitorSettings from './MonitorSettings';
import AuditSettings from './AuditSettings';
import NotifySettings from './NotifySettings';
import CloudSettings from './CloudSettings';
import AlertRulesForm from './AlertRulesForm';

export default function SettingsPage() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchSettings = useCallback(async () => {
    setLoading(true);
    try {
      const [settingsRes, infoRes] = await Promise.all([
        settingsApi.get(),
        settingsApi.getSystem(),
      ]);
      setSettings(settingsRes.data?.data);
      setSystemInfo(infoRes.data?.data || null);
    } catch (error) {
      console.error('Failed to fetch settings:', error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!settings) {
    return <div>Failed to load settings</div>;
  }

  return (
    <div>
      <Tabs
        items={[
          {
            key: 'server',
            label: <span><SettingOutlined /> 服务器</span>,
            children: (
              <ServerSettings
                settings={settings}
                systemInfo={systemInfo}
                onRefresh={fetchSettings}
              />
            ),
          },
          {
            key: 'auth',
            label: <span><SafetyOutlined /> 认证安全</span>,
            children: (
              <AuthSettings
                settings={settings}
                onRefresh={fetchSettings}
              />
            ),
          },
          {
            key: 'monitor',
            label: <span><MonitorOutlined /> 系统监控</span>,
            children: (
              <MonitorSettings
                settings={settings}
                onRefresh={fetchSettings}
              />
            ),
          },
          {
            key: 'audit',
            label: <span><InfoCircleOutlined /> 审计日志</span>,
            children: (
              <AuditSettings
                settings={settings}
                onRefresh={fetchSettings}
              />
            ),
          },
          {
            key: 'notify',
            label: <span><InfoCircleOutlined /> 通知</span>,
            children: (
              <NotifySettings
                settings={settings}
                onRefresh={fetchSettings}
              />
            ),
          },
          {
            key: 'alerts',
            label: <span><AlertOutlined /> 告警</span>,
            children: (
              <Card title="监控告警规则">
                <Alert
                  title="告警规则配置"
                  description="配置监控指标阈值，超过阈值持续指定时间后发送 Webhook 通知。需要先在「通知」标签页配置 Webhook URL。"
                  type="info"
                  showIcon
                  style={{ marginBottom: 16 }}
                />
                <AlertRulesForm />
              </Card>
            ),
          },
          {
            key: 'cloud',
            label: <span><CloudOutlined /> 腾讯云</span>,
            children: (
              <CloudSettings
                settings={settings}
                onRefresh={fetchSettings}
              />
            ),
          },
        ]}
      />
    </div>
  );
}
