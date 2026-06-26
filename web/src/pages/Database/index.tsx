import { useState, useEffect, useRef } from 'react';
import { Form, message, Modal, Tag } from 'antd';
import { dbServerApi } from '../../services/api';
import type { DBServer, Database, DBUser, DBVersion } from '../../types';
import { usePortCheck } from '../../hooks/usePortCheck';
import { getServiceStatusColor } from '../../utils/status';
import DEFAULT_CONFIG_TEMPLATES from './constants';
import ServerList from './ServerList';
import VersionList from './VersionList';
import DatabaseList from './DatabaseList';
import TableExplorer from './TableExplorer';
import type { VersionTemplate, TableData, TableInfo, SqlResult } from './types';

export default function DatabasePage() {
  // ===== Navigation state =====
  const [servers, setServers] = useState<DBServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedServer, setSelectedServer] = useState<DBServer | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<DBVersion | null>(null);
  const [selectedDatabase, setSelectedDatabase] = useState<Database | null>(null);
  const [operating, setOperating] = useState('');

  // ===== Version state =====
  const [versions, setVersions] = useState<DBVersion[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [versionTemplates, setVersionTemplates] = useState<VersionTemplate[]>([]);
  const [installVersionVisible, setInstallVersionVisible] = useState(false);
  const [installVersionForm] = Form.useForm();

  // Port check
  const { result: portCheck, checking: _portChecking, checkPort, clearResult: _clearPortCheck } = usePortCheck();

  // ===== Database state =====
  const [databases, setDatabases] = useState<Database[]>([]);
  const [dbsLoading, setDbsLoading] = useState(false);
  const [dbModalVisible, setDbModalVisible] = useState(false);
  const [dbForm] = Form.useForm();

  // ===== User state =====
  const [dbUsers, setDBUsers] = useState<DBUser[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [userModalVisible, setUserModalVisible] = useState(false);
  const [userForm] = Form.useForm();

  // ===== Grant modal =====
  const [grantVisible, setGrantVisible] = useState(false);
  const [grantUser, setGrantUser] = useState<DBUser | null>(null);
  const [grantForm] = Form.useForm();

  // ===== Config editor =====
  const [_configVisible, setConfigVisible] = useState(false);
  const [_configContent, setConfigContent] = useState('');
  const [_configLoading, setConfigLoading] = useState(false);

  // ===== Service logs =====
  const [logVisible, setLogVisible] = useState(false);
  const [logVersion, setLogVersion] = useState<DBVersion | null>(null);
  const [logContent, setLogContent] = useState('');
  const [logLoading, setLogLoading] = useState(false);
  const [logFollow, setLogFollow] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);

  // ===== Table explorer state =====
  const [tableList, setTableList] = useState<string[]>([]);
  const [tableLoading, setTableLoading] = useState(false);
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [tableInfo, setTableInfo] = useState<TableInfo | null>(null);
  const [tableDataLoading, setTableDataLoading] = useState(false);
  const [selectedTable, setSelectedTable] = useState('');
  const [tablePage, setTablePage] = useState(1);
  const [sqlInput, setSqlInput] = useState('');
  const [sqlResult, setSqlResult] = useState<SqlResult | null>(null);
  const [sqlLoading, setSqlLoading] = useState(false);

  // ===== Backup state =====
  const [backups, setBackups] = useState<any[]>([]);
  const [backupsLoading, setBackupsLoading] = useState(false);
  const [backupCreating, setBackupCreating] = useState(false);

  // ===== Create table state =====
  const [createTableVisible, setCreateTableVisible] = useState(false);
  const [createTableLoading, setCreateTableLoading] = useState(false);
  const [createForm] = Form.useForm();

  // ===== DB config editor (structured) =====
  const [dbConfig, setDBConfig] = useState<any>(null);
  const [dbConfigLoading, setDBConfigLoading] = useState(false);

  // ===== Record modal =====
  const [recordModalVisible, setRecordModalVisible] = useState(false);
  const [editingRecord, setEditingRecord] = useState<any>(null);
  const [recordForm] = Form.useForm();
  const [recordSaving, setRecordSaving] = useState(false);

  // ===== Fetch functions =====
  const fetchServers = async () => {
    try {
      const res = await dbServerApi.list();
      setServers(res.data?.data || []);
    } catch (error) { console.error('Failed to fetch servers:', error); message.error('加载服务器列表失败'); } finally { setLoading(false); }
  };

  // ===== Effects =====
  useEffect(() => { fetchServers(); }, []);

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
  }, [logVisible, logVersion]);

  useEffect(() => {
    if (logFollow && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logContent, logFollow]);

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
        setTableData({ headers: data.headers || [], rows: data.rows || [], total: data.total || 0 });
      } else {
        setTableData({ headers: [], rows: [], total: 0 });
      }
      setTablePage(page);
      const describeData = describeRes.data?.data;
      const columns = describeData?.columns || [];
      const primaryKey = describeData?.primary_key || columns[0]?.name || 'id';
      setTableInfo({
        primaryKey,
        columns: columns.map((c: any) => ({ name: c.name, type: c.type, key: c.is_primary_key ? 'PRI' : '' })),
      });
    } catch (error) {
      console.error('Failed to fetch table data:', error);
      setTableData({ headers: [], rows: [], total: 0 });
      setTableInfo(null);
    } finally { setTableDataLoading(false); }
  };

  const fetchBackups = async (dbId: number) => {
    setBackupsLoading(true);
    try {
      const res = await dbServerApi.listBackups(dbId);
      setBackups(res.data?.data || []);
    } catch (error) {
      console.error('Failed to fetch backups:', error);
      setBackups([]);
    } finally { setBackupsLoading(false); }
  };

  const fetchDBConfig = async (serverName?: string) => {
    const name = serverName || selectedServer?.name;
    if (!name) return;
    setDBConfigLoading(true);
    try {
      let res;
      if (name === 'mysql') res = await dbServerApi.getMySQLConfig();
      else if (name === 'postgresql') res = await dbServerApi.getPostgreSQLConfig();
      else if (name === 'redis') res = await dbServerApi.getRedisConfig();
      else { setDBConfig(null); return; }
      setDBConfig(res.data?.data || null);
    } catch (error) {
      console.error('Failed to load config:', error);
      setDBConfig(null);
    } finally { setDBConfigLoading(false); }
  };

  // ===== Navigation handlers =====
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
    await Promise.all([fetchTables(db.id), fetchBackups(db.id)]);
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

  // ===== Version actions =====
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
    setOperating(`uninstall-${v.id}`);
    try {
      await dbServerApi.uninstallVersion(v.id);
      message.success('已卸载');
      await Promise.all([fetchVersions(server.id), refreshServer(server.id)]);
    } catch (error: any) { message.error(error.message || '卸载失败'); }
    finally { setOperating(''); }
  };

  // ===== Database CRUD =====
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

  // ===== User CRUD =====
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

  // ===== Config handlers =====
  const handleSaveDBConfig = async () => {
    if (!dbConfig?.config?.sections || !selectedServer?.name) return;
    try {
      if (selectedServer.name === 'mysql') await dbServerApi.saveMySQLConfig(dbConfig.config.sections);
      else if (selectedServer.name === 'postgresql') await dbServerApi.savePostgreSQLConfig(dbConfig.config.sections);
      else if (selectedServer.name === 'redis') await dbServerApi.saveRedisConfig(dbConfig.config.sections);
      message.success('配置已保存（已自动备份原文件）');
      fetchDBConfig();
    } catch (error: any) { message.error(error.message || '保存失败'); }
  };

  const updateDBParam = (section: string, key: string, value: string) => {
    setDBConfig((prev: any) => {
      if (!prev?.config?.sections) return prev;
      const newSections = prev.config.sections.map((s: any) => {
        if (s.name === section) return { ...s, params: { ...s.params, [key]: value } };
        return s;
      });
      return { ...prev, config: { ...prev.config, sections: newSections } };
    });
  };

  // ===== Log/Config show functions =====
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

  const showConfig = async (_v: DBVersion) => {
    setConfigVisible(true);
    setConfigLoading(true);
    try {
      const serverName = selectedServer?.name;
      let res;
      if (serverName === 'mysql') res = await dbServerApi.getMySQLConfig();
      else if (serverName === 'postgresql') res = await dbServerApi.getPostgreSQLConfig();
      else if (serverName === 'redis') res = await dbServerApi.getRedisConfig();
      else { setConfigContent('# Unsupported database type'); return; }

      const data = res?.data?.data;
      const config = data?.config;
      if (data?.found && config && config.sections.length > 0) {
        let content = `# ${selectedServer?.display_name} Config: ${config.file_path}\n\n`;
        for (const section of config.sections) {
          if (section.name !== 'main') content += `[${section.name}]\n`;
          for (const [key, val] of Object.entries(section.params || {})) {
            content += serverName === 'redis' ? `${key} ${val}\n` : `${key} = ${val}\n`;
          }
          content += '\n';
        }
        setConfigContent(content);
      } else {
        const dbType = serverName || 'mysql';
        const template = DEFAULT_CONFIG_TEMPLATES[dbType] || DEFAULT_CONFIG_TEMPLATES['mysql'];
        setConfigContent(`# ${selectedServer?.display_name} 默认配置模板\n# 保存后将创建配置文件\n\n${template}`);
      }
    } catch (error: any) { setConfigContent('# Error: ' + error.message); }
    finally { setConfigLoading(false); }
  };

  // ===== Table/Record handlers =====
  const handleExecuteSQL = async () => {
    if (!selectedDatabase || !sqlInput.trim()) return;

    // Confirm destructive operations before execution
    const sqlUpper = sqlInput.trim().toUpperCase();
    const isDestructive = /^(DROP|DELETE|ALTER|TRUNCATE)\b/.test(sqlUpper);
    if (isDestructive) {
      const confirmed = await new Promise<boolean>((resolve) => {
        Modal.confirm({
          title: '确认执行危险 SQL',
          content: `即将执行的 SQL 可能会造成数据丢失，确定要执行吗？\n\n${sqlInput.trim().substring(0, 200)}`,
          okText: '确认执行',
          okType: 'danger',
          cancelText: '取消',
          onOk: () => resolve(true),
          onCancel: () => resolve(false),
        });
      });
      if (!confirmed) return;
    }

    setSqlLoading(true);
    try {
      const res = await dbServerApi.executeSQL(selectedDatabase.id, sqlInput);
      setSqlResult(res.data?.data || null);
      if (selectedTable && /^(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE)/i.test(sqlInput.trim())) {
        fetchTableData(selectedDatabase.id, selectedTable);
      }
    } catch (error: any) { setSqlResult({ success: false, error: error.message }); }
    finally { setSqlLoading(false); }
  };

  const handleCreateBackup = async () => {
    if (!selectedDatabase) return;
    setBackupCreating(true);
    try {
      await dbServerApi.createBackup(selectedDatabase.id);
      message.success('备份已开始，请稍候...');
      setTimeout(() => fetchBackups(selectedDatabase.id), 2000);
    } catch (error: any) { message.error(error.message || '备份失败'); }
    finally { setBackupCreating(false); }
  };

  const handleDownloadBackup = async (backupId: number) => {
    try {
      const res = await dbServerApi.downloadBackup(backupId);
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const a = document.createElement('a');
      a.href = url;
      a.download = `backup_${backupId}.sql`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error: any) { message.error(error.message || '下载失败'); }
  };

  const handleRestoreBackup = async (backupId: number) => {
    try {
      await dbServerApi.restoreBackup(backupId);
      message.success('恢复成功');
      if (selectedDatabase) fetchTables(selectedDatabase.id);
    } catch (error: any) { message.error(error.message || '恢复失败'); }
  };

  const handleDeleteBackup = async (backupId: number) => {
    try {
      await dbServerApi.deleteBackup(backupId);
      message.success('备份已删除');
      if (selectedDatabase) fetchBackups(selectedDatabase.id);
    } catch (error: any) { message.error(error.message || '删除失败'); }
  };

  const handleCreateTable = async () => {
    if (!selectedDatabase) return;
    setCreateTableLoading(true);
    try {
      const values = await createForm.validateFields();
      await dbServerApi.createTable(selectedDatabase.id, { name: values.tableName, columns: values.columns || [] });
      message.success('表创建成功');
      setCreateTableVisible(false);
      createForm.resetFields();
      fetchTables(selectedDatabase.id);
    } catch (error: any) { if (error.message) message.error(error.message); }
    finally { setCreateTableLoading(false); }
  };

  const handleDropTable = async (tableName: string) => {
    if (!selectedDatabase) return;
    try {
      await dbServerApi.dropTable(selectedDatabase.id, tableName);
      message.success('表已删除');
      if (selectedTable === tableName) { setSelectedTable(''); setTableData(null); }
      fetchTables(selectedDatabase.id);
    } catch (error: any) { message.error(error.message || '删除表失败'); }
  };

  const openInsertModal = () => {
    setEditingRecord(null);
    recordForm.resetFields();
    setRecordModalVisible(true);
  };

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
        const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
        const pkVal = editingRecord[pk];
        const res = await dbServerApi.updateRecord(selectedDatabase.id, selectedTable, values, pk, pkVal);
        if (res.data?.data?.success) message.success('更新成功');
        else message.error(res.data?.data?.error || '更新失败');
      } else {
        const res = await dbServerApi.insertRecord(selectedDatabase.id, selectedTable, values);
        if (res.data?.data?.success) message.success('插入成功');
        else message.error(res.data?.data?.error || '插入失败');
      }
      setRecordModalVisible(false);
      fetchTableData(selectedDatabase.id, selectedTable, tablePage);
    } catch (error: any) { if (error.message) message.error(error.message); }
    finally { setRecordSaving(false); }
  };

  const handleDeleteRecord = async (record: any) => {
    if (!selectedDatabase || !selectedTable) return;
    try {
      const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
      const pkVal = record[pk];
      const res = await dbServerApi.deleteRecord(selectedDatabase.id, selectedTable, pk, pkVal);
      if (res.data?.data?.success) {
        message.success('删除成功');
        fetchTableData(selectedDatabase.id, selectedTable, tablePage);
      } else { message.error(res.data?.data?.error || '删除失败'); }
    } catch (error: any) { message.error(error.message || '删除失败'); }
  };

  // ===== Status helpers (shared) =====
  const statusColor = (status: string) => {
    const colorName = getServiceStatusColor(status);
    const colorMap: Record<string, string> = {
      success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
    };
    return colorMap[colorName] || '#999';
  };

  const statusTag = (status: string) => {
    const labels: Record<string, string> = {
      running: '运行中', stopped: '已停止', partial: '部分运行', not_installed: '未安装',
    };
    return <Tag color={getServiceStatusColor(status)}>{labels[status] || status}</Tag>;
  };

  // ===== Render =====

  // Level 4: Database detail (tables + SQL + backups)
  if (selectedDatabase && selectedVersion && selectedServer) {
    return (
      <TableExplorer
        server={selectedServer}
        version={selectedVersion}
        database={selectedDatabase}
        onBack={goBackToVersionDetail}
        tableList={tableList}
        tableLoading={tableLoading}
        selectedTable={selectedTable}
        tableData={tableData}
        tableDataLoading={tableDataLoading}
        tablePage={tablePage}
        tableInfo={tableInfo}
        onSelectTable={(t) => { setSelectedTable(t); if (selectedDatabase) fetchTableData(selectedDatabase.id, t); }}
        onFetchTables={() => selectedDatabase && fetchTables(selectedDatabase.id)}
        onFetchTableData={(t, p) => selectedDatabase && fetchTableData(selectedDatabase.id, t, p)}
        createTableVisible={createTableVisible}
        createTableLoading={createTableLoading}
        createForm={createForm}
        onCreateTableVisibleChange={setCreateTableVisible}
        onCreateTable={handleCreateTable}
        onDropTable={handleDropTable}
        recordModalVisible={recordModalVisible}
        editingRecord={editingRecord}
        recordForm={recordForm}
        recordSaving={recordSaving}
        onRecordModalVisibleChange={setRecordModalVisible}
        onOpenInsertModal={openInsertModal}
        onOpenEditModal={openEditModal}
        onSaveRecord={handleSaveRecord}
        onDeleteRecord={handleDeleteRecord}
        sqlInput={sqlInput}
        sqlResult={sqlResult}
        sqlLoading={sqlLoading}
        onSqlInputChange={setSqlInput}
        onExecuteSQL={handleExecuteSQL}
        backups={backups}
        backupsLoading={backupsLoading}
        backupCreating={backupCreating}
        onCreateBackup={handleCreateBackup}
        onDownloadBackup={handleDownloadBackup}
        onRestoreBackup={handleRestoreBackup}
        onDeleteBackup={handleDeleteBackup}
        logVisible={logVisible}
        logVersion={logVersion}
        logContent={logContent}
        logLoading={logLoading}
        logFollow={logFollow}
        logRef={logRef}
        onLogVisibleChange={setLogVisible}
        onLogFollowChange={setLogFollow}
      />
    );
  }

  // Level 3: Version detail (databases + users + config)
  if (selectedVersion && selectedServer) {
    return (
      <DatabaseList
        server={selectedServer}
        version={selectedVersion}
        databases={databases}
        dbsLoading={dbsLoading}
        dbUsers={dbUsers}
        usersLoading={usersLoading}
        operating={operating}
        onBack={goBackToVersions}
        onEnterDatabase={enterDatabase}
        onRefreshDatabases={() => fetchDatabases(selectedServer.id)}
        onRefreshUsers={() => fetchUsers(selectedServer.id)}
        onDeleteDB={handleDeleteDB}
        onDeleteUser={handleDeleteUser}
        onStartVersion={handleStartVersion}
        onStopVersion={handleStopVersion}
        onRestartVersion={handleRestartVersion}
        dbModalVisible={dbModalVisible}
        onDbModalVisibleChange={setDbModalVisible}
        dbForm={dbForm}
        onCreateDB={handleCreateDB}
        userModalVisible={userModalVisible}
        onUserModalVisibleChange={setUserModalVisible}
        userForm={userForm}
        onCreateUser={handleCreateUser}
        grantVisible={grantVisible}
        grantUser={grantUser}
        grantForm={grantForm}
        onGrantVisibleChange={setGrantVisible}
        onGrant={handleGrant}
        onOpenGrant={(user) => { setGrantUser(user); grantForm.resetFields(); setGrantVisible(true); }}
        dbConfig={dbConfig}
        dbConfigLoading={dbConfigLoading}
        onFetchDBConfig={() => fetchDBConfig()}
        onSaveDBConfig={handleSaveDBConfig}
        onUpdateDBParam={updateDBParam}
        logVisible={logVisible}
        logVersion={logVersion}
        logContent={logContent}
        logLoading={logLoading}
        logFollow={logFollow}
        logRef={logRef}
        onLogVisibleChange={setLogVisible}
        onLogFollowChange={setLogFollow}
        showConfig={showConfig}
        showLogs={showLogs}
      />
    );
  }

  // Level 2: Version list
  if (selectedServer) {
    return (
      <VersionList
        server={selectedServer}
        versions={versions}
        versionsLoading={versionsLoading}
        operating={operating}
        onBack={goBackToServers}
        onEnterVersion={enterVersion}
        onRefreshVersions={() => fetchVersions(selectedServer.id)}
        onStartVersion={handleStartVersion}
        onStopVersion={handleStopVersion}
        onRestartVersion={handleRestartVersion}
        onUninstallVersion={handleUninstallVersion}
        installVersionVisible={installVersionVisible}
        onInstallVersionVisibleChange={setInstallVersionVisible}
        versionTemplates={versionTemplates}
        installVersionForm={installVersionForm}
        onInstallVersion={handleInstallVersion}
        portCheck={portCheck}
        onCheckPort={checkPort}
        logVisible={logVisible}
        logVersion={logVersion}
        logContent={logContent}
        logLoading={logLoading}
        logFollow={logFollow}
        logRef={logRef}
        onLogVisibleChange={setLogVisible}
        onLogFollowChange={setLogFollow}
        onShowLogs={showLogs}
        statusColor={statusColor}
        statusTag={statusTag}
      />
    );
  }

  // Level 1: Server list
  return (
    <ServerList
      servers={servers}
      loading={loading}
      onEnterServer={enterServer}
      onRefresh={() => { setLoading(true); fetchServers(); }}
      installVersionVisible={installVersionVisible}
      onInstallVersionVisibleChange={setInstallVersionVisible}
      versionTemplates={versionTemplates}
      installVersionForm={installVersionForm}
      onInstallVersion={handleInstallVersion}
      portCheck={portCheck}
      onCheckPort={checkPort}
    />
  );
}
