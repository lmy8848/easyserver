import { Space, Select, Button, Input, Alert, Empty, Table } from 'antd';
import { ConsoleSqlOutlined } from '@ant-design/icons';
import type { SqlConsoleTabProps } from './types';
import { renderCell } from './cellRender';

export default function SqlConsoleTab({
  version,
  databases,
  sqlTargetDb,
  onSqlTargetDbChange,
  sqlInput,
  onSqlInputChange,
  sqlResult,
  sqlLoading,
  onExecuteSQL,
}: SqlConsoleTabProps) {
  // 页面层已保证仅在实例 running 时渲染本组件；version 可能为 null 的
  // 一帧（列表已到、实例尚未选中的空档）仍需兜底。
  if (!version) {
    return <Empty description="数据库实例未运行" />;
  }

  const effectiveDb = sqlTargetDb || (databases?.length > 0 ? databases[0]?.name || '' : '');

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      if (sqlInput.trim() && effectiveDb) {
        onExecuteSQL();
      }
    }
  };

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Space size="middle">
          <span>目标数据库：</span>
          <Select
            value={effectiveDb || undefined}
            onChange={onSqlTargetDbChange}
            placeholder="请选择目标数据库"
            style={{ width: 220 }}
          >
            {databases.map(db => (
              <Select.Option key={db.name} value={db.name}>{db.name}</Select.Option>
            ))}
          </Select>
          <Button
            type="primary"
            icon={<ConsoleSqlOutlined />}
            loading={sqlLoading}
            onClick={onExecuteSQL}
            disabled={!sqlInput.trim() || !effectiveDb}
          >
            执行 SQL (Ctrl+Enter)
          </Button>
        </Space>
      </div>

      <Input.TextArea
        value={sqlInput}
        onChange={(e) => onSqlInputChange(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="请输入 SQL 语句，例如：SELECT * FROM users LIMIT 100;"
        rows={6}
        style={{ fontFamily: 'monospace', fontSize: 13, marginBottom: 16 }}
      />

      {sqlResult && (
        <div style={{ overflowX: 'auto', width: '100%' }}>
          {sqlResult.success ? (
            sqlResult.rows ? (
              <Table
                size="small"
                bordered
                columns={(sqlResult.headers || []).map((h: string, hi: number) => ({
                  title: h, dataIndex: h, key: h, ellipsis: true,
                  sorter: (sqlResult.column_types || [])[hi] === 'number'
                    ? (a: any, b: any) => (Number(a[h]) || 0) - (Number(b[h]) || 0) : undefined,
                  render: (v: any) => renderCell(v, (sqlResult.column_types || [])[hi]),
                }))}
                dataSource={(sqlResult.rows || []).map((row: any[], i: number) => {
                  const obj: any = { _key: i };
                  (sqlResult.headers || []).forEach((h: string, j: number) => { obj[h] = row[j]; });
                  return obj;
                })}
                rowKey="_key"
                pagination={{ pageSize: 20, showSizeChanger: true }}
                scroll={{ y: 420 }}
              />
            ) : (
              <pre style={{
                fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', monospace",
                fontSize: 13,
                lineHeight: 1.5,
                background: '#fafafa',
                border: '1px solid #d9d9d9',
                borderRadius: 6,
                padding: 16,
                margin: 0,
                maxHeight: 480,
                overflowY: 'auto',
                whiteSpace: 'pre',
                overflowX: 'auto',
                color: '#222',
              }}>
                {sqlResult.output || 'Query OK'}
              </pre>
            )
          ) : (
            <Alert type="error" message="执行失败" description={sqlResult.error} showIcon />
          )}
        </div>
      )}
    </div>
  );
}
