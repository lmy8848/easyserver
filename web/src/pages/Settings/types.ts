export interface TLSCertInfo {
  domain: string;
  issuer: string;
  expires_at: string;
}

export interface Settings {
  server: {
    port: number;
    host: string;
    serve_frontend: boolean;
    tls: { enabled: boolean; cert_info: TLSCertInfo | null };
    domain: string;
    redirect_mode: string;
    www_handling: string;
    max_upload_size: number;
    assets_rate_limit: number;
    assets_rate_interval: string;
  };
  auth: {
    session_timeout: string;
    idle_timeout: string;
    max_login_attempts: number;
    lockout_duration: string;
    rate_limit: number;
    rate_interval: string;
    login_rate_limit: number;
    login_rate_interval: string;
  };
  monitor: {
    history_retention: string;
    collect_interval: string;
  };
  database: {
    path: string;
  };
  audit: {
    enabled: boolean;
    log_path: string;
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
}

export interface SystemInfo {
  version: string;
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
