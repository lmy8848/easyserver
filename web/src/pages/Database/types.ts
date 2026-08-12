import type { Database, DBUser, DBInstance } from '../../types';

// Version templates from API
export interface VersionTemplate {
  version: string;
  image: string;
  description: string;
}

// DBTypeInfo is what sub-pages need to know about the active database type. It
// is a static front-end constant, not a backend catalog — the top-level tab
// defines it, so the backend /db catalog endpoint was removed.
export interface DBTypeInfo {
  db_type: string;
  display_name: string;
  default_port: number;
  // base_image is the Docker Hub official image for the database type; a custom
  // tag picked from "更多版本" is built as `${base_image}:${tag}`.
  base_image: string;
}

export interface DBTypeTab extends DBTypeInfo {
  templates: VersionTemplate[];
}

// Curated presets per database type. Versions follow current mainstream releases
// (researched 2026-08): MySQL 8.4/9.7 LTS + Innovation lines (8.0 is EOL but
// still widely deployed), PostgreSQL 14–18, Redis 7.4/8.x. The "更多版本" flow
// lists all published Docker Hub tags.
export const DB_TYPE_TABS: DBTypeTab[] = [
  {
    db_type: 'mysql', display_name: 'MySQL', default_port: 3306, base_image: 'mysql',
    templates: [
      { version: '8.0', image: 'docker.io/mysql:8.0', description: 'MySQL 8.0（已 EOL，应用仍广泛）' },
      { version: '8.4', image: 'docker.io/mysql:8.4', description: 'MySQL 8.4 LTS' },
      { version: '9.6', image: 'docker.io/mysql:9.6', description: 'MySQL 9.6 Innovation' },
      { version: '9.7', image: 'docker.io/mysql:9.7', description: 'MySQL 9.7 LTS（当前主线）' },
      { version: '26.7', image: 'docker.io/mysql:26.7', description: 'MySQL 26.7 Innovation（最新）' },
    ],
  },
  {
    db_type: 'postgresql', display_name: 'PostgreSQL', default_port: 5432, base_image: 'postgres',
    templates: [
      { version: '14', image: 'docker.io/postgres:14', description: 'PostgreSQL 14（2026-11 EOL）' },
      { version: '15', image: 'docker.io/postgres:15', description: 'PostgreSQL 15' },
      { version: '16', image: 'docker.io/postgres:16', description: 'PostgreSQL 16' },
      { version: '17', image: 'docker.io/postgres:17', description: 'PostgreSQL 17' },
      { version: '18', image: 'docker.io/postgres:18', description: 'PostgreSQL 18（最新）' },
    ],
  },
  {
    db_type: 'redis', display_name: 'Redis', default_port: 6379, base_image: 'redis',
    templates: [
      { version: '7.4', image: 'docker.io/redis:7.4-alpine', description: 'Redis 7.4（上一主线）' },
      { version: '8.0', image: 'docker.io/redis:8.0-alpine', description: 'Redis 8.0' },
      { version: '8.4', image: 'docker.io/redis:8.4-alpine', description: 'Redis 8.4' },
      { version: '8.8', image: 'docker.io/redis:8.8-alpine', description: 'Redis 8.8' },
      { version: '8.10', image: 'docker.io/redis:8.10-alpine', description: 'Redis 8.10（最新）' },
    ],
  },
];

// Table data structure
export interface TableData {
  headers: string[];
  columnTypes?: string[]; // per-header render category: number | string | time | blob | boolean | null
  rows: any[][];
  total: number;
}

// Table info from describeTable
export interface TableColumnInfo {
  name: string;
  type: string;
  is_primary_key: boolean;
  is_auto_incr: boolean;
  has_default: boolean;
  default: string;
  is_nullable: boolean;
}

export interface TableInfo {
  primaryKey: string;
  columns: TableColumnInfo[];
  collation: string; // MySQL 表排序规则（前端据此显示字符集）
}

// SQL execution result
export interface SqlResult {
  success: boolean;
  output?: string;
  error?: string;
}

// ===== Component Props =====

// InstanceHeader props — the type header card (brand + version picker +
// lifecycle/ops actions) and its modals. Service-log state lives in the page
// (InstanceHeader renders it); the install log is inline (InstallLogPanel),
// rendered by the page when the selected version is installing/failed.
export interface InstanceHeaderProps {
  server: DBTypeInfo;
  versions: DBInstance[];
  versionsLoading: boolean;
  operating: string;
  busy: string;
  // Called when the selected version changes (or its status refreshes) — the
  // parent loads that instance's databases/users and renders the detail below.
  onSelectVersion: (version: DBInstance) => void;
  onStartVersion: (v: DBInstance) => void;
  onStopVersion: (v: DBInstance) => void;
  onRestartVersion: (v: DBInstance) => void;
  onUninstallVersion: (v: DBInstance) => void;
  onCancelInstall: (v: DBInstance) => void;
  onReinstallVersion: (v: DBInstance) => void;
  // Install version modal
  installVersionVisible: boolean;
  onInstallVersionVisibleChange: (visible: boolean) => void;
  versionTemplates: VersionTemplate[];
  installVersionForm: any;
  onInstallVersion: () => void;
  portCheck: { available: boolean; message: string; process?: string } | null;
  onCheckPort: (port: number) => void;
  // Status helpers
  statusTag: (status: string) => React.ReactNode;
  // 一次性跳转指令：安装成功后父组件带上要跟随的版本，列表刷新后自动选中它；
  // 选中后调用 onPendingSelectConsumed 清空，避免后续刷新又跳回去。
  pendingSelectVersion?: string | null;
  onPendingSelectConsumed?: () => void;
}

