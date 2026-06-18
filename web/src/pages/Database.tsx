import { useState, useEffect, useRef } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, Select, InputNumber,
  message, Popconfirm, Row, Col, Table, Tabs, Empty, Spin, Tooltip, Alert,
  Descriptions, Switch, Divider,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, DeleteOutlined, ReloadOutlined,
  PlayCircleOutlined, StopOutlined, DownloadOutlined, CloudServerOutlined,
  FileTextOutlined, UserOutlined, KeyOutlined, UndoOutlined,
  CodeOutlined, ArrowLeftOutlined, TableOutlined, ConsoleSqlOutlined, EditOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../services/api';
import type { DBServer, Database, DBUser, DBVersion } from '../types';
import { usePortCheck } from '../hooks/usePortCheck';

// Styles
const STYLES = {
  cardActions: {
    marginTop: 12,
    borderTop: '1px solid #f0f0f0',
    paddingTop: 12,
    display: 'flex' as const,
    justifyContent: 'space-between' as const,
    flexWrap: 'wrap' as const,
    gap: 8,
  },
  versionInfo: {
    color: '#999',
    fontSize: 12,
    marginTop: 4,
  },
  emptyHint: {
    margin: '4px 0',
    color: '#52c41a',
  },
  logContainer: {
    background: '#fafafa',
    border: '1px solid #e8e8e8',
    fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    lineHeight: 1.8,
    padding: '8px 0',
    borderRadius: 6,
    maxHeight: '60vh',
    overflowY: 'auto' as const,
    overflowX: 'auto' as const,
  },
  logLine: {
    display: 'flex' as const,
    alignItems: 'baseline' as const,
    padding: '0 12px',
    minHeight: 22,
  },
  logLineNumber: {
    color: '#bfbfbf',
    minWidth: 36,
    width: 36,
    textAlign: 'right' as const,
    marginRight: 16,
    userSelect: 'none' as const,
    fontSize: 11,
    flexShrink: 0,
  },
  logLineText: {
    whiteSpace: 'nowrap' as const,
    color: '#262626',
  },
  skeletonLine: {
    display: 'flex' as const,
    gap: 8,
    marginBottom: 6,
  },
  skeletonBar: (width: number | string) => ({
    width,
    height: 14,
    background: '#f5f5f5',
    borderRadius: 2,
  }),
  portHint: {
    color: '#52c41a',
  },
  portHintError: {
    color: '#ff4d4f',
  },
} as const;

// Default config templates per database type
const DEFAULT_CONFIG_TEMPLATES: Record<string, string> = {
  mysql: `# MySQL 默认配置模板
# 保存后将创建此文件

[mysqld]
port = 3306
datadir = /var/lib/mysql
socket = /var/run/mysqld/mysqld.sock
max_connections = 151
innodb_buffer_pool_size = 128M
character-set-server = utf8mb4
collation-server = utf8mb4_general_ci
default-storage-engine = InnoDB
max_allowed_packet = 64M
tmp_table_size = 64M
max_heap_table_size = 64M
sort_buffer_size = 256K
read_buffer_size = 256K
join_buffer_size = 256K
log_error = /var/log/mysql/error.log
slow_query_log = OFF
long_query_time = 10
wait_timeout = 28800
interactive_timeout = 28800

[client]
default-character-set = utf8mb4
port = 3306
socket = /var/run/mysqld/mysqld.sock

[mysql]
default-character-set = utf8mb4

[mysqldump]
max_allowed_packet = 64M
default-character-set = utf8mb4
`,
  postgresql: `# PostgreSQL 默认配置模板

listen_addresses = 'localhost'
port = 5432
max_connections = 100
shared_buffers = 128MB
work_mem = 4MB
maintenance_work_mem = 64MB
log_destination = 'stderr'
logging_collector = on
log_directory = 'pg_log'
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'
`,
  redis: `# Redis 默认配置模板

bind 127.0.0.1
port 6379
maxmemory 256mb
maxmemory-policy allkeys-lru
save 900 1
save 300 10
save 60 10000
`,
};

