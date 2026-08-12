import { useState, useEffect, useRef } from 'react';
import { Form, message, Modal, Tag, Tabs, Card, Button, Space, Empty, Checkbox, Popconfirm } from 'antd';
import { DatabaseOutlined, UserOutlined, CodeOutlined, ReloadOutlined, PlusOutlined } from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import type { Database, DBUser, DBInstance } from '../../types';
import { usePortCheck } from '../../hooks/usePortCheck';
import { getServiceStatusColor } from '../../utils/status';
import InstanceHeader from './InstanceHeader';
import InstallLogPanel from './InstallLogPanel';
import DatabasesTab from './DatabasesTab';
import RedisKeysTab from './RedisKeysTab';
import UsersTab from './UsersTab';
import ConfigTab from './ConfigTab';
import type { TableData, TableInfo, SqlResult, TableExplorerProps } from './types';
import { DB_TYPE_TABS } from './types';

// antd 校验失败抛的是普通对象（{ errorFields }），不是 Error —— 统一从这里
// 提取可读消息（如"请输入 xxx"），避免整对象 toString 成 [object Object]。
function errMsg(err: unknown, fallback = '操作失败'): string {
  if (err instanceof Error) return err.message;
  const e = err as any;
  if (Array.isArray(e?.errorFields)) {
    const first = e.errorFields[0]?.errors?.[0];
    if (typeof first === 'string') return first;
  }
  if (typeof e?.message === 'string') return e.message;
  return fallback;
}

