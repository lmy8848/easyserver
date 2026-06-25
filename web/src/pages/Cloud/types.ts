export interface MonitorPoint {
  timestamp: string;
  value: number;
}

export interface TrafficInfo {
  package_total_gb: number;
  package_used_gb: number;
  package_remaining_gb: number;
  package_expired_at: string;
}
