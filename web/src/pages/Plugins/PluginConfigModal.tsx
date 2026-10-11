import { useEffect, useState } from 'react';
import { Modal, Form, Input, InputNumber, Switch, Select, message, Button } from 'antd';
import { pluginsApi } from '../../services/plugins';
import type { PluginRecord } from '../../types';

// JSON Schema 属性（插件 manifest settings.config_schema）
interface SchemaProperty {
  type?: 'string' | 'number' | 'integer' | 'boolean';
  title?: string;
  description?: string;
  enum?: string[];
  default?: string | number | boolean;
}

interface Schema {
  type?: string;
  properties?: Record<string, SchemaProperty>;
}

// 插件配置表单：按 manifest config_schema 动态渲染（string/number/boolean/enum），
// 保存到面板 plugin_config（经 /plugins/:id/config）。
export default function PluginConfigModal({ plugin, open, onClose }: { plugin: PluginRecord; open: boolean; onClose: () => void }) {
  const [schema, setSchema] = useState<Schema>({});
  const [config, setConfig] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    if (!open || !plugin) return;
    setLoading(true);
    Promise.all([
      pluginsApi.getManifest(plugin.name).then((res) => {
        const data = res.data.data as { contributions?: { settings?: Array<{ config_schema?: Schema }> } } | undefined;
        setSchema(data?.contributions?.settings?.[0]?.config_schema || {});
      }),
      pluginsApi.getConfig(plugin.name).then((res) => setConfig(res.data.data?.config || {})),
    ]).finally(() => setLoading(false));
  }, [open, plugin]);

  useEffect(() => {
    if (!open) return;
    const props = schema.properties || {};
    const initial: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(props)) {
      initial[k] = config[k] ?? v.default ?? (v.type === 'boolean' ? false : v.type === 'number' || v.type === 'integer' ? undefined : '');
    }
    form.setFieldsValue(initial);
  }, [open, schema, config, form]);

  const handleSave = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      // 数值转字符串（配置以 KV 字符串存储）
      const kv: Record<string, string> = {};
      for (const [k, v] of Object.entries(values)) {
        kv[k] = v === null || v === undefined ? '' : String(v);
      }
      await pluginsApi.setConfig(plugin.name, kv);
      message.success('插件配置已保存');
      onClose();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const props = schema.properties || {};
  const hasProps = Object.keys(props).length > 0;

  return (
    <Modal
      title={`${plugin.name} 配置`}
      open={open}
      onCancel={onClose}
      width={520}
      footer={hasProps ? [
        <Button key="cancel" onClick={onClose}>取消</Button>,
        <Button key="save" type="primary" loading={saving} onClick={handleSave}>保存</Button>,
      ] : null}
      destroyOnHidden
    >
      {!hasProps ? (
        <div style={{ opacity: 0.7, padding: '20px 0', textAlign: 'center' }}>
          {loading ? '加载中…' : '该插件未声明配置项（config_schema 为空）'}
        </div>
      ) : (
        <Form form={form} layout="vertical">
          {Object.entries(props).map(([key, p]) => (
            <Form.Item key={key} name={key} label={p.title || key} extra={p.description}>
              {p.type === 'boolean' ? (
                <Switch />
              ) : p.type === 'number' || p.type === 'integer' ? (
                <InputNumber style={{ width: '100%' }} />
              ) : p.enum ? (
                <Select options={p.enum.map((e) => ({ value: e, label: e }))} allowClear />
              ) : (
                <Input />
              )}
            </Form.Item>
          ))}
        </Form>
      )}
    </Modal>
  );
}
