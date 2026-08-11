import {
  Card, Button, Space, Tag, Modal, Form, Input, Select,
  Popconfirm, Row, Col, Table, Tabs, Empty, Spin, Alert, Switch, Tooltip,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, DeleteOutlined, ReloadOutlined,
  DownloadOutlined, UndoOutlined,
  ArrowLeftOutlined, TableOutlined, ConsoleSqlOutlined, EditOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';
import type { DatabasesTabProps, Database as DBType, TableExplorerProps } from './types';

// 数据库 tab — 库列表；选中一个库后在同一 tab 内联表浏览器（原 TableExplorer，
// 已合并进本文件）。建库/建表/记录弹窗都随 tab 走。
export default function DatabasesTab({
  server, version, databases, dbsLoading, busy,
  onEnterDatabase, onDeleteDB,
  dbModalVisible, onDbModalVisibleChange, dbForm, onCreateDB,
  tableExplorer,
}: DatabasesTabProps) {
  const dbColumns = [
    { title: '数据库名', dataIndex: 'name', key: 'name', render: (t: string) => <strong>{t}</strong> },
    { title: '字符集', dataIndex: 'charset', key: 'charset', width: 120, responsive: ['lg'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    {
      title: '操作', key: 'action', width: 200,
      render: (_: unknown, record: DBType) => (
        <Space size="small">
          <Button type="link" size="small" icon={<TableOutlined />} onClick={() => onEnterDatabase(record)}>管理</Button>
          <Popconfirm title="确定删除此数据库？" onConfirm={() => onDeleteDB(record.name)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={busy === `delete-db-${record.name}`}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {tableExplorer ? <TableExplorerView {...tableExplorer} /> : (
        <Table columns={dbColumns} dataSource={databases} rowKey="name" loading={dbsLoading} size="small"
          locale={{ emptyText: <Empty description="暂无数据库" /> }} />
      )}

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
    </div>
  );
}

// ===== 表浏览器（原 TableExplorer.tsx，合并进本文件） =====

// renderCell renders a table cell according to the column's type category. The
// direct-connection channel returns native values (numbers, null, []byte, ISO
// times); the CLI fallback returns plain strings, which render as-is. A string
// literally equal to "NULL" is treated as NULL for display.
function renderCell(v: any, type?: string): ReactNode {
  if (v === null || v === undefined) return <span style={{ color: '#bfbfbf', fontStyle: 'italic' }}>NULL</span>;
  const cat = type || (typeof v === 'number' ? 'number' : 'string');
  if (cat === 'blob') {
    const bytes = typeof v === 'string' ? hexToBytes(v) : Array.isArray(v) ? Uint8Array.from(v) : null;
    const preview = bytes && bytes.length > 0
      ? `0x${Array.from(bytes.slice(0, 8)).map(b => b.toString(16).padStart(2, '0')).join('')}${bytes.length > 8 ? '…' : ''}`
      : 'BLOB';
    return (
      <Tooltip title={`BLOB（${bytes?.length ?? '?'} 字节）`}>
        <code style={{ fontSize: 12 }}>{preview}</code>
      </Tooltip>
    );
  }
  if (cat === 'time') {
    const t = formatCellTime(v);
    return t;
  }
  if (typeof v === 'string' && v === 'NULL') return <span style={{ color: '#bfbfbf', fontStyle: 'italic' }}>NULL</span>;
  return String(v);
}

function hexToBytes(hex: string): Uint8Array | null {
  if (!/^[0-9a-fA-F]+$/.test(hex)) return null;
  const out = new Uint8Array(Math.floor(hex.length / 2));
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

// formatCellTime renders a time cell in the browser's local timezone. The value
// is a driver time (ISO string) or a raw timestamp string from the CLI channel.
function formatCellTime(v: any): string {
  if (typeof v === 'string') {
    const t = new Date(v);
    if (!Number.isNaN(t.getTime()) && /\d/.test(v)) {
      return t.toLocaleString();
    }
  } else if (v instanceof Date) {
    return v.toLocaleString();
  }
  return String(v);
}

function TableExplorerView({
  server, version, database, onBack,
  tableList, tableLoading, selectedTable, tableData, tableDataLoading, tablePage, tableInfo,
  onSelectTable, onFetchTables, onFetchTableData,
  createTableVisible, createTableLoading, createForm, onCreateTableVisibleChange, onCreateTable, onDropTable,
  recordModalVisible, editingRecord, recordForm, recordSaving,
  onRecordModalVisibleChange, onOpenInsertModal, onOpenEditModal, onSaveRecord, onDeleteRecord,
  sqlInput, sqlResult, sqlLoading, onSqlInputChange, onExecuteSQL,
  backups, backupsLoading, backupCreating, busy,
  onCreateBackup, onDownloadBackup, onRestoreBackup, onDeleteBackup,
}: TableExplorerProps) {
  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={onBack}>返回</Button>
          <DatabaseOutlined style={{ fontSize: 20 }} />
          <span style={{ fontSize: 16, fontWeight: 'bold' }}>{database.name}</span>
          <Tag>{database.charset}</Tag>
          <Tag>{server.display_name} {version.version}</Tag>
        </Space>
      </Card>

      <Row gutter={16}>
        {/* Left: Table list */}
        <Col span={6}>
          <Card title={<Space><TableOutlined /> 表列表</Space>} size="small"
            extra={
              <Space>
                <Button size="small" icon={<PlusOutlined />} onClick={() => onCreateTableVisibleChange(true)}>新建</Button>
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

        {/* Right: Data + SQL */}
        <Col span={18}>
          <Tabs items={[
            {
              key: 'data',
              label: <span><TableOutlined /> 数据</span>,
              children: selectedTable ? (
                <div>
                  <div style={{ marginBottom: 12, display: 'flex', justifyContent: 'space-between' }}>
                    <Space>
                      <strong>{selectedTable}</strong>
                      <Tag>{tableData?.total ?? 0} 条记录</Tag>
                    </Space>
                    <Space>
                      <Button icon={<ReloadOutlined />} loading={tableDataLoading}
                        onClick={() => onFetchTableData(selectedTable, tablePage)}>刷新</Button>
                      <Button type="primary" icon={<PlusOutlined />} onClick={onOpenInsertModal}>插入记录</Button>
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
                    dataSource={(tableData?.rows || []).map((row: any[], i: number) => {
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
                    }}
                  />
                </div>
              ) : <Empty description="选择左侧表查看数据" />,
            },
            {
              key: 'sql',
              label: <span><ConsoleSqlOutlined /> SQL 查询</span>,
              children: (
                <div>
                  <Input.TextArea
                    value={sqlInput}
                    onChange={(e) => onSqlInputChange(e.target.value)}
                    placeholder="SELECT * FROM table_name LIMIT 100;"
                    rows={4}
                    style={{ fontFamily: 'monospace', marginBottom: 12 }}
                  />
                  <Button type="primary" icon={<ConsoleSqlOutlined />}
                    loading={sqlLoading} onClick={onExecuteSQL}
                    disabled={!sqlInput.trim()}>执行</Button>
                  {sqlResult && (
                    <div style={{ marginTop: 12 }}>
                      {sqlResult.success ? (
                        <Input.TextArea value={sqlResult.output} readOnly rows={15}
                          style={{ fontFamily: 'monospace', fontSize: 12, background: '#f6ffed' }} />
                      ) : (
                        <Alert type="error" title={sqlResult.error} />
                      )}
                    </div>
                  )}
                </div>
              ),
            },
            {
              key: 'backup',
              label: <span><DownloadOutlined /> 备份</span>,
              children: (
                <div>
                  <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
                    <span>备份列表</span>
                    <Button type="primary" icon={<DownloadOutlined />} onClick={onCreateBackup} loading={backupCreating}>
                      创建备份
                    </Button>
                  </div>
                  <Table
                    columns={[
                      { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
                      { title: '数据库', dataIndex: 'database_name', key: 'database_name' },
                      { title: '类型', dataIndex: 'backup_type', key: 'backup_type', width: 80 },
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
                                <Popconfirm title="确定恢复此备份？这将覆盖当前数据。" onConfirm={() => onRestoreBackup(record.id)}>
                                  <Button type="link" size="small" icon={<UndoOutlined />} loading={busy === `restore-${record.id}`}>
                                    恢复
                                  </Button>
                                </Popconfirm>
                              </>
                            )}
                            <Popconfirm title="确定删除此备份？" onConfirm={() => onDeleteBackup(record.id)}>
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
                </div>
              ),
            },
          ]} />
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
            {tableData.headers.map(h => (
              <Form.Item key={h} name={h} label={h}>
                <Input placeholder={`输入 ${h}`} />
              </Form.Item>
            ))}
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
        <Form form={createForm} layout="vertical">
          <Form.Item name="tableName" label="表名" rules={[{ required: true, message: '请输入表名' }]}>
            <Input placeholder="输入表名" />
          </Form.Item>
          <Form.List name="columns">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <Row key={key} gutter={8} style={{ marginBottom: 8 }}>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'name']} rules={[{ required: true, message: '列名' }]}>
                        <Input placeholder="列名" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'type']} rules={[{ required: true, message: '类型' }]}>
                        <Select placeholder="类型">
                          <Select.Option value="INT">INT</Select.Option>
                          <Select.Option value="VARCHAR(255)">VARCHAR(255)</Select.Option>
                          <Select.Option value="TEXT">TEXT</Select.Option>
                          <Select.Option value="DATETIME">DATETIME</Select.Option>
                          <Select.Option value="TIMESTAMP">TIMESTAMP</Select.Option>
                          <Select.Option value="BOOLEAN">BOOLEAN</Select.Option>
                          <Select.Option value="DECIMAL(10,2)">DECIMAL(10,2)</Select.Option>
                        </Select>
                      </Form.Item>
                    </Col>
                    <Col span={4}>
                      <Form.Item {...restField} name={[name, 'is_primary']} valuePropName="checked">
                        <Switch checkedChildren="主键" unCheckedChildren="主键" />
                      </Form.Item>
                    </Col>
                    <Col span={4}>
                      <Form.Item {...restField} name={[name, 'auto_incr']} valuePropName="checked">
                        <Switch checkedChildren="自增" unCheckedChildren="自增" />
                      </Form.Item>
                    </Col>
                    <Col span={2}>
                      <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} />
                    </Col>
                  </Row>
                ))}
                <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                  添加列
                </Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>
    </div>
  );
}
