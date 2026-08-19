import axios from 'axios';
import api from './client';
import type {
  ApiResponse,
  FileEntry,
  FileSearchResult,
  FileShare,
  Page,
  ShareFileEntry,
  ShareInfo,
} from '../types';

// File API
const UPLOAD_TIMEOUT = 10 * 60 * 1000; // 10 minutes for large files
const UPLOAD_MAX_RETRIES = 2;

export const fileApi = {
  list: (path: string) =>
    api.get<ApiResponse<{ path: string; parent: string; entries: FileEntry[] }>>('/files', { params: { path } }),

  mkdir: (path: string) =>
    api.post<ApiResponse>('/files/mkdir', { path }),

  upload: (file: File, path: string, onProgress?: (percent: number) => void) => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('path', path);
    console.log('[API Upload] File:', { name: file.name, size: file.size, type: file.type });
    console.log('[API Upload] Path:', path);

    const doUpload = (attempt: number): Promise<any> => {
      if (attempt > 0) {
        console.log(`[API Upload] Retry attempt ${attempt}/${UPLOAD_MAX_RETRIES}`);
      }
      return api.post<ApiResponse>('/files/upload', formData, {
        timeout: UPLOAD_TIMEOUT,
        onUploadProgress: (progressEvent) => {
          if (progressEvent.total && onProgress) {
            const percent = Math.round((progressEvent.loaded * 100) / progressEvent.total);
            onProgress(percent);
          }
        },
      }).catch((error) => {
        // Retry on network/timeout errors, not on 4xx/5xx responses
        const isNetworkError = !error.response && (error.code === 'ECONNABORTED' || error.code === 'ERR_NETWORK' || error.message?.includes('timeout') || error.message?.includes('Network Error'));
        if (isNetworkError && attempt < UPLOAD_MAX_RETRIES) {
          console.log(`[API Upload] Network error, will retry: ${error.message}`);
          // Reset progress for retry
          if (onProgress) onProgress(0);
          return new Promise(resolve => setTimeout(resolve, 1000 * (attempt + 1)))
            .then(() => doUpload(attempt + 1));
        }
        throw error;
      });
    };

    return doUpload(0);
  },

  download: (path: string) =>
    api.get('/files/download', { params: { path }, responseType: 'blob' }),

  rename: (oldPath: string, newPath: string) =>
    api.put<ApiResponse>('/files/rename', { old_path: oldPath, new_path: newPath }),

  delete: (path: string, recursive?: boolean) =>
    api.delete<ApiResponse>('/files', { params: { path, recursive } }),

  move: (paths: string[], dest: string) =>
    api.post<ApiResponse>('/files/move', { paths, dest }),

  copy: (source: string, dest: string) =>
    api.post<ApiResponse>('/files/copy', { source, dest }),

  getContent: (path: string) =>
    api.get<ApiResponse<{ content: string }>>('/files/content', { params: { path } }),

  saveContent: (path: string, content: string) =>
    api.put<ApiResponse>('/files/content', { path, content }),

  // New file operations
  search: (path: string, query: string, limit?: number) =>
    api.get<ApiResponse<FileSearchResult[]>>('/files/search', { params: { path, q: query, limit } }),

  searchContent: (path: string, query: string, limit?: number) =>
    api.get<ApiResponse<FileSearchResult[]>>('/files/search-content', { params: { path, q: query, limit } }),

  getDetails: (path: string) =>
    api.get<ApiResponse<Record<string, unknown>>>('/files/details', { params: { path } }),

  getMimeType: (path: string) =>
    api.get<ApiResponse<{ path: string; mime_type: string }>>('/files/mime-type', { params: { path } }),

  compress: (sources: string[], dest: string) =>
    api.post<ApiResponse>('/files/compress', { sources, dest }),

  extract: (source: string, dest: string) =>
    api.post<ApiResponse>('/files/extract', { source, dest }),

  archiveList: (path: string) =>
    api.get<ApiResponse<Page<{ name: string; size: number; is_dir: boolean }>>>('/files/archive-list', { params: { path } }),

  chmod: (path: string, mode: string) =>
    api.put<ApiResponse>('/files/chmod', { path, mode }),

  chown: (path: string, uid: number, gid: number) =>
    api.put<ApiResponse>('/files/chown', { path, uid, gid }),
};

// File Share API
export const fileShareApi = {
  create: (data: { file_path: string; password?: string; expires_at?: string; max_downloads?: number }) =>
    api.post<ApiResponse<FileShare>>('/shares', data),

  list: (page = 1, pageSize = 20) =>
    api.get<ApiResponse<Page<FileShare>>>('/shares', { params: { page, page_size: pageSize } }),

  get: (id: number) =>
    api.get<ApiResponse<FileShare>>(`/shares/${id}`),

  update: (id: number, data: {
    password?: string | null;
    expires_at?: string;
    max_downloads?: number;
    clear_expiry?: boolean;
  }) =>
    api.put<ApiResponse<FileShare>>(`/shares/${id}`, data),

  delete: (id: number) =>
    api.delete<ApiResponse>(`/shares/${id}`),

  cleanupExpired: () =>
    api.post<ApiResponse<{ deleted: number }>>('/shares/cleanup'),
};

// Public share endpoints are now under /api/shares/public/...
// They may be accessed without auth (share recipients).
export const publicShareApi = {
  getShareInfo: (token: string) =>
    axios.get<ApiResponse<ShareInfo>>(`/api/shares/public/${token}/info`),
  getDownloadTicket: (token: string, password?: string) =>
    axios.post<ApiResponse<{ ticket: string }>>(`/api/shares/public/${token}/ticket`, { password }),
  listShareFiles: (token: string, ticket: string, subpath: string = '') =>
    axios.get<ApiResponse<ShareFileEntry[]>>(`/api/shares/public/${token}/list`, { params: { ticket, subpath } }),
};
