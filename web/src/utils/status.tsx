import { Tag } from 'antd';

/* eslint-disable react-refresh/only-export-components */

/**
 * Get color for percentage-based metrics (CPU, memory, disk).
 * Used by Dashboard.
 */

export function getPercentColor(percent: number): string {
  if (percent >= 90) return '#cf1322';
  if (percent >= 70) return '#faad14';
  return '#3f8600';
}

/**
 * Get Ant Design tag color for service status strings.
 * Used by Services, Database, Website pages.
 */

export function getStatusColorName(status: string): string {
  if (!status) return 'default';
  const s = status.toLowerCase();
  if (s === 'active' || s === 'running' || s.includes('running') || s.includes('up')) return 'success';
  if (s === 'failed' || s === 'error' || s.includes('exited') || s.includes('dead')) return 'error';
  if (s === 'inactive' || s === 'stopped') return 'default';
  if (s === 'installing') return 'processing';
  if (s === 'not_installed') return 'default';
  if (s === 'partial') return 'warning';
  if (s.includes('paused')) return 'orange';
  if (s.includes('created')) return 'blue';
  return 'default';
}

/**
 * Get hex color string for service status (for inline styles).
 */
export function getStatusColor(status: string): string {
  const colorName = getStatusColorName(status);
  const colorMap: Record<string, string> = {
    success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
  };
  return colorMap[colorName] || '#999';
}

/**
 * Get Ant Design tag color for HTTP status codes.
 * Used by AuditLog.
 */

export function getHttpStatusColor(status: string): string {
  const code = parseInt(status);
  if (code >= 200 && code < 300) return 'success';
  if (code >= 400 && code < 500) return 'warning';
  if (code >= 500) return 'error';
  return 'default';
}

/**
 * Render a status tag for service/server status.
 * Consolidates duplicated statusTag from Database, Website pages.
 */
export function StatusTag({ status }: { status: string }) {
  const color = getStatusColorName(status);
  const label = status === 'active' ? '运行中'
    : status === 'running' ? '运行中'
    : status === 'installing' ? '安装中'
    : status === 'inactive' ? '已停止'
    : status === 'stopped' ? '已停止'
    : status === 'failed' ? '异常'
    : status === 'installed' ? '已安装'
    : status === 'not_installed' ? '未安装'
    : status === 'partial' ? '部分运行'
    : status;
  return <Tag color={color}>{label}</Tag>;
}
