import axios from 'axios';
import type {
  ApiResponse, CronTask, CronRun, Script, ScriptLogLine,
  FirewallRule, FirewallStatus, FirewallRuleTemplate, FirewallLogEntry,
  DBBackup, User, Service, FileEntry, MonitorSnapshot, HistoryPoint,
  CloudInstance, CloudFirewallRule, Snapshot, TrafficInfo,
  WebServer, Website, DBInstance, Database, DBUser, RedisKey, RedisValue,
  RedisHashField, RedisZSetMember,
  SystemProcess, FileShare, ShareInfo, ShareFileEntry,
  ManagedServiceSpec,
  Notification, FileSearchResult,
  InstanceConfigView, AppSettings,
} from '../types';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
});

// 登录态走 HttpOnly Cookie（浏览自动携带），无需 JS 注入 token。
// 移动端等 header 场景由客户端自行附加，此处不处理。
api.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    if (error.response) {
      const { status, data } = error.response;

      if (status === 401) {
        // Cookie 失效/未登录 - don't redirect if already on login page
        if (!window.location.pathname.startsWith('/login')) {
          localStorage.removeItem('user');
          window.location.href = '/login';
        }
      }

      if (status === 429) {
        // Rate limit exceeded
        const msg = data?.message || '请求过于频繁，请稍后再试';
        import('antd').then(({ message }) => message.warning(msg));
      }

      // 后端错误响应统一为 { code, message, data }：把真实错误信息覆盖到
      // error.message 上，替代 axios 默认的 "Request failed with status code
      // NNN" 文案 —— 各 catch 块里的 error.message 直接显示后端消息，无需
      // 逐个调用点取 error.response.data.message。网络错误（无 response）
      // 不受影响，保持 "Network Error"/timeout 等原始信息。
      const serverMsg = data?.message;
      if (typeof serverMsg === 'string' && serverMsg) {
        error.message = serverMsg;
      }

      // blob 响应（下载/导出）出错时 body 仍是 JSON：读取后提取 message，
      // 否则这些调用点只能看到 "Request failed with status code NNN"。
      if (data instanceof Blob) {
        return data.text().then((text) => {
          try {
            const parsed = JSON.parse(text);
            if (parsed?.message) error.message = parsed.message;
          } catch { /* 非 JSON（如代理返回的 HTML 错误页）保持原样 */ }
          return Promise.reject(error);
        });
      }

      // Pass through original error so catch blocks can inspect error.response?.status
      return Promise.reject(error);
    }
    return Promise.reject(error);
  }
);

