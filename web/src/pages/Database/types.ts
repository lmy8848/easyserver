import type { DBServer, Database, DBUser, DBVersion } from '../../types';

// Version templates from API
export interface VersionTemplate {
  version: string;
  package: string;
  description: string;
}

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

// ServerList props
export interface ServerListProps {
  servers: DBServer[];
  loading: boolean;
  onEnterServer: (server: DBServer) => void;
  onRefresh: () => void;
  installVersionVisible: boolean;
  onInstallVersionVisibleChange: (visible: boolean) => void;
  versionTemplates: VersionTemplate[];
  installVersionForm: any;
  onInstallVersion: () => void;
  portCheck: { available: boolean; message: string; process?: string } | null;
  onCheckPort: (port: number) => void;
}

// VersionList props
export interface VersionListProps {
  server: DBServer;
  versions: DBVersion[];
  versionsLoading: boolean;
  operating: string;
  onBack: () => void;
  onEnterVersion: (version: DBVersion) => void;
  onRefreshVersions: () => void;
  onStartVersion: (v: DBVersion) => void;
  onStopVersion: (v: DBVersion) => void;
  onRestartVersion: (v: DBVersion) => void;
  onUninstallVersion: (v: DBVersion) => void;
  // Install version modal
  installVersionVisible: boolean;
  onInstallVersionVisibleChange: (visible: boolean) => void;
  versionTemplates: VersionTemplate[];
  installVersionForm: any;
  onInstallVersion: () => void;
  portCheck: { available: boolean; message: string; process?: string } | null;
  onCheckPort: (port: number) => void;
  // Log modal
  logVisible: boolean;
  logVersion: DBVersion | null;
  logContent: string;
  logLoading: boolean;
  logFollow: boolean;
  logRef: React.RefObject<HTMLDivElement | null>;
  onLogVisibleChange: (visible: boolean) => void;
  onLogFollowChange: (follow: boolean) => void;
  onShowLogs: (v: DBVersion) => void;
  // Status helpers
  statusColor: (status: string) => string;
  statusTag: (status: string) => React.ReactNode;
}

// DatabaseList props
export interface DatabaseListProps {
  server: DBServer;
  version: DBVersion;
  databases: Database[];
  dbsLoading: boolean;
  dbUsers: DBUser[];
  usersLoading: boolean;
  operating: string;
  onBack: () => void;
  onEnterDatabase: (db: Database) => void;
  onRefreshDatabases: () => void;
  onRefreshUsers: () => void;
  onDeleteDB: (dbId: number) => void;
  onDeleteUser: (userId: number) => void;
  onStartVersion: (v: DBVersion) => void;
  onStopVersion: (v: DBVersion) => void;
  onRestartVersion: (v: DBVersion) => void;
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
  // Log modal
  logVisible: boolean;
  logVersion: DBVersion | null;
  logContent: string;
  logLoading: boolean;
  logFollow: boolean;
  logRef: React.RefObject<HTMLDivElement | null>;
  onLogVisibleChange: (visible: boolean) => void;
  onLogFollowChange: (follow: boolean) => void;
  showConfig: (v: DBVersion) => void;
  showLogs: (v: DBVersion) => void;
}

// TableExplorer props
export interface TableExplorerProps {
  server: DBServer;
  version: DBVersion;
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
  onCreateBackup: () => void;
  onDownloadBackup: (backupId: number) => void;
  onRestoreBackup: (backupId: number) => void;
  onDeleteBackup: (backupId: number) => void;
  // Log modal
  logVisible: boolean;
  logVersion: DBVersion | null;
  logContent: string;
  logLoading: boolean;
  logFollow: boolean;
  logRef: React.RefObject<HTMLDivElement | null>;
  onLogVisibleChange: (visible: boolean) => void;
  onLogFollowChange: (follow: boolean) => void;
}

// Re-export parent types for convenience
export type { DBServer, Database, DBUser, DBVersion };
