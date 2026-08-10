import type { Database, DBUser, DBInstance, ActiveInstall } from '../../types';

// Version templates from API
export interface VersionTemplate {
  version: string;
  image: string;
  description: string;
}

// EngineInfo is what sub-pages need to know about the active engine. It is a
// static front-end constant, not a backend catalog — the top-level tab defines
// it, so the backend /db catalog endpoint was removed.
export interface EngineInfo {
  db_type: string;
  display_name: string;
  default_port: number;
  // base_image is the Docker Hub official image for the engine; a custom tag
  // picked from "更多版本" is built as `${base_image}:${tag}`.
  base_image: string;
}

export interface EngineTab extends EngineInfo {
  templates: VersionTemplate[];
}

// Curated presets per engine. Versions follow current mainstream releases
// (researched 2026-08): MySQL 8.4/9.7 LTS + Innovation lines (8.0 is EOL but
// still widely deployed), PostgreSQL 14–18, Redis 7.4/8.x. The "更多版本" flow
// lists all published Docker Hub tags.
export const ENGINE_TABS: EngineTab[] = [
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
  rows: any[][];
  total: number;
}

// Table info from describeTable
export interface TableInfo {
  primaryKey: string;
  columns: Array<{ name: string; type: string; key?: string }>;
}

// SQL execution result
export interface SqlResult {
  success: boolean;
  output?: string;
  error?: string;
}

// ===== Component Props =====

// InstanceHeader props — the engine header card (brand + version picker +
// lifecycle/ops actions) and its modals. Service-log state lives in the page
// (InstanceHeader renders it); install-log state lives in the page
// too so an install can auto-open it and the "正在安装" button can re-open it.
export interface InstanceHeaderProps {
  server: EngineInfo;
  versions: DBInstance[];
  versionsLoading: boolean;
  operating: string;
  busy: string;
  // Called when the selected version changes (or its status refreshes) — the
  // parent loads that instance's databases/users and renders the detail below.
  onSelectVersion: (version: DBInstance) => void;
  onRefreshVersions: () => void;
  onStartVersion: (v: DBInstance) => void;
  onStopVersion: (v: DBInstance) => void;
  onRestartVersion: (v: DBInstance) => void;
  onUninstallVersion: (v: DBInstance) => void;
  // Install version modal
  installVersionVisible: boolean;
  onInstallVersionVisibleChange: (visible: boolean) => void;
  versionTemplates: VersionTemplate[];
  installVersionForm: any;
  onInstallVersion: () => void;
  portCheck: { available: boolean; message: string; process?: string } | null;
  onCheckPort: (port: number) => void;
  // Install log modal (SSE stream — state lives in the parent). Keyed by
  // install_id (= container id), not instance id.
  activeInstalls: ActiveInstall[];
  installLogInstance: { id: string; version: string } | null;
  installLogLines: string[];
  installLogError: string;
  installLogDone: boolean;
  installLogFollow: boolean;
  installLogRef: React.RefObject<HTMLDivElement | null>;
  onOpenInstallLog: (install: { id: string; version: string }) => void;
  onCloseInstallLog: () => void;
  onInstallLogFollowChange: (follow: boolean) => void;
  // Status helpers
  statusTag: (status: string) => React.ReactNode;
}

// DatabaseList props — instance detail (databases / users / config), rendered
// directly under the InstanceHeader card. Lifecycle/log actions live in the
// header; the service-log modal renders at the page root.
export interface DatabaseListProps {
  server: EngineInfo;
  version: DBInstance;
  databases: Database[];
  dbsLoading: boolean;
  dbUsers: DBUser[];
  usersLoading: boolean;
  busy: string;
  onEnterDatabase: (db: Database) => void;
  onRefreshDatabases: () => void;
  onRefreshUsers: () => void;
  onDeleteDB: (dbName: string) => void;
  onDeleteUser: (user: DBUser) => void;
  // Create DB modal
  dbModalVisible: boolean;
  onDbModalVisibleChange: (visible: boolean) => void;
  dbForm: any;
  onCreateDB: () => void;
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
  // Config editor
  dbConfig: any;
  dbConfigLoading: boolean;
  onFetchDBConfig: () => void;
  onSaveDBConfig: () => void;
  onUpdateDBParam: (section: string, key: string, value: string) => void;
  // Inline table browser — non-null when a database is selected; the 表 tab
  // shows it instead of the database list (no separate screen / back level).
  tableExplorer: TableExplorerProps | null;
}

// TableExplorer props
export interface TableExplorerProps {
  server: EngineInfo;
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
  // SQL console
  sqlInput: string;
  sqlResult: SqlResult | null;
  sqlLoading: boolean;
  onSqlInputChange: (value: string) => void;
  onExecuteSQL: () => void;
  // Backup
  backups: any[];
  backupsLoading: boolean;
  backupCreating: boolean;
  busy: string;
  onCreateBackup: () => void;
  onDownloadBackup: (backupId: number) => void;
  onRestoreBackup: (backupId: number) => void;
  onDeleteBackup: (backupId: number) => void;
}

// Re-export parent types for convenience
export type { Database, DBUser, DBInstance };
