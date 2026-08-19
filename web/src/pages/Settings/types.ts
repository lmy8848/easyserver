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