// Auth API
export const authApi = {
  login: (username: string, password: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User; must_change_pass: boolean; requires_totp?: boolean; temp_token?: string }>>('/auth/login', { username, password, turnstile_token: turnstileToken, client_type: 'web' }),

  logout: () =>
    api.post<ApiResponse>('/auth/logout'),

  getProfile: () =>
    api.get<ApiResponse<User>>('/auth/me'),

  changeUsername: (newUsername: string, password: string) =>
    api.put<ApiResponse<{ user: User; token?: string }>>('/auth/username', { new_username: newUsername, password }),

  changePassword: (oldPassword: string, newPassword: string) =>
    api.post<ApiResponse>('/auth/change-password', { old_password: oldPassword, new_password: newPassword }),

  // TOTP verification (login step 2)
  verifyTOTP: (tempToken: string, code: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User; must_change_pass: boolean }>>('/auth/verify-totp', { temp_token: tempToken, code, turnstile_token: turnstileToken, client_type: 'web' }),

  verifyBackupCode: (tempToken: string, backupCode: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User; must_change_pass: boolean }>>('/auth/verify-backup', { temp_token: tempToken, backup_code: backupCode, turnstile_token: turnstileToken, client_type: 'web' }),

  // TOTP setup (protected)
  setupTOTP: () =>
    api.post<ApiResponse<{ secret: string; otpauth_url: string }>>('/auth/totp/setup'),

  enableTOTP: (code: string) =>
    api.post<ApiResponse<{ backup_codes: string[] }>>('/auth/totp/enable', { code }),

  disableTOTP: (password: string) =>
    api.post<ApiResponse>('/auth/totp/disable', { password }),

  getTOTPStatus: () =>
    api.get<ApiResponse<{ enabled: boolean }>>('/auth/totp/status'),

  // Session management
  getSessions: () =>
    api.get<ApiResponse<Array<{ user_id: number; username: string; role: string; ip: string; user_agent: string; client_type: string; device_id?: string; device_info?: string; is_current: boolean; login_at: string; expires_at: string; token?: string }>>>('/auth/sessions'),

  kickSession: (token: string) =>
    api.post<ApiResponse>('/auth/sessions/kick', { token }),

  kickAllOtherSessions: () =>
    api.post<ApiResponse>('/auth/sessions/kick-all'),

  // Turnstile config (public: site key + enabled flows, no secret).
  getTurnstileConfig: () =>
    api.get<ApiResponse<{ site_key: string; enable_login: boolean; enable_qr_login: boolean; enable_public_share: boolean }>>('/auth/turnstile/config'),

  // Scan-to-login (QR). Web creates+p polls; mobile (authenticated) confirms.
  createQRSession: () =>
    api.post<ApiResponse<{ qr_token: string; expires_at: string }>>('/auth/qr/session'),

  getQRStatus: (qrToken: string) =>
    api.post<ApiResponse<{ status: string; expires_at: string; token?: string; user?: User; must_change_pass?: boolean }>>('/auth/qr/status', { qr_token: qrToken }),

  confirmQRLogin: (qrToken: string) =>
    api.post<ApiResponse<{ ok: boolean }>>('/auth/qr/confirm', { qr_token: qrToken }),

  cancelQRLogin: (qrToken: string) =>
    api.post<ApiResponse>('/auth/qr/cancel', { qr_token: qrToken }),
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
  list: (params?: { managed?: boolean }) =>
    api.get<ApiResponse<Service[]>>('/services', { params }),

  // 创建托管服务（生成 easyserver-svc-* unit）
  create: (data: ManagedServiceSpec) =>
    api.post<ApiResponse>('/services', data),

  getDetails: (names: string[]) =>
    api.post<ApiResponse<Service[]>>('/services/details', { names }),

  get: (name: string) =>
    api.get<ApiResponse<Service>>(`/services/${name}`),

  // 更新托管服务（:name 须为完整名 easyserver-svc-<name>）
  update: (name: string, data: ManagedServiceSpec) =>
    api.put<ApiResponse>(`/services/${name}`, data),

  // 删除托管服务（:name 须为完整名 easyserver-svc-<name>）
  delete: (name: string) =>
    api.delete<ApiResponse>(`/services/${name}`),

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

  archiveList: (path: string) =>
    api.get<ApiResponse<{ entries: Array<{ name: string; size: number; is_dir: boolean }> }>>('/files/archive-list', { params: { path } }),

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

// Monitor API (ports + port availability check)
export const systemApi = {
  getListeningPorts: () =>
    api.get<ApiResponse<{ ports: Array<{ protocol: string; port: number; local_addr: string; state: string; pid: number; process_name: string; user: string }>; total: number }>>('/monitor/ports'),

  checkPort: (port: number) =>
    api.get<ApiResponse<{ available: boolean; port: number; process?: string; message: string }>>('/monitor/check-port', { params: { port } }),
};

// SSH API
export interface SSHLoginRecord {
  time: string;
  user: string;
  ip: string;
  port: number;
  status: string; // success, failed
  method: string; // password, publickey
  tty: string;
}

export const sshApi = {
  getLogins: (limit?: number) =>
    api.get<ApiResponse<{ records: SSHLoginRecord[] }>>('/ssh/logins', { params: { limit } }),
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
    api.post<ApiResponse<FileShare>>('/shares', data),

  list: () =>
    api.get<ApiResponse<FileShare[]>>('/shares'),

  get: (id: number) =>
    api.get<ApiResponse<FileShare>>(`/shares/${id}`),

  update: (id: number, data: {
    password?: string | null;
    expires_at?: string;
    max_downloads?: number;
    clear_expiry?: boolean;
  }) =>
    api.put<ApiResponse<FileShare>>(`/shares/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/shares/${id}`),

  cleanupExpired: () =>
    api.post<ApiResponse<{ deleted: number }>>('/shares/cleanup'),
};

// Public share endpoints are now under /api/shares/public/...
// They may be accessed without auth (share recipients).
export const publicShareApi = {
  getShareInfo: (token: string) =>
    axios.get<ApiResponse<ShareInfo>>(`/api/shares/public/${token}/info`),
  getDownloadTicket: (token: string, password?: string) =>
    axios.post<ApiResponse<{ ticket: string }>>(`/api/shares/public/${token}/ticket`, { password }),
  listShareFiles: (token: string, ticket: string, subpath: string = '') =>
    axios.get<ApiResponse<ShareFileEntry[]>>(`/api/shares/public/${token}/list`, { params: { ticket, subpath } }),
};

// Website Security API (rate limit + IP ban)
export const websiteSecurityApi = {
  getConfig: (websiteId: number) =>
    api.get<ApiResponse<Record<string, unknown>>>(`/websites/${websiteId}/security/config`),

  updateConfig: (websiteId: number, data: Record<string, unknown>) =>
    api.put<ApiResponse<Record<string, unknown>>>(`/websites/${websiteId}/security/config`, data),

  listBanned: (websiteId: number) =>
    api.get<ApiResponse<Array<Record<string, unknown>>>>(`/websites/${websiteId}/security/banned`),

  ban: (websiteId: number, ip: string, reason: string, duration: number) =>
    api.post<ApiResponse>(`/websites/${websiteId}/security/ban`, { ip, reason, duration }),

  unban: (websiteId: number, banId: number) =>
    api.post<ApiResponse>(`/websites/${websiteId}/security/unban/${banId}`),
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
};

// Database Server API
export const dbServerApi = {
  // Instance lifecycle, scoped by engine enum.
  listInstances: (dbtype: string) =>
    api.get<ApiResponse<DBInstance[]>>(`/db/instances`, { params: { dbtype } }),

  createInstance: (dbtype: string, data: { version: string; image?: string; port?: number; container_engine?: string; bind_address?: string; container_name?: string }) =>
    api.post<ApiResponse<{ install_id: string; version: string; image: string; port: number; status: string }>>(`/db/instances`, { ...data, dbtype }),

  // Cancel an in-flight install (image pull or provisioning).
  cancelInstall: (iid: string) =>
    api.post<ApiResponse<null>>(`/db/installs/${iid}/cancel`),

  // Published Docker Hub tags for an engine's official image ("更多版本" flow),
  // paginated — the version Select flips pages through this.
  listDockerTags: (dbtype: string, page = 1, pageSize = 10) =>
    api.get<ApiResponse<{ items: string[]; total: number; page: number; page_size: number }>>(`/db/docker-tags`, { params: { dbtype, page, page_size: pageSize } }),

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
  listDatabases: (instanceId: number) =>
    api.get<ApiResponse<Database[]>>(`/db/instances/${instanceId}/databases`),

  createDatabase: (instanceId: number, data: { name: string; charset?: string; description?: string }) =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/databases`, data),

  deleteDatabase: (instanceId: number, dbName: string) =>
    api.delete<ApiResponse>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}`),

  // DB Users (instance-scoped; username + host for MySQL)
  listUsers: (instanceId: number) =>
    api.get<ApiResponse<DBUser[]>>(`/db/instances/${instanceId}/users`),

  createUser: (instanceId: number, data: { username: string; password: string; host?: string }) =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users`, data),

  deleteUser: (instanceId: number, username: string, host: string = '%') =>
    api.delete<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}`, { params: { host } }),

  grantPrivileges: (instanceId: number, username: string, data: { privileges: string; database?: string }, host: string = '%') =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}/grant`, data, { params: { host } }),

  resetUserPassword: (instanceId: number, username: string, data: { password: string }, host: string = '%') =>
    api.post<ApiResponse>(`/db/instances/${instanceId}/users/${encodeURIComponent(username)}/password`, data, { params: { host } }),

  // Database introspection
  listTables: (instanceId: number, dbName: string) =>
    api.get<ApiResponse<Array<{ name: string }>>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/tables`),

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

  listBackups: (instanceId: number, dbName: string) =>
    api.get<ApiResponse<DBBackup[]>>(`/db/instances/${instanceId}/databases/${encodeURIComponent(dbName)}/backups`),

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

