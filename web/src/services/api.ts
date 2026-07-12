import axios from 'axios';
import type {
  ApiResponse, CronTask, CronLog, Script, CronDoc,
  FirewallRule, FirewallStatus, FirewallRuleTemplate, FirewallLogEntry,
  DBBackup, User, Service, FileEntry, MonitorSnapshot, HistoryPoint,
  CloudInstance, CloudFirewallRule, Snapshot, TrafficInfo,
  WebServer, Website, DBServer, DBVersion, Database, DBUser,
  ManagedProcess, ProcessWithStatus, ProcessLog, ProcessGroup, ProcessStats, PaginatedData,
  SystemProcess, FileShare, ShareInfo,
  Notification, SSHLogin, SSHConfig, FileSearchResult,
  ConfigSection, ParamMeta, AppSettings,
} from '../types';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'X-Requested-With': 'XMLHttpRequest',
  },
});

// Request interceptor - add token
// SECURITY NOTE: Token is stored in localStorage for SPA compatibility.
// This is acceptable for single-admin panels but exposes token to XSS attacks.
// For multi-user production systems, consider migrating to httpOnly cookies.
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => {
    // Log all upload responses for debugging
    if (response.config.url?.includes('/upload')) {
      console.log('[API Response] URL:', response.config.url);
      console.log('[API Response] Status:', response.status, response.statusText);
      console.log('[API Response] Headers:', response.headers);
      console.log('[API Response] Data:', response.data);
    }
    return response;
  },
  (error) => {
    if (error.response) {
      const { status, data } = error.response;

      if (status === 401) {
        // Token expired or invalid - don't redirect if already on login page
        if (!window.location.pathname.startsWith('/login')) {
          localStorage.removeItem('token');
          localStorage.removeItem('user');
          window.location.href = '/login';
        }
      }

      if (status === 429) {
        // Rate limit exceeded
        const msg = data?.message || '请求过于频繁，请稍后再试';
        import('antd').then(({ message }) => message.warning(msg));
      }

      // Pass through original error so catch blocks can inspect error.response?.status
      return Promise.reject(error);
    }
    return Promise.reject(error);
  }
);

// Auth API
export const authApi = {
  login: (username: string, password: string) =>
    api.post<ApiResponse<{ token: string; user: User; must_change_pass: boolean; requires_totp?: boolean; temp_token?: string }>>('/auth/login', { username, password }),

  logout: () =>
    api.post<ApiResponse>('/auth/logout'),

  getProfile: () =>
    api.get<ApiResponse<User>>('/auth/me'),

  changePassword: (oldPassword: string, newPassword: string) =>
    api.post<ApiResponse>('/auth/change-password', { old_password: oldPassword, new_password: newPassword }),

  // TOTP verification (login step 2)
  verifyTOTP: (tempToken: string, code: string) =>
    api.post<ApiResponse<{ token: string; user: User; must_change_pass: boolean }>>('/auth/verify-totp', { temp_token: tempToken, code }),

  verifyBackupCode: (tempToken: string, backupCode: string) =>
    api.post<ApiResponse<{ token: string; user: User; must_change_pass: boolean }>>('/auth/verify-backup', { temp_token: tempToken, backup_code: backupCode }),

  // TOTP setup (protected)
  setupTOTP: () =>
    api.post<ApiResponse<{ secret: string; otpauth_url: string; qr_code_base64: string }>>('/auth/totp/setup'),

  enableTOTP: (code: string) =>
    api.post<ApiResponse<{ backup_codes: string[] }>>('/auth/totp/enable', { code }),

  disableTOTP: (password: string) =>
    api.post<ApiResponse>('/auth/totp/disable', { password }),

  getTOTPStatus: () =>
    api.get<ApiResponse<{ enabled: boolean }>>('/auth/totp/status'),

  // Session management
  getSessions: () =>
    api.get<ApiResponse<Array<{ user_id: number; username: string; role: string; ip: string; user_agent: string; login_at: string; expires_at: string; token?: string }>>>('/auth/sessions'),

  kickSession: (token: string) =>
    api.post<ApiResponse>('/auth/sessions/kick', { token }),

  kickAllOtherSessions: () =>
    api.post<ApiResponse>('/auth/sessions/kick-all'),
};

// Monitor API
export const monitorApi = {
  getStats: () =>
    api.get<ApiResponse<MonitorSnapshot>>('/monitor/stats'),

  getHistory: (start?: string, end?: string, signal?: AbortSignal) =>
    api.get<ApiResponse<{ points: HistoryPoint[] }>>('/monitor/history', { params: { start, end }, signal }),
};

