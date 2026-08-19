import api from './client';
import type { ApiResponse, ManagedServiceSpec, Page, Service } from '../types';

// Service API (systemd services & managed units)
export const serviceApi = {
  list: (params?: { managed?: boolean; page?: number; page_size?: number }) =>
    api.get<ApiResponse<Page<Service>>>('/services', { params }),

  // 创建托管服务（生成 easyserver-svc-* unit）
  create: (data: ManagedServiceSpec) =>
    api.post<ApiResponse>('/services', data),

  getDetails: (names: string[]) =>
    api.post<ApiResponse<Service[]>>('/services/details', { names }),

  get: (name: string) =>
    api.get<ApiResponse<Service>>(`/services/${name}`),

  // 更新托管服务（:name 须为完整名 easyserver-svc-<name>）
  update: (name: string, data: ManagedServiceSpec) =>
    api.put<ApiResponse>(`/services/${name}`, data),

  // 删除托管服务（:name 须为完整名 easyserver-svc-<name>）
  delete: (name: string) =>
    api.delete<ApiResponse>(`/services/${name}`),

  start: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/start`),

  stop: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/stop`),

  restart: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/restart`),

  enable: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/enable`),

  disable: (name: string) =>
    api.post<ApiResponse>(`/services/${name}/disable`),

  getLogs: (name: string, tail?: number) =>
    api.get<ApiResponse<{ lines: Array<{ time: string; message: string; priority: string }> }>>(`/services/${name}/logs`, { params: { tail } }),
};
