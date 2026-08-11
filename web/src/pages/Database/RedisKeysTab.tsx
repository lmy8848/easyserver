import { useCallback, useEffect, useState } from 'react';
import {
  Button, Form, Input, InputNumber, message, Modal, Popconfirm, Select, Space, Spin, Table, Tag, Empty,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, ClearOutlined, ClockCircleOutlined, EditOutlined, SaveOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import type { DBInstance, RedisKey, RedisValue } from '../../types';

interface RedisKeysTabProps {
  instance: DBInstance;
}

const TYPE_LABEL: Record<string, string> = {
  string: '字符串', hash: '哈希', list: '列表', set: '集合', zset: '有序集合',
};

function fmtTTL(ttl: number): string {
  if (ttl === -1) return '永久';
  if (ttl === -2) return '已过期';
  if (ttl < 60) return `${ttl}s`;
  if (ttl < 3600) return `${Math.floor(ttl / 60)}m`;
  if (ttl < 86400) return `${Math.floor(ttl / 3600)}h`;
  return `${Math.floor(ttl / 86400)}d`;
}

function fmtSize(n: number): string {
  if (!n) return '0 B';
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

const TYPE_COLOR: Record<string, string> = {
  string: 'green', hash: 'blue', list: 'orange', set: 'purple', zset: 'cyan',
};

// Redis key 浏览器 — 数据库 tab 对 Redis 实例的渲染。SCAN 游标分页 +
// pattern 过滤，选中 key 查看/编辑值（仅 string 可编辑），支持 DEL / EXPIRE /
// PERSIST / FLUSHDB。组件自包含：自己调 API，不依赖父组件的 SQL 状态。
export default function RedisKeysTab({ instance }: RedisKeysTabProps) {
  const [dbs, setDbs] = useState<{ index: number; size: number }[]>([]);
  const [db, setDb] = useState(0);
  const [keys, setKeys] = useState<RedisKey[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [pattern, setPattern] = useState('*');
  const [patternInput, setPatternInput] = useState('*');
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [busy, setBusy] = useState('');

  // value viewer / editor
  const [valueKey, setValueKey] = useState<RedisKey | null>(null);
  const [value, setValue] = useState<RedisValue | null>(null);
  const [valueLoading, setValueLoading] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editText, setEditText] = useState('');

  const [addVisible, setAddVisible] = useState(false);
  const [addForm] = Form.useForm();
  const [expireTarget, setExpireTarget] = useState<RedisKey | null>(null);
  const [expireVisible, setExpireVisible] = useState(false);
  const [expireForm] = Form.useForm();

  const iid = instance.id;

  const loadDBs = useCallback(async () => {
    try {
      const res = await dbServerApi.listRedisDBs(iid);
      const list = res.data?.data || [];
      setDbs(list);
      const first = list[0];
      if (first && !list.some(d => d.index === db)) setDb(first.index);
    } catch { /* 首次加载失败由 loadKeys 报错 */ }
  }, [iid]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadKeys = useCallback(async (curDb: number, curPattern: string, cursor: string) => {
    const res = await dbServerApi.scanRedisKeys(iid, curDb, cursor, curPattern, 50);
    const data = res.data?.data;
    setKeys(data?.keys || []);
    setNextCursor(String(data?.next_cursor ?? 0));
  }, [iid]);

  const refresh = useCallback(() => {
    setLoading(true);
    Promise.all([loadDBs(), loadKeys(db, pattern, '0')])
      .catch((e: any) => message.error(e?.message || '加载 key 失败'))
      .finally(() => setLoading(false));
  }, [loadDBs, loadKeys, db, pattern]);

  useEffect(() => { refresh(); }, [refresh]);

  const loadMore = async () => {
    if (!nextCursor || nextCursor === '0') return;
    setLoadingMore(true);
    try {
      const res = await dbServerApi.scanRedisKeys(iid, db, nextCursor, pattern, 50);
      const data = res.data?.data;
      setKeys(prev => [...prev, ...(data?.keys || [])]);
      setNextCursor(String(data?.next_cursor ?? 0));
    } catch (e: any) { message.error(e?.message || '加载更多失败'); } finally { setLoadingMore(false); }
  };

  const openValue = async (k: RedisKey) => {
    setValueKey(k); setValue(null); setEditing(false); setValueLoading(true);
    try {
      const res = await dbServerApi.getRedisValue(iid, db, k.name);
      setValue(res.data?.data || null);
    } catch (e: any) { message.error(e?.message || '读取值失败'); } finally { setValueLoading(false); }
  };

  const saveValue = async () => {
    if (!valueKey) return;
    setBusy('save-value');
    try {
      await dbServerApi.setRedisValue(iid, { db, key: valueKey.name, value: editText });
      message.success('已保存');
      setValue({ type: 'string', value: editText });
      setEditing(false);
      refresh();
    } catch (e: any) { message.error(e?.message || '保存失败'); } finally { setBusy(''); }
  };

  const delKey = async (k: RedisKey) => {
    setBusy(`del-${k.name}`);
    try {
      await dbServerApi.delRedisKeys(iid, { db, keys: [k.name] });
      message.success('已删除');
      if (valueKey?.name === k.name) setValueKey(null);
      refresh();
    } catch (e: any) { message.error(e?.message || '删除失败'); } finally { setBusy(''); }
  };

  const flushDB = async () => {
    setBusy('flush');
    try {
      await dbServerApi.flushRedisDB(iid, { db });
      message.success('已清空');
      refresh();
    } catch (e: any) { message.error(e?.message || '清空失败'); } finally { setBusy(''); }
  };

  const addKey = async () => {
    try {
      const v = await addForm.validateFields();
      await dbServerApi.setRedisValue(iid, { db, key: v.key, value: v.value ?? '', ttl: v.ttl || undefined });
      message.success('已添加');
      setAddVisible(false); addForm.resetFields();
      refresh();
    } catch (e: any) { if (e?.message) message.error(e.message); }
  };

  const saveExpire = async () => {
    if (!expireTarget) return;
    try {
      const v = await expireForm.validateFields();
      if (v.ttl) await dbServerApi.expireRedisKey(iid, { db, key: expireTarget.name, ttl: v.ttl });
      else await dbServerApi.persistRedisKey(iid, { db, key: expireTarget.name });
      message.success('已更新过期时间');
      setExpireVisible(false); expireForm.resetFields();
      refresh();
    } catch (e: any) { if (e?.message) message.error(e.message); }
  };

  const dbOptions = dbs.length > 0
    ? dbs.map(d => ({ value: d.index, label: `DB ${d.index}（${d.size} keys）` }))
    : [{ value: 0, label: 'DB 0（空）' }];

  const columns = [
    { title: 'Key', dataIndex: 'name', key: 'name', ellipsis: true, render: (t: string) => <strong>{t}</strong> },
    {
      title: '类型', dataIndex: 'type', key: 'type', width: 110,
      render: (t: string) => <Tag color={TYPE_COLOR[t] || 'default'}>{TYPE_LABEL[t] || t}</Tag>,
    },
    { title: 'TTL', dataIndex: 'ttl', key: 'ttl', width: 90, render: fmtTTL },
    { title: '大小', dataIndex: 'size', key: 'size', width: 90, render: fmtSize },
    {
      title: '操作', key: 'action', width: 190,
      render: (_: unknown, k: RedisKey) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openValue(k)}>查看</Button>
          <Button type="link" size="small" icon={<ClockCircleOutlined />}
            onClick={() => { setExpireTarget(k); expireForm.resetFields(); setExpireVisible(true); }}>TTL</Button>
          <Popconfirm title={`确定删除 key ${k.name}？`} onConfirm={() => delKey(k)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `del-${k.name}`} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // 值查看器：string 可编辑，hash/list/set/zset 只读展示，其他类型提示不支持
  const renderValue = () => {
    if (!value) return null;
    switch (value.type) {
      case 'string':
        return editing ? (
          <Input.TextArea value={editText} onChange={e => setEditText(e.target.value)} rows={6} style={{ fontFamily: 'monospace' }} autoSize />
        ) : (
          <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontFamily: 'monospace', fontSize: 13, maxHeight: 300, overflowY: 'auto' }}>
            {String(value.value ?? '')}
          </pre>
        );
      case 'hash': {
        const m = (value.value as Record<string, string>) || {};
        return (
          <Table size="small" pagination={false} rowKey={(r) => r[0]}
            columns={[{ title: 'Field', dataIndex: 0, width: '40%' }, { title: 'Value', dataIndex: 1 }]}
            dataSource={Object.entries(m).map(([f, v]) => [f, v] as [string, string])} />
        );
      }
      case 'list':
      case 'set':
        return (
          <ul style={{ maxHeight: 300, overflowY: 'auto', paddingLeft: 20, margin: 0 }}>
            {(value.value as string[] || []).map((v, i) => (
              <li key={i} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{v}</li>
            ))}
          </ul>
        );
      case 'zset':
        return (
          <Table size="small" pagination={false} rowKey={(r) => r.member}
            columns={[{ title: 'Member', dataIndex: 'member' }, { title: 'Score', dataIndex: 'score', width: 140 }]}
            dataSource={value.value as Array<{ member: string; score: number }> || []} />
        );
      default:
        return <Empty description={`${value.type} 类型暂不支持结构化展示/编辑`} />;
    }
  };

  return (
    <div>
      <Space style={{ marginBottom: 12 }} wrap>
        <Select value={db} onChange={(v) => { setDb(v); setKeys([]); setNextCursor(''); setPattern('*'); setPatternInput('*'); }}
          options={dbOptions} style={{ width: 140 }} />
        <Input.Search
          placeholder="pattern，如 user:*"
          value={patternInput}
          onChange={e => setPatternInput(e.target.value)}
          onSearch={(v) => { setPattern(v || '*'); }}
          style={{ width: 220 }}
          allowClear
        />
        <Button icon={<ReloadOutlined />} loading={loading} onClick={refresh}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => { addForm.resetFields(); setAddVisible(true); }}>添加 key</Button>
        <Popconfirm title={`确定清空 DB ${db} 全部 key？此操作不可恢复！`} okButtonProps={{ danger: true }} onConfirm={flushDB}>
          <Button danger icon={<ClearOutlined />} loading={busy === 'flush'}>清空当前 DB</Button>
        </Popconfirm>
      </Space>

      <Table
        columns={columns}
        dataSource={keys}
        rowKey="name"
        size="small"
        loading={loading}
        pagination={false}
        locale={{ emptyText: <Empty description={pattern === '*' ? '暂无 key' : `没有匹配 ${pattern} 的 key`} /> }}
      />
      {nextCursor && nextCursor !== '0' && (
        <div style={{ textAlign: 'center', marginTop: 12 }}>
          <Button loading={loadingMore} onClick={loadMore}>加载更多</Button>
        </div>
      )}

      {/* 值查看/编辑 */}
      <Modal title={valueKey ? `Key: ${valueKey.name}` : '查看值'} open={!!valueKey} onCancel={() => setValueKey(null)}
        width={640} footer={value?.type === 'string' ? (
          editing ? [
            <Button key="cancel" onClick={() => setEditing(false)}>取消</Button>,
            <Button key="save" type="primary" icon={<SaveOutlined />} loading={busy === 'save-value'} onClick={saveValue}>保存</Button>,
          ] : [
            <Button key="edit" type="primary" icon={<EditOutlined />} onClick={() => { setEditText(String(value.value ?? '')); setEditing(true); }}>编辑</Button>,
          ]
        ) : null}>
        <Spin spinning={valueLoading}>
          {valueKey && (
            <Space style={{ marginBottom: 8 }}>
              <Tag color={TYPE_COLOR[value?.type || ''] || 'default'}>{value ? (TYPE_LABEL[value.type] || value.type) : '...'}</Tag>
              <Tag>{valueKey.ttl === -1 ? '永久' : `TTL ${fmtTTL(valueKey.ttl)}`}</Tag>
            </Space>
          )}
          {renderValue()}
        </Spin>
      </Modal>

      {/* 添加 key */}
      <Modal title={`添加 Key - DB ${db}`} open={addVisible} onCancel={() => setAddVisible(false)} onOk={addKey} okText="添加" cancelText="取消">
        <Form form={addForm} layout="vertical">
          <Form.Item name="key" label="Key" rules={[{ required: true, message: '请输入 key' }]}>
            <Input placeholder="如 user:1" />
          </Form.Item>
          <Form.Item name="value" label="值" initialValue="">
            <Input.TextArea rows={4} placeholder="字符串值" style={{ fontFamily: 'monospace' }} />
          </Form.Item>
          <Form.Item name="ttl" label="过期秒数（留空 = 永久）">
            <InputNumber min={1} style={{ width: '100%' }} placeholder="如 3600" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 设置 TTL */}
      <Modal title={expireTarget ? `设置过期 - ${expireTarget.name}` : '设置过期'} open={expireVisible}
        onCancel={() => setExpireVisible(false)} onOk={saveExpire} okText="确定" cancelText="取消">
        <Form form={expireForm} layout="vertical">
          <Form.Item name="ttl" label="过期秒数（留空 = 设为永久）" extra="注意：0 秒会被 Redis 立即删除，留空表示移除过期时间">
            <InputNumber min={1} style={{ width: '100%' }} placeholder="如 3600" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
