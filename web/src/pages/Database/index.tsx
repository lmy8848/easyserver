import { useState, useEffect, useRef } from 'react';
import { Form, message, Modal, Tag, Tabs, Card, Button, Space } from 'antd';
import { DatabaseOutlined, UserOutlined, CodeOutlined, ReloadOutlined, PlusOutlined } from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import type { Database, DBUser, DBInstance, ActiveInstall } from '../../types';
import { usePortCheck } from '../../hooks/usePortCheck';
import { getServiceStatusColor } from '../../utils/status';
import InstanceHeader from './InstanceHeader';
import DatabasesTab from './DatabasesTab';
import UsersTab from './UsersTab';
import ConfigTab from './ConfigTab';
import type { TableData, TableInfo, SqlResult, TableExplorerProps } from './types';
import { ENGINE_TABS } from './types';

export default function DatabasePage() {
  // ===== Navigation state =====
  // The engine is a static front-end Tab (MySQL/PostgreSQL/Redis); the instance
  // list, instance detail and database explorer render below it.
  const [activeDbType, setActiveDbType] = useState('mysql');
  const [selectedVersion, setSelectedVersion] = useState<DBInstance | null>(null);
  const [selectedDatabase, setSelectedDatabase] = useState<Database | null>(null);
  const [operating, setOperating] = useState('');
  // Active tab of the instance detail (数据库/用户/配置文件) — controls which
  // tab's action buttons show in the tab bar's extra area.
  const [detailTab, setDetailTab] = useState('databases');
  // busy tracks one in-flight write operation at a time (a short string key);
  // buttons/modals match on it to show their loading state.
  const [busy, setBusy] = useState('');

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

  // ===== Install log (SSE stream) =====
  // Keyed by install_id (= container id), not instance id — no instance row
  // exists while installing. State lives here (not in InstanceHeader) so an
  // install can auto-open it, and the title-bar "正在安装" button can re-open it
  // after close/refresh.
  const [activeInstalls, setActiveInstalls] = useState<ActiveInstall[]>([]);
  const [installLogInstance, setInstallLogInstance] = useState<{ id: string; version: string } | null>(null);
  const [installLogLines, setInstallLogLines] = useState<string[]>([]);
  const [installLogError, setInstallLogError] = useState('');
  const [installLogDone, setInstallLogDone] = useState(false);
  const [installLogFollow, setInstallLogFollow] = useState(true);
  const installLogRef = useRef<HTMLDivElement>(null);

  const fetchActiveInstalls = async () => {
    try {
      const res = await dbServerApi.listActiveInstalls();
      setActiveInstalls(res.data?.data || []);
    } catch { /* polling endpoint, ignore transient errors */ }
  };

  const openInstallLog = (install: { id: string; version: string }) => {
    setInstallLogInstance(install);
    setInstallLogLines([]);
    setInstallLogError('');
    setInstallLogDone(false);
  };
  const closeInstallLog = () => setInstallLogInstance(null);

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
  useEffect(() => { fetchInstances('mysql'); fetchActiveInstalls(); }, []);

  // While an install runs for the current engine, poll for completion so the
  // instance list refreshes even if the log window is closed.
  useEffect(() => {
    const engineActive = activeInstalls.some(a => a.engine === activeDbType);
    if (!engineActive) return;
    const timer = setInterval(async () => {
      try {
        const res = await dbServerApi.listActiveInstalls();
        const next = res.data?.data || [];
        setActiveInstalls(next);
        if (!next.some(a => a.engine === activeDbType)) {
          fetchInstances(activeDbType); // install finished/failed
        }
      } catch { /* keep polling */ }
    }, 5000);
    return () => clearInterval(timer);
  }, [activeInstalls, activeDbType]);

  useEffect(() => {
    if (!installLogInstance) return;
    // SSE replay: the server sends every buffered line first (cursor starts at
    // 0), then follows live until the {type:'done'} frame.
    const es = new EventSource(`/api/db/installs/${installLogInstance.id}/log`);
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'line') setInstallLogLines(prev => [...prev, msg.text]);
        else if (msg.type === 'done') {
          setInstallLogError(msg.error || '');
          setInstallLogDone(true);
          es.close();
          fetchInstances(activeDbType); // row now exists (running/failed)
        }
      } catch { /* ignore malformed frames */ }
    };
    // Server closed the stream (or a transient blip); stop so the "done" state
    // governs the UI. EventSource auto-reconnects otherwise.
    es.onerror = () => { setInstallLogDone(true); es.close(); };
    return () => es.close();
  }, [installLogInstance]);

  useEffect(() => {
    if (installLogFollow && installLogRef.current) {
      installLogRef.current.scrollTop = installLogRef.current.scrollHeight;
    }
  }, [installLogLines, installLogFollow]);

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
  // Switching engine Tab clears all instance-scoped state and reloads. versions
  // is reset to [] (not just left stale) so the header Select empties at once
  // instead of briefly showing the previous engine's instance.
  const changeEngine = (dbtype: string) => {
    setActiveDbType(dbtype);
    setSelectedVersion(null);
    setSelectedDatabase(null);
    setVersions([]);
    setDatabases([]); setDBUsers([]);
    setDBConfig(null);
    setSelectedTable(''); setTableData(null); setSqlResult(null); setBackups([]);
    setDetailTab('databases');
    fetchInstances(dbtype);
    fetchActiveInstalls();
  };

  // Selecting a version (via the header Select / auto-select) sets it as the
  // current instance and loads its databases/users below — the old separate
  // "进入实例" page is gone, the detail renders directly under the header card.
  const enterVersion = async (version: DBInstance) => {
    setSelectedVersion(version);
    setSelectedDatabase(null);
    if (version.status === 'running') {
      await Promise.all([fetchDatabases(version.id), fetchUsers(version.id)]);
    }
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

  const goBackToVersionDetail = () => {
    setSelectedDatabase(null);
    setSelectedTable('');
    setTableData(null);
  };

  // ===== Version actions =====
  const handleInstallVersion = async () => {
    const server = activeEngine;
    if (!server) return;
    let values: any;
    try {
      values = await installVersionForm.validateFields();
    } catch (err: any) {
      // antd 校验失败抛的是普通对象（{ errorFields }），不是 Error —— 取第一条
      // 可读消息（如"请选择版本"），避免整对象 toString 成 [object Object]。
      const msg = err?.errorFields?.[0]?.errors?.[0];
      message.error(typeof msg === 'string' ? msg : '请选择安装版本');
      return;
    }
    setBusy('install-version');
    try {
      // Port left empty → engine default. image is always sent fully qualified
      // (preset/picker already carry `docker.io/`); only as a last resort fall
      // back to `docker.io/<base_image>:<version>`.
      const rawImage = (values.image || '').trim();
      const image = rawImage.includes('/') ? rawImage : `docker.io/${server.base_image}:${values.version}`;
      const res = await dbServerApi.createInstance(server.db_type, {
        ...values,
        image,
        port: values.port || server.default_port,
      });
      message.success('已开始安装，正在打开安装日志…');
      setInstallVersionVisible(false);
      fetchInstances(server.db_type);
      fetchActiveInstalls();
      // Auto-open the live install log; the title-bar "正在安装" button (driven
      // by activeInstalls) re-opens it after close/refresh.
      const data = res.data?.data as { install_id?: string } | undefined;
      if (data?.install_id) {
        openInstallLog({ id: data.install_id, version: values.version });
      }
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
    finally { setBusy(''); }
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
    setBusy('create-db');
    try {
      const values = await dbForm.validateFields();
      await dbServerApi.createDatabase(version.id, values);
      message.success('数据库创建成功');
      setDbModalVisible(false);
      fetchDatabases(version.id);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
    finally { setBusy(''); }
  };

  const handleDeleteDB = async (dbName: string) => {
    const version = selectedVersion;
    if (!version) return;
    setBusy(`delete-db-${dbName}`);
    try {
      await dbServerApi.deleteDatabase(version.id, dbName);
      message.success('数据库已删除');
      fetchDatabases(version.id);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
    finally { setBusy(''); }
  };

  // ===== User CRUD =====
  const handleCreateUser = async () => {
    const version = selectedVersion;
    if (!version) return;
    setBusy('create-user');
    try {
      const values = await userForm.validateFields();
      await dbServerApi.createUser(version.id, values);
      message.success('用户创建成功');
      setUserModalVisible(false);
      fetchUsers(version.id);
    } catch (error: unknown) { if ((error instanceof Error ? error.message : String(error))) message.error((error instanceof Error ? error.message : String(error))); }
    finally { setBusy(''); }
  };

  const handleDeleteUser = async (user: DBUser) => {
    const version = selectedVersion;
    if (!version) return;
    setBusy(`delete-user-${user.username}@${user.host}`);
    try {
      await dbServerApi.deleteUser(version.id, user.username, user.host || '%');
      message.success('用户已删除');
      fetchUsers(version.id);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
    finally { setBusy(''); }
  };

  const handleGrant = async () => {
    const version = selectedVersion;
    const user = grantUser;
    if (!version || !user) return;
    setBusy('grant');
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
    finally { setBusy(''); }
  };

  // ===== Config handlers =====
  const handleSaveDBConfig = async () => {
    if (!dbConfig?.config?.sections || !selectedVersion) return;
    setBusy('save-config');
    try {
      const content = dbConfig.config.sections[0]?.params?.content || '';
      await dbServerApi.saveInstanceConfig(selectedVersion.id, content);
      message.success('实例配置已保存，重启后生效');
      fetchDBConfig();
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '保存失败')); }
    finally { setBusy(''); }
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
    setBusy(`restore-${backupId}`);
    try {
      await dbServerApi.restoreBackup(backupId);
      message.success('恢复成功');
      if (selectedDatabase) fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '恢复失败')); }
    finally { setBusy(''); }
  };

  const handleDeleteBackup = async (backupId: number) => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    setBusy(`delete-backup-${backupId}`);
    try {
      await dbServerApi.deleteBackup(backupId);
      message.success('备份已删除');
      if (selectedDatabase) fetchBackups(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
    finally { setBusy(''); }
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
    setBusy(`drop-table-${tableName}`);
    try {
      await dbServerApi.dropTable(version.id, selectedDatabase.name, tableName);
      message.success('表已删除');
      if (selectedTable === tableName) { setSelectedTable(''); setTableData(null); }
      fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除表失败')); }
    finally { setBusy(''); }
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
    setBusy(`delete-record-${record._key}`);
    try {
      const pk = tableInfo?.primaryKey || tableData?.headers?.[0] || 'id';
      const pkVal = record[pk];
      const res = await dbServerApi.deleteRecord(version.id, selectedDatabase.name, selectedTable, pk, pkVal);
      if (res.data?.data?.success) {
        message.success('删除成功');
        fetchTableData(version.id, selectedDatabase.name, selectedTable, tablePage);
      } else { message.error(res.data?.data?.error || '删除失败'); }
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
    finally { setBusy(''); }
  };

  // ===== Status helpers (shared) =====
  const statusTag = (status: string) => {
    const labels: Record<string, string> = {
      running: '运行中', stopped: '已停止', provisioning: '正在安装', failed: '安装失败',
      partial: '部分运行', not_installed: '未安装',
    };
    const colors: Record<string, string> = {
      running: 'success', provisioning: 'processing', failed: 'error', stopped: 'default',
    };
    return <Tag color={colors[status] || getServiceStatusColor(status)}>{labels[status] || status}</Tag>;
  };

  // ===== Render =====
  // The engine is a persistent top-level Tab. InstanceHeader is the header card
  // (version picker + lifecycle actions + instance-level modals). When a version
  // is selected, the 数据库/用户/配置文件 tabs render below — the 数据库 tab shows
  // the table browser inline once a database is picked (no separate screen).
  const renderContent = () => {
    // The table browser props are built once and handed to the 数据库 tab; the
    // tab renders it inline when a database is selected.
    const tableExplorer: TableExplorerProps | null = selectedDatabase && selectedVersion ? {
      server: activeEngine,
      version: selectedVersion,
      database: selectedDatabase,
      onBack: goBackToVersionDetail,
      tableList, tableLoading, selectedTable, tableData, tableDataLoading, tablePage, tableInfo,
      onSelectTable: (t) => { setSelectedTable(t); if (selectedDatabase) fetchTableData(selectedVersion.id, selectedDatabase.name, t); },
      onFetchTables: () => selectedDatabase && fetchTables(selectedVersion.id, selectedDatabase.name),
      onFetchTableData: (t, p) => selectedDatabase && fetchTableData(selectedVersion.id, selectedDatabase.name, t, p),
      createTableVisible, createTableLoading, createForm,
      onCreateTableVisibleChange: setCreateTableVisible,
      onCreateTable: handleCreateTable,
      onDropTable: handleDropTable,
      recordModalVisible, editingRecord, recordForm, recordSaving,
      onRecordModalVisibleChange: setRecordModalVisible,
      onOpenInsertModal: openInsertModal,
      onOpenEditModal: openEditModal,
      onSaveRecord: handleSaveRecord,
      onDeleteRecord: handleDeleteRecord,
      sqlInput, sqlResult, sqlLoading,
      onSqlInputChange: setSqlInput,
      onExecuteSQL: handleExecuteSQL,
      backups, backupsLoading, backupCreating, busy,
      onCreateBackup: handleCreateBackup,
      onDownloadBackup: handleDownloadBackup,
      onRestoreBackup: handleRestoreBackup,
      onDeleteBackup: handleDeleteBackup,
    } : null;

    // Action buttons live in the inner tab bar's extra area — they follow the
    // active detail tab (数据库/用户) and are hidden until a version is selected
    // (there is no instance to act on otherwise).
    const tabExtra = !selectedVersion ? null : detailTab === 'databases' ? (
      <Space style={{ marginRight: 8 }}>
        <Button icon={<ReloadOutlined />} loading={dbsLoading}
          onClick={() => fetchDatabases(selectedVersion.id)}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />}
          onClick={() => { dbForm.resetFields(); setDbModalVisible(true); }}
          disabled={selectedVersion.status !== 'running'}>创建数据库</Button>
      </Space>
    ) : detailTab === 'users' ? (
      <Space style={{ marginRight: 8 }}>
        <Button icon={<ReloadOutlined />} loading={usersLoading}
          onClick={() => fetchUsers(selectedVersion.id)}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />}
          onClick={() => { userForm.resetFields(); setUserModalVisible(true); }}
          disabled={selectedVersion.status !== 'running'}>创建用户</Button>
      </Space>
    ) : null;

    return (
      <div>
        {/* key remounts the header on engine switch — its internal selection and
            notify-dedup state (lastNotifiedKey is `id:status`, and ids repeat
            across engines since they share one table) must start fresh, or a new
            engine's instance is never reported to the parent and the detail
            tables below stay stale. */}
        <InstanceHeader key={activeDbType}
          server={activeEngine}
          versions={versions}
          versionsLoading={versionsLoading}
          operating={operating}
          onSelectVersion={enterVersion}
          onRefreshVersions={() => fetchInstances(activeDbType)}
          onStartVersion={handleStartVersion}
          onStopVersion={handleStopVersion}
          onRestartVersion={handleRestartVersion}
          onUninstallVersion={handleUninstallVersion}
          installVersionVisible={installVersionVisible}
          onInstallVersionVisibleChange={setInstallVersionVisible}
          versionTemplates={activeEngine.templates}
          installVersionForm={installVersionForm}
          busy={busy}
          onInstallVersion={handleInstallVersion}
          portCheck={portCheck}
          onCheckPort={checkPort}
          activeInstalls={activeInstalls}
          installLogInstance={installLogInstance}
          installLogLines={installLogLines}
          installLogError={installLogError}
          installLogDone={installLogDone}
          installLogFollow={installLogFollow}
          installLogRef={installLogRef}
          onOpenInstallLog={openInstallLog}
          onCloseInstallLog={closeInstallLog}
          onInstallLogFollowChange={setInstallLogFollow}
          statusTag={statusTag}
        />
        {/* The detail tabs always render — with no installed version the tables
            just show their built-in empty state (暂无数据库 / 暂无用户), and the
            header auto-selects the first instance once one appears. */}
        <Card>
          <Tabs
            activeKey={detailTab}
            onChange={setDetailTab}
            tabBarExtraContent={tabExtra}
            items={[
              {
                key: 'databases',
                label: <span><DatabaseOutlined /> 数据库</span>,
                children: <DatabasesTab
                  server={activeEngine}
                  version={selectedVersion}
                  databases={databases}
                  dbsLoading={dbsLoading}
                  busy={busy}
                  onEnterDatabase={enterDatabase}
                  onDeleteDB={handleDeleteDB}
                  dbModalVisible={dbModalVisible}
                  onDbModalVisibleChange={setDbModalVisible}
                  dbForm={dbForm}
                  onCreateDB={handleCreateDB}
                  tableExplorer={tableExplorer}
                />,
              },
              {
                key: 'users',
                label: <span><UserOutlined /> 用户</span>,
                children: <UsersTab
                  server={activeEngine}
                  dbUsers={dbUsers}
                  usersLoading={usersLoading}
                  busy={busy}
                  databases={databases}
                  onDeleteUser={handleDeleteUser}
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
                />,
              },
              {
                key: 'config',
                label: <span><CodeOutlined /> 配置文件</span>,
                children: <ConfigTab
                  server={activeEngine}
                  dbConfig={dbConfig}
                  dbConfigLoading={dbConfigLoading}
                  busy={busy}
                  onFetchDBConfig={() => fetchDBConfig()}
                  onSaveDBConfig={handleSaveDBConfig}
                  onUpdateDBParam={updateDBParam}
                />,
              },
            ]} />
          </Card>
      </div>
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
