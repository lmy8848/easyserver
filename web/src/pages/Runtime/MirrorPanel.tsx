import { useEffect, useState, useMemo } from 'react';
import { Modal, Table, Switch, Input, Button, Space, Tag, message, Popconfirm, Form, Select } from 'antd';
import { SyncOutlined, PlusOutlined } from '@ant-design/icons';
import api from '../../services/api';
import type { CatalogEntry } from './types';

// MirrorPanel now operates entirely on the /api/env-config endpoint.
// Mirror sources are just env vars (MISE_NODE_MIRROR_URL etc.) stored in
// env_configs. This panel filters env_configs by the catalog's mirror_envs
// keys and renders them grouped by language, offering the extra UX of
// pre-seeded candidate URLs (from catalog.mirror_candidates, which excludes
// mise's default source) on the add form.

interface EnvConfigRow {
  id: number;
  name: string;
  value: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface MirrorPanelProps {
  visible: boolean;
  onClose: () => void;
  catalog: CatalogEntry[];
}

// Build a lookup: env_key -> lang, and env_key -> display name, driven by
// catalog. Only languages with non-empty mirror_envs are considered.
function useMirrorCatalog(catalog: CatalogEntry[]) {
  return useMemo(() => {
    const keyToLang = new Map<string, string>();
    const keyToDisplay = new Map<string, string>();
    const langToEntry = new Map<string, CatalogEntry>();
    const supportedKeys = new Set<string>();
    for (const c of catalog) {
      if (c.mirror_envs.length === 0) continue;
      langToEntry.set(c.lang, c);
      for (const k of c.mirror_envs) {
        keyToLang.set(k, c.lang);
        keyToDisplay.set(k, c.display);
        supportedKeys.add(k);
      }
    }
    return { keyToLang, keyToDisplay, langToEntry, supportedKeys };
  }, [catalog]);
}

export default function MirrorPanel({ visible, onClose, catalog }: MirrorPanelProps) {
  const [configs, setConfigs] = useState<EnvConfigRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<Record<number, string>>({});
  const [addVisible, setAddVisible] = useState(false);
  const [addForm] = Form.useForm();
  const [addSubmitting, setAddSubmitting] = useState(false);

  const { keyToLang, keyToDisplay, langToEntry, supportedKeys } = useMirrorCatalog(catalog);

  const fetchConfigs = async () => {
    setLoading(true);
    try {
      const res = await api.get('/env-config');
      const all: EnvConfigRow[] = res.data.data?.configs || [];
      // Only show env vars whose name is a declared mirror env key.
      setConfigs(all.filter(c => supportedKeys.has(c.name)));
    } catch {
      message.error('获取镜像配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible && catalog.length > 0) fetchConfigs();
  }, [visible, catalog.length]);

  const handleToggle = async (c: EnvConfigRow, enabled: boolean) => {
    try {
      await api.put(`/env-config/${c.id}`, { name: c.name, value: c.value, enabled });
      message.success(enabled ? '已启用' : '已禁用');
      fetchConfigs();
    } catch (e: any) {
      message.error(e?.message || '更新失败');
    }
  };

  const handleSave = async (c: EnvConfigRow) => {
    const next = editing[c.id];
    if (next === undefined || next === c.value) {
      setEditing(prev => { const cp = { ...prev }; delete cp[c.id]; return cp; });
      return;
    }
    try {
      await api.put(`/env-config/${c.id}`, { name: c.name, value: next, enabled: c.enabled });
      message.success('已保存');
      setEditing(prev => { const cp = { ...prev }; delete cp[c.id]; return cp; });
      fetchConfigs();
    } catch (e: any) {
      message.error(e?.message || '保存失败');
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await api.delete(`/env-config/${id}`);
      message.success('已删除');
      fetchConfigs();
    } catch (e: any) {
      message.error(e?.message || '删除失败');
    }
  };

  const handleAdd = async () => {
    try {
      const values = await addForm.validateFields();
      setAddSubmitting(true);
      // Trim trailing slash for consistency with how mirror URLs are stored.
      const value = (values.env_value as string).replace(/\/+$/, '');
      await api.post('/env-config', {
        name: values.env_key,
        value,
        enabled: values.enabled ?? true,
      });
      message.success('已新增');
      setAddVisible(false);
      addForm.resetFields();
      fetchConfigs();
    } catch (e: any) {
      if (e?.errorFields) return; // form validation error, keep modal open
      message.error(e?.message || '新增失败');
    } finally {
      setAddSubmitting(false);
    }
  };

  // When the user picks a language in the add form, prefill the env_key with
  // that language's first (and typically only) mirror env key.
  const handleAddLangChange = (lang: string) => {
    const entry = langToEntry.get(lang);
    addForm.setFieldsValue({
      env_key: entry?.mirror_envs?.[0] || '',
      env_value: entry?.mirror_candidates?.[0] || '',
    });
  };

  const handleAddKeyChange = (envKey: string) => {
    // When env_key changes (e.g. user types a different one), prefill the
    // first candidate for the lang that owns that key, if any.
    const lang = keyToLang.get(envKey);
    const entry = lang ? langToEntry.get(lang) : undefined;
    if (entry?.mirror_candidates?.length) {
      addForm.setFieldsValue({ env_value: entry.mirror_candidates[0] });
    }
  };

  const langsForNew = catalog.filter(c => c.mirror_envs.length > 0);

  // Column render helpers need the maps, so they're defined inside the component.
  const columns = [
    {
      title: '语言',
      dataIndex: 'name',
      width: 100,
      render: (name: string) => {
        const lang = keyToLang.get(name);
        return <Tag color="blue">{lang ? keyToDisplay.get(name) || lang : name}</Tag>;
      },
    },
    {
      title: 'Env Key',
      dataIndex: 'name',
      width: 240,
      render: (v: string) => <code style={{ fontSize: 12 }}>{v}</code>,
    },
    {
      title: '镜像地址',
      dataIndex: 'value',
      render: (v: string, c: EnvConfigRow) => {
        const isEditing = editing[c.id] !== undefined;
        return isEditing ? (
          <Space.Compact style={{ width: '100%' }}>
            <Input
              value={editing[c.id]}
              onChange={e => setEditing(prev => ({ ...prev, [c.id]: e.target.value }))}
              onPressEnter={() => handleSave(c)}
            />
            <Button type="primary" onClick={() => handleSave(c)}>保存</Button>
            <Button onClick={() => setEditing(prev => { const cp = { ...prev }; delete cp[c.id]; return cp; })}>取消</Button>
          </Space.Compact>
        ) : (
          <span
            style={{ cursor: 'pointer' }}
            onClick={() => setEditing(prev => ({ ...prev, [c.id]: v }))}
            title="点击编辑"
          >
            {v || <span style={{ color: '#999' }}>（点击设置）</span>}
          </span>
        );
      },
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      width: 80,
      render: (v: boolean, c: EnvConfigRow) => (
        <Switch checked={v} onChange={checked => handleToggle(c, checked)} size="small" />
      ),
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, c: EnvConfigRow) => (
        <Popconfirm title="删除此镜像配置？" onConfirm={() => handleDelete(c.id)}>
          <Button type="link" danger size="small">删除</Button>
        </Popconfirm>
      ),
    },
  ];

  // Build candidate options for the currently selected language in the add form.
  const selectedLang = Form.useWatch('lang', addForm);
  const candidateOptions = useMemo(() => {
    if (!selectedLang) return [];
    const entry = langToEntry.get(selectedLang);
    return entry?.mirror_candidates || [];
  }, [selectedLang, langToEntry]);

  return (
    <>
    <Modal
      title={
        <Space>
          <span>镜像源配置</span>
          <Button icon={<SyncOutlined />} size="small" onClick={fetchConfigs} loading={loading}>
            刷新
          </Button>
          <Button
            icon={<PlusOutlined />}
            size="small"
            type="primary"
            onClick={() => setAddVisible(true)}
            disabled={langsForNew.length === 0}
          >
            新增镜像
          </Button>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={900}
      destroyOnHidden
    >
      <Table
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={configs}
        pagination={false}
        locale={{
          emptyText: catalog.length === 0
            ? '正在加载运行环境目录...'
            : supportedKeys.size === 0
              ? '当前 catalog 中没有任何语言声明了镜像 env key'
              : '暂无镜像配置，点击右上角「新增镜像」添加',
        }}
        columns={columns}
      />
      <div style={{ marginTop: 12, color: '#999', fontSize: 12 }}>
        提示：修改后在下次安装运行时版本时写入 <code>/etc/mise/config.toml</code> 生效。SSH 会话需重新登录后才能拾取。未配置时 mise 使用默认下载源。
      </div>
    </Modal>

    <Modal
      title="新增镜像"
      open={addVisible}
      onCancel={() => { setAddVisible(false); addForm.resetFields(); }}
      onOk={handleAdd}
      okText="保存"
      cancelText="取消"
      confirmLoading={addSubmitting}
      destroyOnHidden
    >
      <Form form={addForm} layout="vertical" initialValues={{ enabled: true }}>
        <Form.Item
          name="lang"
          label="语言"
          rules={[{ required: true, message: '请选择语言' }]}
        >
          <Select placeholder="选择语言" onChange={handleAddLangChange}>
            {langsForNew.map(c => (
              <Select.Option key={c.lang} value={c.lang}>{c.display}</Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item
          name="env_key"
          label="Env Key"
          rules={[{ required: true, message: '请输入 env_key' }]}
          extra="例：MISE_NODE_MIRROR_URL（选择语言后会自动填入默认 key，可改）"
        >
          <Input placeholder="MISE_NODE_MIRROR_URL" onChange={e => handleAddKeyChange(e.target.value)} />
        </Form.Item>
        <Form.Item
          name="env_value"
          label="镜像地址"
          rules={[{ required: true, message: '请输入镜像地址' }]}
          extra={candidateOptions.length > 0 ? '可从下方候选中选择，或自行填写镜像地址' : undefined}
        >
          <Input placeholder="https://npmmirror.com/mirrors/node" />
        </Form.Item>
        {candidateOptions.length > 0 && (
          <div style={{ marginBottom: 16 }}>
            <span style={{ color: '#999', fontSize: 12, marginRight: 8 }}>候选镜像：</span>
            {candidateOptions.map(url => (
              <Tag
                key={url}
                style={{ cursor: 'pointer', marginBottom: 4 }}
                onClick={() => addForm.setFieldsValue({ env_value: url })}
              >
                {url}
              </Tag>
            ))}
          </div>
        )}
        <Form.Item name="enabled" valuePropName="checked">
          <span />
        </Form.Item>
      </Form>
    </Modal>
    </>
  );
}
