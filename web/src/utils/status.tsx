import React from 'react';
import { Tag } from 'antd';

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
export function getServiceStatusColor(status: string): string {
  if (!status) return 'default';
  const s = status.toLowerCase();
  if (s === 'active' || s === 'running' || s.includes('running') || s.includes('up')) return 'success';
  if (s === 'failed' || s === 'error' || s.includes('exited') || s.includes('dead')) return 'error';
  if (s === 'inactive' || s === 'stopped') return 'default';
  if (s === 'not_installed') return 'default';
  if (s === 'partial') return 'warning';
  if (s.includes('paused')) return 'orange';
  if (s.includes('created')) return 'blue';
  return 'default';
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
export function ServiceStatusTag({ status }: { status: string }) {
  const color = getServiceStatusColor(status);
  const label = status === 'active' ? '运行中'
    : status === 'running' ? '运行中'
    : status === 'inactive' ? '已停止'
    : status === 'stopped' ? '已停止'
    : status === 'failed' ? '异常'
    : status === 'not_installed' ? '未安装'
    : status === 'partial' ? '部分运行'
    : status;
  return <Tag color={color}>{label}</Tag>;
}
