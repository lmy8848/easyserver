// API Response types
export interface ApiResponse<T = void> {
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
  role: string;
  must_change_pass?: boolean;
  last_login_at?: string;
  created_at?: string;
  is_locked?: boolean;
  ip_whitelist?: string;
  totp_enabled?: boolean;
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
  timestamp: string;
}

export type HistoryPoint = MonitorSnapshot;

// Service types
export interface Service {
  name: string;
  description: string;
  state: string;
  sub_state: string;
  enabled: boolean;
  unit_file_state: string;
  pid: number;
  memory_bytes: number;
  cpu_percent: number;
  uptime_seconds: number;
  // 托管服务元数据（系统服务为零值）
  managed: boolean;
  runtime_version_id: number;
  runtime_lang: string;
  runtime_exact: string;
  // 托管服务配置回显（解析 [Unit]/[Service] 段，编辑表单用）
  exec_start: string;
  dir: string;
  env: Record<string, string>;
  auto_restart: boolean;
  max_restarts?: number;
  restart_delay?: number;
  stop_timeout?: number;
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

export interface CloudFirewallRule {
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
  config_options: string;
  process_id: number;
  build_command: string;
  start_command: string;
  runtime_version_id: number;
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

// Database Backup types
export interface DBBackup {
  id: number;
  db_server_id: number;
  db_version_id: number;
  database_id: number;
  database_name: string;
  backup_type: string; // manual, scheduled
  file_path: string;
  file_size: number;
  status: string; // pending, completed, failed
  error_message: string;
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

// Cron task types
export interface CronTask {
  id: number;
  name: string;
  command: string;
  schedule: string;
  description: string;
  enabled: boolean;
  status: string; // idle, running, success, failed
  last_run: string;
  last_result: string;
  next_run: string;
  script_id: number;
  timeout: number;
  max_retry: number;
  env_vars: string;
  work_dir: string;
  runtime_version_id: number;
  runtime_lang: string;
  runtime_exact: string;
  created_at: string;
  updated_at: string;
}

export interface CronLog {
  id: number;
  task_id: number;
  status: string; // success, failed
  output: string;
  duration: number;
  created_at: string;
}

export interface Script {
  id: number;
  name: string;
  description: string;
  content: string;
  language: string;
  created_at: string;
  updated_at: string;
}

export interface CronDoc {
  id: number;
  title: string;
  content: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

// Firewall types
export interface FirewallRule {
  id: number;
  chain: string; // INPUT, OUTPUT, FORWARD
  protocol: string; // tcp, udp, all
  port: string;
  action: string; // ACCEPT, DROP, REJECT
  source: string;
  target: string;
  enabled: boolean;
  priority: number; // lower = higher precedence
  ip_version: string; // ipv4, ipv6, both
  remark: string;
  created_at: string;
}

export interface FirewallStatus {
  enabled: boolean;
  tool: string; // iptables, nftables, ufw, none
  version: string;
  rule_count: number;
  custom_rule_count: number;
  default_in: string;
  default_out: string;
}

export interface FirewallRuleTemplate {
  name: string;
  protocol: string;
  port: string;
  action: string;
  remark: string;
}

export interface FirewallLogEntry {
  timestamp: string;
  action: string;
  protocol: string;
  src_ip: string;
  dst_ip: string;
  src_port: number;
  dst_port: number;
  interface: string;
  raw: string;
}

// Process Guardian types
// Managed service spec（创建/更新托管服务的请求体，对应后端 ManagedUnitSpec）
export interface ManagedServiceSpec {
  name: string;
  description: string;
  exec_start: string;
  dir: string;
  env: Record<string, string>;
  auto_restart: boolean;
  max_restarts: number;
  restart_delay: number;
  stop_timeout: number;
  auto_start: boolean;
  runtime_version_id: number;
  runtime_lang: string;
  runtime_exact: string;
}

// System Process types
export interface SystemProcess {
  pid: number;
  ppid: number;
  name: string;
  user: string;
  state: string;
  cpu_percent: number;
  memory_mb: number;
  mem_percent: number;
  start_time: string;
  command: string;
  threads: number;
}

// Notification types
export interface Notification {
  id: number;
  type: string;      // alert/security/deploy/cron/update/system
  title: string;
  message: string;
  level: string;     // info/warning/error
  is_read: boolean;
  metadata: string;
  created_at: string;
}

// User Activity
export interface UserActivity {
  id: number;
  user_id: number;
  username: string;
  action: string;
  ip: string;
  user_agent: string;
  created_at: string;
}

// SSH types
export interface SSHLogin {
  username: string;
  ip: string;
  time: string;
  type: string; // login, logout, failed
  terminal: string;
}

export interface SSHConfig {
  port: number;
  permit_root_login: string;
  password_auth: string;
  status: string;
}

// File search
export interface FileSearchResult {
  path: string;
  name: string;
  is_dir: boolean;
  size: number;
  match?: string;
}

// File Share types
export interface FileShare {
  id: number;
  file_path: string;
  file_name: string;
  file_size: number;
  token: string;
  password: string;
  expires_at: string;
  max_downloads: number;
  download_count: number;
  created_by: number;
  created_at: string;
  updated_at: string;
  file_exists?: boolean;
  current_size?: number;
  has_password?: boolean;
}

// Public (non-sensitive) metadata for a share link, consumed by the download page.
export interface ShareInfo {
  file_name: string;
  file_size: number;
  exists: boolean;
  needs_password: boolean;
  expired: boolean;
  downloads_left: number; // -1 = unlimited
  download_count: number;
  max_downloads: number;
  expires_at: string;
}

// DB config
export interface ConfigSection {
  name: string;
  params: Record<string, string>;
}

export interface ParamMeta {
  key: string;
  label: string;
  description: string;
  type: string;
  unit?: string;
  options?: string[];
  default: string;
}

// TLS certificate info parsed from the configured cert
export interface TLSCertInfo {
  domain: string;
  issuer: string;
  expires_at: string;
}

// Settings
export interface AppSettings {
  server: { port: number; host: string; serve_frontend: boolean; tls: { enabled: boolean; cert_info: TLSCertInfo | null }; domain: string; redirect_mode: string; www_handling: string; max_upload_size: number; assets_rate_limit: number; assets_rate_interval: string; turnstile: { site_key: string; secret_key: string; enable_login: boolean; enable_qr_login: boolean; enable_public_share: boolean } };
  auth: { session_timeout: number; idle_timeout: number; max_login_attempts: number; lockout_duration: number; rate_limit: number; rate_interval: number; login_rate_limit: number; login_rate_interval: number; allow_multi_session: boolean; mobile_device_binding: boolean };
  monitor: { history_retention: number; collect_interval: number };
  database: { path: string };
  audit: { enabled: boolean; log_path: string };
  notify: { enabled: boolean; webhook_url: string };
  tencentcloud: { enabled: boolean; region: string; instance_id: string; has_secret: boolean };
}
