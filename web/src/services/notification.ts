import api from './client';
import type { ApiResponse, Notification, Page } from '../types';

// Notification API
export const notificationApi = {
  list: (unreadOnly = false, page = 1, pageSize = 50, level?: string, type?: string) =>
    api.get<ApiResponse<Page<Notification>>>('/notifications', {
      params: {
        unread: unreadOnly || undefined,
        level: level || undefined,
        type: type || undefined,
        page,
        page_size: pageSize,
      },
    }),

  unreadCount: () =>
    api.get<ApiResponse<{ count: number }>>('/notifications/unread-count'),

  create: (data: { type: string; title: string; message: string; level?: string }) =>
    api.post<ApiResponse>('/notifications', data),

  markAsRead: (id: number) =>
    api.put<ApiResponse>(`/notifications/${id}/read`),

  markAllAsRead: () =>
    api.put<ApiResponse>('/notifications/read-all'),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/notifications/${id}`),
};
