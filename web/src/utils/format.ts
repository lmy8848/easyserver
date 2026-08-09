/**
 * Format bytes to human-readable string.
 * Extracted from Dashboard, Services, Container/types duplicates.
 */
export function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * Format uptime seconds to human-readable string.
 */
export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}天${hours}小时${minutes}分钟`;
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  return `${minutes}分钟`;
}

/**
 * Format a created-time string to `YYYY-MM-DD HH:mm`. Handles both Docker's
 * `2006-01-02 15:04:05 +0800 CST` and Podman's RFC3339. Falls back to the raw
 * value when unparseable.
 */
export function formatCreatedAt(value: string): string {
  if (!value) return '-';
  let d = new Date(value);
  if (isNaN(d.getTime()) && / [A-Z]{3,4}$/.test(value)) {
    d = new Date(value.replace(/ [A-Z]{3,4}$/, ''));
  }
  if (isNaN(d.getTime())) return value;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
