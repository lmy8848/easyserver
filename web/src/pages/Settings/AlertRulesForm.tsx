import { useState, useEffect } from 'react';
import {
  Card, Spin, Form, Input, Select, Switch, Button, Space, message,
  InputNumber,
} from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import { settingsApi } from '../../services/api';
import type { AlertRule } from './types';

export default function AlertRulesForm() {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setLoading(true);
    settingsApi.getAlertRules()
      .then(res => {
        setRules(res.data?.data?.rules || []);
      })
      .catch((err) => {
        message.error('Failed to load alert rules: ' + (err.message || 'unknown error'));
      })
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async () => {
    setSaving(true);
    try {
      await settingsApi.updateAlertRules(rules);
      message.success('告警规则已保存');
    } catch {
      message.error('保存失败');
    } finally {
      setSaving(false);
    }
  };

  const addRule = () => {
    setRules([...rules, { name: '', metric: 'cpu_percent', threshold: 80, duration: 60, enabled: true }]);
  };

  const removeRule = (index: number) => {
    setRules(rules.filter((_, i) => i !== index));
  };

  const updateRule = (index: number, field: keyof AlertRule, value: string | number | boolean) => {
    const newRules = [...rules];
    newRules[index] = { ...newRules[index], [field]: value };
    setRules(newRules);
  };

  if (loading) return <Spin />;

  return (
    <div>
      {rules.map((rule, index) => (
        <Card
          key={index}
          size="small"
          style={{ marginBottom: 8 }}
          extra={
            <Button
              icon={<DeleteOutlined />}
              size="small"
              danger
              onClick={() => removeRule(index)}
            />
          }
        >
          <Space wrap>
            <Input
              placeholder="规则名称"
              value={rule.name}
              onChange={e => updateRule(index, 'name', e.target.value)}
              style={{ width: 150 }}
            />
            <Select
              value={rule.metric}
              onChange={v => updateRule(index, 'metric', v)}
              style={{ width: 130 }}
              options={[
                { label: 'CPU 使用率', value: 'cpu_percent' },
                { label: '内存使用率', value: 'mem_percent' },
                { label: '磁盘使用率', value: 'disk_percent' },
                { label: '1分钟负载', value: 'load_1m' },
                { label: '5分钟负载', value: 'load_5m' },
                { label: '15分钟负载', value: 'load_15m' },
              ]}
            />
            <InputNumber
              placeholder="阈值"
              value={rule.threshold}
              onChange={v => updateRule(index, 'threshold', v || 0)}
              min={0}
              max={100}
              addonAfter="%"
              style={{ width: 120 }}
            />
            <InputNumber
              placeholder="持续时间"
              value={rule.duration}
              onChange={v => updateRule(index, 'duration', v || 0)}
              min={0}
              addonAfter="秒"
              style={{ width: 120 }}
            />
            <Switch
              checked={rule.enabled}
              onChange={v => updateRule(index, 'enabled', v)}
              checkedChildren="启用"
              unCheckedChildren="禁用"
            />
          </Space>
        </Card>
      ))}
      <Space style={{ marginTop: 8 }}>
        <Button icon={<PlusOutlined />} onClick={addRule}>
          添加规则
        </Button>
        <Button type="primary" onClick={handleSave} loading={saving}>
          保存规则
        </Button>
      </Space>
    </div>
  );
}
