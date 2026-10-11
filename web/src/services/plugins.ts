import api from './client';
import type { ApiResponse, PluginRecord, PluginMenuItem } from '../types';

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

  disable: (name: string) => api.post<ApiResponse>(`/plugins/${name}/disable`),

  remove: (name: string) => api.delete<ApiResponse>(`/plugins/${name}`),

  getManifest: (name: string) => api.get<ApiResponse<Record<string, unknown>>>(`/plugins/${name}/manifest`),

  getConfig: (name: string) => api.get<ApiResponse<{ config: Record<string, string> }>>(`/plugins/${name}/config`),

  setConfig: (name: string, kv: Record<string, string>) => api.put<ApiResponse>(`/plugins/${name}/config`, kv),

  getMenus: () => api.get<ApiResponse<{ items: PluginMenuItem[] }>>('/plugins/menus'),

  getWidgets: () => api.get<ApiResponse<{ items: PluginWidgetItem[] }>>('/plugins/widgets'),

  upload: (fd: FormData) => api.post<ApiResponse<{ plugin: string; version: string }>>('/plugins/upload', fd),
};
