import api from './client';
import type { ApiResponse, MonitorSnapshot, HistoryPoint, Page, SystemProcess } from '../types';

// Monitor API
export const monitorApi = {
  getStats: () =>
    api.get<ApiResponse<MonitorSnapshot>>('/monitor/stats'),

  getHistory: (start?: string, end?: string, signal?: AbortSignal) =>
    api.get<ApiResponse<{ points: HistoryPoint[] }>>('/monitor/history', { params: { start, end }, signal }),
};

// System API (ports + port availability check)
export const systemApi = {
  getListeningPorts: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<{ protocol: string; port: number; local_addr: string; state: string; pid: number; process_name: string; user: string }>>>('/monitor/ports', { params: { page, page_size: pageSize } }),

  checkPort: (port: number) =>
    api.get<ApiResponse<{ available: boolean; port: number; process?: string; message: string }>>('/monitor/check-port', { params: { port } }),
};

// System Process API
export const systemProcessApi = {
  listProcesses: (params?: { sort_by?: string; order?: string; search?: string; page?: number; page_size?: number }) =>
    api.get<ApiResponse<Page<SystemProcess>>>('/monitor/processes', { params }),

  getProcess: (pid: number) =>
    api.get<ApiResponse<SystemProcess>>(`/monitor/processes/${pid}`),
};
