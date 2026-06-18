import axios from 'axios';
import type { ApiResponse } from '../types';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor - add token
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
    return response;
  },
  (error) => {
    if (error.response) {
      const { status, data } = error.response;

      if (status === 401) {
        // Token expired or invalid
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        window.location.href = '/login';
      }

      return Promise.reject(data);
    }
    return Promise.reject(error);
  }
);

// Auth API
export const authApi = {
  login: (username: string, password: string) =>
    api.post<ApiResponse<{ token: string; user: any; must_change_pass: boolean }>>('/auth/login', { username, password }),

  logout: () =>
    api.post<ApiResponse>('/auth/logout'),

  getProfile: () =>
    api.get<ApiResponse<any>>('/auth/me'),

  changePassword: (oldPassword: string, newPassword: string) =>
    api.post<ApiResponse>('/auth/change-password', { old_password: oldPassword, new_password: newPassword }),
};

// Monitor API
export const monitorApi = {
  getStats: () =>
    api.get<ApiResponse<any>>('/monitor/stats'),

  getHistory: (start?: string, end?: string) =>
    api.get<ApiResponse<{ points: any[] }>>('/monitor/history', { params: { start, end } }),
};

// Service API
export const serviceApi = {
  list: () =>
    api.get<ApiResponse<any[]>>('/services'),

  get: (name: string) =>
    api.get<ApiResponse<any>>(`/services/${name}`),

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
    api.get<ApiResponse<{ lines: any[] }>>(`/services/${name}/logs`, { params: { tail } }),
};

// File API
export const fileApi = {
  getBasePath: () =>
    api.get<ApiResponse<{ base_path: string }>>('/files/base-path'),

  list: (path: string) =>
    api.get<ApiResponse<{ entries: any[] }>>('/files', { params: { path } }),

  mkdir: (path: string) =>
    api.post<ApiResponse>('/files/mkdir', { path }),

  upload: (file: File, path: string) => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('path', path);
    return api.post<ApiResponse>('/files/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
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
    api.get<ApiResponse<any[]>>('/files/search', { params: { path, q: query, limit } }),

  searchContent: (path: string, query: string, limit?: number) =>
    api.get<ApiResponse<any[]>>('/files/search-content', { params: { path, q: query, limit } }),

  getDetails: (path: string) =>
    api.get<ApiResponse<any>>('/files/details', { params: { path } }),

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

// User API
export const userApi = {
  list: (role?: string) =>
    api.get<ApiResponse<any[]>>('/users', { params: { role } }),

  create: (username: string, password: string, role: string) =>
    api.post<ApiResponse>('/users', { username, password, role }),

  update: (id: number, data: { role?: string; is_locked?: boolean }) =>
    api.put<ApiResponse>(`/users/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/users/${id}`),

  unlock: (id: number) =>
    api.post<ApiResponse>(`/users/${id}/unlock`),

  resetPassword: (id: number, password: string) =>
    api.post<ApiResponse>(`/users/${id}/reset-password`, { password }),

  getActivities: (id: number, limit?: number) =>
    api.get<ApiResponse<any[]>>(`/users/${id}/activities`, { params: { limit } }),

  getAllActivities: (limit?: number) =>
    api.get<ApiResponse<any[]>>('/users/activities', { params: { limit } }),

  setExpiry: (id: number, expiresAt: string | null) =>
    api.put<ApiResponse>(`/users/${id}/expiry`, { expires_at: expiresAt }),

  setIPWhitelist: (id: number, ipWhitelist: string) =>
    api.put<ApiResponse>(`/users/${id}/ip-whitelist`, { ip_whitelist: ipWhitelist }),

  getSessions: () =>
    api.get<ApiResponse<any[]>>('/users/sessions'),
};

// Cloud API
export const cloudApi = {
  getInstances: () =>
    api.get<ApiResponse<{ instances: any[] }>>('/cloud/instances'),

  getInstance: (id: string) =>
    api.get<ApiResponse<any>>(`/cloud/instances/${id}`),

  startInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/start`),

  stopInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/stop`),

  restartInstance: (id: string) =>
    api.post<ApiResponse>(`/cloud/instances/${id}/restart`),

  getMonitor: (id: string, metric: string, start: string, end: string) =>
    api.get<ApiResponse<any>>(`/cloud/monitor/${id}`, { params: { metric, start, end } }),

  getFirewall: (id: string) =>
    api.get<ApiResponse<{ rules: any[] }>>(`/cloud/firewall/${id}`),

  addFirewallRule: (id: string, rule: any) =>
    api.post<ApiResponse>(`/cloud/firewall/${id}`, rule),

  deleteFirewallRule: (id: string, ruleId: string) =>
    api.delete<ApiResponse>(`/cloud/firewall/${id}/${ruleId}`),

  getSnapshots: () =>
    api.get<ApiResponse<{ snapshots: any[] }>>('/cloud/snapshots'),

  createSnapshot: (instanceId: string, name: string) =>
    api.post<ApiResponse>('/cloud/snapshots', { instance_id: instanceId, name }),

  applySnapshot: (id: string) =>
    api.post<ApiResponse>(`/cloud/snapshots/${id}/apply`),

  getTraffic: () =>
    api.get<ApiResponse<any>>('/cloud/traffic'),
};

// System API
export const systemApi = {
  getSSHLogins: (limit?: number) =>
    api.get<ApiResponse<any[]>>('/system/ssh-logins', { params: { limit } }),

  getSSHConfig: () =>
    api.get<ApiResponse<any>>('/system/ssh-config'),

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
  }) => api.get<ApiResponse<{ total: number; items: any[] }>>('/audit-logs', { params }),

  getActions: () =>
    api.get<ApiResponse<string[]>>('/audit-logs/actions'),

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
  }) => api.get('/audit-logs/export', { params, responseType: 'blob' }),

  clean: (days?: number) =>
    api.delete<ApiResponse<{ deleted: number }>>('/audit-logs/clean', { params: { days } }),
};