// Cron task management
export const cronApi = {
  list: () =>
    api.get<ApiResponse<CronTask[]>>('/cron/tasks'),

  get: (name: string) =>
    api.get<ApiResponse<CronTask>>(`/cron/tasks/${name}`),

  create: (data: { name: string; command?: string; schedule: string; persistent?: boolean; description?: string; script_id?: number; timeout?: number; max_retry?: number; env_vars?: string; work_dir?: string; runtime?: string }) =>
    api.post<ApiResponse<CronTask>>('/cron/tasks', data),

  update: (name: string, data: { name?: string; command?: string; schedule?: string; persistent?: boolean; enabled?: boolean; description?: string; script_id?: number; timeout?: number; max_retry?: number; env_vars?: string; work_dir?: string; runtime?: string }) =>
    api.put<ApiResponse<CronTask>>(`/cron/tasks/${name}`, data),

  delete: (name: string) =>
    api.delete<ApiResponse>(`/cron/tasks/${name}`),

  enable: (name: string) =>
    api.post<ApiResponse>(`/cron/tasks/${name}/enable`),

  disable: (name: string) =>
    api.post<ApiResponse>(`/cron/tasks/${name}/disable`),

  run: (name: string) =>
    api.post<ApiResponse>(`/cron/tasks/${name}/run`),

  getRuns: (name: string, limit?: number) =>
    api.get<ApiResponse<CronRun[]>>(`/cron/tasks/${name}/runs`, { params: { limit: limit || 100 } }),

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

  // 运行中脚本 id 列表（刷新后显示「运行中」标记）
  getRunningScripts: () =>
    api.get<ApiResponse<number[]>>('/cron/scripts/running'),

  // 启动脚本执行（独立于 WS 订阅；已运行则复用）
  runScript: (id: number) =>
    api.post<ApiResponse>(`/cron/scripts/${id}/run`),

  // 停止运行中的脚本（列表「停止」按钮）
  stopScript: (id: number) =>
    api.post<ApiResponse>(`/cron/scripts/${id}/stop`),

  // 脚本的历史执行日志（journald，刷新后回看）
  getScriptLogs: (id: number, limit?: number) =>
    api.get<ApiResponse<ScriptLogLine[]>>(`/cron/scripts/${id}/logs`, { params: { limit: limit || 200 } }),

  // 脚本实时日志走 SSE（EventSource，HttpOnly cookie 同源鉴权），返回 SSE 相对路径
  scriptLogsStreamPath: (id: number) => `/api/cron/scripts/${id}/logs?stream=1`,
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
    api.get<ApiResponse<{ version: string; build_id?: string }>>('/settings/system'),

  checkUpdate: () =>
    api.get<ApiResponse<{
      current_version: string;
      latest_version: string;
      release_title: string;
      has_update: boolean;
      release_url: string;
      release_notes: string;
      published_at: string;
    }>>('/settings/check-update'),

  updateServer: (data: { port?: number; host?: string; domain?: string; force_domain?: boolean; allowed_origins?: string[]; max_upload_size?: number; assets_rate_limit?: number; assets_rate_interval?: string; turnstile?: { site_key?: string; secret_key?: string; enable_login?: boolean; enable_qr_login?: boolean; enable_public_share?: boolean } }) =>
    api.put<ApiResponse<{ requires_restart: boolean }>>('/settings/server', data),

  updateTLS: (data: { enabled: boolean; cert_content?: string; key_content?: string }) =>
    api.put<ApiResponse<{ requires_restart: boolean; cert_info: { domain: string; issuer: string; expires_at: string } | null }>>('/settings/tls', data),

  updateAuth: (data: { session_timeout?: number; idle_timeout?: number; max_login_attempts?: number; lockout_duration?: number; rate_limit?: number; rate_interval?: number; login_rate_limit?: number; login_rate_interval?: number; allow_multi_session?: boolean; ip_whitelist?: string[]; session_cleanup_interval?: number }) =>
    api.put<ApiResponse>('/settings/auth', data),

  updateMonitor: (data: { history_retention?: number; collect_interval?: number }) =>
    api.put<ApiResponse>('/settings/monitor', data),

  updateAudit: (data: { retention_days?: number }) =>
    api.put<ApiResponse>('/settings/audit', data),

  updateCloud: (data: { enabled?: boolean; secret_id?: string; secret_key?: string; region?: string; instance_id?: string }) =>
    api.put<ApiResponse>('/settings/cloud', data),

  testCloud: () =>
    api.post<ApiResponse<{ message: string; instance_count: number }>>('/settings/cloud/test'),

  restart: (force?: boolean) =>
    api.post<ApiResponse>('/settings/restart', force ? { force: true } : undefined),

  updateNotify: (data: { enabled?: boolean; webhook_url?: string }) =>
    api.put<ApiResponse>('/settings/notify', data),

  updateFeatures: (data: { fim?: boolean }) =>
    api.put<ApiResponse>('/settings/features', data),

  updateLogs: (data: { level?: string; format?: string; max_size_mb?: number; max_files?: number }) =>
    api.put<ApiResponse>('/settings/logs', data),

  testWebhook: () =>
    api.post<ApiResponse>('/settings/notify/test'),

  getAlertRules: () =>
    api.get<ApiResponse<{ rules: Array<{ name: string; metric: string; threshold: number; duration: number; enabled: boolean }> }>>('/alerts/rules'),

  updateAlertRules: (rules: Array<{ name: string; metric: string; threshold: number; duration: number; enabled: boolean }>) =>
    api.put<ApiResponse>('/alerts/rules', { rules }),
};

// Process Guardian API
// System Process API
export const systemProcessApi = {
  listProcesses: (params?: { sort_by?: string; order?: string; search?: string; limit?: number }) =>
    api.get<ApiResponse<SystemProcess[]>>('/monitor/processes', { params }),

  getProcess: (pid: number) =>
    api.get<ApiResponse<SystemProcess>>(`/monitor/processes/${pid}`),
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

export default api;
