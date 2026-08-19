import api from './client';
import type { ApiResponse } from '../types';

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
