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
  disk: {
    mount_point: string;
    total_bytes: number;
    used_bytes: number;
    usage_percent: number;
  };
  disk_io?: {
    read_bytes: number;
    write_bytes: number;
  };
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
  short_name?: string; // 托管服务去前缀短名；系统服务等于 name
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
  runtime: string; // lang@exact，"" = 不绑定（ADR-0009 绑定键）
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
  container_name: string;
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
  access_log: string;
  error_log: string;
  status: string;
  created_at: string;
  updated_at: string;
}

// Database Backup types
export interface DBBackup {
  id: number;
  db_type: string;
  db_version_id: number;
  database_name: string;
  backup_type: string; // manual, scheduled
  file_path: string;
  file_size: number;
  status: string; // running, success, failed
  error_message: string;
  created_at: string;
}

export interface DBInstance {
  id: number;
  db_type: string;
  version: string;
  port: number;
  status: string; // running, stopped, unhealthy
  created_at: string;
  container_engine?: string;
  image?: string;
  container_name: string;
  volume_name?: string;
  config_dir?: string;
  bind_address?: string;
}

export interface Database {
  // Logical database inside an instance — live database state, no persisted id.
  name: string;
  charset: string;
}

export interface DBUser {
  username: string;
  host: string; // MySQL only; empty for PostgreSQL
  privileges?: string;
}

export interface RedisKey {
  name: string;
  type: string; // string | hash | list | set | zset | ...
  ttl: number; // seconds; -1 = no expiry
  size: number; // bytes
}

export interface RedisValue {
  type: string;
  value: string | Record<string, string> | string[] | Array<{ member: string; score: number }> | null;
}

// 添加 Redis key 时按类型的载荷（hash 字段对 / zset 分值-成员对）。
export interface RedisHashField {
  field: string;
  value: string;
}

export interface RedisZSetMember {
  member: string;
  score: number;
}

// Cron task types（systemd timer 承载，name 为唯一标识）
export interface CronTask {
  name: string;
  command: string;
  schedule: string; // OnCalendar 表达式
  description: string;
  persistent: boolean;
  enabled: boolean;
  status: string; // active, inactive, failed
  last_run: string;
  last_result: string;
  next_run: string;
  timeout: number;
  max_retry: number;
  env_vars: string;
  work_dir: string;
  runtime: string; // lang@exact，"" = 不绑定
}

export interface CronLog {
  time: string;
  message: string;
  priority: string;
}

export interface CronRun {
  invocation_id: string;
  started_at: string;
  status: string; // success / failed / running
  logs: CronLog[];
}

export interface Script {
  id: number;
  name: string;
  description: string;
  content: string;
  path: string;
  created_at: string;
  updated_at: string;
}

// 脚本执行实时日志行（WS 消息 data 字段）
export interface ScriptLogLine {
  stream: 'stdout' | 'stderr';
  message: string;
  time: string;
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
  runtime: string; // lang@exact，"" = 不绑定
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
// (SSHLogin/SSHConfig removed: ssh domain uses LoginRecord shape via sshApi)

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
  is_dir?: boolean;
}

// Public (non-sensitive) metadata for a share link, consumed by the download page.
export interface ShareInfo {
  file_name: string;
  file_size: number;
  is_dir: boolean;
  exists: boolean;
  needs_password: boolean;
  expired: boolean;
  downloads_left: number; // -1 = unlimited
  download_count: number;
  max_downloads: number;
  expires_at: string;
}

export interface ShareFileEntry {
  name: string;
  size: number;
  is_dir: boolean;
}

// ParamMeta is the UI metadata for one config param. type reflects the engine's
// actual variable type: "number" params are numeric variables whose SET value must
// be a bare literal (MySQL 1232 otherwise), "string" params are quoted. key is the
// actual engine parameter name (zero conversion). Editor renders a Select when
// options is present, an InputNumber when type is "number", text otherwise.
export interface ParamMeta {
  key: string;
  label: string;
  description: string;
  type: 'number' | 'string';
  unit?: string;
  options?: string[];
}

// Structured config of one database instance (GET /db/instances/:iid/config):
// params = current engine values for the panel-managed params, meta = editor
// metadata. No multi-section shape — one config namespace per engine.
export interface InstanceConfigView {
  params: Record<string, string>;
  meta: ParamMeta[];
}

// TLS certificate info parsed from the configured cert
export interface TLSCertInfo {
  domain: string;
  issuer: string;
  expires_at: string;
}

// Settings
export interface AppSettings {
  server: { port: number; host: string; tls: { enabled: boolean; cert_file: string; key_file: string; cert_info: TLSCertInfo | null }; domain: string; force_domain: boolean; max_upload_size: number; assets_rate_limit: number; assets_rate_interval: string; allowed_origins: string[]; turnstile: { site_key: string; secret_key: string; enable_login: boolean; enable_qr_login: boolean; enable_public_share: boolean } };
  auth: { session_timeout: number; idle_timeout: number; max_login_attempts: number; lockout_duration: number; rate_limit: number; rate_interval: number; login_rate_limit: number; login_rate_interval: number; allow_multi_session: boolean; ip_whitelist: string[]; session_cleanup_interval: number };
  monitor: { history_retention: number; collect_interval: number };
  audit: { retention_days: number };
  notify: { enabled: boolean; webhook_url: string };
  features: { fim: boolean };
  logs: { level: string; path: string; format: string; max_size_mb: number; max_files: number };
}