export default function DatabasePage() {
  // ===== Navigation state =====
  // The database type is a static front-end Tab (MySQL/PostgreSQL/Redis); the instance
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
  // 安装成功后让头部 select 一次性跳转到新装版本（InstanceHeader 消费后清空）。
  const [pendingSelectVersion, setPendingSelectVersion] = useState<string | null>(null);

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
  // activeDbType only ever holds one of DB_TYPE_TABS' db_type keys (initial value
  // and Tabs items are both hard-coded), so find() always matches.
  const activeDBTypeInfo = DB_TYPE_TABS.find(e => e.db_type === activeDbType)!;

  const fetchInstances = async (dbtype: string) => {
    setVersionsLoading(true);
    try { const res = await dbServerApi.listInstances(dbtype); setVersions(res.data?.data || []); }
    catch (error) { console.error('Failed to fetch instances:', error); message.error('加载实例列表失败'); } finally { setVersionsLoading(false); }
  };

  // ===== Effects =====
  useEffect(() => { fetchInstances('mysql'); }, []);

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
        setTableData({
          headers: data.headers || [],
          columnTypes: data.column_types || [],
          rows: data.rows || [],
          total: data.total || 0,
        });
      } else {
        setTableData({ headers: [], rows: [], total: 0 });
      }
      setTablePage(page);
      const describeData = describeRes.data?.data;
      const columns = describeData?.columns || [];
      const primaryKey = describeData?.primary_key || columns[0]?.name || 'id';
      // 保留后端的列完整信息（类型/自增/可空/默认值），记录弹窗按列渲染输入。
      setTableInfo({
        primaryKey,
        collation: describeData?.collation || '',
        columns: columns.map((c: any) => ({
          name: c.name, type: c.type,
          is_primary_key: !!c.is_primary_key, is_auto_incr: !!c.is_auto_incr,
          has_default: !!c.has_default, default: c.default || '',
          is_nullable: !!c.is_nullable,
        })),
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
      // 结构化配置：params（引擎当前值）+ meta（编辑元数据）。
      setDBConfig({ found: true, config: res.data?.data });
    } catch (error) {
      console.error('Failed to load config:', error);
      setDBConfig(null);
    } finally { setDBConfigLoading(false); }
  };

  // 切到「配置文件」tab 自动加载,无需先点「加载配置」;同一实例只加载一次
  // (记住上次加载的实例 id,切换版本才重新拉)。加载失败时 ConfigTab 仍保留
  // 「加载配置」按钮作为重试入口。
  const lastConfigInstanceRef = useRef<number | null>(null);
  useEffect(() => {
    if (detailTab !== 'config' || !selectedVersion || selectedVersion.status !== 'running') return;
    if (lastConfigInstanceRef.current === selectedVersion.id) return;
    lastConfigInstanceRef.current = selectedVersion.id;
    fetchDBConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailTab, selectedVersion]);

  // 恢复状态 SSE 连接：组件卸载时关闭，避免泄漏。
  const restoreEsRef = useRef<EventSource | null>(null);
  useEffect(() => () => { restoreEsRef.current?.close(); }, []);

  // ===== Navigation handlers =====
  // Switching database type Tab clears all instance-scoped state and reloads.
  // versions is reset to [] (not just left stale) so the header Select empties
  // at once instead of briefly showing the previous type's instance.
  const changeDBType = (dbtype: string) => {
    setActiveDbType(dbtype);
    setSelectedVersion(null);
    setSelectedDatabase(null);
    setVersions([]);
    setDatabases([]); setDBUsers([]);
    setDBConfig(null);
    setSelectedTable(''); setTableData(null); setSqlResult(null); setBackups([]);
    setDetailTab('databases');
    fetchInstances(dbtype);
  };

  // Selecting a version (via the header Select / auto-select) sets it as the
  // current instance and loads its databases/users below — the old separate
  // "进入实例" page is gone, the detail renders directly under the header card.
  const enterVersion = async (version: DBInstance) => {
    setSelectedVersion(version);
    setSelectedDatabase(null);
    if (version.status === 'running') {
      // Redis has no SQL databases/users — its "数据库" tab is a key browser
      // (RedisKeysTab) that loads its own state, so skip the SQL fetches.
      if (activeDbType === 'redis') return;
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
    const server = activeDBTypeInfo;
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
      // Port left empty → type default. image is always sent fully qualified
      // (preset/picker already carry `docker.io/`); only as a last resort fall
      // back to `docker.io/<base_image>:<version>`.
      const rawImage = (values.image || '').trim();
      const image = rawImage.includes('/') ? rawImage : `docker.io/${server.base_image}:${values.version}`;
      await dbServerApi.createInstance(server.db_type, {
        ...values,
        image,
        port: values.port || server.default_port,
      });
      message.success('已开始安装');
      setInstallVersionVisible(false);
      // 安装成功后头部 select 跟随新装的版本（其 installing 行进入列表，
      // InstanceHeader 据此跳转并显示内联安装日志）。
      setPendingSelectVersion(values.version);
      fetchInstances(server.db_type);
      // The new "installing" row auto-selects in the header; its log panel
      // (SSE) renders inline below.
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
    finally { setBusy(''); }
  };

  // Cancel an in-flight install lives in InstallLogPanel (the only place the
  // install's progress is watched). No page-level handler needed.

  const handleStartVersion = async (v: DBInstance) => {
    const server = activeDBTypeInfo;
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
    const server = activeDBTypeInfo;
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
    const server = activeDBTypeInfo;
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
    const server = activeDBTypeInfo;
    if (!server) return;
    // Uninstall keeps the data volume by default so the instance can be
    // re-installed onto it; the checkbox opts into deleting the data too.
    let purge = false;
    Modal.confirm({
      title: `卸载 ${server.display_name} ${v.version}？`,
      content: (
        <div>
          <p>卸载将删除该数据库实例。默认保留数据卷（可重新安装以恢复数据）。</p>
          <Checkbox onChange={(e) => { purge = e.target.checked; }}>同时删除数据卷（不可恢复）</Checkbox>
        </div>
      ),
      okText: '卸载',
      okType: 'danger',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setOperating(`uninstall-${v.id}`);
        try {
          await dbServerApi.uninstallInstance(v.id, purge);
          message.success(purge ? '已卸载并删除数据卷' : '已卸载（数据卷已保留）');
          fetchInstances(server.db_type);
        } catch (error: unknown) { message.error((error instanceof Error ? error.message : '卸载失败')); }
        finally { setOperating(''); }
      },
    });
  };

  // Reinstall a failed instance directly with its original parameters (no modal):
  // the backend rolled the container back, so this purges the failed row +
  // volumes for a clean start, then immediately kicks off the same install.
  const handleReinstallVersion = async (v: DBInstance) => {
    const server = activeDBTypeInfo;
    if (!server) return;
    setOperating(`reinstall-${v.id}`);
    try {
      await dbServerApi.uninstallInstance(v.id, true);
      await dbServerApi.createInstance(server.db_type, {
        version: v.version,
        image: v.image,
        port: v.port || server.default_port,
        container_engine: v.container_engine,
        bind_address: v.bind_address,
        container_name: v.container_name, // 复用原容器名，卸载已删同名容器
      });
      message.success('正在重新安装…');
      fetchInstances(server.db_type);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '重新安装失败')); }
    finally { setOperating(''); }
  };

  // Cancel an in-flight install from the header card. The backend aborts the
  // goroutine and removes the container + instance row; the row disappearing
  // flips the UI back to the not-installed placeholder.
  const handleCancelInstall = (v: DBInstance) => {
    const server = activeDBTypeInfo;
    if (!server) return;
    Modal.confirm({
      title: '取消安装',
      content: `确定取消 ${server.display_name} ${v.version} 的安装吗？已下载的镜像层会保留。`,
      okText: '取消安装',
      okButtonProps: { danger: true },
      cancelText: '继续安装',
      onOk: async () => {
        setOperating(`cancel-install-${v.id}`);
        try {
          await dbServerApi.cancelInstall(v.container_name);
          message.success('已取消安装');
          fetchInstances(server.db_type);
        } catch (error: unknown) { message.error((error instanceof Error ? error.message : '取消失败')); }
        finally { setOperating(''); }
      },
    });
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
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
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
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
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
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
    finally { setBusy(''); }
  };

  // ===== Config handlers =====
  const handleSaveDBConfig = async () => {
    if (!dbConfig?.config?.params || !selectedVersion) return;
    setBusy('save-config');
    try {
      // 只提交改过的非空字段（meta 是渲染元数据，服务端不落库）；空值不提交
      // —— 保存只应用改过的字段，清空输入框等于不改。
      const params = Object.fromEntries(
        Object.entries(dbConfig.config.params || {}).filter(([, v]) => String(v ?? '').trim() !== '')
      ) as Record<string, string>;
      await dbServerApi.saveInstanceConfig(selectedVersion.id, params);
      message.success('配置已保存');
      fetchDBConfig();
      fetchInstances(activeDbType);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '保存失败')); }
    finally { setBusy(''); }
  };

  const updateDBParam = (key: string, value: string) => {
    setDBConfig((prev: any) => {
      if (!prev?.config?.params) return prev;
      return { ...prev, config: { ...prev.config, params: { ...prev.config.params, [key]: value } } };
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
    } catch (error: unknown) { setSqlResult({ success: false, error: errMsg(error, '执行失败') }); }
    finally { setSqlLoading(false); }
  };

  const handleCreateBackup = async (dbName: string) => {
    const version = selectedVersion;
    if (!version) return;
    setBackupCreating(true);
    try {
      await dbServerApi.createBackup(version.id, dbName);
      message.success('备份已开始，请稍候...');
      // 立即拉一次把 running 行带进 state；弹窗里的 SSE（/db/backups/:id/status）
      // 会等终态到达自动刷新，无需 setTimeout 延迟轮询。
      fetchBackups(version.id, dbName);
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

  const handleRestoreBackup = async (backupId: number, dbName: string) => {
    const version = selectedVersion;
    if (!version) return;
    setBusy(`restore-${backupId}`);
    try {
      await dbServerApi.restoreBackup(backupId);
      // 恢复是异步内存任务：SSE 订阅 restore-status，终态到达即推送（替代轮询）。
      const es = new EventSource(`/api/db/backups/${backupId}/restore-status`);
      restoreEsRef.current = es;
      es.onmessage = (e) => {
        let ev: any;
        try { ev = JSON.parse(e.data); } catch { return; }
        if (ev.type !== 'done') return; // running 心跳帧忽略
        restoreEsRef.current = null;
        es.close();
        setBusy('');
        if (ev.status === 'success') message.success('恢复成功');
        else message.error(ev.error || '恢复失败');
        fetchBackups(version.id, dbName);
      };
      // 连接中断不自行终结：EventSource 自动重连；服务重启内存态丢失后，
      // 重连收到 {type:'done', error} 帧，仍能正确提示。
    } catch (error: unknown) {
      setBusy('');
      message.error(error instanceof Error ? error.message : '恢复失败');
    }
  };

  const handleDeleteBackup = async (backupId: number, dbName: string) => {
    const version = selectedVersion;
    if (!version) return;
    setBusy(`delete-backup-${backupId}`);
    try {
      await dbServerApi.deleteBackup(backupId);
      message.success('备份已删除');
      fetchBackups(version.id, dbName);
    } catch (error: unknown) { message.error((error instanceof Error ? error.message : '删除失败')); }
    finally { setBusy(''); }
  };

  const handleCreateTable = async () => {
    const version = selectedVersion;
    if (!selectedDatabase || !version) return;
    setCreateTableLoading(true);
    try {
      const values = await createForm.validateFields();
      // PG 无表级字符集（编码在数据库级），排序规则默认留空继承；MySQL 默认 utf8mb4_0900_ai_ci。
      const isPg = activeDBTypeInfo?.db_type === 'postgresql';
      const formattedColumns = (values.columns || []).map((col: any) => {
        let type = col.type || '';
        const length = col.length ? String(col.length).trim() : '';
        if (length && !type.includes('(')) {
          type = `${type}(${length})`;
        } else if (!length && !type.includes('(')) {
          if (type.toUpperCase() === 'VARCHAR') {
            type = 'VARCHAR(255)';
          }
        }
        return {
          name: col.name,
          type: type,
          length: length || undefined,
          nullable: !!col.nullable,
          is_primary: !!col.is_primary,
          auto_incr: !!col.auto_incr,
          unique: !!col.unique,
          default_value: col.default_value || '',
        };
      });
      await dbServerApi.createTable(version.id, selectedDatabase.name, {
        name: values.tableName,
        columns: formattedColumns,
        charset: isPg ? 'UTF8' : (values.charset || 'utf8mb4'),
        collation: isPg ? (values.collation || '') : (values.collation || 'utf8mb4_0900_ai_ci'),
      });
      message.success('表创建成功');
      setCreateTableVisible(false);
      createForm.resetFields();
      fetchTables(version.id, selectedDatabase.name);
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
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
    // 布尔列存的值可能是 1/0 或 true/false（取决于引擎），归一成 Select 的选项值。
    (tableInfo?.columns || []).forEach(c => {
      if (/^(BOOLEAN|BOOL|BIT)\b/i.test(c.type) && values[c.name] !== undefined) {
        const v = values[c.name];
        values[c.name] = (v === true || v === 1 || v === '1' || String(v).toLowerCase() === 'true') ? '1' : '0';
      }
    });
    recordForm.setFieldsValue(values);
    setRecordModalVisible(true);
  };

  const handleSaveRecord = async () => {
    const version = selectedVersion;
    if (!selectedDatabase || !version || !selectedTable) return;
    setRecordSaving(true);
    try {
      const values = await recordForm.validateFields();
      // 自增列由数据库生成，不提交（后端 BuildInsert 不跳过 nil 自增值）。
      (tableInfo?.columns || [])
        .filter(c => c.is_auto_incr)
        .forEach(c => { delete values[c.name]; });
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
    } catch (error: unknown) { message.error(errMsg(error, '操作失败')); }
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
      running: '运行中', stopped: '已停止', installing: '安装中', failed: '安装失败',
      partial: '部分运行', not_installed: '未安装',
    };
    const colors: Record<string, string> = {
      running: 'success', installing: 'processing', failed: 'error', stopped: 'default',
    };
    return <Tag color={colors[status] || getServiceStatusColor(status)}>{labels[status] || status}</Tag>;
  };

  // ===== Render =====
  // The database type is a persistent top-level Tab. InstanceHeader is the header card
  // (version picker + lifecycle actions + instance-level modals). When a version
  // is selected, the 数据库/用户/配置文件 tabs render below — the 数据库 tab shows
  // the table browser inline once a database is picked (no separate screen).
  const renderContent = () => {
    // The table browser props are built once and handed to the 数据库 tab; the
    // tab renders it inline when a database is selected.
    const tableExplorer: TableExplorerProps | null = selectedDatabase && selectedVersion ? {
      server: activeDBTypeInfo,
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
      busy,
      sqlInput, sqlResult, sqlLoading,
      onSqlInputChange: setSqlInput,
      onExecuteSQL: handleExecuteSQL,
    } : null;

    // Action buttons live in the inner tab bar's extra area — they follow the
    // active detail tab (数据库/用户) and are hidden until a version is selected
    // (there is no instance to act on otherwise).
    const tabExtra = !selectedVersion ? null : detailTab === 'databases' ? (
      // Redis 的数据库 tab 是自包含 key 浏览器（自带 DB/pattern/刷新工具栏），
      // tab 栏不重复放 SQL 的刷新/创建数据库按钮。
      activeDbType === 'redis' ? null : (
        <Space style={{ marginRight: 8 }}>
          <Button icon={<ReloadOutlined />} loading={dbsLoading}
            onClick={() => fetchDatabases(selectedVersion.id)}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />}
            onClick={() => { dbForm.resetFields(); setDbModalVisible(true); }}
            disabled={selectedVersion.status !== 'running'}>创建数据库</Button>
        </Space>
      )
    ) : detailTab === 'users' ? (
      <Space style={{ marginRight: 8 }}>
        <Button icon={<ReloadOutlined />} loading={usersLoading}
          onClick={() => fetchUsers(selectedVersion.id)}>刷新</Button>
        <Button type="primary" icon={<PlusOutlined />}
          onClick={() => { userForm.resetFields(); setUserModalVisible(true); }}
          disabled={selectedVersion.status !== 'running'}>创建用户</Button>
      </Space>
    ) : detailTab === 'config' ? (
      <Space style={{ marginRight: 8 }}>
        <Popconfirm
          title="将重启数据库"
          description="保存配置后将重启实例，确定保存吗？"
          okText="保存"
          okButtonProps={{ danger: true }}
          cancelText="取消"
          onConfirm={handleSaveDBConfig}
        >
          <Button type="primary" loading={busy === 'save-config'}>保存配置</Button>
        </Popconfirm>
        <Button icon={<ReloadOutlined />} loading={dbConfigLoading}
          onClick={() => fetchDBConfig()}>刷新</Button>
      </Space>
    ) : null;

    return (
      <div>
        {/* key remounts the header on database-type switch — its internal selection
            and notify-dedup state (lastNotifiedKey is `id:status`, and ids repeat
            across types since they share one table) must start fresh, or a new
            type's instance is never reported to the parent and the detail
            tables below stay stale. */}
        <InstanceHeader key={activeDbType}
          server={activeDBTypeInfo}
          versions={versions}
          versionsLoading={versionsLoading}
          operating={operating}
          onSelectVersion={enterVersion}
          onStartVersion={handleStartVersion}
          onStopVersion={handleStopVersion}
          onRestartVersion={handleRestartVersion}
          onUninstallVersion={handleUninstallVersion}
          onCancelInstall={handleCancelInstall}
          onReinstallVersion={handleReinstallVersion}
          installVersionVisible={installVersionVisible}
          onInstallVersionVisibleChange={setInstallVersionVisible}
          versionTemplates={activeDBTypeInfo.templates}
          installVersionForm={installVersionForm}
          busy={busy}
          onInstallVersion={handleInstallVersion}
          portCheck={portCheck}
          onCheckPort={checkPort}
          statusTag={statusTag}
          pendingSelectVersion={pendingSelectVersion}
          onPendingSelectConsumed={() => setPendingSelectVersion(null)}
        />
        {/* No installed version — plain placeholder with an install action. The
            installing state is no longer a placeholder: the row exists from
            submit time, so it appears in the list and selects into the log panel. */}
        {versions.length === 0 && !versionsLoading ? (
          <Card>
            <Empty style={{ padding: '48px 0' }}
              description={`尚未安装 ${activeDBTypeInfo.display_name} 数据库实例`}>
              <Button type="primary" icon={<PlusOutlined />}
                onClick={() => { installVersionForm.resetFields(); setInstallVersionVisible(true); }}>
                安装数据库
              </Button>
            </Empty>
          </Card>
        ) : selectedVersion && (selectedVersion.status === 'installing' || selectedVersion.status === 'failed') ? (
          // key=instance id: a reinstall reuses the same container name (so the
          // containerName prop is unchanged), but it is a brand-new install with a
          // fresh log stream — remounting on id change resets the SSE connection
          // so the new install's log shows instead of the stale failed one.
          <InstallLogPanel
            key={selectedVersion.id}
            containerName={selectedVersion.container_name}
            version={selectedVersion.version}
            onDone={() => fetchInstances(activeDbType)}
          />
        ) : (
          <Card>
          <Tabs
            activeKey={detailTab}
            onChange={setDetailTab}
            tabBarExtraContent={tabExtra}
            items={[
              {
                key: 'databases',
                label: <span><DatabaseOutlined /> 数据库</span>,
                children: activeDbType === 'redis' && selectedVersion
                  ? <RedisKeysTab instance={selectedVersion} />
                  : <DatabasesTab
                      server={activeDBTypeInfo}
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
                      backups={backups}
                      backupsLoading={backupsLoading}
                      backupCreating={backupCreating}
                      onFetchBackups={(dbName) => selectedVersion && fetchBackups(selectedVersion.id, dbName)}
                      onCreateBackup={handleCreateBackup}                      onDownloadBackup={handleDownloadBackup}
                      onRestoreBackup={handleRestoreBackup}
                      onDeleteBackup={handleDeleteBackup}
                    />,
              },
              {
                key: 'users',
                label: <span><UserOutlined /> 用户</span>,
                children: <UsersTab
                  server={activeDBTypeInfo}
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
                label: <span><CodeOutlined /> 配置</span>,
                children: <ConfigTab
                  server={activeDBTypeInfo}
                  dbConfig={dbConfig}
                  dbConfigLoading={dbConfigLoading}
                  onUpdateDBParam={updateDBParam}
                />,
              },
            ]} />
          </Card>
        )}
      </div>
    );
  };

  return (
    <div>
      <Tabs
        activeKey={activeDbType}
        onChange={changeDBType}
        items={DB_TYPE_TABS.map(e => ({ key: e.db_type, label: e.display_name }))}
      />
      {renderContent()}
    </div>
  );
}
