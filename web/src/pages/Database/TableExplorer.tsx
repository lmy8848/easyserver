import {
  Card, Button, Space, Tag, Modal, Form, Input, Select,
  Popconfirm, Row, Col, Table, Tabs, Empty, Spin, Alert,
  Switch,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, DeleteOutlined, ReloadOutlined,
  DownloadOutlined,
  FileTextOutlined, UndoOutlined,
  ArrowLeftOutlined, TableOutlined, ConsoleSqlOutlined, EditOutlined,
} from '@ant-design/icons';
import STYLES from './styles';
import type { TableExplorerProps } from './types';

export default function TableExplorer({
  server, version, database, onBack,
  tableList, tableLoading, selectedTable, tableData, tableDataLoading, tablePage, tableInfo,
  onSelectTable, onFetchTables, onFetchTableData,
  createTableVisible, createTableLoading, createForm, onCreateTableVisibleChange, onCreateTable, onDropTable,
  recordModalVisible, editingRecord, recordForm, recordSaving,
  onRecordModalVisibleChange, onOpenInsertModal, onOpenEditModal, onSaveRecord, onDeleteRecord,
  sqlInput, sqlResult, sqlLoading, onSqlInputChange, onExecuteSQL,
  backups, backupsLoading, backupCreating,
  onCreateBackup, onDownloadBackup, onRestoreBackup, onDeleteBackup,
  logVisible, logVersion, logContent, logLoading, logFollow, logRef,
  onLogVisibleChange, onLogFollowChange,
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
                      <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={(e) => e.stopPropagation()} />
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
                      ...(tableData?.headers || []).map((h: string) => ({
                        title: h, dataIndex: h, key: h, ellipsis: true,
                      })),
                      {
                        title: '操作', key: 'action', width: 140,
                        render: (_: any, record: any) => (
                          <Space size="small">
                            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => onOpenEditModal(record)}>编辑</Button>
                            <Popconfirm title="确定删除此记录？" onConfirm={() => onDeleteRecord(record)}>
                              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
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
                        <Alert type="error" message={sqlResult.error} />
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
                          <Tag color={status === 'completed' ? 'success' : status === 'failed' ? 'error' : 'processing'}>
                            {status === 'completed' ? '完成' : status === 'failed' ? '失败' : '进行中'}
                          </Tag>
                        )},
                      { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
                      { title: '操作', key: 'action', width: 200,
                        render: (_: any, record: any) => (
                          <Space size="small">
                            {record.status === 'completed' && (
                              <>
                                <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => onDownloadBackup(record.id)}>
                                  下载
                                </Button>
                                <Popconfirm title="确定恢复此备份？这将覆盖当前数据。" onConfirm={() => onRestoreBackup(record.id)}>
                                  <Button type="link" size="small" icon={<UndoOutlined />}>
                                    恢复
                                  </Button>
                                </Popconfirm>
                              </>
                            )}
                            <Popconfirm title="确定删除此备份？" onConfirm={() => onDeleteBackup(record.id)}>
                              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
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

      {/* Log Modal */}
      <Modal
        title={<Space><FileTextOutlined /><span>{server.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
        open={logVisible} onCancel={() => onLogVisibleChange(false)}
        footer={<Row justify="space-between"><Col><Space><span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span><span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span></Space></Col><Col><Space><Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => onLogFollowChange(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button><Button size="small" onClick={() => onLogVisibleChange(false)}>关闭</Button></Space></Col></Row>}
        width="90vw" style={{ maxWidth: 960 }}>
        <div ref={logRef} style={{ ...STYLES.logContainer }}>
          {logContent.split('\n').map((line, i) => (
            <div key={i} style={STYLES.logLine}>
              <span style={STYLES.logLineNumber}>{i + 1}</span>
              <span style={STYLES.logLineText}>{line || ' '}</span>
            </div>
          ))}
        </div>
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
