import api from './client';
import type { ApiResponse, AppSettings } from '../types';

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
