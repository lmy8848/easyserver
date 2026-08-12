import { useCallback, useEffect, useState } from 'react';
import {
  Button, Form, Input, InputNumber, message, Modal, Popconfirm, Select, Space, Table, Tag, Empty,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, ClearOutlined, ClockCircleOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import type { DBInstance, RedisKey } from '../../types';

interface RedisKeysTabProps {
  instance: DBInstance;
}

const TYPE_LABEL: Record<string, string> = {
  string: '字符串', hash: '哈希', list: '列表', set: '集合', zset: '有序集合',
};

// fmtTTL 展示剩余到期时间：-1 永久，-2 已过期，否则拆成 x天x小时x分钟x秒。
function fmtTTL(ttl: number): string {
  if (ttl === -1) return '永久';
  if (ttl === -2) return '已过期';
  const d = Math.floor(ttl / 86400);
  const h = Math.floor((ttl % 86400) / 3600);
  const m = Math.floor((ttl % 3600) / 60);
  const s = ttl % 60;
  const parts = [];
  if (d) parts.push(`${d}天`);
  if (h) parts.push(`${h}小时`);
  if (m) parts.push(`${m}分钟`);
  if (s || parts.length === 0) parts.push(`${s}秒`);
  return parts.join('');
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
  // 数据库槽位数来自实例配置（CONFIG GET databases，默认 16 但可改），不是写死的
  // 常量——启动期参数 databases 改了多少，这里就有多少项。
  const [dbCount, setDbCount] = useState(16);
  const [db, setDb] = useState(0);
  const [keys, setKeys] = useState<RedisKey[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [pattern, setPattern] = useState('*'); // SCAN pattern；'*' = 全部
  const [patternInput, setPatternInput] = useState(''); // 搜索框文本，可为空
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [busy, setBusy] = useState('');

  // 添加/编辑共用弹窗：editTarget 非空 = 编辑（回显值，键/类型不可变）
  const [editTarget, setEditTarget] = useState<RedisKey | null>(null);
  const [addVisible, setAddVisible] = useState(false);
  const [addForm] = Form.useForm();
  const addType = Form.useWatch('type', addForm) || 'string';
  const [expireTarget, setExpireTarget] = useState<RedisKey | null>(null);
  const [expireVisible, setExpireVisible] = useState(false);
  const [expireForm] = Form.useForm();

  const iid = instance.id;

  // 进 tab 拉一次槽位数；失败保留默认 16（databases 极少改动，一次足够）。
  useEffect(() => {
    dbServerApi.redisDBCount(iid)
      .then(res => { const n = res.data?.data?.databases; if (n && n >= 1) setDbCount(n); })
      .catch(() => { /* 默认 16 */ });
  }, [iid]);

  const loadKeys = useCallback(async (curDb: number, curPattern: string, cursor: string) => {
    const res = await dbServerApi.scanRedisKeys(iid, curDb, cursor, curPattern, 50);
    const data = res.data?.data;
    setKeys(data?.keys || []);
    setNextCursor(String(data?.next_cursor ?? 0));
  }, [iid]);

  const refresh = useCallback(() => {
    setLoading(true);
    loadKeys(db, pattern, '0')
      .catch((e: any) => message.error(e?.message || '加载键失败'))
      .finally(() => setLoading(false));
  }, [loadKeys, db, pattern]);

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

  // 编辑：拉取当前值回显到添加弹窗；键/类型锁死，值按类型填充。
  const openEdit = async (k: RedisKey) => {
    setEditTarget(k);
    try {
      const res = await dbServerApi.getRedisValue(iid, db, k.name);
      const val = res.data?.data;
      // 回显值。直接构造 plain object 传给 setFieldsValue（Form.List 的名字就是
      // fields/items/members）。TTL 不回显：编辑弹窗不设 TTL，到期时间归「续期」管。
      const formVals: {
        key: string; type: string;
        fields?: Array<{ field: string; value: string }>;
        items?: Array<{ value: string }>;
        members?: Array<{ member: string; score: number }>;
        value?: string;
      } = {
        key: k.name,
        type: k.type,
      };
      switch (k.type) {
        case 'hash':
          formVals.fields = Object.entries((val?.value as Record<string, string>) || {}).map(([field, value]) => ({ field, value }));
          break;
        case 'list':
        case 'set':
          formVals.items = ((val?.value as string[]) || []).map(value => ({ value }));
          break;
        case 'zset':
          formVals.members = ((val?.value as Array<{ member: string; score: number }>) || []).map(({ member, score }) => ({ member, score }));
          break;
        default:
          formVals.value = String(val?.value ?? '');
      }
      addForm.setFieldsValue(formVals);
      setAddVisible(true);
    } catch (e: any) { message.error(e?.message || '读取值失败'); setEditTarget(null); }
  };

  const submitKey = async () => {
    try {
      const v = await addForm.validateFields();
      const typ = editTarget ? editTarget.type : (v.type || 'string');
      const key = editTarget ? editTarget.name : v.key;
      // 按类型组装 value：string 是文本，hash 是 {field:value}，list/set 是数组，
      // zset 是 [{member,score}]。Form 里直接存这些结构，POST 原样走 JSON。
      let value: unknown;
      switch (typ) {
        case 'hash': value = (v.fields || []).filter((r: any) => r?.field).reduce((m: any, r: any) => { m[r.field] = r.value ?? ''; return m; }, {}); break;
        case 'list':
        case 'set': value = (v.items || []).map((r: any) => r.value).filter((s: string) => s !== ''); break;
        case 'zset': value = (v.members || []).filter((r: any) => r?.member).map((r: any) => ({ member: r.member, score: Number(r.score) || 0 })); break;
        default: value = v.value ?? '';
      }
      if (editTarget) {
        // 编辑 = 替换：集合类命令（RPUSH/SADD/ZADD）只会追加，先 DEL 再重建才是
        // "编辑"语义；string 的 SET 本就覆盖。重建会清 TTL，这是预期——编辑只
        // 管值，到期时间归「续期」按钮，职责分离。
        await dbServerApi.delRedisKeys(iid, { db, keys: [key] });
        await dbServerApi.setRedisValue(iid, { db, type: typ, key, value });
      } else {
        await dbServerApi.setRedisValue(iid, { db, type: typ, key, value, ttl: v.ttl || undefined });
      }
      message.success(editTarget ? '已保存' : '已添加');
      setAddVisible(false); addForm.resetFields(); setEditTarget(null);
      refresh();
    } catch (e: any) { if (e?.message) message.error(e.message); }
  };

  const delKey = async (k: RedisKey) => {
    setBusy(`del-${k.name}`);
    try {
      await dbServerApi.delRedisKeys(iid, { db, keys: [k.name] });
      message.success('已删除');
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

  // 槽位数由实例的 databases 配置决定（上面 dbCount）；key 数量由 key 列表加载后
  // 可见，没必要为个 DBSIZE 每次进 tab 跑 N 次 redis 往返。
  const dbOptions = Array.from({ length: dbCount }, (_, i) => ({ value: i, label: `DB ${i}` }));

  const columns = [
    { title: '键', dataIndex: 'name', key: 'name', ellipsis: true, render: (t: string) => <strong>{t}</strong> },
    {
      title: '类型', dataIndex: 'type', key: 'type', width: 110,
      render: (t: string) => <Tag color={TYPE_COLOR[t] || 'default'}>{TYPE_LABEL[t] || t}</Tag>,
    },
    { title: '过期时间', dataIndex: 'ttl', key: 'ttl', width: 130, render: fmtTTL },
    { title: '大小', dataIndex: 'size', key: 'size', width: 90, render: fmtSize },
    {
      title: '操作', key: 'action', width: 190,
      render: (_: unknown, k: RedisKey) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(k)}>编辑</Button>
          <Button type="link" size="small" icon={<ClockCircleOutlined />}
            onClick={() => { setExpireTarget(k); expireForm.resetFields(); setExpireVisible(true); }}>续期</Button>
          <Popconfirm title={`确定删除键 ${k.name}？`} onConfirm={() => delKey(k)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `del-${k.name}`} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 12 }} wrap>
        <Select value={db} onChange={(v) => { setDb(v); setKeys([]); setNextCursor(''); setPattern('*'); setPatternInput(''); }}
          options={dbOptions} style={{ width: 140 }} />
        <Input.Search
          placeholder="搜索键（留空 = 全部）"
          value={patternInput}
          onChange={e => setPatternInput(e.target.value)}
          onSearch={(v) => {
            const raw = (v || '').trim();
            if (!raw) { setPattern('*'); return; }
            // Redis SCAN 原生只认 glob pattern（* ? []），不认"包含子串"。
            // 纯文本按包含匹配包成 *text*，带通配符的原样透传给引擎。
            setPattern(/[?*[\]]/.test(raw) ? raw : `*${raw}*`);
          }}
          style={{ width: 220 }}
          allowClear
        />
        <Button icon={<ReloadOutlined />} loading={loading} onClick={refresh}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => { addForm.resetFields(); setEditTarget(null); setAddVisible(true); }}>添加键值对</Button>
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
        locale={{ emptyText: <Empty description={pattern === '*' ? '暂无键' : `没有匹配 ${pattern} 的键`} /> }}
      />
      {nextCursor && nextCursor !== '0' && (
        <div style={{ textAlign: 'center', marginTop: 12 }}>
          <Button loading={loadingMore} onClick={loadMore}>加载更多</Button>
        </div>
      )}

      {/* 添加 / 编辑键值对：编辑时回显值，键/类型锁死 */}
      <Modal title={editTarget ? `编辑键值对 - ${editTarget.name}` : `添加键值对 - DB ${db}`}
        open={addVisible} onCancel={() => setAddVisible(false)} onOk={submitKey}
        okText={editTarget ? '保存' : '添加'} cancelText="取消">
        <Form form={addForm} layout="vertical">
          <Form.Item name="key" label="键" rules={[{ required: true, message: '请输入键' }]}>
            <Input placeholder="如 user:1" disabled={!!editTarget} />
          </Form.Item>
          {!editTarget && (
            <Form.Item name="ttl" label="过期秒数（留空 = 永久）">
              <InputNumber min={1} style={{ width: '100%' }} placeholder="如 3600" />
            </Form.Item>
          )}
          <Form.Item name="type" label="类型" initialValue="string">
            <Select options={Object.entries(TYPE_LABEL).map(([v, l]) => ({ value: v, label: l }))} disabled={!!editTarget} />
          </Form.Item>
          {addType === 'string' && (
            <Form.Item name="value" label="值" initialValue="">
              <Input.TextArea rows={4} placeholder="字符串值" style={{ fontFamily: 'monospace' }} />
            </Form.Item>
          )}
          {addType === 'hash' && (
            <Form.Item label="字段" required>
              <Form.List name="fields" initialValue={[{ field: '', value: '' }]}>
                {(fields, { add, remove }) => (
                  <div>
                    {fields.map(({ key, name }) => (
                      <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                        <Form.Item name={[name, 'field']} rules={[{ required: true, message: '字段名必填' }]} style={{ marginBottom: 0 }}>
                          <Input placeholder="字段名" style={{ width: 180 }} />
                        </Form.Item>
                        <Form.Item name={[name, 'value']} style={{ marginBottom: 0 }}>
                          <Input placeholder="值" style={{ width: 220 }} />
                        </Form.Item>
                        <Button type="text" icon={<DeleteOutlined />} onClick={() => remove(name)} />
                      </Space>
                    ))}
                    <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ field: '', value: '' })}>添加字段</Button>
                  </div>
                )}
              </Form.List>
            </Form.Item>
          )}
          {(addType === 'list' || addType === 'set') && (
            <Form.Item label={TYPE_LABEL[addType]} required>
              <Form.List name="items" initialValue={[{ value: '' }]}>
                {(fields, { add, remove }) => (
                  <div>
                    {fields.map(({ key, name }) => (
                      <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                        <Form.Item name={[name, 'value']} rules={[{ required: true, message: '元素必填' }]} style={{ marginBottom: 0, flex: 1 }}>
                          <Input placeholder="元素" style={{ width: 400 }} />
                        </Form.Item>
                        <Button type="text" icon={<DeleteOutlined />} onClick={() => remove(name)} />
                      </Space>
                    ))}
                    <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ value: '' })}>添加元素</Button>
                  </div>
                )}
              </Form.List>
            </Form.Item>
          )}
          {addType === 'zset' && (
            <Form.Item label="成员（分数）" required>
              <Form.List name="members" initialValue={[{ member: '', score: 0 }]}>
                {(fields, { add, remove }) => (
                  <div>
                    {fields.map(({ key, name }) => (
                      <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                        <Form.Item name={[name, 'member']} rules={[{ required: true, message: '成员必填' }]} style={{ marginBottom: 0 }}>
                          <Input placeholder="成员" style={{ width: 280 }} />
                        </Form.Item>
                        <Form.Item name={[name, 'score']} initialValue={0} style={{ marginBottom: 0 }}>
                          <InputNumber placeholder="分数" style={{ width: 120 }} />
                        </Form.Item>
                        <Button type="text" icon={<DeleteOutlined />} onClick={() => remove(name)} />
                      </Space>
                    ))}
                    <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ member: '', score: 0 })}>添加成员</Button>
                  </div>
                )}
              </Form.List>
            </Form.Item>
          )}
        </Form>
      </Modal>

      {/* 续期（设置 TTL） */}
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