// Service API
export const serviceApi = {
  list: () =>
    api.get<ApiResponse<Service[]>>('/services'),

  getDetails: (names: string[]) =>
    api.post<ApiResponse<Service[]>>('/services/details', { names }),

  get: (name: string) =>
    api.get<ApiResponse<Service>>(`/services/${name}`),

  start: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/start`),

  stop: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/stop`),

  restart: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/restart`),

  enable: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/enable`),

  disable: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/disable`),

  getLogs: (name: string, tail?: number) =>
    api.get<ApiResponse<{ lines: Array<{ time: string; message: string; priority: string }> }>>(`/services/${name}/logs`, { params: { tail } }),
};

// File API
const UPLOAD_TIMEOUT = 10 * 60 * 1000; // 10 minutes for large files
const UPLOAD_MAX_RETRIES = 2;

export const fileApi = {
  list: (path: string) =>
    api.get<ApiResponse<{ path: string; parent: string; entries: FileEntry[] }>>('/files', { params: { path } }),

  mkdir: (path: string) =>
    api.post<ApiResponse>('/files/mkdir', { path }),

  upload: (file: File, path: string, onProgress?: (percent: number) => void) => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('path', path);
    console.log('[API Upload] File:', { name: file.name, size: file.size, type: file.type });
    console.log('[API Upload] Path:', path);

    const doUpload = (attempt: number): Promise<any> => {
      if (attempt > 0) {
        console.log(`[API Upload] Retry attempt ${attempt}/${UPLOAD_MAX_RETRIES}`);
      }
      return api.post<ApiResponse>('/files/upload', formData, {
        timeout: UPLOAD_TIMEOUT,
        onUploadProgress: (progressEvent) => {
          if (progressEvent.total && onProgress) {
            const percent = Math.round((progressEvent.loaded * 100) / progressEvent.total);
            onProgress(percent);
          }
        },
      }).catch((error) => {
        // Retry on network/timeout errors, not on 4xx/5xx responses
        const isNetworkError = !error.response && (error.code === 'ECONNABORTED' || error.code === 'ERR_NETWORK' || error.message?.includes('timeout') || error.message?.includes('Network Error'));
        if (isNetworkError && attempt < UPLOAD_MAX_RETRIES) {
          console.log(`[API Upload] Network error, will retry: ${error.message}`);
          // Reset progress for retry
          if (onProgress) onProgress(0);
          return new Promise(resolve => setTimeout(resolve, 1000 * (attempt + 1)))
            .then(() => doUpload(attempt + 1));
        }
        throw error;
      });
    };

    return doUpload(0);
  },

  download: (path: string) =>
    api.get('/files/download', { params: { path }, responseType: 'blob' }),

  rename: (oldPath: string, newPath: string) =>
    api.put<ApiResponse>('/files/rename', { old_path: oldPath, new_path: newPath }),

  delete: (path: string, recursive?: boolean) =>
    api.delete<ApiResponse>('/files', { params: { path, recursive } }),

  move: (paths: string[], dest: string) =>
    api.post<ApiResponse>('/files/move', { paths, dest }),

  copy: (source: string, dest: string) =>
    api.post<ApiResponse>('/files/copy', { source, dest }),

  getContent: (path: string) =>
    api.get<ApiResponse<{ content: string }>>('/files/content', { params: { path } }),

  saveContent: (path: string, content: string) =>
    api.put<ApiResponse>('/files/content', { path, content }),

  // New file operations
  search: (path: string, query: string, limit?: number) =>
    api.get<ApiResponse<FileSearchResult[]>>('/files/search', { params: { path, q: query, limit } }),

  searchContent: (path: string, query: string, limit?: number) =>
    api.get<ApiResponse<FileSearchResult[]>>('/files/search-content', { params: { path, q: query, limit } }),

  getDetails: (path: string) =>
    api.get<ApiResponse<Record<string, unknown>>>('/files/details', { params: { path } }),

  getMimeType: (path: string) =>
    api.get<ApiResponse<{ path: string; mime_type: string }>>('/files/mime-type', { params: { path } }),

  compress: (sources: string[], dest: string) =>
    api.post<ApiResponse>('/files/compress', { sources, dest }),

  extract: (source: string, dest: string) =>
    api.post<ApiResponse>('/files/extract', { source, dest }),

  chmod: (path: string, mode: string) =>
    api.put<ApiResponse>('/files/chmod', { path, mode }),

  chown: (path: string, uid: number, gid: number) =>
    api.put<ApiResponse>('/files/chown', { path, uid, gid }),
};

// Cloud API
export const cloudApi = {
  getInstances: () =>
    api.get<ApiResponse<{ instances: CloudInstance[] }>>('/cloud/instances'),

  getInstance: (id: string) =>
    api.get<ApiResponse<CloudInstance>>(`/cloud/instances/${id}`),

  startInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/start`),

  stopInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/stop`),

  restartInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/restart`),

  getMonitor: (id: string, metric: string, start: string, end: string) =>
    api.get<ApiResponse<{ metric: string; points: Array<{ timestamp: string; value: number }> }>>(`/cloud/monitor/${id}`, { params: { metric, start, end } }),

  getFirewall: (id: string) =>
    api.get<ApiResponse<{ rules: CloudFirewallRule[] }>>(`/cloud/firewall/${id}`),

  addFirewallRule: (id: string, rule: Omit<CloudFirewallRule, 'rule_id'>) =>
    api.post<ApiResponse>(`/cloud/firewall/${id}`, rule),

  deleteFirewallRule: (id: string, ruleId: string) =>
    api.delete<ApiResponse>(`/cloud/firewall/${id}/${ruleId}`),

  getSnapshots: () =>
    api.get<ApiResponse<{ snapshots: Snapshot[] }>>('/cloud/snapshots'),

  createSnapshot: (instanceId: string, name: string) =>
    api.post<ApiResponse>('/cloud/snapshots', { instance_id: instanceId, name }),

  applySnapshot: (id: string) =>
    api.post<ApiResponse>(`/cloud/snapshots/${id}/apply`),

  getTraffic: () =>
    api.get<ApiResponse<TrafficInfo>>('/cloud/traffic'),
};

