import api from './client';
import type { ApiResponse, PluginRecord, PluginMenuItem } from '../types';

// 插件管理
export const pluginsApi = {
  list: () => api.get<ApiResponse<{ items: PluginRecord[] }>>('/plugins'),

  start: (name: string) => api.post<ApiResponse>(`/plugins/${name}/start`),

  stop: (name: string) => api.post<ApiResponse>(`/plugins/${name}/stop`),

  enable: (name: string) => api.post<ApiResponse>(`/plugins/${name}/enable`),

  disable: (name: string) => api.post<ApiResponse>(`/plugins/${name}/disable`),

  remove: (name: string) => api.delete<ApiResponse>(`/plugins/${name}`),

  getManifest: (name: string) => api.get<ApiResponse<Record<string, unknown>>>(`/plugins/${name}/manifest`),

  getMenus: () => api.get<ApiResponse<{ items: PluginMenuItem[] }>>('/plugins/menus'),

  upload: (fd: FormData) => api.post<ApiResponse<{ plugin: string; version: string }>>('/plugins/upload', fd),
};
