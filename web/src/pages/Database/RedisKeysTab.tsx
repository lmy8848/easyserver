import { useCallback, useEffect, useState } from 'react';
import {
  Button, Flex, Form, Input, InputNumber, message, Modal, Popconfirm, Select, Space, Table, Tag, Empty,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, ClearOutlined, ClockCircleOutlined, MinusCircleOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import type { DBInstance, RedisKey } from '../../types';
import { formatBytes } from '../../utils/format';

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

const TYPE_COLOR: Record<string, string> = {
  string: 'green', hash: 'blue', list: 'orange', set: 'purple', zset: 'cyan',
};

// Redis key 浏览器 — 数据库 tab 对 Redis 实例的渲染。SCAN 游标分页 +
// pattern 过滤。添加键值对支持全部类型（string 用值文本域，hash 用字段对，
// list/set 用元素列表，zset 用分值-成员对，走后端原生原子命令）；编辑仅
// string 回显可改，集合类型打开弹窗显示"暂不支持编辑"。支持 DEL / EXPIRE /
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

  // 添加/编辑共用弹窗：editTarget 非空 = 编辑（回显值，键/类型锁死）。
  // 类型始终显示：string 可编辑，其他类型值区提示暂不支持。
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

  // 编辑：所有类型都有编辑按钮。string 拉值回显可编辑；其他类型不获取值，
  // 弹窗正常显示键和类型，值区提示"暂不支持编辑"。过期秒数默认留空 = 保持
  // 原有过期时间，填 0 = 永久，填 N = 重设。
  const openEdit = async (k: RedisKey) => {
    setEditTarget(k);
    if (k.type !== 'string') {
      addForm.resetFields();
      addForm.setFieldsValue({ key: k.name, type: k.type, value: '' });
      setAddVisible(true);
      return;
    }
    try {
      const res = await dbServerApi.getRedisValue(iid, db, k.name);
      const val = res.data?.data;
      addForm.setFieldsValue({
        key: k.name,
        type: k.type,
        value: String(val?.value ?? ''),
      });
      setAddVisible(true);
    } catch (e: any) { message.error(e?.message || '读取值失败'); setEditTarget(null); }
  };

  const submitKey = async () => {
    try {
      const v = await addForm.validateFields();
      const typ = editTarget ? editTarget.type : (v.type || 'string');
      // 集合类型不支持编辑：弹窗里显示"暂不支持编辑"，提交走不到这里也兜底拦一下。
      if (editTarget && typ !== 'string') return;
      const key = editTarget ? editTarget.name : v.key;
      // ttl 三态（0 已由输入框校验拦截）：留空 = 不更新；-1 = 永久；>0 = 过期 N 秒。
      const ttl = v.ttl === undefined || v.ttl === null || v.ttl === '' ? undefined : v.ttl;
      const payload: any = { db, key, type: typ, ttl };
      if (typ === 'string') {
        // SET 覆盖（编辑 = 全量替换 value）。
        payload.value = v.value ?? '';
      } else if (typ === 'hash') {
        if (!v.hash_fields?.length) { message.error('请至少添加一个字段'); return; }
        payload.hash_fields = v.hash_fields;
      } else if (typ === 'list' || typ === 'set') {
        if (!v.values?.length) { message.error('请至少添加一个元素'); return; }
        payload.values = v.values;
      } else if (typ === 'zset') {
        if (!v.zset_members?.length) { message.error('请至少添加一个成员'); return; }
        payload.zset_members = v.zset_members;
      }
      await dbServerApi.setRedisValue(iid, payload);
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
      // 留空 = 保持原样，不调用 API。
      if (v.ttl === undefined || v.ttl === null || v.ttl === '') {
        setExpireVisible(false); expireForm.resetFields();
        return;
      }
      // 永久 = -1（与原生 TTL 一致）；0 已由输入框校验拦截，>0 = 重设过期。
      if (v.ttl === -1) await dbServerApi.persistRedisKey(iid, { db, key: expireTarget.name });
      else await dbServerApi.expireRedisKey(iid, { db, key: expireTarget.name, ttl: v.ttl });
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
    { title: '大小', dataIndex: 'size', key: 'size', width: 90, render: (s: number) => formatBytes(s) },
    {
      title: '操作', key: 'action', width: 200,
      render: (_: unknown, k: RedisKey) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(k)}>编辑</Button>
          <Button type="link" size="small" icon={<ClockCircleOutlined />}
            onClick={() => {
              setExpireTarget(k);
              // 默认留空 = 不修改；填 0 = 永久；填 N = 重设。
              expireForm.resetFields();
              setExpireVisible(true);
            }}>续期</Button>
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
        <Button type="primary" icon={<PlusOutlined />} onClick={() => {
          addForm.resetFields();
          addForm.setFieldsValue({ ttl: -1 }); // 添加：过期秒数必填，默认永久
          setEditTarget(null); setAddVisible(true);
        }}>添加键值对</Button>
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

      {/* 添加 / 编辑键值对：编辑时回显值，键/类型锁死。类型始终显示；只有 string 可编辑。 */}
      <Modal title={editTarget ? `编辑键值对 - ${editTarget.name}` : `添加键值对 - DB ${db}`}
        open={addVisible} onCancel={() => setAddVisible(false)} onOk={submitKey}
        okText={editTarget ? '保存' : '添加'} cancelText="取消">
        <Form form={addForm} layout="vertical" initialValues={{
          type: 'string',
          values: [''],                                  // list / set 每行一个元素
          hash_fields: [{ field: '', value: '' }],       // hash 每行一对字段-值
          zset_members: [{ score: 0, member: '' }],      // zset 每行一对分值-成员
        }}>
          <Form.Item name="type" label="类型" initialValue="string">
            <Select
              options={Object.entries(TYPE_LABEL).map(([v, l]) => ({ value: v, label: l }))}
              disabled={!!editTarget}
            />
          </Form.Item>
          <Form.Item name="ttl" label="过期秒数"
            extra={editTarget ? '留空 = 保持原有过期时间；-1 = 永久；0 无效' : '-1 = 永久；0 无效'}
            rules={[
              ...(editTarget ? [] : [{ required: true, message: '请填写过期秒数' }]),
              { validator: (_: unknown, val: any) => (val === 0 ? Promise.reject(new Error('0 秒会被 Redis 立即删除，永久请填 -1')) : Promise.resolve()) },
            ]}>
            <InputNumber min={-1} style={{ width: '100%' }} placeholder="如 3600" />
          </Form.Item>
          <Form.Item name="key" label="键" rules={[{ required: true, message: '请输入键' }]}>
            <Input placeholder="如 user:1" disabled={!!editTarget} />
          </Form.Item>
          {/* 编辑：string 回显值可改；集合类型显示"暂不支持编辑"（不获取值）。
              添加：全部类型可选，按类型切换输入组件。 */}
          {editTarget ? (
            editTarget.type !== 'string' ? (
              <Empty description={`${TYPE_LABEL[editTarget.type] || editTarget.type} 类型暂不支持编辑`} style={{ padding: '24px 0' }} />
            ) : (
              <Form.Item name="value" label="值" initialValue="">
                <Input.TextArea rows={4} placeholder="字符串值" style={{ fontFamily: 'monospace' }} />
              </Form.Item>
            )
          ) : (
            <>
              {addType === 'string' && (
                <Form.Item name="value" label="值" initialValue="">
                  <Input.TextArea rows={4} placeholder="字符串值" style={{ fontFamily: 'monospace' }} />
                </Form.Item>
              )}
              {addType === 'hash' && (
                <Form.Item label="字段">
                  <Form.List name="hash_fields">
                    {(fields, { add, remove }) => (
                      <>
                        {fields.map((f) => (
                          <Flex key={f.key} align="flex-start" gap={8}>
                            <Form.Item name={[f.name, 'field']} rules={[{ required: true, message: '字段名必填' }]} style={{ flex: 1 }}>
                              <Input placeholder="字段名" />
                            </Form.Item>
                            <Form.Item name={[f.name, 'value']} rules={[{ required: true, message: '值必填' }]} style={{ flex: 1 }}>
                              <Input placeholder="字段值" />
                            </Form.Item>
                            <Button type="text" danger icon={<MinusCircleOutlined />} onClick={() => remove(f.name)} />
                          </Flex>
                        ))}
                        <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加字段</Button>
                      </>
                    )}
                  </Form.List>
                </Form.Item>
              )}
              {(addType === 'list' || addType === 'set') && (
                <Form.Item label={addType === 'list' ? '列表元素' : '集合成员'}>
                  <Form.List name="values">
                    {(fields, { add, remove }) => (
                      <>
                        {fields.map((f, i) => (
                          <Flex key={f.key} align="flex-start" gap={8}>
                            <Form.Item name={f.name} rules={[{ required: true, message: '元素必填' }]} style={{ flex: 1 }}>
                              <Input placeholder={`${addType === 'list' ? '元素' : '成员'} ${i + 1}`} />
                            </Form.Item>
                            <Button type="text" danger icon={<MinusCircleOutlined />} onClick={() => remove(f.name)} />
                          </Flex>
                        ))}
                        <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加元素</Button>
                      </>
                    )}
                  </Form.List>
                </Form.Item>
              )}
              {addType === 'zset' && (
                <Form.Item label="成员">
                  <Form.List name="zset_members">
                    {(fields, { add, remove }) => (
                      <>
                        {fields.map((f) => (
                          <Flex key={f.key} align="flex-start" gap={8}>
                            <Form.Item name={[f.name, 'score']} rules={[{ required: true, message: '分值必填' }]}>
                              <InputNumber step={0.1} placeholder="分值" style={{ width: 110 }} />
                            </Form.Item>
                            <Form.Item name={[f.name, 'member']} rules={[{ required: true, message: '成员必填' }]} style={{ flex: 1 }}>
                              <Input placeholder="成员" />
                            </Form.Item>
                            <Button type="text" danger icon={<MinusCircleOutlined />} onClick={() => remove(f.name)} />
                          </Flex>
                        ))}
                        <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加成员</Button>
                      </>
                    )}
                  </Form.List>
                </Form.Item>
              )}
            </>
          )}
        </Form>
      </Modal>

      {/* 续期（设置 TTL） */}
      <Modal title={expireTarget ? `设置过期 - ${expireTarget.name}` : '设置过期'} open={expireVisible}
        onCancel={() => setExpireVisible(false)} onOk={saveExpire} okText="确定" cancelText="取消">
        <Form form={expireForm} layout="vertical">
          <Form.Item name="ttl" label="过期秒数" extra="留空 = 不修改；-1 = 设为永久；填值 = 重设过期时间；0 无效"
            rules={[{ validator: (_: unknown, val: any) => (val === 0 ? Promise.reject(new Error('0 秒会被 Redis 立即删除，永久请填 -1')) : Promise.resolve()) }]}>
            <InputNumber min={-1} style={{ width: '100%' }} placeholder="如 3600" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