// System API
export const systemApi = {
  getListeningPorts: () =>
    api.get<ApiResponse<{ ports: Array<{ protocol: string; port: number; local_addr: string; state: string; pid: number; process_name: string; user: string }>; total: number }>>('/system/ports'),

  getSSHLogins: (limit?: number) =>
    api.get<ApiResponse<SSHLogin[]>>('/system/ssh-logins', { params: { limit } }),

  getSSHConfig: () =>
    api.get<ApiResponse<SSHConfig>>('/system/ssh-config'),

  checkPort: (port: number) =>
    api.get<ApiResponse<{ available: boolean; port: number; process?: string; message: string }>>('/system/check-port', { params: { port } }),

  checkPorts: (ports: number[]) =>
    api.get<ApiResponse<Array<{ port: number; available: boolean; message: string }>>>('/system/check-ports', { params: { ports: ports.join(',') } }),
};

// Audit Log API
export const auditApi = {
  list: (params?: {
    page?: number;
    page_size?: number;
    username?: string;
    action?: string;
    resource?: string;
    ip?: string;
    start_date?: string;
    end_date?: string;
    type?: string;
  }) => api.get<ApiResponse<{ total: number; items: Array<{ id: number; user_id: number; username: string; action: string; resource: string; detail: string; status: number; ip: string; user_agent: string; type: string; created_at: string }> }>>('/audit-logs', { params }),

  getActions: (type?: string) =>
    api.get<ApiResponse<string[]>>('/audit-logs/actions', { params: { type } }),

  getStats: (days?: number) =>
    api.get<ApiResponse<{
      user_stats: { username: string; count: number }[];
      action_stats: { action: string; count: number }[];
      day_stats: { day: string; count: number }[];
      status_stats: { status: string; count: number }[];
      alerts: { id: number; username: string; action: string; resource: string; status: number; ip: string; created_at: string }[];
    }>>('/audit-logs/stats', { params: { days } }),

  getCleanPolicy: () =>
    api.get<ApiResponse<{ retention_days: number; total_records: number; auto_clean: boolean }>>('/audit-logs/clean-policy'),

  export: (params?: {
    username?: string;
    action?: string;
    resource?: string;
    ip?: string;
    start_date?: string;
    end_date?: string;
    type?: string;
  }) => api.get('/audit-logs/export', { params, responseType: 'blob' }),

  clean: (days?: number) =>
    api.delete<ApiResponse<{ deleted: number }>>('/audit-logs/clean', { params: { days } }),
};

// File Share API
export const fileShareApi = {
  create: (data: { file_path: string; password?: string; expires_at?: string; max_downloads?: number }) =>
    api.post<ApiResponse<FileShare>>('/file-shares', data),

  list: () =>
    api.get<ApiResponse<FileShare[]>>('/file-shares'),

  get: (id: number) =>
    api.get<ApiResponse<FileShare>>(`/file-shares/${id}`),

  update: (id: number, data: {
    password?: string | null;
    expires_at?: string;
    max_downloads?: number;
    clear_expiry?: boolean;
  }) =>
    api.put<ApiResponse<FileShare>>(`/file-shares/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/file-shares/${id}`),

  cleanupExpired: () =>
    api.post<ApiResponse<{ deleted: number }>>('/file-shares/cleanup'),
};

