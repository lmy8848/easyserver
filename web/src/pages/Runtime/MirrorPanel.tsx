import { useEffect, useState, useMemo } from 'react';
import { Modal, Table, Input, Button, Space, Tag, message, Popconfirm, Form, Select } from 'antd';
import { SyncOutlined, PlusOutlined } from '@ant-design/icons';
import api from '../../services/client';
import type { CatalogEntry } from './types';

// MirrorPanel operates on the dedicated /api/runtime/mirrors API.
// Mirror sources are env keys in /opt/easyserver/mise/config.toml [env]
// (file is the authority — no enabled/source state, write = take effect,
// delete = gone). The panel filters file entries by the catalog's mirror_envs
// keys and renders them grouped by language, offering the extra UX of
// pre-seeded candidate URLs (from catalog.mirror_candidates, which excludes
// mise's default source) on the add form.

interface MirrorRow {
  env_key: string;
  env_value: string;
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
  const [mirrors, setMirrors] = useState<MirrorRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<Record<string, string>>({});
  const [addVisible, setAddVisible] = useState(false);
  const [addForm] = Form.useForm();
  const [addSubmitting, setAddSubmitting] = useState(false);

  const { keyToLang, keyToDisplay, langToEntry, supportedKeys } = useMirrorCatalog(catalog);

  const fetchMirrors = async () => {
    setLoading(true);
    try {
      const res = await api.get('/runtime/mirrors');
      const all: MirrorRow[] = res.data.data?.mirrors || [];
      // Only show env keys declared as mirror env keys by the catalog.
      setMirrors(all.filter(m => supportedKeys.has(m.env_key)));
    } catch {
      message.error('获取镜像配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible && catalog.length > 0) fetchMirrors();
  }, [visible, catalog.length]);

  const handleSave = async (m: MirrorRow) => {
    const next = editing[m.env_key];
    if (next === undefined || next === m.env_value) {
      setEditing(prev => { const cp = { ...prev }; delete cp[m.env_key]; return cp; });
      return;
    }
    try {
      await api.put(`/runtime/mirrors/${encodeURIComponent(m.env_key)}`, { env_value: next });
      message.success('已保存，立即生效');
      setEditing(prev => { const cp = { ...prev }; delete cp[m.env_key]; return cp; });
      fetchMirrors();
    } catch (e: any) {
      message.error(e?.message || '保存失败');
    }
  };

  const handleDelete = async (envKey: string) => {
    try {
      await api.delete(`/runtime/mirrors/${encodeURIComponent(envKey)}`);
      message.success('已删除');
      fetchMirrors();
    } catch (e: any) {
      message.error(e?.message || '删除失败');
    }
  };

  const handleAdd = async () => {
    try {
      const values = await addForm.validateFields();
      setAddSubmitting(true);
      // Trim trailing slash for consistency with how mirror URLs are stored.
      const envValue = (values.env_value as string).replace(/\/+$/, '');
      await api.post('/runtime/mirrors', {
        env_key: values.env_key,
        env_value: envValue,
      });
      message.success('已新增，立即生效');
      setAddVisible(false);
      addForm.resetFields();
      fetchMirrors();
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
      dataIndex: 'env_key',
      width: 100,
      render: (name: string) => {
        const lang = keyToLang.get(name);
        return <Tag color="blue">{lang ? keyToDisplay.get(name) || lang : name}</Tag>;
      },
    },
    {
      title: 'Env Key',
      dataIndex: 'env_key',
      width: 240,
      render: (v: string) => <code style={{ fontSize: 12 }}>{v}</code>,
    },
    {
      title: '镜像地址',
      dataIndex: 'env_value',
      render: (v: string, m: MirrorRow) => {
        const isEditing = editing[m.env_key] !== undefined;
        return isEditing ? (
          <Space.Compact style={{ width: '100%' }}>
            <Input
              value={editing[m.env_key]}
              onChange={e => setEditing(prev => ({ ...prev, [m.env_key]: e.target.value }))}
              onPressEnter={() => handleSave(m)}
            />
            <Button type="primary" onClick={() => handleSave(m)}>保存</Button>
            <Button onClick={() => setEditing(prev => { const cp = { ...prev }; delete cp[m.env_key]; return cp; })}>取消</Button>
          </Space.Compact>
        ) : (
          <span
            style={{ cursor: 'pointer' }}
            onClick={() => setEditing(prev => ({ ...prev, [m.env_key]: v }))}
            title="点击编辑"
          >
            {v || <span style={{ color: '#999' }}>（点击设置）</span>}
          </span>
        );
      },
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, m: MirrorRow) => (
        <Popconfirm title="删除此镜像配置？" onConfirm={() => handleDelete(m.env_key)}>
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
          <Button icon={<SyncOutlined />} size="small" onClick={fetchMirrors} loading={loading}>
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
        rowKey="env_key"
        size="small"
        loading={loading}
        dataSource={mirrors}
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
        修改后立即生效
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
      <Form form={addForm} layout="vertical">
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
      </Form>
    </Modal>
    </>
  );
}
