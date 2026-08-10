import {
  Button, Space, Modal, Form, Input, Select, Table, Empty, Popconfirm,
} from 'antd';
import {
  PlusOutlined, DeleteOutlined, ReloadOutlined, TableOutlined,
} from '@ant-design/icons';
import TableExplorer from './TableExplorer';
import type { TablesTabProps, Database as DBType } from './types';

// 表 tab — 库列表；选中一个库后在同一 tab 内联表浏览器（TableExplorer）。
// 建库弹窗随 tab 走。
export default function TablesTab({
  server, version, databases, dbsLoading, busy,
  onEnterDatabase, onRefreshDatabases, onDeleteDB,
  dbModalVisible, onDbModalVisibleChange, dbForm, onCreateDB,
  tableExplorer,
}: TablesTabProps) {
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
      {tableExplorer ? (
        <TableExplorer {...tableExplorer} />
      ) : (
        <div>
          <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button icon={<ReloadOutlined />} loading={dbsLoading} onClick={onRefreshDatabases}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />}
              onClick={() => { dbForm.resetFields(); onDbModalVisibleChange(true); }}
              disabled={version.status !== 'running'}>创建数据库</Button>
          </div>
          <Table columns={dbColumns} dataSource={databases} rowKey="name" loading={dbsLoading} size="small"
            locale={{ emptyText: <Empty description="暂无数据库" /> }} />
        </div>
      )}

      {/* 创建数据库弹窗 */}
      <Modal title="创建数据库" open={dbModalVisible} onCancel={() => onDbModalVisibleChange(false)}
        onOk={onCreateDB} okText="创建" cancelText="取消" confirmLoading={busy === 'create-db'}>
        <Form form={dbForm} layout="vertical">
          <Form.Item label="版本"><Input value={`${server.display_name} ${version.version}`} disabled /></Form.Item>
          <Form.Item name="name" label="数据库名" rules={[{ required: true }]}><Input placeholder="如：my_app" /></Form.Item>
          <Form.Item name="charset" label="字符集" initialValue="utf8mb4">
            <Select><Select.Option value="utf8mb4">utf8mb4</Select.Option><Select.Option value="utf8">utf8</Select.Option></Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
