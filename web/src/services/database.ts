import api from './client';
import type {
  ApiResponse,
  DBBackup,
  DBInstance,
  DBUser,
  Database,
  InstanceConfigView,
  Page,
  RedisHashField,
  RedisKey,
  RedisValue,
  RedisZSetMember,
} from '../types';

// Database Server API
export const dbServerApi = {
  // Instance lifecycle, scoped by engine enum.
  listInstances: (dbtype: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<DBInstance>>>(`/db/instances`, { params: { dbtype, page, page_size: pageSize } }),

  createInstance: (dbtype: string, data: { version: string; image?: string; port?: number; container_engine?: string; bind_address?: string; container_name?: string }) =>
    api.post<ApiResponse<{ install_id: string; version: string; image: string; port: number; status: string }>>(`/db/instances`, { ...data, dbtype }),

  // Cancel an in-flight install (image pull or provisioning).
  cancelInstall: (iid: string) =>
    api.post<ApiResponse<null>>(`/db/installs/${iid}/cancel`),

  // Published Docker Hub tags for an engine's official image ("更多版本" flow),
  // paginated — the version Select flips pages through this.
  listDockerTags: (dbtype: string, page = 1, pageSize = 10) =>
    api.get<ApiResponse<Page<string>>>(`/db/docker-tags`, { params: { dbtype, page, page_size: pageSize } }),

  // Uninstall the instance. purge=true also deletes the data (and config) volumes.
  uninstallInstance: (iid: number, purge = false) =>
    api.delete<ApiResponse>(`/db/instances/${iid}`, { params: { purge: purge ? '1' : undefined } }),

  resetAdminPassword: (iid: number) =>
    api.post<ApiResponse<{ admin_password: string }>>(`/db/instances/${iid}/reset-password`),

  startInstance: (iid: number) =>
    api.post<ApiResponse>(`/db/instances/${iid}/start`),

  stopInstance: (iid: number) =>
    api.post<ApiResponse>(`/db/instances/${iid}/stop`),

  restartInstance: (iid: number) =>
    api.post<ApiResponse>(`/db/instances/${iid}/restart`),

  getInstanceLogs: (iid: number, lines: number = 200) =>
    api.get<ApiResponse<{ logs: string }>>(`/db/instances/${iid}/logs`, { params: { lines } }),

  getInstanceConfig: (iid: number) =>
    api.get<ApiResponse<InstanceConfigView>>(`/db/instances/${iid}/config`),

  saveInstanceConfig: (iid: number, params: Record<string, string>) =>
    api.put<ApiResponse>(`/db/instances/${iid}/config`, { params }),

  // Databases (instance-scoped; logical databases are live engine state, so the
  // database name is the identifier — never a persisted id)
  listDatabases: (instanceId: number, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Database>>>(`/db/instances/${instanceId}/databases`, { params: { page, page_size: pageSize } }),

  createDatabase: (instanceId: number, data: { name: string; charset?: string; description?: string }) =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/databases`, data),

  deleteDatabase: (instanceId: number, dbName: string) =>
    api.delete<ApiResponse>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}`),

  // DB Users (instance-scoped; username + host for MySQL)
  listUsers: (instanceId: number, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<DBUser>>>(`/db/instances/${instanceId}/users`, { params: { page, page_size: pageSize } }),

  createUser: (instanceId: number, data: { username: string; password: string; host?: string }) =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users`, data),

  deleteUser: (instanceId: number, username: string, host: string = '%') =>
    api.delete<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}`, { params: { host } }),

  grantPrivileges: (instanceId: number, username: string, data: { privileges: string; database?: string }, host: string = '%') =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}/grant`, data, { params: { host } }),

  resetUserPassword: (instanceId: number, username: string, data: { password: string }, host: string = '%') =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}/password`, data, { params: { host } }),

  // Database introspection
  listTables: (instanceId: number, dbName: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<{ name: string }>>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/tables`, { params: { page, page_size: pageSize } }),

  describeTable: (instanceId: number, dbName: string, table: string) =>
    api.get<ApiResponse<{ table_name: string; primary_key: string; collation: string; columns: Array<{ name: string; type: string; is_primary_key: boolean; is_nullable: boolean; is_auto_incr: boolean; default: string }> }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/describe`, { params: { table } }),

  // Table management
  createTable: (instanceId: number, dbName: string, data: { name: string; charset?: string; collation?: string; columns: Array<{ name: string; type: string; length?: string; nullable?: boolean; is_primary?: boolean; auto_incr?: boolean; unique?: boolean; default_value?: string }> }) =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/tables`, data),

  dropTable: (instanceId: number, dbName: string, table: string) =>
    api.delete<ApiResponse>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/tables`, { params: { table } }),

  queryTable: (instanceId: number, dbName: string, table: string, page: number = 1, pageSize: number = 50) =>
    api.get<ApiResponse<{ headers: string[]; column_types?: string[]; rows: (string | number | null)[][]; total: number; page: number; page_size: number }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/query`, { params: { table, page, page_size: pageSize } }),

  executeSQL: (instanceId: number, dbName: string, sql: string) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/execute`, { sql }),

  insertRecord: (instanceId: number, dbName: string, table: string, data: Record<string, string | number | null>) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/insert`, { table, data }),

  updateRecord: (instanceId: number, dbName: string, table: string, data: Record<string, string | number | null>, primaryKey: string, primaryVal: string | number) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/update`, { table, data, primary_key: primaryKey, primary_val: primaryVal }),

  deleteRecord: (instanceId: number, dbName: string, table: string, primaryKey: string, primaryVal: string | number) =>
    api.post<ApiResponse<{ success: boolean; error?: string }>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/delete`, { table, primary_key: primaryKey, primary_val: primaryVal }),

  // Backup management (scoped by instance + database name)
  createBackup: (instanceId: number, dbName: string) =>
    api.post<ApiResponse<DBBackup>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/backup`),

  listBackups: (instanceId: number, dbName: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<DBBackup>>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/backups`, { params: { page, page_size: pageSize } }),

  downloadBackup: (backupId: number) =>
    api.get(`/db/backups/${backupId}/download`, { responseType: 'blob' }),

  restoreBackup: (backupId: number) =>
    api.post<ApiResponse>(`/db/backups/${backupId}/restore`, { confirm: true }),

  deleteBackup: (backupId: number) =>
    api.delete<ApiResponse>(`/db/backups/${backupId}`),

  // Redis key browser (instance-scoped, addressed by logical DB index)
  redisDBCount: (instanceId: number) =>
    api.get<ApiResponse<{ databases: number }>>(`/db/redis/instances/${instanceId}/databases-count`),

  scanRedisKeys: (instanceId: number, db: number, cursor: number | string, pattern = '*', count = 50) =>
    api.get<ApiResponse<{ keys: RedisKey[]; next_cursor: number | string; db: number }>>(`/db/redis/instances/${instanceId}/keys`, { params: { db, cursor, pattern, count } }),

  getRedisValue: (instanceId: number, db: number, key: string) =>
    api.get<ApiResponse<RedisValue>>(`/db/redis/instances/${instanceId}/value`, { params: { db, key } }),

  setRedisValue: (instanceId: number, data: {
    db: number;
    type: 'string' | 'hash' | 'list' | 'set' | 'zset';
    key: string;
    value?: string;                                     // string
    hash_fields?: RedisHashField[];                     // hash
    values?: string[];                                  // list / set
    zset_members?: RedisZSetMember[];                   // zset
    ttl?: number;
  }) =>
    api.post<ApiResponse>(`/db/redis/instances/${instanceId}/value`, data),

  delRedisKeys: (instanceId: number, data: { db: number; keys: string[] }) =>
    api.post<ApiResponse<{ deleted: number }>>(`/db/redis/instances/${instanceId}/del`, data),

  expireRedisKey: (instanceId: number, data: { db: number; key: string; ttl: number }) =>
    api.post<ApiResponse>(`/db/redis/instances/${instanceId}/expire`, data),

  persistRedisKey: (instanceId: number, data: { db: number; key: string }) =>
    api.post<ApiResponse>(`/db/redis/instances/${instanceId}/persist`, data),

  flushRedisDB: (instanceId: number, data: { db: number }) =>
    api.post<ApiResponse>(`/db/redis/instances/${instanceId}/flushdb`, data),
};
