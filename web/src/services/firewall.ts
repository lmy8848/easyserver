import api from './client';
import type {
  ApiResponse,
  FirewallLogEntry,
  FirewallRule,
  FirewallRuleTemplate,
  FirewallStatus,
  Page,
} from '../types';

// Firewall management
export const firewallApi = {
  getStatus: () =>
    api.get<ApiResponse<FirewallStatus>>('/firewall/status'),

  enable: () =>
    api.post<ApiResponse>('/firewall/enable'),

  disable: () =>
    api.post<ApiResponse>('/firewall/disable', { confirm: true }),

  listRules: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<FirewallRule>>>('/firewall/rules', { params: { page, page_size: pageSize } }),

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

  getSystemRules: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<FirewallRule>>>('/firewall/system-rules', { params: { page, page_size: pageSize } }),

  deleteSystemRule: (rule: FirewallRule) =>
    api.post<ApiResponse>('/firewall/system-rules/delete', rule),

  setDefaultPolicy: (data: { chain: string; policy: string }) =>
    api.post<ApiResponse>('/firewall/default-policy', data),

  getTemplates: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<FirewallRuleTemplate>>>('/firewall/templates', { params: { page, page_size: pageSize } }),

  applyTemplate: (name: string) =>
    api.post<ApiResponse<FirewallRule>>('/firewall/templates/apply', { name }),

  exportRules: () =>
    api.get('/firewall/rules/export', { responseType: 'blob' as const }),

  importRules: (data: { version: number; exported_at: string; rules: Array<{ chain: string; protocol: string; port: string; action: string; source: string; remark: string }> }) =>
    api.post<ApiResponse<{ succeeded: number; failed: number; errors: string[] }>>('/firewall/rules/import', data),

  getLogs: (lines?: number) =>
    api.get<ApiResponse<FirewallLogEntry[]>>('/firewall/logs', { params: { lines } }),
};
