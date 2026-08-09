import { useState, useEffect, useRef } from 'react';
import { Form, message, Modal, Tag, Tabs } from 'antd';
import { dbServerApi } from '../../services/api';
import type { Database, DBUser, DBInstance } from '../../types';
import { usePortCheck } from '../../hooks/usePortCheck';
import { getServiceStatusColor } from '../../utils/status';
import VersionList from './VersionList';
import DatabaseList from './DatabaseList';
import TableExplorer from './TableExplorer';
import type { TableData, TableInfo, SqlResult } from './types';
import { ENGINE_TABS } from './types';

export default function DatabasePage() {
  // ===== Navigation state =====
  // The engine is a static front-end Tab (MySQL/PostgreSQL/Redis); the instance
  // list, instance detail and database explorer render below it.
  const [activeDbType, setActiveDbType] = useState('mysql');
  const [selectedVersion, setSelectedVersion] = useState<DBInstance | null>(null);
  const [selectedDatabase, setSelectedDatabase] = useState<Database | null>(null);
  const [operating, setOperating] = useState('');

  // ===== Version state =====
  const [versions, setVersions] = useState<DBInstance[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
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
  const [logVersion, setLogVersion] = useState<DBInstance | null>(null);
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
  // activeDbType only ever holds one of ENGINE_TABS' db_type keys (initial value
  // and Tabs items are both hard-coded), so find() always matches.
  const activeEngine = ENGINE_TABS.find(e => e.db_type === activeDbType)!;

  const fetchInstances = async (dbtype: string) => {
    setVersionsLoading(true);
    try { const res = await dbServerApi.listInstances(dbtype); setVersions(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch instances:', error); message.error('加载实例列表失败'); } finally { setVersionsLoading(false); }
  };

  // ===== Effects =====
  useEffect(() => { fetchInstances('mysql'); }, []);

  useEffect(() => {
    if (!logVisible || !logVersion) return;
    const refresh = async () => {
      try {
        const res = await dbServerApi.getInstanceLogs(logVersion.id, 200);
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

  const fetchDatabases = async (instanceId: number) => {
    setDbsLoading(true);
    try { const res = await dbServerApi.listDatabases(instanceId); setDatabases(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch databases:', error); message.error('加载数据库列表失败'); } finally { setDbsLoading(false); }
  };

  const fetchUsers = async (instanceId: number) => {
    setUsersLoading(true);
    try { const res = await dbServerApi.listUsers(instanceId); setDBUsers(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch users:', error); message.error('加载用户列表失败'); } finally { setUsersLoading(false); }
  };

  const fetchTables = async (instanceId: number, dbName: string) => {
    setTableLoading(true);
    try {
      const res = await dbServerApi.listTables(instanceId, dbName);
      const data = res.data?.data;
      setTableList(Array.isArray(data) ? data.map((t: any) => t.name) : []);
    } catch (error) {
      console.error('Failed to fetch tables:', error);
      setTableList([]);
    } finally { setTableLoading(false); }
  };

  const fetchTableData = async (instanceId: number, dbName: string, table: string, page: number = 1) => {
    setTableDataLoading(true);
    try {
      const [queryRes, describeRes] = await Promise.all([
        dbServerApi.queryTable(instanceId, dbName, table, page, 50),
        dbServerApi.describeTable(instanceId, dbName, table),
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

  const fetchBackups = async (instanceId: number, dbName: string) => {
    setBackupsLoading(true);
    try {
      const res = await dbServerApi.listBackups(instanceId, dbName);
      setBackups(res.data?.data || []);
    } catch (error) {
      console.error('Failed to fetch backups:', error);
      setBackups([]);
    } finally { setBackupsLoading(false); }
  };

  const fetchDBConfig = async (serverName?: string) => {
    void serverName;
    if (!selectedVersion) return;
    setDBConfigLoading(true);
    try {
      const res = await dbServerApi.getInstanceConfig(selectedVersion.id);
      const data = res.data?.data;
      const section = { name: 'main', params: { content: data?.content || '' } };
      setDBConfig({ found: true, config: { file_path: data?.file_path, sections: [section] }, sections: { main: { params: section.params, meta: [] } } });
    } catch (error) {
      console.error('Failed to load config:', error);
      setDBConfig(null);
    } finally { setDBConfigLoading(false); }
  };

  // ===== Navigation handlers =====
  // ===== Navigation handlers =====
  // Switching engine Tab resets to the instance list and reloads it.
  const changeEngine = (dbtype: string) => {
    setActiveDbType(dbtype);
    setSelectedVersion(null);
    setSelectedDatabase(null);
    setDatabases([]); setDBUsers([]);
    fetchInstances(dbtype);
  };

  const enterVersion = async (version: DBInstance) => {
    setSelectedVersion(version);
    setSelectedDatabase(null);
    // Scope by instance id — databases/users belong to one instance, never the
    // engine. (Previously the engine id was used here, leaking every instance's
    // databases/users into the same view.)
    await Promise.all([fetchDatabases(version.id), fetchUsers(version.id)]);
  };

  const enterDatabase = async (db: Database) => {
    const instance = selectedVersion;
    if (!instance) return;
    setSelectedDatabase(db);
    setSelectedTable('');
    setTableData(null);
    setSqlResult(null);
    await Promise.all([fetchTables(instance.id, db.name), fetchBackups(instance.id, db.name)]);
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
    const server = activeEngine;
    if (!server) return;
    try {
      const values = await installVersionForm.validateFields();
      // Port left empty → engine default. image comes from the version Select
      // onChange (preset) or the Docker Hub picker; fall back to the preset
      // template in case the form wasn't populated.
      const tpl = server.templates.find(t => t.version === values.version);
      await dbServerApi.createInstance(server.db_type, {
        ...values,
        image: values.image || tpl?.image,
        port: values.port || server.default_port,
      });
      message.success('版本安装成功');
      setInstallVersionVisible(false);
      fetchInstances(server.db_type);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
  };

  const handleStartVersion = async (v: DBInstance) => {
    const server = activeEngine;
    if (!server) return;
    setOperating(`start-${v.id}`);
    try {
      await dbServerApi.startInstance(v.id);
      message.success('已启动');
      fetchInstances(server.db_type);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '启动失败')); }
    finally { setOperating(''); }
  };

  const handleStopVersion = async (v: DBInstance) => {
    const server = activeEngine;
    if (!server) return;
    setOperating(`stop-${v.id}`);
    try {
      await dbServerApi.stopInstance(v.id);
      message.success('已停止');
      fetchInstances(server.db_type);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '停止失败')); }
    finally { setOperating(''); }
  };

  const handleRestartVersion = async (v: DBInstance) => {
    const server = activeEngine;
    if (!server) return;
    setOperating(`restart-${v.id}`);
    try {
      await dbServerApi.restartInstance(v.id);
      message.success('已重启');
      fetchInstances(server.db_type);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '重启失败')); }
    finally { setOperating(''); }
  };

  const handleUninstallVersion = async (v: DBInstance) => {
    const server = activeEngine;
    if (!server) return;
    setOperating(`uninstall-${v.id}`);
    try {
      await dbServerApi.uninstallInstance(v.id);
      message.success('已卸载');
      fetchInstances(server.db_type);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '卸载失败')); }
    finally { setOperating(''); }
  };

  // ===== Database CRUD =====
  const handleCreateDB = async () => {
    const version = selectedVersion;
    if (!version) return;
    try {
      const values = await dbForm.validateFields();
      await dbServerApi.createDatabase(version.id, values);
      message.success('数据库创建成功');
      setDbModalVisible(false);
      fetchDatabases(version.id);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
  };

  const handleDeleteDB = async (dbName: string) => {
    const version = selectedVersion;
    if (!version) return;
    try {
      await dbServerApi.deleteDatabase(version.id, dbName);
      message.success('数据库已删除');
      fetchDatabases(version.id);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
  };

  // ===== User CRUD =====
  const handleCreateUser = async () => {
    const version = selectedVersion;
    if (!version) return;
    try {
      const values = await userForm.validateFields();
      await dbServerApi.createUser(version.id, values);
      message.success('用户创建成功');
      setUserModalVisible(false);
      fetchUsers(version.id);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
  };

  const handleDeleteUser = async (user: DBUser) => {
    const version = selectedVersion;
    if (!version) return;
    try {
      await dbServerApi.deleteUser(version.id, user.username, user.host || '%');
      message.success('用户已删除');
      fetchUsers(version.id);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
  };

  const handleGrant = async () => {
    const version = selectedVersion;
    const user = grantUser;
    if (!version || !user) return;
    try {
      const values = await grantForm.validateFields();
      const payload = {
        ...values,
        privileges: Array.isArray(values.privileges) ? values.privileges.join(', ') : values.privileges,
      };
      await dbServerApi.grantPrivileges(version.id, user.username, payload, user.host || '%');
      message.success('授权成功');
      setGrantVisible(false);
      fetchUsers(version.id);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
  };

  // ===== Config handlers =====
  const handleSaveDBConfig = async () => {
    if (!dbConfig?.config?.sections || !selectedVersion) return;
    try {
      const content = dbConfig.config.sections[0]?.params?.content || '';
      await dbServerApi.saveInstanceConfig(selectedVersion.id, content);
      message.success('实例配置已保存，重启后生效');
      fetchDBConfig();
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '保存失败')); }
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
  const showLogs = async (v: DBInstance) => {
    setLogVersion(v);
    setLogVisible(true);
    setLogLoading(true);
    try {
      const res = await dbServerApi.getInstanceLogs(v.id, 200);
      setLogContent(res.data?.data?.logs || '(empty)');
    } catch (error: unknown) { setLogContent('Failed: ' + (error instanceof Error ? error.message : String(error))); }
    finally { setLogLoading(false); }
  };

  const showConfig = async (v: DBInstance) => {
    setConfigVisible(true);
    setConfigLoading(true);
    try {
      const res = await dbServerApi.getInstanceConfig(v.id);
      const data = res?.data?.data;
      setConfigContent(`# ${activeEngine.display_name} Config: ${data?.file_path || ''}\n\n${data?.content || ''}`);
    } catch (error: unknown) { setConfigContent('# Error: ' + (error instanceof Error ? error.message : String(error))); }
    finally { setConfigLoading(false); }
  };

  // ===== Table/Record handlers =====
  const handleExecuteSQL = async () => {
    const version = selectedVersion;
    if (!selectedDatabase || !version || !sqlInput.trim()) return;

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
      const res = await dbServerApi.executeSQL(version.id, selectedDatabase.name, sqlInput);
      setSqlResult(res.data?.data || null);
      if (selectedTable && /^(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE)/i.test(sqlInput.trim())) {
        fetchTableData(version.id, selectedDatabase.name, selectedTable);
      }
    } catch (error: unknown) { setSqlResult({ success: false, error: (error instanceof Error ? error.message : String(error)) }); }
    finally { setSqlLoading(false); }
  };

  const handleCreateBackup = async () => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    setBackupCreating(true);
    try {
      await dbServerApi.createBackup(version.id, selectedDatabase.name);
      message.success('备份已开始，请稍候...');
      setTimeout(() => fetchBackups(version.id, selectedDatabase.name), 2000);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '备份失败')); }
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
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '下载失败')); }
  };

  const handleRestoreBackup = async (backupId: number) => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    try {
      await dbServerApi.restoreBackup(backupId);
      message.success('恢复成功');
      if (selectedDatabase) fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '恢复失败')); }
  };

  const handleDeleteBackup = async (backupId: number) => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    try {
      await dbServerApi.deleteBackup(backupId);
      message.success('备份已删除');
      if (selectedDatabase) fetchBackups(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
  };

  const handleCreateTable = async () => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    setCreateTableLoading(true);
    try {
      const values = await createForm.validateFields();
      await dbServerApi.createTable(version.id, selectedDatabase.name, { name: values.tableName, columns: values.columns || [] });
      message.success('表创建成功');
      setCreateTableVisible(false);
      createForm.resetFields();
      fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
    finally { setCreateTableLoading(false); }
  };

  const handleDropTable = async (tableName: string) => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    try {
      await dbServerApi.dropTable(version.id, selectedDatabase.name, tableName);
      message.success('表已删除');
      if (selectedTable === tableName) { setSelectedTable(''); setTableData(null); }
      fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除表失败')); }
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
    const version = selectedVersion;
    if (!selectedDatabase || !version || !selectedTable) return;
    setRecordSaving(true);
    try {
      const values = await recordForm.validateFields();
      if (editingRecord) {
        const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
        const pkVal = editingRecord[pk];
        const res = await dbServerApi.updateRecord(version.id, selectedDatabase.name, selectedTable, values, pk, pkVal);
        if (res.data?.data?.success) message.success('更新成功');
        else message.error(res.data?.data?.error || '更新失败');
      } else {
        const res = await dbServerApi.insertRecord(version.id, selectedDatabase.name, selectedTable, values);
        if (res.data?.data?.success) message.success('插入成功');
        else message.error(res.data?.data?.error || '插入失败');
      }
      setRecordModalVisible(false);
      fetchTableData(version.id, selectedDatabase.name, selectedTable, tablePage);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
    finally { setRecordSaving(false); }
  };

  const handleDeleteRecord = async (record: any) => {
    const version = selectedVersion;
    if (!selectedDatabase || !version || !selectedTable) return;
    try {
      const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
      const pkVal = record[pk];
      const res = await dbServerApi.deleteRecord(version.id, selectedDatabase.name, selectedTable, pk, pkVal);
      if (res.data?.data?.success) {
        message.success('删除成功');
        fetchTableData(version.id, selectedDatabase.name, selectedTable, tablePage);
      } else { message.error(res.data?.data?.error || '删除失败'); }
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
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
  // The engine is a persistent top-level Tab; below it the three detail levels
  // (instance list → instance detail → database explorer) render by selection.
  const renderContent = () => {
    if (selectedDatabase && selectedVersion) {
      return (
        <TableExplorer
          server={activeEngine}
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
          onSelectTable={(t) => { setSelectedTable(t); if (selectedDatabase) fetchTableData(selectedVersion.id, selectedDatabase.name, t); }}
          onFetchTables={() => selectedDatabase && fetchTables(selectedVersion.id, selectedDatabase.name)}
          onFetchTableData={(t, p) => selectedDatabase && fetchTableData(selectedVersion.id, selectedDatabase.name, t, p)}
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
    if (selectedVersion) {
      return (
        <DatabaseList
          server={activeEngine}
          version={selectedVersion}
          databases={databases}
          dbsLoading={dbsLoading}
          dbUsers={dbUsers}
          usersLoading={usersLoading}
          operating={operating}
          onBack={goBackToVersions}
          onEnterDatabase={enterDatabase}
          onRefreshDatabases={() => fetchDatabases(selectedVersion.id)}
          onRefreshUsers={() => fetchUsers(selectedVersion.id)}
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
    return (
      <VersionList
        server={activeEngine}
        versions={versions}
        versionsLoading={versionsLoading}
        operating={operating}
        onEnterVersion={enterVersion}
        onRefreshVersions={() => fetchInstances(activeDbType)}
        onStartVersion={handleStartVersion}
        onStopVersion={handleStopVersion}
        onRestartVersion={handleRestartVersion}
        onUninstallVersion={handleUninstallVersion}
        installVersionVisible={installVersionVisible}
        onInstallVersionVisibleChange={setInstallVersionVisible}
        versionTemplates={activeEngine.templates}
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
  };

  return (
    <div>
      <Tabs
        activeKey={activeDbType}
        onChange={changeEngine}
        items={ENGINE_TABS.map(e => ({ key: e.db_type, label: e.display_name }))}
      />
      {renderContent()}
    </div>
  );
}