// Public share endpoints live at the root (/share/:token/...), NOT under /api,
// and may be accessed without auth (share recipients). Use a bare axios call.
export const publicShareApi = {
  getInfo: (token: string) =>
    axios.get<ApiResponse<ShareInfo>>(`/share/${token}/info`),

  verify: (token: string, password: string) =>
    axios.post<ApiResponse<{ ok: boolean }>>(`/share/${token}/verify`, { password }),
};

// Web Server API
export const webServerApi = {
  list: () =>
    api.get<ApiResponse<WebServer[]>>('/web-servers'),

  get: (id: number) =>
    api.get<ApiResponse<WebServer>>(`/web-servers/${id}`),

  create: (data: Partial<WebServer>) =>
    api.post<ApiResponse>('/web-servers', data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/web-servers/${id}`),

  install: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/install`),

  uninstall: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/uninstall`),

  start: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/start`),

  stop: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/stop`),

  restart: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/restart`),

  status: (id: number) =>
    api.get<ApiResponse<{ status: string; version: string }>>(`/web-servers/${id}/status`),

  reload: (id: number) =>
    api.post<ApiResponse>(`/web-servers/${id}/reload`),

  testConfig: (id: number) =>
    api.get<ApiResponse<{ valid: boolean; message: string }>>(`/web-servers/${id}/test-config`),

  getConfig: (id: number) =>
    api.get<ApiResponse<{ content: string }>>(`/web-servers/${id}/config`),

  saveConfig: (id: number, content: string) =>
    api.put<ApiResponse>(`/web-servers/${id}/config`, { content }),

  getServiceLogs: (id: number, lines: number = 100) =>
    api.get<ApiResponse<{ logs: string }>>(`/web-servers/${id}/logs`, { params: { lines } }),

  setAutoStart: (id: number, enabled: boolean) =>
    api.post<ApiResponse>(`/web-servers/${id}/auto-start`, { enabled }),

  getProcessInfo: (id: number) =>
    api.get<ApiResponse<{ pid: number; memory_bytes: number; uptime: string }>>(`/web-servers/${id}/process`),

  browseDirs: (path: string) =>
    api.get<ApiResponse<{ current: string; entries: Array<{ name: string; path: string; is_dir: boolean; has_items: boolean; project: string }> }>>('/web-servers/browse', { params: { path } }),

  validatePath: (path: string) =>
    api.get<ApiResponse<{ valid: boolean; message: string; exists?: boolean; writable?: boolean; project?: string }>>('/web-servers/validate-path', { params: { path } }),

  getProjectTypes: () =>
    api.get<ApiResponse<Array<{ name: string; label: string; description: string; default_port: number; proxy: boolean }>>>('/web-servers/project-types'),
};

