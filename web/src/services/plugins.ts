import api from './client';
import type { ApiResponse, PluginRecord, PluginMenuItem, PluginSource, PluginSourceCatalog } from '../types';

// 插件详情（含贡献点与 manifest）
export interface PluginDetail {
  record: {
    id: number; name: string; version: string; api_version: number; enabled: boolean;
    status: string; pid: number; restart_count: number; last_error: string;
    permissions_approved: string[];
  };
  manifest: {
    name: string; version: string; description: string;
    permissions: string[];
    runtime?: { exec: string; socket: string; startup_timeout?: string; request_timeout?: string };
    health?: { path: string; interval: string; fail_threshold: number };
  };
  contributions: {
    menus?: Array<{ id: string; title: string; icon: string; path: string; order: number }>;
    pages?: Array<{ id: string; path: string }>;
    routes?: Array<{ id: string; prefix: string }>;
    widgets?: Array<{ id: string; title: string; slot: string; path: string; size: string }>;
    services?: Array<{ id: string; schedule: string; command: string }>;
    eventsubs?: Array<{ event: string }>;
    eventpubs?: Array<{ event: string }>;
    settings?: Array<{ id: string; title: string }>;
  };
}

// 插件管理
export interface PluginWidgetItem {
  plugin: string;
  id: string;
  title: string;
  slot: string;
  path: string;
  size: string;
}

export const pluginsApi = {
  list: () => api.get<ApiResponse<{ items: PluginRecord[] }>>('/plugins'),

  start: (name: string) => api.post<ApiResponse>(`/plugins/${name}/start`),

  stop: (name: string) => api.post<ApiResponse>(`/plugins/${name}/stop`),

  enable: (name: string) => api.post<ApiResponse>(`/plugins/${name}/enable`),

  // 启用时权限审批：批准声明权限的子集后启动（§11，不可超声明）
  approve: (name: string, permissions: string[]) => api.post<ApiResponse>(`/plugins/${name}/approve`, { permissions }),

  disable: (name: string) => api.post<ApiResponse>(`/plugins/${name}/disable`),

  remove: (name: string) => api.delete<ApiResponse>(`/plugins/${name}`),

  getManifest: (name: string) => api.get<ApiResponse<Record<string, unknown>>>(`/plugins/${name}/manifest`),

  getConfig: (name: string) => api.get<ApiResponse<{ config: Record<string, string> }>>(`/plugins/${name}/config`),

  setConfig: (name: string, kv: Record<string, string>) => api.put<ApiResponse>(`/plugins/${name}/config`, kv),

  getMenus: () => api.get<ApiResponse<{ items: PluginMenuItem[] }>>('/plugins/menus'),

  // 插件样式贡献（前端注入 <link>，面板自身样式之后生效）
  getStyles: () => api.get<ApiResponse<{ items: Array<{ plugin: string; id: string; url: string }> }>>('/plugins/styles'),

  getLogs: (name: string, lines = 500) => api.get<ApiResponse<{ log: string }>>(`/plugins/${name}/logs`, { params: { lines } }),

  // 插件管理子系统全局日志（面板侧管理动作）
  getSystemLogs: (lines = 500) => api.get<ApiResponse<{ log: string }>>('/plugins/system-logs', { params: { lines } }),

  getDetail: (name: string) => api.get<ApiResponse<PluginDetail>>(`/plugins/${name}/detail`),

  getWidgets: () => api.get<ApiResponse<{ items: PluginWidgetItem[] }>>('/plugins/widgets'),

  // 插件源（市场入口）
  listSources: () => api.get<ApiResponse<{ items: PluginSource[] }>>('/plugins/sources'),
  addSource: (data: { name: string; url: string }) => api.post<ApiResponse>('/plugins/sources', data),
  deleteSource: (id: number) => api.delete<ApiResponse>(`/plugins/sources/${id}`),
  fetchSourceCatalog: (id: number) => api.post<ApiResponse<{ source: PluginSource; catalog: PluginSourceCatalog }>>(`/plugins/sources/${id}/fetch`),
  installFromSource: (data: { url: string }) => api.post<ApiResponse<{ plugin: string; version: string }>>('/plugins/sources/install', data),

  upload: (fd: FormData) => api.post<ApiResponse<{ plugin: string; version: string }>>('/plugins/upload', fd),
};
