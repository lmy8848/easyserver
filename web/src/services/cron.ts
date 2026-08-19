import api from './client';
import type { ApiResponse, CronRun, CronTask, Page, Script, ScriptLogLine } from '../types';

// Cron task management
export const cronApi = {
  list: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<CronTask>>>('/cron/tasks', { params: { page, page_size: pageSize } }),

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

  getRuns: (name: string, page = 1, pageSize = 20, since?: string, until?: string) =>
    api.get<ApiResponse<Page<CronRun>>>(`/cron/tasks/${name}/runs`, {
      params: { page, page_size: pageSize, since: since || undefined, until: until || undefined },
    }),

  // Scripts
  listScripts: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Script>>>('/cron/scripts', { params: { page, page_size: pageSize } }),

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
