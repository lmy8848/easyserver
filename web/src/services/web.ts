import api from './client';
import type { ApiResponse, Page, WebServer, Website } from '../types';

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
  list: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<WebServer>>>('/web-servers', { params: { page, page_size: pageSize } }),

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
  list: (serverId: number, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Website>>>(`/web-servers/${serverId}/websites`, { params: { page, page_size: pageSize } }),

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