// Web Server API
export const webServerApi = {
  list: () =>
    api.get<ApiResponse<any[]>>('/web-servers'),

  get: (id: number) =>
    api.get<ApiResponse<any>>(`/web-servers/${id}`),

  create: (data: any) =>
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
    api.get<ApiResponse<any[]>>(`/web-servers/${serverId}/websites`),

  get: (serverId: number, id: number) =>
    api.get<ApiResponse<any>>(`/web-servers/${serverId}/websites/${id}`),

  create: (serverId: number, data: any) =>
    api.post<ApiResponse>(`/web-servers/${serverId}/websites`, data),

  update: (serverId: number, id: number, data: any) =>
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
};

// Database Server API
export const dbServerApi = {
  list: () =>
    api.get<ApiResponse<any[]>>('/db-servers'),

  get: (id: number) =>
    api.get<ApiResponse<any>>(`/db-servers/${id}`),

  // Version management
  getVersionTemplates: (id: number) =>
    api.get<ApiResponse<Array<{ version: string; package: string; description: string }>>>(`/db-servers/${id}/version-templates`),

  listVersions: (id: number) =>
    api.get<ApiResponse<any[]>>(`/db-servers/${id}/versions`),

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
    api.get<ApiResponse<any[]>>(`/db-servers/${serverId}/databases`),

  createDatabase: (serverId: number, data: any) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/databases`, data),

  deleteDatabase: (serverId: number, dbId: number) =>
    api.delete<ApiResponse>(`/db-servers/${serverId}/databases/${dbId}`),

  // DB Users
  listUsers: (serverId: number) =>
    api.get<ApiResponse<any[]>>(`/db-servers/${serverId}/users`),

  createUser: (serverId: number, data: any) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/users`, data),

  deleteUser: (serverId: number, userId: number) =>
    api.delete<ApiResponse>(`/db-servers/${serverId}/users/${userId}`),

  grantPrivileges: (serverId: number, userId: number, data: any) =>
    api.post<ApiResponse>(`/db-servers/${serverId}/users/${userId}/grant`, data),

  // Database introspection
  listTables: (dbId: number) =>
    api.get<ApiResponse<Array<{ name: string }>>>(`/db-servers/databases/${dbId}/tables`),

  describeTable: (dbId: number, table: string) =>
    api.get<ApiResponse<Array<{ name: string; type: string; null?: string; key?: string; default?: string }>>>(`/db-servers/databases/${dbId}/describe`, { params: { table } }),

  queryTable: (dbId: number, table: string, page: number = 1, pageSize: number = 50) =>
    api.get<ApiResponse<{ headers: string[]; rows: any[][]; total: number; page: number; page_size: number }>>(`/db-servers/databases/${dbId}/query`, { params: { table, page, page_size: pageSize } }),

  executeSQL: (dbId: number, sql: string) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/execute`, { sql }),

  insertRecord: (dbId: number, table: string, data: Record<string, any>) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/insert`, { table, data }),

  updateRecord: (dbId: number, table: string, data: Record<string, any>, primaryKey: string, primaryVal: any) =>
    api.post<ApiResponse<{ success: boolean; output?: string; error?: string }>>(`/db-servers/databases/${dbId}/update`, { table, data, primary_key: primaryKey, primary_val: primaryVal }),

  deleteRecord: (dbId: number, table: string, primaryKey: string, primaryVal: any) =>
    api.post<ApiResponse<{ success: boolean; error?: string }>>(`/db-servers/databases/${dbId}/delete`, { table, primary_key: primaryKey, primary_val: primaryVal }),

  // MySQL config management
  getMySQLConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: any; sections?: Record<string, { params: Record<string, string>; meta: any[] }> }>>('/db-servers/mysql/config'),

  saveMySQLConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/mysql/config', { sections }),

  getMySQLCommonParams: (section: string = 'mysqld') =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/mysql/common-params', { params: { section } }),

  // PostgreSQL config management
  getPostgreSQLConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: any; sections?: Record<string, { params: Record<string, string>; meta: any[] }> }>>('/db-servers/postgresql/config'),

  savePostgreSQLConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/postgresql/config', { sections }),

  getPGCommonParams: () =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/postgresql/common-params'),

  // Redis config management
  getRedisConfig: () =>
    api.get<ApiResponse<{ found: boolean; config?: any; sections?: Record<string, { params: Record<string, string>; meta: any[] }> }>>('/db-servers/redis/config'),

  saveRedisConfig: (sections: Array<{ name: string; params: Record<string, string> }>) =>
    api.post<ApiResponse>('/db-servers/redis/config', { sections }),

  getRedisCommonParams: () =>
    api.get<ApiResponse<Array<{ key: string; label: string; description: string; type: string; unit?: string; options?: string[]; default: string }>>>('/db-servers/redis/common-params'),
};

export default api;
