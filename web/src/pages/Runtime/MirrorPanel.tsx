import { useEffect, useState } from 'react';
import { Card, Table, Switch, Input, Button, Space, Tag, message, Popconfirm } from 'antd';
import { SyncOutlined } from '@ant-design/icons';
import api from '../../services/api';
import type { RuntimeMirror, CatalogEntry } from './types';

interface MirrorPanelProps {
  catalog: CatalogEntry[];
}

// MirrorPanel renders only the mirrors the catalog actually advertises mirror
// env keys for (today: node, go). Other languages have an empty `mirror_envs`
// in the catalog and never show up here, satisfying Issue 08 AC2 without an
// explicit allowlist — the backend seed (only node/go) is the SSOT.
export default function MirrorPanel({ catalog }: MirrorPanelProps) {
  const [mirrors, setMirrors] = useState<RuntimeMirror[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<Record<number, string>>({});

  const langsWithMirrors = new Set(catalog.filter(c => c.mirror_envs.length > 0).map(c => c.lang));
  const displayMap = new Map(catalog.map(c => [c.lang, c.display]));

  const fetchMirrors = async () => {
    setLoading(true);
    try {
      const res = await api.get('/runtime/mirrors');
      const all: RuntimeMirror[] = res.data.data?.mirrors || [];
      setMirrors(all.filter(m => langsWithMirrors.has(m.lang)));
    } catch {
      message.error('获取镜像列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (catalog.length > 0) fetchMirrors();
  }, [catalog.length]);

  const handleToggle = async (m: RuntimeMirror, enabled: boolean) => {
    try {
      await api.put(`/runtime/mirrors/${m.id}`, { enabled: enabled ? 1 : 0 });
      message.success(enabled ? '已启用' : '已禁用');
      fetchMirrors();
    } catch (e: any) {
      message.error(e?.message || '更新失败');
    }
  };

  const handleSave = async (m: RuntimeMirror) => {
    const next = editing[m.id];
    if (next === undefined || next === m.env_value) {
      setEditing(prev => { const c = { ...prev }; delete c[m.id]; return c; });
      return;
    }
    try {
      await api.put(`/runtime/mirrors/${m.id}`, { env_value: next });
      message.success('已保存');
      setEditing(prev => { const c = { ...prev }; delete c[m.id]; return c; });
      fetchMirrors();
    } catch (e: any) {
      message.error(e?.message || '保存失败');
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await api.delete(`/runtime/mirrors/${id}`);
      message.success('已删除');
      fetchMirrors();
    } catch (e: any) {
      message.error(e?.message || '删除失败');
    }
  };

  return (
    <Card
      title="镜像源配置"
      style={{ marginTop: 16 }}
      extra={
        <Button icon={<SyncOutlined />} size="small" onClick={fetchMirrors} loading={loading}>
          刷新
        </Button>
      }
    >
      <Table
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={mirrors}
        pagination={false}
        locale={{
          emptyText: catalog.length === 0
            ? '正在加载运行环境目录...'
            : langsWithMirrors.size === 0
              ? '当前 catalog 中没有任何语言声明了镜像 env key'
              : '暂无镜像配置（仅 Node / Go 支持镜像加速）',
        }}
        columns={[
          {
            title: '语言',
            dataIndex: 'lang',
            width: 100,
            render: (lang: string) => <Tag color="blue">{displayMap.get(lang) || lang}</Tag>,
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
            render: (v: string, m: RuntimeMirror) => {
              const isEditing = editing[m.id] !== undefined;
              return isEditing ? (
                <Space.Compact style={{ width: '100%' }}>
                  <Input
                    value={editing[m.id]}
                    onChange={e => setEditing(prev => ({ ...prev, [m.id]: e.target.value }))}
                    onPressEnter={() => handleSave(m)}
                  />
                  <Button type="primary" onClick={() => handleSave(m)}>保存</Button>
                  <Button onClick={() => setEditing(prev => { const c = { ...prev }; delete c[m.id]; return c; })}>取消</Button>
                </Space.Compact>
              ) : (
                <span
                  style={{ cursor: 'pointer' }}
                  onClick={() => setEditing(prev => ({ ...prev, [m.id]: v }))}
                  title="点击编辑"
                >
                  {v || <span style={{ color: '#999' }}>（点击设置）</span>}
                </span>
              );
            },
          },
          {
            title: '来源',
            dataIndex: 'source',
            width: 80,
            render: (s: string) => <Tag color={s === 'seed' ? 'default' : 'green'}>{s}</Tag>,
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (v: number, m: RuntimeMirror) => (
              <Switch checked={v === 1} onChange={c => handleToggle(m, c)} size="small" />
            ),
          },
          {
            title: '操作',
            width: 80,
            render: (_: unknown, m: RuntimeMirror) =>
              m.source === 'user' ? (
                <Popconfirm title="删除此镜像配置？" onConfirm={() => handleDelete(m.id)}>
                  <Button type="link" danger size="small">删除</Button>
                </Popconfirm>
              ) : null,
          },
        ]}
      />
      <div style={{ marginTop: 12, color: '#999', fontSize: 12 }}>
        提示：修改后立即写入 <code>/etc/mise/config.toml</code>，对新安装版本生效。SSH 会话需重新登录后才能拾取。
      </div>
    </Card>
  );
}
