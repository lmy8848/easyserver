import api from './client';
import type { ApiResponse, Page, SSHSession } from '../types';

// SSH API
export interface SSHLoginRecord {
  time: string;
  user: string;
  ip: string;
  port: number;
  status: string; // success, failed
  method: string; // password, publickey
  tty: string;
}

export interface AuthorizedKey {
  comment: string;
  type: string;
  key: string;
  fingerprint: string;
}

export const sshApi = {
  getStatus: () =>
    api.get<ApiResponse<{ available: boolean; reason?: string; installed: boolean; running: boolean }>>('/ssh/status'),
  getConfig: () =>
    api.get<ApiResponse<any>>('/ssh/config'),
  saveConfig: (data: any) =>
    api.put<ApiResponse>('/ssh/config', data),
  testConfig: () =>
    api.post<ApiResponse<{ message: string }>>('/ssh/config/test'),
  reloadService: () =>
    api.post<ApiResponse>('/ssh/config/reload'),
  listSessions: (page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<SSHSession>>>('/ssh/sessions', { params: { page, page_size: pageSize } }),
  killSession: (pid: number) =>
    api.post<ApiResponse>(`/ssh/sessions/${pid}/kill`),
  getLogins: (limit?: number) =>
    api.get<ApiResponse<{ records: SSHLoginRecord[] }>>('/ssh/logins', { params: { limit } }),
  listAuthorizedKeys: (page = 1, pageSize = 20) =>
    api.get<ApiResponse<Page<AuthorizedKey>>>('/ssh/authorized-keys', {
      params: { page, page_size: pageSize },
    }),
  addAuthorizedKey: (data: { key: string }) =>
    api.post<ApiResponse>('/ssh/authorized-keys', data),
  deleteAuthorizedKey: (comment: string) =>
    api.delete<ApiResponse>('/ssh/authorized-keys', { params: { comment } }),
  generateKey: (data: { name: string; key_type: string }) =>
    api.post<ApiResponse<{ private_key: string; public_key: string }>>('/ssh/keys/generate', data),
  harden: (data?: any) =>
    api.post<ApiResponse>('/ssh/harden', data),
  getFail2banStatus: () =>
    api.get<ApiResponse<any>>('/ssh/fail2ban'),
  installFail2ban: () =>
    api.post<ApiResponse>('/ssh/fail2ban/install'),
  reloadFail2ban: () =>
    api.post<ApiResponse>('/ssh/fail2ban/reload'),
};
