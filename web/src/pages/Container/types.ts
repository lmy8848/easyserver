// ==================== Shared Types ====================

export interface ImageTemplate {
  name: string;
  image: string;
  description: string;
  ports?: string[];
  env?: Record<string, string>;
  volumes?: string[];
}

export interface ImageCategory {
  name: string;
  images: ImageTemplate[];
}

export interface DockerStatus {
  installed: boolean;
  version: string;
  compose_version: string;
  running: boolean;
  os: string;
}

export interface Container {
  id: string;
  name: string;
  image: string;
  status: string;
  state: string;
  ports: Array<{ host_port: string; container_port: string; protocol: string }>;
  created_at: string;
  cpu_usage: number;
  mem_usage: number;
}

export interface Image {
  id: string;
  repository: string;
  tag: string;
  size: number;
  created_at: string;
}

export interface ComposeProject {
  name: string;
  status: string;
  config_file: string;
  services: string[];
  created_at: string;
}

export interface Volume {
  name: string;
  driver: string;
  mountpoint: string;
  created_at: string;
  size: number;
}

export interface Network {
  id: string;
  name: string;
  driver: string;
  scope: string;
  subnet: string;
  gateway: string;
}

export interface ContainerStats {
  cpu_percent: number;
  mem_usage: number;
  mem_limit: number;
  mem_percent: number;
  net_rx: number;
  net_tx: number;
  block_read: number;
  block_write: number;
  pids: number;
}

// ==================== Helpers ====================

// Re-export from shared utils (backward compatible)
export { formatBytes } from '../../utils/format';
export { getServiceStatusColor as getStatusColor } from '../../utils/status';
