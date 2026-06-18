// API Response types
export interface ApiResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

export interface PaginatedData<T> {
  total: number;
  items: T[];
}

// User types
export interface User {
  id: number;
  username: string;
  role: 'admin' | 'operator' | 'viewer';
  must_change_pass?: boolean;
  last_login_at?: string;
  created_at?: string;
  is_locked?: boolean;
  ip_whitelist?: string;
}

// Monitor types
export interface SystemInfo {
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  cpu_cores: number;
  uptime_seconds: number;
}

export interface SwapInfo {
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  usage_percent: number;
}

export interface DiskPartition {
  mount_point: string;
  device: string;
  fs_type: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  usage_percent: number;
}

export interface ProcessInfo {
  pid: number;
  name: string;
  user: string;
  cpu_percent: number;
  mem_percent: number;
  mem_bytes: number;
  state: string;
}

export interface MonitorSnapshot {
  cpu: {
    usage_percent: number;
    load_1m: number;
    load_5m: number;
    load_15m: number;
  };
  memory: {
    total_bytes: number;
    used_bytes: number;
    usage_percent: number;
  };
  swap?: SwapInfo;
  disk: Array<{
    mount_point: string;
    total_bytes: number;
    used_bytes: number;
    usage_percent: number;
  }>;
  partitions?: DiskPartition[];
  network: {
    bytes_sent: number;
    bytes_recv: number;
  };
  system?: SystemInfo;
  top_process?: ProcessInfo[];
  timestamp: string;
}

export interface HistoryPoint extends MonitorSnapshot {
	// Matches backend MonitorSnapshot nested format
}

// Service types
export interface Service {
  name: string;
  description: string;
  state: string;
  sub_state: string;
  enabled: boolean;
  pid: number;
  memory_bytes: number;
  cpu_percent: number;
  uptime_seconds: number;
}

// File types
export interface FileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size_bytes: number;
  mode: string;
  modified_at: string;
  is_symlink: boolean;
}

export interface FileContent {
  path: string;
  content: string;
  encoding: string;
}

// Cloud types
export interface CloudInstance {
  instance_id: string;
  name: string;
  state: string;
  region: string;
  public_ip: string;
  private_ip: string;
  cpu: number;
  memory_gb: number;
  disk_gb: number;
  created_at: string;
  expired_at: string;
}

export interface FirewallRule {
  rule_id: string;
  protocol: string;
  port: string;
  source: string;
  action: string;
  remark: string;
}

export interface Snapshot {
  snapshot_id: string;
  name: string;
  instance_id: string;
  status: string;
  disk_gb: number;
  created_at: string;
}

export interface TrafficInfo {
  package_total_gb: number;
  package_used_gb: number;
  package_remaining_gb: number;
  package_expired_at: string;
}

// Web Server types
export interface WebServer {
  id: number;
  name: string;
  display_name: string;
  description: string;
  install_cmd: string;
  uninstall_cmd: string;
  config_path: string;
  config_file: string;
  sites_available: string;
  sites_enabled: string;
  service_name: string;
  binary_path: string;
  default_port: number;
  log_dir: string;
  status: string; // not_installed, running, stopped
  version: string;
  pid: number;
  memory_bytes: number;
  uptime: string;
  auto_start: boolean;
  config_ok: boolean;
  created_at: string;
}

// Website types
export interface Website {
  id: number;
  web_server_id: number;
  name: string;
  domain: string;
  root_path: string;
  port: number;
  project_type: string;
  app_port: number;
  ssl_enabled: boolean;
  ssl_cert_path: string;
  ssl_key_path: string;
  proxy_enabled: boolean;
  proxy_pass: string;
  custom_config: string;
  access_log: string;
  error_log: string;
  status: string;
  created_at: string;
  updated_at: string;
}

// Database Server types
export interface DBServer {
  id: number;
  name: string;
  display_name: string;
  description: string;
  default_port: number;
  status: string; // not_installed, running, stopped, partial
  version: string;
  created_at: string;
}

export interface DBVersion {
  id: number;
  db_server_id: number;
  version: string;
  service_name: string;
  config_file: string;
  data_dir: string;
  port: number;
  status: string; // running, stopped
  created_at: string;
  pid?: number;
  memory_bytes?: number;
  uptime?: string;
  connections?: number;
}

export interface Database {
  id: number;
  db_server_id: number;
  db_version_id: number;
  name: string;
  charset: string;
  description: string;
  size_bytes: number;
  status: string;
  version: string;
  created_at: string;
  updated_at: string;
}

export interface DBUser {
  id: number;
  db_server_id: number;
  username: string;
  host: string;
  privileges: string;
  created_at: string;
}
