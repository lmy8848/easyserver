import {
  Card, Button, Space, Tag, Modal, Form, Input, InputNumber, Select,
  Popconfirm, Row, Col, Table, Empty, Spin, Switch,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, DeleteOutlined, ReloadOutlined,
  DownloadOutlined, UndoOutlined,
  ArrowLeftOutlined, TableOutlined, EditOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';
import { useState, useEffect } from 'react';
import { renderCell } from './cellRender';
import type { DatabasesTabProps, Database as DBType, TableExplorerProps, TableColumnInfo } from './types';

// 数据库 tab — 库列表；选中一个库后在同一 tab 内联表浏览器（原 TableExplorer，
// 已合并进本文件）。建库/建表/记录弹窗都随 tab 走；备份从库列表每行操作列的
// 备份按钮打开弹窗（展示该库备份列表 + 创建）。
export default function DatabasesTab({
  server, version, databases, dbsLoading, busy,
  onFetchDatabases, onOpenCreateDB,
  onEnterDatabase, onDeleteDB,
  dbModalVisible, onDbModalVisibleChange, dbForm, onCreateDB,
  backups, backupsLoading, backupCreating,
  onFetchBackups, onCreateBackup, onDownloadBackup, onRestoreBackup, onDeleteBackup,
}: DatabasesTabProps) {
  // 备份弹窗：当前选中备份的库 + 开关。打开时拉一次最新列表。
  const [backupDbName, setBackupDbName] = useState('');
  const [backupModalVisible, setBackupModalVisible] = useState(false);
  const openBackup = (db: DBType) => {
    setBackupDbName(db.name);
    setBackupModalVisible(true);
    onFetchBackups(db.name);
  };

  // 备份 SSE：弹窗开着且列表有 running 行时，订阅该行状态流，终态到达刷新列表
  // （替代定时轮询）。同库去重保证 running 最多一行。连接随弹窗生命周期开合。
  useEffect(() => {
    if (!backupModalVisible) return;
    const running = backups.find(b => b.status === 'running');
    if (!running) return;
    const es = new EventSource(`/api/db/backups/${running.id}/status`);
    es.onmessage = (e) => {
      let ev: any;
      try { ev = JSON.parse(e.data); } catch { return; }
      if (ev.type !== 'done') return; // running 心跳帧忽略
      es.close();
      onFetchBackups(backupDbName);
    };
    return () => es.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [backupModalVisible, backups, backupDbName]);

  const dbColumns = [
    { title: '数据库名', dataIndex: 'name', key: 'name', render: (t: string) => <strong>{t}</strong> },
    { title: '字符集', dataIndex: 'charset', key: 'charset', width: 120, responsive: ['lg'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    {
      title: '操作', key: 'action', width: 240,
      render: (_: unknown, record: DBType) => (
        <Space size="small">
          <Button type="link" size="small" icon={<TableOutlined />} onClick={() => onEnterDatabase(record)}>管理</Button>
          <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => openBackup(record)}>备份</Button>
          <Popconfirm title="确定删除此数据库？" onConfirm={() => onDeleteDB(record.name)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-db-${record.name}`}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const [searchText, setSearchText] = useState('');
  const filteredDatabases = databases.filter(d => d.name.toLowerCase().includes(searchText.toLowerCase()));

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space size="middle">
          <span style={{ fontSize: 16, fontWeight: 'bold' }}>数据库列表</span>
          <Tag color="blue">共 {filteredDatabases.length} 个</Tag>
        </Space>
        <Space>
          <Input.Search
            placeholder="搜索数据库名称"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
            allowClear
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={onOpenCreateDB} disabled={version?.status !== 'running'}>创建数据库</Button>
          <Button icon={<ReloadOutlined />} loading={dbsLoading} onClick={onFetchDatabases}>刷新</Button>
        </Space>
      </div>

      <Table
        columns={dbColumns}
        dataSource={filteredDatabases}
        rowKey="name"
        loading={dbsLoading}
        size="small"
        pagination={{ defaultPageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 条` }}
        locale={{ emptyText: <Empty description="暂无数据库" /> }}
      />

      {/* 创建数据库弹窗 */}
      <Modal title="创建数据库" open={dbModalVisible} onCancel={() => onDbModalVisibleChange(false)}
        onOk={onCreateDB} okText="创建" cancelText="取消" confirmLoading={busy === 'create-db'}>
        <Form form={dbForm} layout="vertical">
          <Form.Item label="版本"><Input value={`${server.display_name} ${version?.version || ''}`} disabled /></Form.Item>
          <Form.Item name="name" label="数据库名" rules={[{ required: true }]}><Input placeholder="如：my_app" /></Form.Item>
          <Form.Item name="charset" label="字符集" initialValue="utf8mb4">
            <Select><Select.Option value="utf8mb4">utf8mb4</Select.Option><Select.Option value="utf8">utf8</Select.Option></Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 备份弹窗：库级备份列表 + 创建（从库列表操作列打开） */}
      <Modal
        title={`备份 - ${backupDbName}`}
        open={backupModalVisible}
        onCancel={() => setBackupModalVisible(false)}
        footer={null}
        width={760}
      >
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
          <span>备份列表</span>
          {/* 同库有备份正在跑（列表中 running 行）时禁用，防止并发触发第二次备份 */}
          <Button type="primary" icon={<DownloadOutlined />} onClick={() => onCreateBackup(backupDbName)}
            disabled={backups.some(b => b.status === 'running')}
            loading={backupCreating}>
            创建备份
          </Button>
        </div>
        <Table
          columns={[
            { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
            { title: '大小', dataIndex: 'file_size', key: 'file_size', width: 100,
              render: (size: number) => size ? `${(size / 1024).toFixed(1)} KB` : '-' },
            { title: '状态', dataIndex: 'status', key: 'status', width: 100,
              render: (status: string) => (
                <Tag color={status === 'success' ? 'success' : status === 'failed' ? 'error' : 'processing'}>
                  {status === 'success' ? '成功' : status === 'failed' ? '失败' : '进行中'}
                </Tag>
              )},
            { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
            { title: '操作', key: 'action', width: 200,
              render: (_: unknown, record: any) => (
                <Space size="small">
                  {record.status === 'success' && (
                    <>
                      <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => onDownloadBackup(record.id)}>
                        下载
                      </Button>
                      <Popconfirm title="确定恢复此备份？这将覆盖当前数据。" onConfirm={() => onRestoreBackup(record.id, backupDbName)}>
                        <Button type="link" size="small" icon={<UndoOutlined />} loading={busy === `restore-${record.id}`}>
                          恢复
                        </Button>
                      </Popconfirm>
                    </>
                  )}
                  <Popconfirm title="确定删除此备份？" onConfirm={() => onDeleteBackup(record.id, backupDbName)}>
                    <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-backup-${record.id}`}>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
          dataSource={backups}
          rowKey="id"
          loading={backupsLoading}
          size="small"
          pagination={false}
        />
      </Modal>
    </div>
  );
}

// ===== 表浏览器（原 TableExplorer.tsx，合并进本文件） =====

// 记录弹窗：按列类型挑输入控件。编辑态锁主键列，插入态自增列置灰（由数据库生成）。
function recordInputFor(col: TableColumnInfo | undefined, editing: boolean): ReactNode {
  const type = (col?.type || '').toUpperCase();
  const disabled = editing ? !!col?.is_primary_key : !!col?.is_auto_incr;
  if (col?.is_auto_incr && !editing) {
    return <Input disabled placeholder="自增，自动生成" />;
  }
  if (/^(INT|BIGINT|SMALLINT|TINYINT|MEDIUMINT|INTEGER|SERIAL)\b/.test(type)) {
    return <InputNumber style={{ width: '100%' }} precision={0} step={1} disabled={disabled} />;
  }
  if (/^(DECIMAL|FLOAT|DOUBLE|NUMERIC|REAL)\b/.test(type)) {
    return <InputNumber style={{ width: '100%' }} disabled={disabled} />;
  }
  if (/^(BOOLEAN|BOOL|BIT)\b/.test(type)) {
    // 1/0 对 MySQL TINYINT(1) 与 PG boolean 都有效（PG 接受 '1'/'0' 输入）。
    return (
      <Select disabled={disabled}>
        <Select.Option value="1">true / 1</Select.Option>
        <Select.Option value="0">false / 0</Select.Option>
      </Select>
    );
  }
  if (/^(DATETIME|TIMESTAMP|DATE|TIME)\b/.test(type)) {
    const hint = type === 'DATE' ? '2024-01-01' : type === 'TIME' ? '12:00:00' : '2024-01-01 12:00:00';
    return <Input placeholder={hint} disabled={disabled} />;
  }
  if (/^(TEXT|LONGTEXT|MEDIUMTEXT|JSON|CLOB)\b/.test(type)) {
    return <Input.TextArea rows={3} placeholder={type === 'JSON' ? '{ "key": "value" }' : `输入${type}内容`} disabled={disabled} />;
  }
  return <Input disabled={disabled} />;
}

// 必填规则：自增不填；其余 NOT NULL 且无默认值的必填（可空或有默认值由数据库兜底）。
function isRecordColumnRequired(col: TableColumnInfo | undefined): boolean {
  if (!col) return false;
  if (col.is_auto_incr) return false;
  if (col.is_nullable) return false;
  if (col.has_default) return false;
  return true;
}

// 建表字符集/排序规则预设。
// MySQL：字符集 + 按字符集联动的排序规则。utf8mb4_0900_ai_ci 是 MySQL 8.0+ 官方默认
// （Unicode 9.0，比 unicode_ci 快且准）；unicode_520_ci 是 WordPress 等兼容旧系统的常用项；
// _bin 用于精确字节匹配（token/hash）。切换字符集后排序规则重置为该字符集首选。
const MYSQL_CHARSET_OPTIONS = [
  'utf8mb4', 'utf8', 'gbk', 'gb18030', 'gb2312', 'big5',
  'latin1', 'latin2', 'ascii', 'binary', 'utf16', 'utf32', 'cp1251', 'sjis', 'euckr',
];
const MYSQL_COLLATIONS: Record<string, string[]> = {
  utf8mb4: ['utf8mb4_0900_ai_ci', 'utf8mb4_unicode_ci', 'utf8mb4_unicode_520_ci', 'utf8mb4_general_ci', 'utf8mb4_bin', 'utf8mb4_0900_as_cs'],
  utf8: ['utf8_unicode_ci', 'utf8_general_ci', 'utf8_bin'],
  gbk: ['gbk_chinese_ci', 'gbk_bin'],
  gb18030: ['gb18030_chinese_ci', 'gb18030_bin', 'gb18030_unicode_520_ci'],
  gb2312: ['gb2312_chinese_ci', 'gb2312_bin'],
  big5: ['big5_chinese_ci', 'big5_bin'],
  latin1: ['latin1_swedish_ci', 'latin1_general_ci', 'latin1_bin'],
  latin2: ['latin2_general_ci', 'latin2_bin'],
  ascii: ['ascii_general_ci', 'ascii_bin'],
  binary: ['binary'],
  utf16: ['utf16_unicode_ci', 'utf16_general_ci', 'utf16_bin'],
  utf32: ['utf32_unicode_ci', 'utf32_general_ci', 'utf32_bin'],
  cp1251: ['cp1251_bulgarian_ci', 'cp1251_ukrainian_ci', 'cp1251_bin'],
  sjis: ['sjis_japanese_ci', 'sjis_bin'],
  euckr: ['euckr_korean_ci', 'euckr_bin'],
};
// PostgreSQL：无表级字符集（编码在数据库级，UTF8 主流），排序规则是 locale ——
// C/C.UTF-8 为字节序（快、无本地化），其余按语言。留空继承数据库默认 locale；
// 选值会拼到字符串列的 COLLATE，容器需装有对应 locale 才生效。
const PG_COLLATIONS = [
  'C', 'C.UTF-8', 'POSIX', 'ucs_basic', 'unicode',
  'zh_CN.UTF-8', 'zh_HK.UTF-8', 'zh_TW.UTF-8',
  'en_US.UTF-8', 'en_GB.UTF-8',
  'de_DE.UTF-8', 'fr_FR.UTF-8', 'es_ES.UTF-8', 'it_IT.UTF-8',
  'ja_JP.UTF-8', 'ko_KR.UTF-8', 'ru_RU.UTF-8',
];

const MYSQL_COLUMN_TYPES = [
  'INT', 'BIGINT', 'SMALLINT', 'TINYINT', 'MEDIUMINT',
  'VARCHAR', 'CHAR', 'TEXT', 'MEDIUMTEXT', 'LONGTEXT',
  'DATETIME', 'TIMESTAMP', 'DATE', 'TIME', 'YEAR',
  'DECIMAL', 'FLOAT', 'DOUBLE',
  'BOOLEAN', 'JSON', 'BLOB', 'MEDIUMBLOB', 'LONGBLOB',
  'VARBINARY', 'BIT', 'ENUM', 'SET',
];

const PG_COLUMN_TYPES = [
  'INTEGER', 'BIGINT', 'SMALLINT',
  'SERIAL', 'BIGSERIAL', 'SMALLSERIAL',
  'VARCHAR', 'CHAR', 'TEXT',
  'NUMERIC', 'REAL', 'DOUBLE PRECISION', 'MONEY',
  'TIMESTAMP', 'TIMESTAMPTZ', 'DATE', 'TIME', 'TIMETZ', 'INTERVAL',
  'BOOLEAN', 'BIT', 'JSON', 'JSONB', 'BYTEA', 'UUID', 'INET', 'CIDR', 'MACADDR',
];

export function TableExplorerView({
  server, database, onBack,
  tableList, tableLoading, selectedTable, tableData, tableDataLoading, tablePage, tableInfo,
  onSelectTable, onFetchTables, onFetchTableData,
  createTableVisible, createTableLoading, createForm, onCreateTableVisibleChange, onCreateTable, onDropTable,
  recordModalVisible, editingRecord, recordForm, recordSaving,
  onRecordModalVisibleChange, onOpenInsertModal, onOpenEditModal, onSaveRecord, onDeleteRecord,
  busy,
}: TableExplorerProps) {
  // 主键单行：watch 创建表弹窗的列，存在已设主键的行时其余行的主键开关禁用
  // （主键行本身保持可开关，取消后所有行恢复可选）。
  const columnRows = Form.useWatch('columns', createForm) || [];
  const hasPrimary = columnRows.some((c: any) => c?.is_primary);
  // PG 无表级字符集，排序规则是 locale；MySQL 是字符集 + 联动排序规则。
  const isPg = server?.db_type === 'postgresql';
  const createCharset = Form.useWatch('charset', createForm) || 'utf8mb4';
  const collationOptions = MYSQL_COLLATIONS[createCharset] || MYSQL_COLLATIONS['utf8mb4'] || [];

  const [dataSearchText, setDataSearchText] = useState('');

  return (
    <Card
      title={
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={onBack}>返回</Button>
          <DatabaseOutlined style={{ fontSize: 18 }} />
          <span style={{ fontSize: 16, fontWeight: 'bold' }}>{database.name}</span>
          <Tag>{database.charset}</Tag>
        </Space>
      }
    >
      <Row gutter={16}>
        {/* Left: Table list */}
        <Col span={6}>
          <Card title={<Space><TableOutlined /> 表列表</Space>} size="small"
            extra={
              <Space>
                <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => onCreateTableVisibleChange(true)}>新建</Button>
                <Button size="small" icon={<ReloadOutlined />} onClick={onFetchTables} />
              </Space>
            }>
            <div style={{ maxHeight: '60vh', overflowY: 'auto' }}>
              {tableLoading ? <Spin /> : tableList.length === 0 ? <Empty description="无表" /> : (
                tableList.map(t => (
                  <div key={t} onClick={() => { onSelectTable(t); onFetchTableData(t); }}
                    style={{
                      padding: '6px 12px', cursor: 'pointer', borderRadius: 4,
                      background: selectedTable === t ? '#e6f7ff' : 'transparent',
                      marginBottom: 2,
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                    }}>
                    <span><TableOutlined style={{ marginRight: 8 }} />{t}</span>
                    <Popconfirm title={`确定删除表 ${t}？此操作不可恢复！`} onConfirm={(e) => { e?.stopPropagation(); onDropTable(t); }}>
                      <Button type="text" size="small" danger icon={<DeleteOutlined />} loading={busy === `drop-table-${t}`} onClick={(e) => e.stopPropagation()} />
                    </Popconfirm>
                  </div>
                ))
              )}
            </div>
          </Card>
        </Col>

        {/* Right: Table Data */}
        <Col span={18}>
          {selectedTable ? (
            <div>
              <div style={{ marginBottom: 12, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Space>
                  <strong>{selectedTable}</strong>
                  <Tag>{(tableInfo?.collation || '').split('_')[0] || database.charset}</Tag>
                </Space>
                <Space>
                  <Input.Search
                    placeholder="搜索数据"
                    value={dataSearchText}
                    onChange={(e) => setDataSearchText(e.target.value)}
                    style={{ width: 200 }}
                    allowClear
                  />
                  <Button type="primary" icon={<PlusOutlined />} onClick={onOpenInsertModal}>插入记录</Button>
                  <Button icon={<ReloadOutlined />} loading={tableDataLoading}
                    onClick={() => onFetchTableData(selectedTable, tablePage)}>刷新</Button>
                </Space>
              </div>
              <Table
                columns={[
                  ...(tableData?.headers || []).map((h: string, hi: number) => ({
                    title: h, dataIndex: h, key: h, ellipsis: true,
                    align: ((tableData?.columnTypes || [])[hi] === 'number') ? 'right' as const : undefined,
                    sorter: ((tableData?.columnTypes || [])[hi] === 'number') ? (a: any, b: any) => (Number(a[h]) || 0) - (Number(b[h]) || 0) : undefined,
                    render: (v: any) => renderCell(v, (tableData?.columnTypes || [])[hi]),
                  })),
                  {
                    title: '操作', key: 'action', width: 140,
                    render: (_: unknown, record: any) => (
                      <Space size="small">
                        <Button type="link" size="small" icon={<EditOutlined />} onClick={() => onOpenEditModal(record)}>编辑</Button>
                        <Popconfirm title="确定删除此记录？" onConfirm={() => onDeleteRecord(record)}>
                          <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-record-${record._key}`}>删除</Button>
                        </Popconfirm>
                      </Space>
                    ),
                  },
                ]}
                dataSource={(tableData?.rows || [])
                  .filter((row: any[]) => {
                    if (!dataSearchText.trim()) return true;
                    const term = dataSearchText.toLowerCase();
                    return row.some((cell: any) => String(cell ?? '').toLowerCase().includes(term));
                  })
                  .map((row: any[], i: number) => {
                    const obj: any = { _key: i };
                    (tableData?.headers || []).forEach((h: string, j: number) => { obj[h] = row[j]; });
                    return obj;
                  })}
                rowKey="_key"
                loading={tableDataLoading}
                size="small"
                pagination={{
                  current: tablePage,
                  pageSize: 50,
                  total: tableData?.total || 0,
                  onChange: (p) => onFetchTableData(selectedTable, p),
                  showTotal: (t) => `共 ${t} 条`,
                  showSizeChanger: true,
                }}
              />
            </div>
          ) : <Empty description="选择左侧表查看数据" style={{ padding: '60px 0' }} />}
        </Col>
      </Row>

      {/* Insert/Edit Record Modal */}
      <Modal
        title={editingRecord ? `编辑记录 - ${selectedTable}` : `插入记录 - ${selectedTable}`}
        open={recordModalVisible}
        onCancel={() => onRecordModalVisibleChange(false)}
        onOk={onSaveRecord}
        okText={editingRecord ? '保存' : '插入'}
        cancelText="取消"
        confirmLoading={recordSaving}
        width={600}
      >
        {tableData?.headers && tableData.headers.length > 0 ? (
          <Form form={recordForm} layout="vertical">
            {tableData.headers.map(h => {
              const col = tableInfo?.columns?.find(c => c.name === h);
              return (
                <Form.Item
                  key={h}
                  name={h}
                  label={col
                    ? <span>{col.name} <Tag style={{ fontSize: 12, marginInlineStart: 4 }}>{col.type}</Tag></span>
                    : h}
                  rules={isRecordColumnRequired(col) ? [{ required: true, message: `请输入 ${h}` }] : undefined}
                >
                  {recordInputFor(col, !!editingRecord)}
                </Form.Item>
              );
            })}
          </Form>
        ) : (
          <div style={{ textAlign: 'center', padding: 20, color: '#999' }}>
            请先选择一个表并等待数据加载完成
          </div>
        )}
        {editingRecord && (
          <div style={{ color: '#8c8c8c', fontSize: 12, marginTop: -8 }}>
            主键: {tableInfo?.primaryKey || tableData?.headers?.[0]} = {editingRecord[tableInfo?.primaryKey || tableData?.headers?.[0] || '']}
          </div>
        )}
      </Modal>

      {/* Create Table Modal */}
      <Modal
        title="创建表"
        open={createTableVisible}
        onCancel={() => { onCreateTableVisibleChange(false); createForm.resetFields(); }}
        onOk={onCreateTable}
        okText="创建"
        cancelText="取消"
        confirmLoading={createTableLoading}
        width={700}
      >
        <Form form={createForm} layout="vertical" initialValues={isPg
          ? { collation: '' } // PG 无表级字符集，排序规则留空 = 继承数据库 locale
          : { charset: 'utf8mb4', collation: 'utf8mb4_0900_ai_ci' }}>
          <Form.Item name="tableName" label="表名" rules={[{ required: true, message: '请输入表名' }]}>
            <Input placeholder="输入表名" />
          </Form.Item>
          {isPg ? (
            <Form.Item name="collation" label="排序规则（留空继承数据库 locale）">
              <Select showSearch allowClear placeholder="默认（继承数据库）">
                {PG_COLLATIONS.map(c => <Select.Option key={c} value={c}>{c}</Select.Option>)}
              </Select>
            </Form.Item>
          ) : (
            <Row gutter={8}>
              <Col span={12}>
                <Form.Item name="charset" label="字符集">
                  <Select showSearch onChange={(v: string) => {
                    // 字符集切换后若当前排序规则不匹配，重置为该字符集首选规则。
                    const cur = createForm.getFieldValue('collation');
                    const opts = MYSQL_COLLATIONS[v] || [];
                    if (!opts.includes(cur)) createForm.setFieldValue('collation', opts[0] || '');
                  }}>
                    {MYSQL_CHARSET_OPTIONS.map(c => <Select.Option key={c} value={c}>{c}</Select.Option>)}
                  </Select>
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="collation" label="排序规则">
                  <Select showSearch>
                    {collationOptions.map(c => (
                      <Select.Option key={c} value={c}>{c}</Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
            </Row>
          )}
          <Form.List name="columns">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => {
                  // 当前行列的类型/主键（watch columns 数组实时取），自增只在整数类型下展示。
                  const col = columnRows[name] || {};
                  const isInt = /^(INT|BIGINT|SMALLINT|TINYINT|MEDIUMINT|INTEGER|SERIAL)\b/i.test(col.type || '');
                  return (
                    <div key={key} style={{ border: '1px solid #f0f0f0', borderRadius: 8, padding: '12px 12px 0', marginBottom: 12 }}>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <div style={{ flex: 1 }}>
                          <Form.Item {...restField} name={[name, 'name']} rules={[{ required: true, message: '列名' }]}>
                            <Input placeholder="列名" />
                          </Form.Item>
                        </div>
                        <div style={{ flex: 1 }}>
                          <Form.Item {...restField} name={[name, 'type']} rules={[{ required: true, message: '类型' }]}>
                            <Select placeholder="类型" showSearch onChange={(v: string) => {
                              // 类型改成非整数时清掉自增，避免提交出无效 AUTO_INCREMENT。
                              if (!/^(INT|BIGINT|SMALLINT|TINYINT|MEDIUMINT|INTEGER|SERIAL)\b/i.test(v)) {
                                createForm.setFieldValue(['columns', name, 'auto_incr'], false);
                              }
                            }}>
                              {(isPg ? PG_COLUMN_TYPES : MYSQL_COLUMN_TYPES).map(t => (
                                <Select.Option key={t} value={t}>{t}</Select.Option>
                              ))}
                            </Select>
                          </Form.Item>
                        </div>
                        <div style={{ width: 140 }}>
                          <Form.Item {...restField} name={[name, 'length']}>
                            <Input placeholder="长度/精度" />
                          </Form.Item>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'flex-start' }}>
                          <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} />
                        </div>
                      </div>
                      <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                        <div style={{ flex: 1 }}>
                          <div style={{ display: 'flex', gap: 8 }}>
                            <div style={{ flex: 1 }}>
                              <Form.Item {...restField} name={[name, 'is_primary']} valuePropName="checked" style={{ marginBottom: 0 }}>
                                <Switch checkedChildren="主键" unCheckedChildren="主键"
                                  disabled={(hasPrimary && !col.is_primary) || !!col.nullable || !!col.unique}
                                  onChange={(v: boolean) => {
                                    // 互斥：主键与可空/唯一不共存，开启时清掉冲突项。
                                    if (v) {
                                      createForm.setFieldValue(['columns', name, 'nullable'], false);
                                      createForm.setFieldValue(['columns', name, 'unique'], false);
                                      // 主键+自增时不允许 DEFAULT，清掉已填默认值。
                                      if (col.auto_incr) {
                                        createForm.setFieldValue(['columns', name, 'default_value'], undefined);
                                      }
                                    }
                                  }} />
                              </Form.Item>
                            </div>
                            {isInt && (
                              <div style={{ flex: 1 }}>
                                <Form.Item {...restField} name={[name, 'auto_incr']} valuePropName="checked" style={{ marginBottom: 0 }}>
                                  <Switch checkedChildren="自增" unCheckedChildren="自增" onChange={(v: boolean) => {
                                    // 自增必须落在键上：开启时顺带把主键也打开，并清掉与之互斥的可空/唯一。
                                    if (v) {
                                      createForm.setFieldValue(['columns', name, 'is_primary'], true);
                                      createForm.setFieldValue(['columns', name, 'nullable'], false);
                                      createForm.setFieldValue(['columns', name, 'unique'], false);
                                      // 自增列不允许 DEFAULT，清掉已填默认值。
                                      createForm.setFieldValue(['columns', name, 'default_value'], undefined);
                                    }
                                  }} />
                                </Form.Item>
                              </div>
                            )}
                            <div style={{ flex: 1 }}>
                              <Form.Item {...restField} name={[name, 'nullable']} valuePropName="checked" style={{ marginBottom: 0 }}>
                                <Switch checkedChildren="可空" unCheckedChildren="可空"
                                  disabled={!!col.is_primary}
                                  onChange={(v: boolean) => {
                                    // 可空与主键互斥：开启时清掉主键。
                                    if (v) createForm.setFieldValue(['columns', name, 'is_primary'], false);
                                  }} />
                              </Form.Item>
                            </div>
                            <div style={{ flex: 1 }}>
                              <Form.Item {...restField} name={[name, 'unique']} valuePropName="checked" style={{ marginBottom: 0 }}>
                                <Switch checkedChildren="唯一" unCheckedChildren="唯一"
                                  disabled={!!col.is_primary}
                                  onChange={(v: boolean) => {
                                    // 唯一与主键互斥：开启时清掉主键。
                                    if (v) createForm.setFieldValue(['columns', name, 'is_primary'], false);
                                  }} />
                              </Form.Item>
                            </div>
                          </div>
                        </div>
                        <div style={{ flex: 1 }}>
                          <Form.Item {...restField} name={[name, 'default_value']} style={{ marginBottom: 0 }}>
                            <Input placeholder="默认值（如 0 / CURRENT_TIMESTAMP）"
                              disabled={!!col.is_primary && !!col.auto_incr} />
                          </Form.Item>
                        </div>
                      </div>
                    </div>
                  );
                })}
                <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                  添加列
                </Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>
    </Card>
  );
}