// Website API (nested under web server)
export const websiteApi = {
  list: (serverId: number) =>
    api.get<ApiResponse<Website[]>>(`/web-servers/${serverId}/websites`),

  get: (serverId: number, id: number) =>
    api.get<ApiResponse<Website>>(`/web-servers/${serverId}/websites/${id}`),

  create: (serverId: number, data: Partial<Website>) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites`, data),

  update: (serverId: number, id: number, data: Partial<Website>) =>
    api.put<ApiResponse>(`/web-servers/${serverId}/websites/${id}`, data),

  delete: (serverId: number, id: number) =>
    api.delete<ApiResponse>(`/web-servers/${serverId}/websites/${id}`),

  enable: (serverId: number, id: number) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/enable`),

  disable: (serverId: number, id: number) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/disable`),

  getLogs: (serverId: number, id: number, type: string = 'access', lines: number = 200) =>
    api.get<ApiResponse<{ logs: string; type: string }>>(`/web-servers/${serverId}/websites/${id}/logs`, { params: { type, lines } }),

  applySSL: (serverId: number, id: number, email?: string) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/ssl`, { email }),

  uploadSSL: (serverId: number, id: number, certContent: string, keyContent: string) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/ssl/upload`, { cert_content: certContent, key_content: keyContent }),

  build: (serverId: number, id: number) =>
    api.post<ApiResponse<{ success: boolean; output: string }>>(`/web-servers/${serverId}/websites/${id}/build`),

  startProcess: (serverId: number, id: number) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/process/start`),

  stopProcess: (serverId: number, id: number) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites/${id}/process/stop`),

  getProcessStatus: (serverId: number, id: number) =>
    api.get<ApiResponse<{ process_id: number; status: string; managed: boolean; process?: any }>>(`/web-servers/${serverId}/websites/${id}/process`),
};

// Database Server API
export const dbServerApi = {
  list: () =>
    api.get<ApiResponse<DBServer[]>>('/db-servers'),

  get: (id: number) =>
    api.get<ApiResponse<DBServer>>(`/db-servers/${id}`),

  // Version management
  getVersionTemplates: (id: number) =>
    api.get<ApiResponse<Array<{ version: string; package: string; description: string }>>>(`/db-servers/${id}/version-templates`),

  listVersions: (id: number) =>
    api.get<ApiResponse<DBVersion[]>>(`/db-servers/${id}/versions`),

  installVersion: (id: number, data: { version: string; port?: number }) =>
    api.post<ApiResponse>(`/db-servers/${id}/versions`, data),

  uninstallVersion: (vid: number) =>
    api.delete<ApiResponse>(`/db-servers/versions/${vid}`),

  startVersion: (vid: number) =>
    api.post<ApiResponse>(`/db-servers/versions/${vid}/start`),

  stopVersion: (vid: number) =>
    api.post<ApiResponse>(`/db-servers/versions/${vid}/stop`),

  restartVersion: (vid: number) =>
    api.post<ApiResponse>(`/db-servers/versions/${vid}/restart`),

  updateVersionPort: (vid: number, port: number) =>
    api.put<ApiResponse>(`/db-servers/versions/${vid}/port`, { port }),

  getVersionLogs: (vid: number, lines: number = 200) =>
    api.get<ApiResponse<{ logs: string }>>(`/db-servers/versions/${vid}/logs`, { params: { lines } }),

  // Databases
  listDatabases: (serverId: number) =>
    api.get<ApiResponse<Database[]>>(`/db-servers/${serverId}/databases`),

  createDatabase: (serverId: number, data: { name: string; charset?: string; description?: string }) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/databases`, data),

  deleteDatabase: (serverId: number, dbId: number) =>
    api.delete<ApiResponse>(`/db-servers/${serverId}/databases/${dbId}`),

  // DB Users
  listUsers: (serverId: number) =>
    api.get<ApiResponse<DBUser[]>>(`/db-servers/${serverId}/users`),

  createUser: (serverId: number, data: { username: string; password: string; host?: string }) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/users`, data),

  deleteUser: (serverId: number, userId: number) =>
    api.delete<ApiResponse>(`/db-servers/${serverId}/users/${userId}`),

  grantPrivileges: (serverId: number, userId: number, data: { privileges: string; database?: string }) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/users/${userId}/grant`, data),

  // Database introspection
  listTables: (dbId: number) =>
    api.get<ApiResponse<Array<{ name: string }>>>(`/db-servers/databases/${dbId}/tables`),

  describeTable: (dbId: number, table: string) =>
    api.get<ApiResponse<{ table_name: string; primary_key: string; columns: Array<{ name: string; type: string; is_primary_key: boolean; is_nullable: boolean; is_auto_incr: boolean; default: string }> }>>(`/db-servers/databases/${dbId}/describe`, { params: { table } }),

  // Table management
  createTable: (dbId: number, data: { name: string; columns: Array<{ name: string; type: string; nullable?: boolean; is_primary?: boolean; auto_incr?: boolean }> }) =>
    api.post<ApiResponse>(`/db-servers/databases/${dbId}/tables`, data),

  dropTable: (dbId: number, table: string) =>
    api.delete<ApiResponse>(`/db-servers/databases/${dbId}/tables`, { params: { table } }),

  queryTable: (dbId: number, table: string, page: number = 1, pageSize: number = 50) =>
    api.get<ApiResponse<{ headers: string[]; rows: (string | number | null)[][]; total: number; page: number; page_size: number }>>(`/db-servers/databases/${dbId}/query`, { params: { table, page, page_size: pageSize } }),

  executeSQL: (dbId: number, sql: string) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/execute`, { sql }),

  insertRecord: (dbId: number, table: string, data: Record<string, string | number | null>) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/insert`, { table, data }),

  updateRecord: (dbId: number, table: string, data: Record<string, string | number | null>, primaryKey: string, primaryVal: string | number) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/update`, { table, data, primary_key: primaryKey, primary_val: primaryVal }),

  deleteRecord: (dbId: number, table: string, primaryKey: string, primaryVal: string | number) =>
    api.post<ApiResponse<{ success: boolean; error?: string }>>(`/db-servers/databases/${dbId}/delete`, { table, primary_key: primaryKey, primary_val: primaryVal }),

  // MySQL config management
  getMySQLConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: { file_path: string; sections: ConfigSection[] }; sections?: Record<string, { params: Record<string, string>; meta: ParamMeta[] }> }>>('/db-servers/mysql/config'),

  saveMySQLConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/mysql/config', { sections }),

  getMySQLCommonParams: (section: string = 'mysqld') =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/mysql/common-params', { params: { section } }),

  // PostgreSQL config management
  getPostgreSQLConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: { file_path: string; sections: ConfigSection[] }; sections?: Record<string, { params: Record<string, string>; meta: ParamMeta[] }> }>>('/db-servers/postgresql/config'),

  savePostgreSQLConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/postgresql/config', { sections }),

  getPGCommonParams: () =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/postgresql/common-params'),

  // Redis config management
  getRedisConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: { file_path: string; sections: ConfigSection[] }; sections?: Record<string, { params: Record<string, string>; meta: ParamMeta[] }> }>>('/db-servers/redis/config'),

  saveRedisConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/redis/config', { sections }),

  getRedisCommonParams: () =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/redis/common-params'),

  // Backup management
  createBackup: (dbId: number) =>
    api.post<ApiResponse<DBBackup>>(`/db-servers/databases/${dbId}/backup`),

  listBackups: (dbId: number) =>
    api.get<ApiResponse<DBBackup[]>>(`/db-servers/databases/${dbId}/backups`),

  downloadBackup: (backupId: number) =>
    api.get(`/db-servers/backups/${backupId}/download`, { responseType: 'blob' }),

  restoreBackup: (backupId: number) =>
    api.post<ApiResponse>(`/db-servers/backups/${backupId}/restore`, { confirm: true }),

  deleteBackup: (backupId: number) =>
    api.delete<ApiResponse>(`/db-servers/backups/${backupId}`),
};

// Cron task management
export const cronApi = {
  getPresets: () =>
    api.get<ApiResponse<Array<{ label: string; value: string; description: string }>>>('/cron/presets'),

  describeSchedule: (schedule: string) =>
    api.get<ApiResponse<{ description: string }>>('/cron/describe', { params: { schedule } }),

  getNextRuns: (schedule: string) =>
    api.get<ApiResponse<{ next_runs: string[] }>>('/cron/next-runs', { params: { schedule } }),

  list: () =>
    api.get<ApiResponse<CronTask[]>>('/cron/tasks'),

  get: (id: number) =>
    api.get<ApiResponse<CronTask>>(`/cron/tasks/${id}`),

  create: (data: { name: string; command?: string; schedule: string; description?: string; script_id?: number; timeout?: number; max_retry?: number; env_vars?: string; work_dir?: string }) =>
    api.post<ApiResponse<CronTask>>('/cron/tasks', data),

  update: (id: number, data: { name?: string; command?: string; schedule?: string; description?: string; script_id?: number; timeout?: number; max_retry?: number; env_vars?: string; work_dir?: string }) =>
    api.put<ApiResponse<CronTask>>(`/cron/tasks/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/cron/tasks/${id}`),

  enable: (id: number) =>
    api.post<ApiResponse>(`/cron/tasks/${id}/enable`),

  disable: (id: number) =>
    api.post<ApiResponse>(`/cron/tasks/${id}/disable`),

  run: (id: number) =>
    api.post<ApiResponse>(`/cron/tasks/${id}/run`),

  getLogs: (id: number, limit?: number) =>
    api.get<ApiResponse<CronLog[]>>(`/cron/tasks/${id}/logs`, { params: { limit: limit || 50 } }),

  // Scripts
  listScripts: () =>
    api.get<ApiResponse<Script[]>>('/cron/scripts'),

  getScript: (id: number) =>
    api.get<ApiResponse<Script>>(`/cron/scripts/${id}`),

  createScript: (data: { name: string; description?: string; content: string; language?: string }) =>
    api.post<ApiResponse<Script>>('/cron/scripts', data),

  updateScript: (id: number, data: { name?: string; description?: string; content?: string; language?: string }) =>
    api.put<ApiResponse<Script>>(`/cron/scripts/${id}`, data),

  deleteScript: (id: number) =>
    api.delete<ApiResponse>(`/cron/scripts/${id}`),

  // Docs
  listDocs: () =>
    api.get<ApiResponse<CronDoc[]>>('/cron/docs'),

  getDoc: (id: number) =>
    api.get<ApiResponse<CronDoc>>(`/cron/docs/${id}`),

  createDoc: (data: { title: string; content: string; sort_order?: number }) =>
    api.post<ApiResponse<CronDoc>>('/cron/docs', data),

  updateDoc: (id: number, data: { title?: string; content?: string; sort_order?: number }) =>
    api.put<ApiResponse<CronDoc>>(`/cron/docs/${id}`, data),

  deleteDoc: (id: number) =>
    api.delete<ApiResponse>(`/cron/docs/${id}`),
};

// Firewall management
export const firewallApi = {
  getStatus: () =>
    api.get<ApiResponse<FirewallStatus>>('/firewall/status'),

  enable: () =>
    api.post<ApiResponse>('/firewall/enable'),

  disable: () =>
    api.post<ApiResponse>('/firewall/disable', { confirm: true }),

  listRules: () =>
    api.get<ApiResponse<FirewallRule[]>>('/firewall/rules'),

  getRule: (id: number) =>
    api.get<ApiResponse<FirewallRule>>(`/firewall/rules/${id}`),

  createRule: (data: { chain: string; protocol?: string; port?: string; action: string; source?: string; ip_version?: string; remark?: string }) =>
    api.post<ApiResponse<FirewallRule>>('/firewall/rules', data),

  updateRule: (id: number, data: { chain?: string; protocol?: string; port?: string; action?: string; source?: string; ip_version?: string; remark?: string }) =>
    api.put<ApiResponse<FirewallRule>>(`/firewall/rules/${id}`, data),

  deleteRule: (id: number) =>
    api.delete<ApiResponse>(`/firewall/rules/${id}`),

  enableRule: (id: number) =>
    api.post<ApiResponse>(`/firewall/rules/${id}/enable`),

  disableRule: (id: number) =>
    api.post<ApiResponse>(`/firewall/rules/${id}/disable`),

  moveRuleUp: (id: number) =>
    api.post<ApiResponse>(`/firewall/rules/${id}/move-up`),

  moveRuleDown: (id: number) =>
    api.post<ApiResponse>(`/firewall/rules/${id}/move-down`),

  bulkEnableRules: (ids: number[]) =>
    api.post<ApiResponse<{ succeeded: number; failed: number; errors: string[] }>>('/firewall/rules/bulk-enable', { ids }),

  bulkDisableRules: (ids: number[]) =>
    api.post<ApiResponse<{ succeeded: number; failed: number; errors: string[] }>>('/firewall/rules/bulk-disable', { ids }),

  bulkDeleteRules: (ids: number[]) =>
    api.post<ApiResponse<{ succeeded: number; failed: number; errors: string[] }>>('/firewall/rules/bulk-delete', { ids }),

  getSystemRules: () =>
    api.get<ApiResponse<FirewallRule[]>>('/firewall/system-rules'),

  deleteSystemRule: (rule: FirewallRule) =>
    api.post<ApiResponse>('/firewall/system-rules/delete', rule),

  setDefaultPolicy: (data: { chain: string; policy: string }) =>
    api.post<ApiResponse>('/firewall/default-policy', data),

  getTemplates: () =>
    api.get<ApiResponse<FirewallRuleTemplate[]>>('/firewall/templates'),

  applyTemplate: (name: string) =>
    api.post<ApiResponse<FirewallRule>>('/firewall/templates/apply', { name }),

  exportRules: () =>
    api.get('/firewall/rules/export', { responseType: 'blob' as const }),

  importRules: (data: { version: number; exported_at: string; rules: Array<{ chain: string; protocol: string; port: string; action: string; source: string; remark: string }> }) =>
    api.post<ApiResponse<{ succeeded: number; failed: number; errors: string[] }>>('/firewall/rules/import', data),

  getLogs: (lines?: number) =>
    api.get<ApiResponse<FirewallLogEntry[]>>('/firewall/logs', { params: { lines } }),
};

// Settings API
export const settingsApi = {
  get: () =>
    api.get<ApiResponse<AppSettings>>('/settings'),

  getSystem: () =>
    api.get<ApiResponse<{ version: string }>>('/settings/system'),

  updateServer: (data: { port?: number; host?: string; serve_frontend?: boolean; domain?: string; redirect_mode?: string; www_handling?: string; max_upload_size?: number; assets_rate_limit?: number; assets_rate_interval?: string }) =>
    api.put<ApiResponse<{ requires_restart: boolean }>>('/settings/server', data),

  updateTLS: (data: { enabled: boolean; cert_content?: string; key_content?: string }) =>
    api.put<ApiResponse<{ requires_restart: boolean; cert_info: { domain: string; issuer: string; expires_at: string } | null }>>('/settings/tls', data),

  updateAuth: (data: { session_timeout?: string; idle_timeout?: string; max_login_attempts?: number; lockout_duration?: string; rate_limit?: number; rate_interval?: string; login_rate_limit?: number; login_rate_interval?: string }) =>
    api.put<ApiResponse>('/settings/auth', data),

  updateMonitor: (data: { history_retention?: string; collect_interval?: string }) =>
    api.put<ApiResponse>('/settings/monitor', data),

  updateAudit: (data: { enabled?: boolean }) =>
    api.put<ApiResponse>('/settings/audit', data),

  updateCloud: (data: { enabled?: boolean; secret_id?: string; secret_key?: string; region?: string; instance_id?: string }) =>
    api.put<ApiResponse>('/settings/cloud', data),

  testCloud: () =>
    api.post<ApiResponse<{ message: string; instance_count: number }>>('/settings/cloud/test'),

  restart: (force?: boolean) =>
    api.post<ApiResponse>('/settings/restart', force ? { force: true } : undefined),

  updateNotify: (data: { enabled?: boolean; webhook_url?: string }) =>
    api.put<ApiResponse>('/settings/notify', data),

  testWebhook: () =>
    api.post<ApiResponse>('/settings/notify/test'),

  getAlertRules: () =>
    api.get<ApiResponse<{ rules: Array<{ name: string; metric: string; threshold: number; duration: number; enabled: boolean }> }>>('/alerts/rules'),

  updateAlertRules: (rules: Array<{ name: string; metric: string; threshold: number; duration: number; enabled: boolean }>) =>
    api.put<ApiResponse>('/alerts/rules', { rules }),
};

// Process Guardian API
export const processApi = {
  list: () =>
    api.get<ApiResponse<ProcessWithStatus[]>>('/processes'),

  get: (id: number) =>
    api.get<ApiResponse<ProcessWithStatus>>(`/processes/${id}`),

  create: (data: Partial<ManagedProcess>) =>
    api.post<ApiResponse>('/processes', data),

  update: (id: number, data: Partial<ManagedProcess>) =>
    api.put<ApiResponse>(`/processes/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/processes/${id}`),

  start: (id: number) =>
    api.post<ApiResponse>(`/processes/${id}/start`),

  stop: (id: number) =>
    api.post<ApiResponse>(`/processes/${id}/stop`),

  restart: (id: number) =>
    api.post<ApiResponse>(`/processes/${id}/restart`),

  getLogs: (id: number, limit = 50, offset = 0) =>
    api.get<ApiResponse<PaginatedData<ProcessLog>>>(`/processes/${id}/logs`, { params: { limit, offset } }),

  getStats: (id: number) =>
    api.get<ApiResponse<ProcessStats>>(`/processes/${id}/stats`),

  batchStart: (ids: number[]) =>
    api.post<ApiResponse>('/processes/batch/start', { ids }),

  batchStop: (ids: number[]) =>
    api.post<ApiResponse>('/processes/batch/stop', { ids }),

  batchRestart: (ids: number[]) =>
    api.post<ApiResponse>('/processes/batch/restart', { ids }),

  listGroups: () =>
    api.get<ApiResponse<ProcessGroup[]>>('/process-groups'),

  createGroup: (data: { name: string; description?: string }) =>
    api.post<ApiResponse>('/process-groups', data),

  updateGroup: (id: number, data: { name?: string; description?: string }) =>
    api.put<ApiResponse>(`/process-groups/${id}`, data),

  deleteGroup: (id: number) =>
    api.delete<ApiResponse>(`/process-groups/${id}`),

  export: () =>
    api.get<ApiResponse<ManagedProcess[]>>('/processes/export'),

  import: (processes: ManagedProcess[]) =>
    api.post<ApiResponse>('/processes/import', processes),
};

// System Process API
export const systemProcessApi = {
  listProcesses: (params?: { sort_by?: string; order?: string; search?: string; limit?: number }) =>
    api.get<ApiResponse<SystemProcess[]>>('/system/processes', { params }),

  getProcess: (pid: number) =>
    api.get<ApiResponse<SystemProcess>>(`/system/processes/${pid}`),
};

// Notification API
export const notificationApi = {
  list: (unreadOnly = false, limit = 50) =>
    api.get<ApiResponse<Notification[]>>('/notifications', { params: { unread: unreadOnly, limit } }),

  unreadCount: () =>
    api.get<ApiResponse<{ count: number }>>('/notifications/unread-count'),

  create: (data: { type: string; title: string; message: string; level?: string }) =>
    api.post<ApiResponse>('/notifications', data),

  markAsRead: (id: number) =>
    api.put<ApiResponse>(`/notifications/${id}/read`),

  markAllAsRead: () =>
    api.put<ApiResponse>('/notifications/read-all'),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/notifications/${id}`),
};

// Template API
export const templateApi = {
  getDockerImages: () =>
    api.get<ApiResponse<{ categories: Array<{ name: string; description: string; images: Array<{ name: string; tag: string; description: string }> }> }>>('/templates/docker-images'),

  getScriptTemplates: () =>
    api.get<ApiResponse<{ categories: Array<{ name: string; description: string; templates: Array<{ name: string; description: string; content: string }> }> }>>('/templates/scripts'),
};

export default api;
