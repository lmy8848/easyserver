import { useState, useEffect } from 'react';
import { Card, Tabs, Tag, Button, Space, Spin, Alert, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { cloudApi } from '../../services/api';
import type { CloudInstance, CloudFirewallRule, Snapshot } from '../../types';
import CloudInstances from './CloudInstances';
import CloudFirewall from './CloudFirewall';
import CloudSnapshots from './CloudSnapshots';
import CloudMonitor from './CloudMonitor';
import CloudTraffic from './CloudTraffic';

export default function Cloud() {
  const [instances, setInstances] = useState<CloudInstance[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedInstance, setSelectedInstance] = useState('');
  const [firewallRules, setFirewallRules] = useState<CloudFirewallRule[]>([]);
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);

  const fetchInstances = async () => {
    setError(null);
    try {
      const res = await cloudApi.getInstances();
      setInstances(res.data.data?.instances || []);
    } catch (error: unknown) {
      console.error('Failed to fetch instances:', error);
      setError((error instanceof Error ? error.message : '获取实例列表失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchInstances();
  }, []);

  const fetchFirewall = async (instanceId: string) => {
    try {
      const res = await cloudApi.getFirewall(instanceId);
      setFirewallRules(res.data.data?.rules || []);
    } catch (error: unknown) {
      console.error('Failed to fetch firewall rules:', error);
      message.error((error instanceof Error ? error.message : '获取防火墙规则失败'));
    }
  };

  const fetchSnapshots = async () => {
    try {
      const res = await cloudApi.getSnapshots();
      setSnapshots(res.data.data?.snapshots || []);
    } catch (error: unknown) {
      console.error('Failed to fetch snapshots:', error);
      message.error((error instanceof Error ? error.message : '获取快照列表失败'));
    }
  };

  const handleSelectInstance = (instanceId: string) => {
    setSelectedInstance(instanceId);
    fetchFirewall(instanceId);
  };

  const handleInstanceAction = async (instanceId: string, action: string) => {
    try {
      switch (action) {
        case 'start':
          await cloudApi.startInstance(instanceId);
          message.success('实例已启动');
          break;
        case 'stop':
          await cloudApi.stopInstance(instanceId);
          message.success('实例已停止');
          break;
        case 'restart':
          await cloudApi.restartInstance(instanceId);
          message.success('实例已重启');
          break;
      }
      setLoading(true);
      fetchInstances();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    }
  };

  return (
    <div>
      {error && (
        <Alert
          message="错误"
          description={error}
          type="error"
          closable
          onClose={() => setError(null)}
          style={{ marginBottom: 16 }}
        />
      )}
      <Card
        title="腾讯云管理"
        extra={
          <Space>
            {selectedInstance && (
              <Tag color="blue">
                当前实例: {instances.find(i => i.instance_id === selectedInstance)?.name || selectedInstance}
              </Tag>
            )}
            <Button icon={<ReloadOutlined />} onClick={() => { setLoading(true); fetchInstances(); }} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      >
        <Spin spinning={loading}>
          <Tabs
            items={[
              {
                key: 'instances',
                label: '实例管理',
                children: (
                  <CloudInstances
                    instances={instances}
                    loading={loading}
                    selectedInstance={selectedInstance}
                    onSelect={handleSelectInstance}
                    onAction={handleInstanceAction}
                  />
                ),
              },
              {
                key: 'firewall',
                label: '防火墙规则',
                disabled: !selectedInstance,
                children: (
                  <CloudFirewall
                    firewallRules={firewallRules}
                    selectedInstance={selectedInstance}
                    onRefresh={() => fetchFirewall(selectedInstance)}
                  />
                ),
              },
              {
                key: 'snapshots',
                label: '快照管理',
                disabled: !selectedInstance,
                children: (
                  <CloudSnapshots
                    snapshots={snapshots}
                    selectedInstance={selectedInstance}
                    onRefresh={fetchSnapshots}
                  />
                ),
              },
              {
                key: 'monitor',
                label: '监控数据',
                disabled: !selectedInstance,
                children: <CloudMonitor selectedInstance={selectedInstance} />,
              },
              {
                key: 'traffic',
                label: '流量统计',
                disabled: !selectedInstance,
                children: <CloudTraffic selectedInstance={selectedInstance} />,
              },
            ]}
          />
        </Spin>
      </Card>
    </div>
  );
}
