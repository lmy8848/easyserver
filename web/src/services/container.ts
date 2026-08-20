import api from './client';
import type {
  ApiResponse,
  ComposeProject,
  Container,
  ContainerStats,
  DockerStatus,
  Image,
  Network,
  Page,
  Volume,
} from '../types';

// Container API
export const containerApi = {
  getStatus: (engine?: string) =>
    api.get<ApiResponse<DockerStatus>>('/container/status', { params: { engine: engine === 'podman' ? 'podman' : undefined } }),
  install: (engine?: string) =>
    api.post<ApiResponse>('/container/install', null, { params: { engine: engine === 'podman' ? 'podman' : undefined } }),
  start: (engine?: string) =>
    api.post<ApiResponse>('/container/start', null, { params: { engine: engine === 'podman' ? 'podman' : undefined } }),
  enableSocket: (engine?: string) =>
    api.post<ApiResponse>('/container/socket/enable', null, { params: { engine: engine === 'podman' ? 'podman' : undefined } }),

  listContainers: (engine?: string, all = true, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Container>>>('/container/instances', {
      params: { engine: engine === 'podman' ? 'podman' : undefined, all: all ? 'true' : 'false', page, page_size: pageSize },
    }),
  getContainer: (id: string, engine?: string) =>
    api.get<ApiResponse<Container>>(`/container/instances/${id}`, { params: { engine: engine === 'podman' ? 'podman' : undefined } }),
  actionContainer: (id: string, action: string, engine?: string) =>
    api.post<ApiResponse>(`/container/instances/${id}/${action}`, null, { params: { engine: engine === 'podman' ? 'podman' : undefined } }),
  deleteContainer: (id: string, force = true, engine?: string) =>
    api.delete<ApiResponse>(`/container/instances/${id}`, { params: { force, engine: engine === 'podman' ? 'podman' : undefined } }),
  createContainer: (data: any, engine?: string) =>
    api.post<ApiResponse<{ id: string; name: string }>>('/container/instances', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
      timeout: 600000,
    }),
  execContainer: (id: string, data: { command: string }, engine?: string) =>
    api.post<ApiResponse<{ exit_code: number; output: string }>>(`/container/instances/${id}/exec`, data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  getContainerLogs: (id: string, tail = 200, engine?: string) =>
    api.get<ApiResponse<{ logs: string }>>(`/container/instances/${id}/logs`, {
      params: { tail, engine: engine === 'podman' ? 'podman' : undefined },
    }),
  getContainerStats: (id: string, engine?: string) =>
    api.get<ApiResponse<ContainerStats>>(`/container/instances/${id}/stats`, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),

  listComposeProjects: (engine?: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<ComposeProject>>>('/container/compose/projects', {
      params: { engine: engine === 'podman' ? 'podman' : undefined, page, page_size: pageSize },
    }),
  composeAction: (action: string, projectDir: string, engine?: string) =>
    api.post<ApiResponse>(`/container/compose/${action}`, { project_dir: projectDir }, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  getComposeLogs: (dir: string, tail = 200, engine?: string) =>
    api.get<ApiResponse<{ logs: string }>>('/container/compose/logs', {
      params: { dir, tail, engine: engine === 'podman' ? 'podman' : undefined },
    }),
  getComposeConfig: (dir: string, engine?: string) =>
    api.get<ApiResponse<{ content: string }>>('/container/compose/config', {
      params: { dir, engine: engine === 'podman' ? 'podman' : undefined },
    }),
  saveComposeConfig: (projectDir: string, content: string, engine?: string) =>
    api.put<ApiResponse>('/container/compose/config', { project_dir: projectDir, content }, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),

  listImages: (engine?: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Image>>>('/container/images', {
      params: { engine: engine === 'podman' ? 'podman' : undefined, page, page_size: pageSize },
    }),
  pullImage: (data: { image: string; tag?: string }, engine?: string) =>
    api.post<ApiResponse>('/container/images/pull', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  deleteImage: (id: string, force = true, engine?: string) =>
    api.delete<ApiResponse>(`/container/images/${id}`, {
      params: { force, engine: engine === 'podman' ? 'podman' : undefined },
    }),

  listNetworks: (engine?: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Network>>>('/container/networks', {
      params: { engine: engine === 'podman' ? 'podman' : undefined, page, page_size: pageSize },
    }),
  createNetwork: (data: any, engine?: string) =>
    api.post<ApiResponse>('/container/networks', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  deleteNetwork: (id: string, engine?: string) =>
    api.delete<ApiResponse>(`/container/networks/${id}`, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),

  listVolumes: (engine?: string, page = 1, pageSize = 50) =>
    api.get<ApiResponse<Page<Volume>>>('/container/volumes', {
      params: { engine: engine === 'podman' ? 'podman' : undefined, page, page_size: pageSize },
    }),
  createVolume: (data: any, engine?: string) =>
    api.post<ApiResponse>('/container/volumes', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  deleteVolume: (name: string, force = true, engine?: string) =>
    api.delete<ApiResponse>(`/container/volumes/${name}`, {
      params: { force, engine: engine === 'podman' ? 'podman' : undefined },
    }),

  getRegistryConfig: (engine?: string) =>
    api.get<ApiResponse<any>>('/container/registry', {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  getRegistryAuth: (engine?: string) =>
    api.get<ApiResponse<any>>('/container/registry/auth', {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  saveRegistryConfig: (data: { mirrors: string[]; insecure_registries: string[] }, engine?: string) =>
    api.post<ApiResponse>('/container/registry', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  loginRegistry: (data: any, engine?: string) =>
    api.post<ApiResponse>('/container/registry/login', data, {
      params: { engine: engine === 'podman' ? 'podman' : undefined },
    }),
  logoutRegistry: (server: string, engine?: string) =>
    api.post<ApiResponse>('/container/registry/logout', null, {
      params: { server, engine: engine === 'podman' ? 'podman' : undefined },
    }),
};
