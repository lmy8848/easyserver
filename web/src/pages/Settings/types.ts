export interface TLSCertInfo {
  domain: string;
  issuer: string;
  expires_at: string;
}

export interface Settings {
  server: {
    port: number;
    host: string;
    tls: { enabled: boolean; cert_file: string; key_file: string; cert_info: TLSCertInfo | null };
    domain: string;
    force_domain: boolean;
    max_upload_size: number;
    assets_rate_limit: number;
    assets_rate_interval: string;
    allowed_origins: string[];
    turnstile: {
      site_key: string;
      secret_key: string;
      enable_login: boolean;
      enable_qr_login: boolean;
      enable_public_share: boolean;
    };
  };
  auth: {
    session_timeout: number;
    idle_timeout: number;
    max_login_attempts: number;
    lockout_duration: number;
    rate_limit: number;
    rate_interval: number;
    login_rate_limit: number;
    login_rate_interval: number;
    allow_multi_session: boolean;
    ip_whitelist: string[];
    session_cleanup_interval: number;
  };
  monitor: {
    history_retention: number;
    collect_interval: number;
  };
  audit: {
    retention_days: number;
  };
  notify: {
    enabled: boolean;
    webhook_url: string;
  };
  tencentcloud: {
    enabled: boolean;
    region: string;
    instance_id: string;
    has_secret: boolean;
  };
  features: {
    fim: boolean;
  };
  logs: {
    level: string;
    path: string;
    format: string;
    max_size_mb: number;
    max_files: number;
  };
}

export interface SystemInfo {
  version: string;
  build_id?: string;
}

export interface AlertRule {
  name: string;
  metric: string;
  threshold: number;
  duration: number;
  enabled: boolean;
}

export const REGION_OPTIONS = [
  { label: '广州 (ap-guangzhou)', value: 'ap-guangzhou' },
  { label: '上海 (ap-shanghai)', value: 'ap-shanghai' },
  { label: '北京 (ap-beijing)', value: 'ap-beijing' },
  { label: '南京 (ap-nanjing)', value: 'ap-nanjing' },
  { label: '成都 (ap-chengdu)', value: 'ap-chengdu' },
  { label: '重庆 (ap-chongqing)', value: 'ap-chongqing' },
  { label: '中国香港 (ap-hongkong)', value: 'ap-hongkong' },
  { label: '新加坡 (ap-singapore)', value: 'ap-singapore' },
  { label: '东京 (ap-tokyo)', value: 'ap-tokyo' },
  { label: '硅谷 (na-siliconvalley)', value: 'na-siliconvalley' },
  { label: '法兰克福 (eu-frankfurt)', value: 'eu-frankfurt' },
];