// 数据库 tab — 库列表（选中库后内联表浏览器）+ 创建数据库弹窗。刷新/创建
// 按钮在 tab 栏右侧（tabBarExtraContent），不在内容区。备份从库列表每行的
// 操作列打开（弹窗展示该库备份列表 + 创建）。
export interface DatabasesTabProps {
  server: DBTypeInfo;
  // null while the database type has no installed version — the tab still renders and
  // its table shows the built-in empty state; create-db is hidden in that case.
  version: DBInstance | null;
  databases: Database[];
  dbsLoading: boolean;
  busy: string;
  onFetchDatabases: () => void;
  onOpenCreateDB: () => void;
  onEnterDatabase: (db: Database) => void;
  onDeleteDB: (dbName: string) => void;
  // Create DB modal
  dbModalVisible: boolean;
  onDbModalVisibleChange: (visible: boolean) => void;
  dbForm: any;
  onCreateDB: () => void;
  // Backup（库级，从库列表操作列打开）
  backups: any[];
  backupsLoading: boolean;
  backupCreating: boolean;
  onFetchBackups: (dbName: string) => void;
  onCreateBackup: (dbName: string) => void;
  onDownloadBackup: (backupId: number) => void;
  onRestoreBackup: (backupId: number, dbName: string) => void;
  onDeleteBackup: (backupId: number, dbName: string) => void;
}

// 用户 tab — 用户列表 + 创建用户/授权弹窗。
export interface UsersTabProps {
  server: DBTypeInfo;
  version: DBInstance | null;
  dbUsers: DBUser[];
  usersLoading: boolean;
  busy: string;
  databases: Database[];
  onFetchUsers: () => void;
  onOpenCreateUser: () => void;
  onDeleteUser: (user: DBUser) => void;
  // Create User modal
  userModalVisible: boolean;
  onUserModalVisibleChange: (visible: boolean) => void;
  userForm: any;
  onCreateUser: () => void;
  // Grant modal
  grantVisible: boolean;
  grantUser: DBUser | null;
  grantForm: any;
  onGrantVisibleChange: (visible: boolean) => void;
  onGrant: () => void;
  onOpenGrant: (user: DBUser) => void;
  // Reset Password modal
  resetPasswordVisible: boolean;
  resetPasswordUser: DBUser | null;
  resetPasswordForm: any;
  onResetPasswordVisibleChange: (visible: boolean) => void;
  onResetPassword: () => void;
  onOpenResetPassword: (user: DBUser) => void;
}

// 配置 tab — 结构化参数编辑
export interface ConfigTabProps {
  server: DBTypeInfo;
  version: DBInstance | null;
  busy: string;
  dbConfig: any;
  dbConfigLoading: boolean;
  onSaveConfig: () => void;
  onFetchConfig: () => void;
  onUpdateDBParam: (key: string, value: string) => void;
}

// 表浏览器（内联在表 tab 中）— 选中数据库后展示
export interface TableExplorerProps {
  server: DBTypeInfo;
  version: DBInstance;
  database: Database;
  onBack: () => void;
  // Table state
  tableList: string[];
  tableLoading: boolean;
  selectedTable: string;
  tableData: TableData | null;
  tableDataLoading: boolean;
  tablePage: number;
  tableInfo: TableInfo | null;
  onSelectTable: (table: string) => void;
  onFetchTables: () => void;
  onFetchTableData: (table: string, page?: number) => void;
  // Table management
  createTableVisible: boolean;
  createTableLoading: boolean;
  createForm: any;
  onCreateTableVisibleChange: (visible: boolean) => void;
  onCreateTable: () => void;
  onDropTable: (tableName: string) => void;
  // Record operations
  recordModalVisible: boolean;
  editingRecord: any;
  recordForm: any;
  recordSaving: boolean;
  onRecordModalVisibleChange: (visible: boolean) => void;
  onOpenInsertModal: () => void;
  onOpenEditModal: (record: any) => void;
  onSaveRecord: () => void;
  onDeleteRecord: (record: any) => void;
  // busy 标记进行中的单行写操作（记录删除等）
  busy: string;
}

// SQL 控制台 tab Props
export interface SqlConsoleTabProps {
  server: DBTypeInfo;
  version: DBInstance | null;
  databases: Database[];
  sqlTargetDb: string;
  onSqlTargetDbChange: (dbName: string) => void;
  sqlInput: string;
  onSqlInputChange: (sql: string) => void;
  sqlResult: SqlResult | null;
  sqlLoading: boolean;
  onExecuteSQL: () => void;
}

// Re-export parent types for convenience
export type { Database, DBUser, DBInstance };
