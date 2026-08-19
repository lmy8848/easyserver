import api from './client';
import type { ApiResponse, User } from '../types';

// Auth API
export const authApi = {
  login: (username: string, password: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User; requires_totp?: boolean; temp_token?: string }>>('/auth/login', { username, password, turnstile_token: turnstileToken, client_type: 'web' }),

  logout: () =>
    api.post<ApiResponse>('/auth/logout'),

  getProfile: () =>
    api.get<ApiResponse<User>>('/auth/me'),

  changeUsername: (newUsername: string, password: string) =>
    api.put<ApiResponse<{ user: User; token?: string }>>('/auth/username', { new_username: newUsername, password }),

  changePassword: (oldPassword: string, newPassword: string) =>
    api.post<ApiResponse>('/auth/change-password', { old_password: oldPassword, new_password: newPassword }),

  // TOTP verification (login step 2)
  verifyTOTP: (tempToken: string, code: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User }>>('/auth/verify-totp', { temp_token: tempToken, code, turnstile_token: turnstileToken, client_type: 'web' }),

  verifyBackupCode: (tempToken: string, backupCode: string, turnstileToken?: string) =>
    api.post<ApiResponse<{ token?: string; user: User }>>('/auth/verify-backup', { temp_token: tempToken, backup_code: backupCode, turnstile_token: turnstileToken, client_type: 'web' }),

  // TOTP setup (protected)
  setupTOTP: () =>
    api.post<ApiResponse<{ secret: string; otpauth_url: string }>>('/auth/totp/setup'),

  enableTOTP: (code: string) =>
    api.post<ApiResponse<{ backup_codes: string[] }>>('/auth/totp/enable', { code }),

  disableTOTP: (password: string) =>
    api.post<ApiResponse>('/auth/totp/disable', { password }),

  getTOTPStatus: () =>
    api.get<ApiResponse<{ enabled: boolean }>>('/auth/totp/status'),

  // Session management
  getSessions: () =>
    api.get<ApiResponse<Array<{ user_id: number; ip: string; user_agent: string; client_type: string; is_current: boolean; login_at: string; expires_at: string; token?: string }>>>('/auth/sessions'),

  kickSession: (token: string) =>
    api.post<ApiResponse>('/auth/sessions/kick', { token }),

  kickAllOtherSessions: () =>
    api.post<ApiResponse>('/auth/sessions/kick-all'),

  // Turnstile config (public: site key + enabled flows, no secret).
  getTurnstileConfig: () =>
    api.get<ApiResponse<{ site_key: string; enable_login: boolean; enable_qr_login: boolean; enable_public_share: boolean }>>('/auth/turnstile/config'),

  // Scan-to-login (QR). Web creates+p polls; mobile (authenticated) confirms.
  createQRSession: () =>
    api.post<ApiResponse<{ qr_token: string; expires_at: string }>>('/auth/qr/session'),

  getQRStatus: (qrToken: string) =>
    api.post<ApiResponse<{ status: string; expires_at: string; token?: string; user?: User }>>('/auth/qr/status', { qr_token: qrToken }),

  confirmQRLogin: (qrToken: string) =>
    api.post<ApiResponse<{ ok: boolean }>>('/auth/qr/confirm', { qr_token: qrToken }),

  cancelQRLogin: (qrToken: string) =>
    api.post<ApiResponse>('/auth/qr/cancel', { qr_token: qrToken }),
};