export default function DatabasePage() {
  const [servers, setServers] = useState<DBServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedServer, setSelectedServer] = useState<DBServer | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<DBVersion | null>(null);
  const [selectedDatabase, setSelectedDatabase] = useState<Database | null>(null);
  const [operating, setOperating] = useState('');

  // Version state
  const [versions, setVersions] = useState<DBVersion[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [versionTemplates, setVersionTemplates] = useState<Array<{ version: string; package: string; description: string }>>([]);
  const [installVersionVisible, setInstallVersionVisible] = useState(false);
  const [installVersionForm] = Form.useForm();

  // Port check
  const { result: portCheck, checking: portChecking, checkPort, clearResult: clearPortCheck } = usePortCheck();

  // Database state
  const [databases, setDatabases] = useState<Database[]>([]);
  const [dbsLoading, setDbsLoading] = useState(false);
  const [dbModalVisible, setDbModalVisible] = useState(false);
  const [dbForm] = Form.useForm();

  // User state
  const [dbUsers, setDBUsers] = useState<DBUser[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [userModalVisible, setUserModalVisible] = useState(false);
  const [userForm] = Form.useForm();

  // Grant modal
  const [grantVisible, setGrantVisible] = useState(false);
  const [grantUser, setGrantUser] = useState<DBUser | null>(null);
  const [grantForm] = Form.useForm();

  // Config editor
  const [configVisible, setConfigVisible] = useState(false);
  const [configContent, setConfigContent] = useState('');
  const [configLoading, setConfigLoading] = useState(false);

  // Service logs
  const [logVisible, setLogVisible] = useState(false);
  const [logVersion, setLogVersion] = useState<DBVersion | null>(null);
  const [logContent, setLogContent] = useState('');
  const [logLoading, setLogLoading] = useState(false);
  const [logFollow, setLogFollow] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);

  // Database introspection
  const [tableList, setTableList] = useState<string[]>([]);
  const [tableLoading, setTableLoading] = useState(false);
  const [tableData, setTableData] = useState<{ headers: string[]; rows: any[][]; total: number } | null>(null);
  const [tableInfo, setTableInfo] = useState<{ primaryKey: string; columns: Array<{ name: string; type: string; key?: string }> } | null>(null);
  const [tableDataLoading, setTableDataLoading] = useState(false);
  const [selectedTable, setSelectedTable] = useState<string>('');
  const [tablePage, setTablePage] = useState(1);
  const [sqlInput, setSqlInput] = useState('');
  const [sqlResult, setSqlResult] = useState<{ success: boolean; output?: string; error?: string } | null>(null);
  const [sqlLoading, setSqlLoading] = useState(false);

  // DB config editor (generic for MySQL/PostgreSQL/Redis)
  const [dbConfig, setDBConfig] = useState<any>(null);
  const [dbConfigLoading, setDBConfigLoading] = useState(false);

  const fetchDBConfig = async (serverName?: string) => {
    const name = serverName || selectedServer?.name;
    if (!name) return;
    setDBConfigLoading(true);
    try {
      let res;
      if (name === 'mysql') {
        res = await dbServerApi.getMySQLConfig();
      } else if (name === 'postgresql') {
        res = await dbServerApi.getPostgreSQLConfig();
      } else if (name === 'redis') {
        res = await dbServerApi.getRedisConfig();
      } else {
        setDBConfig(null);
        return;
      }
      setDBConfig(res.data?.data || null);
    } catch (error) {
      console.error('Failed to load config:', error);
      setDBConfig(null);
    } finally {
      setDBConfigLoading(false);
    }
  };

  const handleSaveDBConfig = async () => {
    if (!dbConfig?.config?.sections || !selectedServer?.name) return;
    try {
      if (selectedServer.name === 'mysql') {
        await dbServerApi.saveMySQLConfig(dbConfig.config.sections);
      } else if (selectedServer.name === 'postgresql') {
        await dbServerApi.savePostgreSQLConfig(dbConfig.config.sections);
      } else if (selectedServer.name === 'redis') {
        await dbServerApi.saveRedisConfig(dbConfig.config.sections);
      }
      message.success('配置已保存（已自动备份原文件）');
      fetchDBConfig();
    } catch (error: any) {
      message.error(error.message || '保存失败');
    }
  };

  const updateDBParam = (section: string, key: string, value: string) => {
    setDBConfig((prev: any) => {
      if (!prev?.config?.sections) return prev;
      const newSections = prev.config.sections.map((s: any) => {
        if (s.name === section) {
          return { ...s, params: { ...s.params, [key]: value } };
        }
        return s;
      });
      return { ...prev, config: { ...prev.config, sections: newSections } };
    });
  };

  // Insert/Edit record modal
  const [recordModalVisible, setRecordModalVisible] = useState(false);
  const [editingRecord, setEditingRecord] = useState<any>(null); // null = insert, object = edit
  const [recordForm] = Form.useForm();
  const [recordSaving, setRecordSaving] = useState(false);

  useEffect(() => { fetchServers(); }, []);

  // Auto-refresh logs
  useEffect(() => {
    if (!logVisible || !logVersion) return;
    const refresh = async () => {
      try {
        const res = await dbServerApi.getVersionLogs(logVersion.id, 200);
        setLogContent(res.data?.data?.logs || '(empty)');
      } catch (error) { console.error('Failed to refresh logs:', error); }
    };
    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, [logVisible, logVersion?.id]);

  useEffect(() => {
    if (logFollow && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logContent, logFollow]);

  const fetchServers = async () => {
    setLoading(true);
    try {
      const res = await dbServerApi.list();
      setServers(res.data?.data || []);
    } catch (error) { console.error('Failed to fetch servers:', error); message.error('加载服务器列表失败'); } finally { setLoading(false); }
  };

  const refreshServer = async (serverId: number) => {
    try {
      const res = await dbServerApi.get(serverId);
      const updated = res.data?.data;
      if (updated) {
        setServers(prev => prev.map(s => s.id === serverId ? { ...s, ...updated } : s));
        setSelectedServer(prev => prev && prev.id === serverId ? { ...prev, ...updated } : prev);
      }
    } catch (error) { console.error('Failed to refresh server:', error); }
  };

  const enterServer = async (server: DBServer) => {
    setSelectedServer(server);
    setSelectedVersion(null);
    setSelectedDatabase(null);
    await fetchVersions(server.id);
    try {
      const res = await dbServerApi.getVersionTemplates(server.id);
      setVersionTemplates(res.data?.data || []);
    } catch (error) { console.error('Failed to fetch version templates:', error); }
  };

  const enterVersion = async (version: DBVersion) => {
    setSelectedVersion(version);
    setSelectedDatabase(null);
    const server = selectedServer;
    if (!server) return;
    await Promise.all([fetchDatabases(server.id), fetchUsers(server.id)]);
  };

  const enterDatabase = async (db: Database) => {
    setSelectedDatabase(db);
    setSelectedTable('');
    setTableData(null);
    setSqlResult(null);
    await fetchTables(db.id);
  };

  const goBackToServers = () => {
    setSelectedServer(null);
    setSelectedVersion(null);
    setSelectedDatabase(null);
    setVersions([]); setDatabases([]); setDBUsers([]);
  };

  const goBackToVersions = () => {
    setSelectedVersion(null);
    setSelectedDatabase(null);
    setDatabases([]); setDBUsers([]);
  };

  const goBackToVersionDetail = () => {
    setSelectedDatabase(null);
    setSelectedTable('');
    setTableData(null);
  };

  const fetchVersions = async (serverId: number) => {
    setVersionsLoading(true);
    try { const res = await dbServerApi.listVersions(serverId); setVersions(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch versions:', error); message.error('加载版本列表失败'); } finally { setVersionsLoading(false); }
  };

  const fetchDatabases = async (serverId: number) => {
    setDbsLoading(true);
    try { const res = await dbServerApi.listDatabases(serverId); setDatabases(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch databases:', error); message.error('加载数据库列表失败'); } finally { setDbsLoading(false); }
  };

  const fetchUsers = async (serverId: number) => {
    setUsersLoading(true);
    try { const res = await dbServerApi.listUsers(serverId); setDBUsers(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch users:', error); message.error('加载用户列表失败'); } finally { setUsersLoading(false); }
  };

  const fetchTables = async (dbId: number) => {
    setTableLoading(true);
    try {
      const res = await dbServerApi.listTables(dbId);
      const data = res.data?.data;
      setTableList(Array.isArray(data) ? data.map((t: any) => t.name) : []);
    } catch (error) {
      console.error('Failed to fetch tables:', error);
      setTableList([]);
    } finally { setTableLoading(false); }
  };

  const fetchTableData = async (dbId: number, table: string, page: number = 1) => {
    setTableDataLoading(true);
    try {
      const [queryRes, describeRes] = await Promise.all([
        dbServerApi.queryTable(dbId, table, page, 50),
        dbServerApi.describeTable(dbId, table),
      ]);
      const data = queryRes.data?.data;
      if (data && data.headers) {
        setTableData({
          headers: data.headers || [],
          rows: data.rows || [],
          total: data.total || 0,
        });
      } else {
        setTableData({ headers: [], rows: [], total: 0 });
      }
      setTablePage(page);
      // Store table info with primary key
      const columns = describeRes.data?.data || [];
      const pkCol = columns.find((c: any) => c.key === 'PRI');
      setTableInfo({
        primaryKey: pkCol?.name || columns[0]?.name || 'id',
        columns: columns.map((c: any) => ({ name: c.name, type: c.type, key: c.key })),
      });
    } catch (error) {
      console.error('Failed to fetch table data:', error);
      setTableData({ headers: [], rows: [], total: 0 });
      setTableInfo(null);
    } finally { setTableDataLoading(false); }
  };

  const handleExecuteSQL = async () => {
    if (!selectedDatabase || !sqlInput.trim()) return;
    setSqlLoading(true);
    try {
      const res = await dbServerApi.executeSQL(selectedDatabase.id, sqlInput);
      setSqlResult(res.data?.data || null);
      // Refresh table data if it was a mutation
      if (selectedTable && /^(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE)/i.test(sqlInput.trim())) {
        fetchTableData(selectedDatabase.id, selectedTable);
      }
    } catch (error: any) { setSqlResult({ success: false, error: error.message }); }
    finally { setSqlLoading(false); }
  };

  // Insert record
  const openInsertModal = () => {
    setEditingRecord(null);
    recordForm.resetFields();
    setRecordModalVisible(true);
  };

  // Edit record
  const openEditModal = (record: any) => {
    setEditingRecord(record);
    const values: any = {};
    (tableData?.headers || []).forEach(h => { values[h] = record[h]; });
    recordForm.setFieldsValue(values);
    setRecordModalVisible(true);
  };

  const handleSaveRecord = async () => {
    if (!selectedDatabase || !selectedTable) return;
    setRecordSaving(true);
    try {
      const values = await recordForm.validateFields();
      if (editingRecord) {
        // Update - use actual primary key from describeTable
        const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
        const pkVal = editingRecord[pk];
        const res = await dbServerApi.updateRecord(selectedDatabase.id, selectedTable, values, pk, pkVal);
        if (res.data?.data?.success) {
          message.success('更新成功');
        } else {
          message.error(res.data?.data?.error || '更新失败');
        }
      } else {
        // Insert
        const res = await dbServerApi.insertRecord(selectedDatabase.id, selectedTable, values);
        if (res.data?.data?.success) {
          message.success('插入成功');
        } else {
          message.error(res.data?.data?.error || '插入失败');
        }
      }
      setRecordModalVisible(false);
      fetchTableData(selectedDatabase.id, selectedTable, tablePage);
    } catch (error: any) { if (error.message) message.error(error.message); }
    finally { setRecordSaving(false); }
  };

  // Delete record
  const handleDeleteRecord = async (record: any) => {
    if (!selectedDatabase || !selectedTable) return;
    try {
      const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
      const pkVal = record[pk];
      const res = await dbServerApi.deleteRecord(selectedDatabase.id, selectedTable, pk, pkVal);
      if (res.data?.data?.success) {
        message.success('删除成功');
        fetchTableData(selectedDatabase.id, selectedTable, tablePage);
      } else {
        message.error(res.data?.data?.error || '删除失败');
      }
    } catch (error: any) { message.error(error.message || '删除失败'); }
  };

  // Version actions
  const handleInstallVersion = async () => {
    const server = selectedServer;
    if (!server) return;
    try {
      const values = await installVersionForm.validateFields();
      await dbServerApi.installVersion(server.id, values);
      message.success('版本安装成功');
      setInstallVersionVisible(false);
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { if (error.message) message.error(error.message); }
  };

  const handleStartVersion = async (v: DBVersion) => {
    const server = selectedServer;
    if (!server) return;
    setOperating(`start-${v.id}`);
    try {
      await dbServerApi.startVersion(v.id);
      message.success('已启动');
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { message.error(error.message || '启动失败'); }
    finally { setOperating(''); }
  };

  const handleStopVersion = async (v: DBVersion) => {
    const server = selectedServer;
    if (!server) return;
    setOperating(`stop-${v.id}`);
    try {
      await dbServerApi.stopVersion(v.id);
      message.success('已停止');
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { message.error(error.message || '停止失败'); }
    finally { setOperating(''); }
  };

  const handleRestartVersion = async (v: DBVersion) => {
    const server = selectedServer;
    if (!server) return;
    setOperating(`restart-${v.id}`);
    try {
      await dbServerApi.restartVersion(v.id);
      message.success('已重启');
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { message.error(error.message || '重启失败'); }
    finally { setOperating(''); }
  };

  const handleUninstallVersion = async (v: DBVersion) => {
    const server = selectedServer;
    if (!server) return;
    try {
      await dbServerApi.uninstallVersion(v.id);
      message.success('已卸载');
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { message.error(error.message || '卸载失败'); }
  };

  // Database CRUD
  const handleCreateDB = async () => {
    const server = selectedServer;
    const version = selectedVersion;
    if (!server || !version) return;
    try {
      const values = await dbForm.validateFields();
      await dbServerApi.createDatabase(server.id, { ...values, db_version_id: version.id });
      message.success('数据库创建成功');
      setDbModalVisible(false);
      fetchDatabases(server.id);
    } catch (error: any) { if (error.message) message.error(error.message); }
  };

  const handleDeleteDB = async (dbId: number) => {
    const server = selectedServer;
    if (!server) return;
    try {
      await dbServerApi.deleteDatabase(server.id, dbId);
      message.success('数据库已删除');
      fetchDatabases(server.id);
    } catch (error: any) { message.error(error.message || '删除失败'); }
  };

  // User CRUD
  const handleCreateUser = async () => {
    const server = selectedServer;
    if (!server) return;
    try {
      const values = await userForm.validateFields();
      await dbServerApi.createUser(server.id, values);
      message.success('用户创建成功');
      setUserModalVisible(false);
      fetchUsers(server.id);
    } catch (error: any) { if (error.message) message.error(error.message); }
  };

  const handleDeleteUser = async (userId: number) => {
    const server = selectedServer;
    if (!server) return;
    try {
      await dbServerApi.deleteUser(server.id, userId);
      message.success('用户已删除');
      fetchUsers(server.id);
    } catch (error: any) { message.error(error.message || '删除失败'); }
  };

  const handleGrant = async () => {
    const server = selectedServer;
    const user = grantUser;
    if (!server || !user) return;
    try {
      const values = await grantForm.validateFields();
      const payload = {
        ...values,
        privileges: Array.isArray(values.privileges) ? values.privileges.join(', ') : values.privileges,
      };
      await dbServerApi.grantPrivileges(server.id, user.id, payload);
      message.success('授权成功');
      setGrantVisible(false);
      fetchUsers(server.id);
    } catch (error: any) { if (error.message) message.error(error.message); }
  };

  const showLogs = async (v: DBVersion) => {
    setLogVersion(v);
    setLogVisible(true);
    setLogLoading(true);
    try {
      const res = await dbServerApi.getVersionLogs(v.id, 200);
      setLogContent(res.data?.data?.logs || '(empty)');
    } catch (error: any) { setLogContent('Failed: ' + error.message); }
    finally { setLogLoading(false); }
  };

  const showConfig = async (v: DBVersion) => {
    setConfigVisible(true);
    setConfigLoading(true);
    try {
      const serverName = selectedServer?.name;
      let res;
      if (serverName === 'mysql') {
        res = await dbServerApi.getMySQLConfig();
      } else if (serverName === 'postgresql') {
        res = await dbServerApi.getPostgreSQLConfig();
      } else if (serverName === 'redis') {
        res = await dbServerApi.getRedisConfig();
      }

      const data = res?.data?.data;
      if (data?.found && data.config?.sections?.length > 0) {
        let content = `# ${selectedServer?.display_name} Config: ${data.config.file_path}\n\n`;
        for (const section of data.config.sections) {
          if (section.name !== 'main') {
            content += `[${section.name}]\n`;
          }
          for (const [key, val] of Object.entries(section.params || {})) {
            if (serverName === 'redis') {
              content += `${key} ${val}\n`;
            } else {
              content += `${key} = ${val}\n`;
            }
          }
          content += '\n';
        }
        setConfigContent(content);
      } else {
        const dbType = serverName || 'mysql';
        const template = DEFAULT_CONFIG_TEMPLATES[dbType] || DEFAULT_CONFIG_TEMPLATES.mysql;
        setConfigContent(`# ${selectedServer?.display_name} 默认配置模板\n# 保存后将创建配置文件\n\n${template}`);
      }
    } catch (error: any) { setConfigContent('# Error: ' + error.message); }
    finally { setConfigLoading(false); }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'running': return '#52c41a';
      case 'stopped': return '#ff4d4f';
      case 'partial': return '#faad14';
      default: return '#999';
    }
  };

  const statusTag = (status: string) => {
    switch (status) {
      case 'running': return <Tag color="success">运行中</Tag>;
      case 'stopped': return <Tag color="error">已停止</Tag>;
      case 'partial': return <Tag color="warning">部分运行</Tag>;
      case 'not_installed': return <Tag color="default">未安装</Tag>;
      default: return <Tag color="default">{status}</Tag>;
    }
  };

  // ========== Level 4: Database detail (tables + SQL) ==========
  if (selectedDatabase && selectedVersion) {
    return (
      <div>
        <Card style={{ marginBottom: 16 }}>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={goBackToVersionDetail}>返回</Button>
            <DatabaseOutlined style={{ fontSize: 20 }} />
            <span style={{ fontSize: 16, fontWeight: 'bold' }}>{selectedDatabase.name}</span>
            <Tag>{selectedDatabase.charset}</Tag>
            <Tag>{selectedServer?.display_name} {selectedVersion.version}</Tag>
          </Space>
        </Card>

        <Row gutter={16}>
          {/* Left: Table list */}
          <Col span={6}>
            <Card title={<Space><TableOutlined /> 表列表</Space>} size="small"
              extra={<Button size="small" icon={<ReloadOutlined />} onClick={() => fetchTables(selectedDatabase.id)} />}>
              <div style={{ maxHeight: '60vh', overflowY: 'auto' }}>
                {tableLoading ? <Spin /> : tableList.length === 0 ? <Empty description="无表" /> : (
                  tableList.map(t => (
                    <div key={t} onClick={() => { setSelectedTable(t); fetchTableData(selectedDatabase.id, t); }}
                      style={{
                        padding: '6px 12px', cursor: 'pointer', borderRadius: 4,
                        background: selectedTable === t ? '#e6f7ff' : 'transparent',
                        marginBottom: 2,
                      }}>
                      <TableOutlined style={{ marginRight: 8 }} />{t}
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
                          onClick={() => fetchTableData(selectedDatabase.id, selectedTable, tablePage)}>刷新</Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={openInsertModal}>插入记录</Button>
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
                              <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditModal(record)}>编辑</Button>
                              <Popconfirm title="确定删除此记录？" onConfirm={() => handleDeleteRecord(record)}>
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
                        onChange: (p) => fetchTableData(selectedDatabase.id, selectedTable, p),
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
                      onChange={(e) => setSqlInput(e.target.value)}
                      placeholder="SELECT * FROM table_name LIMIT 100;"
                      rows={4}
                      style={{ fontFamily: 'monospace', marginBottom: 12 }}
                    />
                    <Button type="primary" icon={<ConsoleSqlOutlined />}
                      loading={sqlLoading} onClick={handleExecuteSQL}
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
            ]} />
          </Col>
        </Row>

        {/* Insert/Edit Record Modal */}
        <Modal
          title={editingRecord ? `编辑记录 - ${selectedTable}` : `插入记录 - ${selectedTable}`}
          open={recordModalVisible}
          onCancel={() => setRecordModalVisible(false)}
          onOk={handleSaveRecord}
          okText={editingRecord ? '保存' : '插入'}
          cancelText="取消"
          confirmLoading={recordSaving}
          width={600}
        >
          <Form form={recordForm} layout="vertical">
            {(tableData?.headers || []).map(h => (
              <Form.Item key={h} name={h} label={h}>
                <Input placeholder={`输入 ${h}`} />
              </Form.Item>
            ))}
          </Form>
          {editingRecord && (
            <div style={{ color: '#8c8c8c', fontSize: 12, marginTop: -8 }}>
              主键: {tableInfo?.primaryKey || tableData?.headers?.[0]} = {editingRecord[tableInfo?.primaryKey || tableData?.headers?.[0] || '']}
            </div>
          )}
        </Modal>

        {/* Log Modal for Level 4 */}
        <Modal
          title={<Space><FileTextOutlined /><span>{selectedServer?.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
          open={logVisible} onCancel={() => setLogVisible(false)}
          footer={<Row justify="space-between"><Col><Space><span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span><span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span></Space></Col><Col><Space><Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => setLogFollow(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button><Button size="small" onClick={() => setLogVisible(false)}>关闭</Button></Space></Col></Row>}
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
      </div>
    );
  }

  // ========== Level 3: Version detail ==========
  if (selectedVersion && selectedServer) {
    const versionDatabases = databases.filter(d => d.db_version_id === selectedVersion.id);

    const dbColumns = [
      { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
      { title: '数据库名', dataIndex: 'name', key: 'name', render: (t: string) => <strong>{t}</strong> },
      { title: '字符集', dataIndex: 'charset', key: 'charset', width: 100 },
      { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
      {
        title: '操作', key: 'action', width: 200,
        render: (_: any, record: Database) => (
          <Space size="small">
            <Button type="link" size="small" icon={<TableOutlined />} onClick={() => enterDatabase(record)}>管理</Button>
            <Popconfirm title="确定删除此数据库？" onConfirm={() => handleDeleteDB(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    const userColumns = [
      { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
      { title: '用户名', dataIndex: 'username', key: 'username', render: (t: string) => <strong>{t}</strong> },
      { title: '主机', dataIndex: 'host', key: 'host', width: 120 },
      { title: '权限', dataIndex: 'privileges', key: 'privileges', ellipsis: true },
      {
        title: '操作', key: 'action', width: 180,
        render: (_: any, record: DBUser) => (
          <Space size="small">
            <Button type="link" size="small" icon={<KeyOutlined />}
              onClick={() => { setGrantUser(record); grantForm.resetFields(); setGrantVisible(true); }}>授权</Button>
            <Popconfirm title="确定删除此用户？" onConfirm={() => handleDeleteUser(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    return (
      <div>
        <Card style={{ marginBottom: 16 }}>
          <Row justify="space-between" align="middle">
            <Col>
              <Space size="middle">
                <Button icon={<ArrowLeftOutlined />} onClick={goBackToVersions}>返回</Button>
                <DatabaseOutlined style={{ fontSize: 24, color: statusColor(selectedVersion.status) }} />
                <div>
                  <Space>
                    <span style={{ fontSize: 18, fontWeight: 'bold' }}>{selectedServer.display_name} {selectedVersion.version}</span>
                    {statusTag(selectedVersion.status)}
                  </Space>
                  <div style={STYLES.versionInfo}>
                    <Space size="middle">
                      <span>服务: <strong>{selectedVersion.service_name}</strong></span>
                      <span>端口: <strong>{selectedVersion.port}</strong></span>
                    </Space>
                  </div>
                </div>
              </Space>
            </Col>
            <Col>
              <Space wrap>
                {selectedVersion.status === 'running' ? (
                  <>
                    <Button icon={<StopOutlined />} danger loading={operating === `stop-${selectedVersion.id}`}
                      onClick={() => handleStopVersion(selectedVersion)}>停止</Button>
                    <Button icon={<ReloadOutlined />} loading={operating === `restart-${selectedVersion.id}`}
                      onClick={() => handleRestartVersion(selectedVersion)}>重启</Button>
                  </>
                ) : (
                  <Button type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${selectedVersion.id}`}
                    onClick={() => handleStartVersion(selectedVersion)}>启动</Button>
                )}
                <Button icon={<CodeOutlined />} onClick={() => showConfig(selectedVersion)}>配置文件</Button>
                <Button icon={<FileTextOutlined />} onClick={() => showLogs(selectedVersion)}>服务日志</Button>
              </Space>
            </Col>
          </Row>
          {selectedVersion.status === 'running' && (
            <div style={{ marginTop: 12, padding: '8px 0', borderTop: '1px solid #f0f0f0' }}>
              <Space size="large">
                {selectedVersion.pid && selectedVersion.pid > 0 && <span>PID: <strong>{selectedVersion.pid}</strong></span>}
                {selectedVersion.memory_bytes && selectedVersion.memory_bytes > 0 && <span>内存: <strong>{(selectedVersion.memory_bytes / 1024 / 1024).toFixed(1)} MB</strong></span>}
                {selectedVersion.uptime && <span>运行时间: <strong>{selectedVersion.uptime}</strong></span>}
                {selectedVersion.connections !== undefined && <span>连接数: <strong>{selectedVersion.connections}</strong></span>}
                <span>配置: <Tag>{selectedVersion.config_file || 'N/A'}</Tag></span>
              </Space>
            </div>
          )}
        </Card>

        <Card>
          <Tabs items={[
            {
              key: 'databases',
              label: <span><DatabaseOutlined /> 数据库 ({versionDatabases.length})</span>,
              children: (
                <div>
                  <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <Button icon={<ReloadOutlined />} loading={dbsLoading} onClick={() => fetchDatabases(selectedServer.id)}>刷新</Button>
                    <Button type="primary" icon={<PlusOutlined />}
                      onClick={() => { dbForm.resetFields(); setDbModalVisible(true); }}
                      disabled={selectedVersion.status !== 'running'}>创建数据库</Button>
                  </div>
                  <Table columns={dbColumns} dataSource={versionDatabases} rowKey="id" loading={dbsLoading} size="small"
                    locale={{ emptyText: <Empty description="暂无数据库" /> }} />
                </div>
              ),
            },
            {
              key: 'users',
              label: <span><UserOutlined /> 用户 ({dbUsers.length})</span>,
              children: (
                <div>
                  <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <Button icon={<ReloadOutlined />} loading={usersLoading} onClick={() => fetchUsers(selectedServer.id)}>刷新</Button>
                    <Button type="primary" icon={<PlusOutlined />}
                      onClick={() => { userForm.resetFields(); setUserModalVisible(true); }}
                      disabled={selectedVersion.status !== 'running'}>创建用户</Button>
                  </div>
                  <Table columns={userColumns} dataSource={dbUsers} rowKey="id" loading={usersLoading} size="small"
                    locale={{ emptyText: <Empty description="暂无用户" /> }} />
                </div>
              ),
            },
            ...(selectedServer.name === 'mysql' || selectedServer.name === 'postgresql' || selectedServer.name === 'redis' ? [{
              key: 'config',
              label: <span><CodeOutlined /> 配置文件</span>,
              children: (
                <div>
                  {!dbConfig ? (
                    <div style={{ textAlign: 'center', padding: 40 }}>
                      <Button type="primary" icon={<CodeOutlined />} loading={dbConfigLoading}
                        onClick={() => fetchDBConfig()}>加载配置</Button>
                      <p style={{ color: '#999', marginTop: 12 }}>读取服务器上的配置文件</p>
                    </div>
                  ) : dbConfig.found === false ? (
                    <Empty description={`未找到 ${selectedServer.display_name} 配置文件`} />
                  ) : (
                    <div>
                      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Space>
                          <Tag>{dbConfig.config?.file_path}</Tag>
                          <span style={{ color: '#8c8c8c', fontSize: 12 }}>修改后需重启 {selectedServer.display_name} 生效</span>
                        </Space>
                        <Space>
                          <Button icon={<ReloadOutlined />} onClick={() => fetchDBConfig()}>重新加载</Button>
                          <Button type="primary" onClick={handleSaveDBConfig}>保存配置</Button>
                        </Space>
                      </div>
                      <Tabs
                        type="card"
                        items={(dbConfig.config?.sections || []).map((section: any) => ({
                          key: section.name,
                          label: section.name === 'main' ? selectedServer.display_name : `[${section.name}]`,
                          children: (
                            <div>
                              {(dbConfig.sections?.[section.name]?.meta || []).map((param: any) => (
                                <div key={param.key} style={{ marginBottom: 16 }}>
                                  <div style={{ marginBottom: 4 }}>
                                    <strong>{param.label}</strong>
                                    <span style={{ color: '#8c8c8c', marginLeft: 8, fontSize: 12 }}>{param.key}</span>
                                    {param.unit && <Tag style={{ marginLeft: 8 }}>{param.unit}</Tag>}
                                  </div>
                                  <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>{param.description}</div>
                                  {param.type === 'select' ? (
                                    <Select
                                      value={section.params?.[param.key] || param.default}
                                      onChange={(val) => updateDBParam(section.name, param.key, val)}
                                      style={{ width: 300 }}
                                    >
                                      {(param.options || []).map((opt: string) => (
                                        <Select.Option key={opt} value={opt}>{opt}</Select.Option>
                                      ))}
                                    </Select>
                                  ) : param.type === 'number' ? (
                                    <InputNumber
                                      value={Number(section.params?.[param.key]) || Number(param.default)}
                                      onChange={(val) => updateDBParam(section.name, param.key, String(val || ''))}
                                      style={{ width: 300 }}
                                    />
                                  ) : (
                                    <Input
                                      value={section.params?.[param.key] || ''}
                                      onChange={(e) => updateDBParam(section.name, param.key, e.target.value)}
                                      placeholder={param.default}
                                      style={{ width: 400 }}
                                    />
                                  )}
                                </div>
                              ))}
                              {/* Show extra params not in common list */}
                              {Object.entries(section.params || {}).filter(([key]) =>
                                !(dbConfig.sections?.[section.name]?.meta || []).some((m: any) => m.key === key)
                              ).map(([key, value]) => (
                                <div key={key} style={{ marginBottom: 16 }}>
                                  <div style={{ marginBottom: 4 }}>
                                    <Tag color="default">{key}</Tag>
                                  </div>
                                  <Input
                                    value={value as string}
                                    onChange={(e) => updateDBParam(section.name, key, e.target.value)}
                                    style={{ width: 400 }}
                                  />
                                </div>
                              ))}
                            </div>
                          ),
                        }))}
                      />
                    </div>
                  )}
                </div>
              ),
            }] : []),
          ]} />
        </Card>

        {/* Modals for Level 3 */}
        <Modal title="创建数据库" open={dbModalVisible} onCancel={() => setDbModalVisible(false)}
          onOk={handleCreateDB} okText="创建" cancelText="取消">
          <Form form={dbForm} layout="vertical">
            <Form.Item label="版本"><Input value={`${selectedServer.display_name} ${selectedVersion.version}`} disabled /></Form.Item>
            <Form.Item name="name" label="数据库名" rules={[{ required: true }]}><Input placeholder="如：my_app" /></Form.Item>
            <Form.Item name="charset" label="字符集" initialValue="utf8mb4">
              <Select><Select.Option value="utf8mb4">utf8mb4</Select.Option><Select.Option value="utf8">utf8</Select.Option></Select>
            </Form.Item>
            <Form.Item name="description" label="描述"><Input placeholder="可选" /></Form.Item>
          </Form>
        </Modal>
        <Modal title="创建用户" open={userModalVisible} onCancel={() => setUserModalVisible(false)}
          onOk={handleCreateUser} okText="创建" cancelText="取消">
          <Form form={userForm} layout="vertical">
            <Form.Item name="username" label="用户名" rules={[{ required: true }]}><Input placeholder="如：app_user" /></Form.Item>
            <Form.Item name="password" label="密码" rules={[{ required: true }, { min: 6 }]}><Input.Password /></Form.Item>
            <Form.Item name="host" label="主机" initialValue="localhost">
              <Select><Select.Option value="localhost">localhost</Select.Option><Select.Option value="%">任意主机（%）</Select.Option></Select>
            </Form.Item>
          </Form>
        </Modal>
        <Modal title={`授权 - ${grantUser?.username || ''}`} open={grantVisible} onCancel={() => setGrantVisible(false)}
          onOk={handleGrant} okText="授权" cancelText="取消">
          <Form form={grantForm} layout="vertical">
            <Form.Item name="db_version_id" hidden initialValue={selectedVersion.id}><Input /></Form.Item>
            <Form.Item name="database" label="数据库" rules={[{ required: true }]}>
              <Select>{databases.filter(d => d.db_version_id === selectedVersion.id).map(db => <Select.Option key={db.id} value={db.name}>{db.name}</Select.Option>)}</Select>
            </Form.Item>
            <Form.Item name="privileges" label="权限" rules={[{ required: true }]}>
              <Select mode="multiple">
                <Select.Option value="ALL PRIVILEGES">全部权限</Select.Option>
                <Select.Option value="SELECT">SELECT</Select.Option>
                <Select.Option value="INSERT">INSERT</Select.Option>
                <Select.Option value="UPDATE">UPDATE</Select.Option>
                <Select.Option value="DELETE">DELETE</Select.Option>
              </Select>
            </Form.Item>
          </Form>
        </Modal>
        <Modal title={<Space><CodeOutlined /> 配置文件</Space>} open={configVisible} onCancel={() => setConfigVisible(false)}
          footer={<Space>
            <Button onClick={() => setConfigVisible(false)}>关闭</Button>
            {selectedServer?.name && ['mysql', 'postgresql', 'redis'].includes(selectedServer.name) && (
              <Button type="primary" onClick={async () => {
                try {
                  if (selectedServer.name === 'mysql') {
                    await dbServerApi.saveMySQLConfig([{ name: 'custom', params: { raw: configContent } }]);
                  } else if (selectedServer.name === 'postgresql') {
                    await dbServerApi.savePostgreSQLConfig([{ name: 'custom', params: { raw: configContent } }]);
                  } else if (selectedServer.name === 'redis') {
                    await dbServerApi.saveRedisConfig([{ name: 'custom', params: { raw: configContent } }]);
                  }
                  message.success('配置已保存');
                } catch (error: any) {
                  message.error(error.message || '保存失败');
                }
              }}>保存</Button>
            )}
          </Space>} width={900}>
          <div style={{ marginBottom: 8, color: '#8c8c8c', fontSize: 12 }}>
            {selectedServer?.display_name || ''} 配置文件
          </div>
          <Input.TextArea value={configLoading ? 'Loading...' : configContent} onChange={(e) => setConfigContent(e.target.value)} rows={25} style={{ fontFamily: 'monospace', fontSize: 12 }} />
        </Modal>
        <Modal
          title={<Space><FileTextOutlined /><span>{selectedServer.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
          open={logVisible} onCancel={() => setLogVisible(false)}
          footer={<Row justify="space-between"><Col><Space><span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span><span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span></Space></Col><Col><Space><Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => setLogFollow(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button><Button size="small" onClick={() => setLogVisible(false)}>关闭</Button></Space></Col></Row>}
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
      </div>
    );
  }

  // ========== Level 2: Version list ==========
  if (selectedServer) {
    return (
      <div>
        <Card style={{ marginBottom: 16 }}>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={goBackToServers}>返回</Button>
            <DatabaseOutlined style={{ fontSize: 24, color: statusColor(selectedServer.status) }} />
            <span style={{ fontSize: 18, fontWeight: 'bold' }}>{selectedServer.display_name}</span>
            {statusTag(selectedServer.status)}
            {selectedServer.version && <Tag color="blue">已安装: {selectedServer.version}</Tag>}
          </Space>
        </Card>

        <Card title="已安装版本" extra={
          <Space>
            <Button icon={<ReloadOutlined />} loading={versionsLoading} onClick={() => fetchVersions(selectedServer.id)}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => { installVersionForm.resetFields(); setInstallVersionVisible(true); }}>安装版本</Button>
          </Space>
        }>
          <Row gutter={[16, 16]}>
            {versions.length === 0 && !versionsLoading && <Col span={24}><Empty description="暂未安装任何版本" /></Col>}
            {versions.map(v => (
              <Col xs={24} sm={12} lg={8} key={v.id}>
                <Card hoverable onClick={() => v.status === 'running' && enterVersion(v)}
                  style={{ borderColor: statusColor(v.status), opacity: v.status !== 'running' ? 0.7 : 1 }}>
                  <Card.Meta
                    title={<Space>{selectedServer.display_name} {v.version}{statusTag(v.status)}</Space>}
                    description={<div>
                      <p style={{ margin: '4px 0' }}>端口: <strong>{v.port}</strong></p>
                      <p style={{ margin: '4px 0' }}>服务: <Tag>{v.service_name}</Tag></p>
                    </div>} />
                  <div style={STYLES.cardActions}>
                    {v.status === 'running' ? (
                      <Button size="small" danger icon={<StopOutlined />} loading={operating === `stop-${v.id}`}
                        onClick={(e) => { e.stopPropagation(); handleStopVersion(v); }}>停止</Button>
                    ) : (
                      <Button size="small" type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${v.id}`}
                        onClick={(e) => { e.stopPropagation(); handleStartVersion(v); }}>启动</Button>
                    )}
                    <Button size="small" icon={<FileTextOutlined />} onClick={(e) => { e.stopPropagation(); showLogs(v); }}>日志</Button>
                    <Button size="small" icon={<EditOutlined />} onClick={(e) => {
                      e.stopPropagation();
                      if (v.status === 'running') {
                        message.warning('请先停止服务再修改端口');
                        return;
                      }
                      let newPort = v.port;
                      Modal.confirm({
                        title: `修改端口 - ${selectedServer.display_name} ${v.version}`,
                        content: (
                          <div>
                            <p>当前端口: {v.port}</p>
                            <InputNumber min={1} max={65535} defaultValue={v.port}
                              style={{ width: '100%' }}
                              onChange={(val) => { newPort = val as number || v.port; }} />
                          </div>
                        ),
                        onOk: async () => {
                          if (newPort > 0 && newPort !== v.port) {
                            try {
                              await dbServerApi.updateVersionPort(v.id, newPort);
                              message.success('端口已修改，启动服务后生效');
                              fetchVersions(selectedServer.id);
                            } catch (error: any) {
                              message.error(error.message || '修改失败');
                            }
                          }
                        },
                      });
                    }}>修改端口</Button>
                    <Popconfirm title="确定卸载？" onConfirm={(e) => { e?.stopPropagation(); handleUninstallVersion(v); }}>
                      <Button size="small" danger icon={<UndoOutlined />} onClick={(e) => e.stopPropagation()}>卸载</Button>
                    </Popconfirm>
                  </div>
                </Card>
              </Col>
            ))}
          </Row>
        </Card>

        {/* Modals for Level 2 */}
        <Modal title="安装数据库版本" open={installVersionVisible} onCancel={() => setInstallVersionVisible(false)}
          onOk={handleInstallVersion} okText="安装" cancelText="取消">
          <Form form={installVersionForm} layout="vertical">
            <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
              <Select placeholder="选择要安装的版本">
                {versionTemplates.map(t => (
                  <Select.Option key={t.version} value={t.version}>
                    <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
            <Form.Item name="port" label="端口（留空使用默认）"
              extra={portCheck && (
                portCheck.available
                  ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                  : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
              )}>
              <InputNumber min={1} max={65535} style={{ width: '100%' }}
                onChange={(val) => val && checkPort(val as number)} />
            </Form.Item>
          </Form>
        </Modal>

        {/* Service Logs Modal */}
        <Modal
          title={<Space><FileTextOutlined /><span>{selectedServer.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
          open={logVisible} onCancel={() => setLogVisible(false)}
          footer={
            <Row justify="space-between" align="middle">
              <Col><Space size="middle">
                <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
                <span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span>
              </Space></Col>
              <Col><Space size="small">
                <Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => setLogFollow(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button>
                <Button size="small" onClick={() => setLogVisible(false)}>关闭</Button>
              </Space></Col>
            </Row>
          }
          width="90vw" style={{ maxWidth: 960 }}>
          <div ref={logRef} style={{
            background: '#fafafa', border: '1px solid #e8e8e8',
            fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
            fontSize: 13, lineHeight: 1.8, padding: '8px 0', borderRadius: 6,
            maxHeight: '60vh', overflowY: 'auto', overflowX: 'auto',
          }}>
            {logContent.split('\n').map((line, i) => (
              <div key={i} style={STYLES.logLine}>
                <span style={STYLES.logLineNumber}>{i + 1}</span>
                <span style={STYLES.logLineText}>{line || ' '}</span>
              </div>
            ))}
          </div>
        </Modal>
      </div>
    );
  }

  // ========== Level 1: DB Server cards ==========
  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchServers}>刷新</Button>
      </div>
      <Row gutter={[16, 16]}>
        {servers.map(server => (
          <Col xs={24} sm={12} lg={8} key={server.id}>
            <Card hoverable onClick={() => enterServer(server)} style={{ borderColor: statusColor(server.status) }}>
              <Card.Meta
                avatar={<DatabaseOutlined style={{ fontSize: 32, color: statusColor(server.status) }} />}
                title={<Space>{server.display_name}{statusTag(server.status)}</Space>}
                description={<div>
                  <p style={{ margin: '8px 0', color: '#666' }}>{server.description}</p>
                  {server.version && <Tag color="blue">已安装: {server.version}</Tag>}
                  <Tag>默认端口: {server.default_port}</Tag>
                </div>} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* Install Version Modal */}
      <Modal title="安装数据库版本" open={installVersionVisible} onCancel={() => setInstallVersionVisible(false)}
        onOk={handleInstallVersion} okText="安装" cancelText="取消">
        <Form form={installVersionForm} layout="vertical">
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select placeholder="选择要安装的版本">
              {versionTemplates.map(t => (
                <Select.Option key={t.version} value={t.version}>
                  <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="port" label="端口（留空使用默认）">
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
