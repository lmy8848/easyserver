import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';

dayjs.extend(duration);

/**
 * 格式化字节大小为人类可读字符串（如 1.5 KB / 24.3 MB / 2.1 GB）
 */
export function formatBytes(bytes: number | undefined | null, decimals = 1): string {
  if (bytes === undefined || bytes === null || isNaN(bytes) || bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  if (i === 0) return `${bytes} B`;
  const val = bytes / Math.pow(k, i);
  return `${val.toFixed(decimals)} ${sizes[i] || 'B'}`;
}

/**
 * formatBytes 的反向操作：将人类可读的大小字符串（如 "1.5 GB"、"500MB"、"100KB"、"2048 B"）解析为字节数（bytes）。
 * 支持各种常见单位格式（B, KB, MB, GB, TB, PB 以及 K, M, G, T, P）。
 */
export function parseBytes(input: string | number | undefined | null): number {
  if (input === undefined || input === null) return 0;
  if (typeof input === 'number') return isNaN(input) ? 0 : input;

  const trimmed = String(input).trim();
  if (!trimmed) return 0;

  const match = trimmed.match(/^([+-]?(?:\d*\.)?\d+)\s*([a-zA-Z]*)$/);
  if (!match || !match[1]) return 0;

  const val = parseFloat(match[1]);
  if (isNaN(val)) return 0;

  const unit = (match[2] || '').toUpperCase();
  const unitMultipliers: Record<string, number> = {
    '': 1,
    B: 1,
    BYTE: 1,
    BYTES: 1,
    K: 1024,
    KB: 1024,
    KIB: 1024,
    M: 1024 * 1024,
    MB: 1024 * 1024,
    MIB: 1024 * 1024,
    G: 1024 * 1024 * 1024,
    GB: 1024 * 1024 * 1024,
    GIB: 1024 * 1024 * 1024,
    T: 1024 * 1024 * 1024 * 1024,
    TB: 1024 * 1024 * 1024 * 1024,
    TIB: 1024 * 1024 * 1024 * 1024,
    P: 1024 * 1024 * 1024 * 1024 * 1024,
    PB: 1024 * 1024 * 1024 * 1024 * 1024,
    PIB: 1024 * 1024 * 1024 * 1024 * 1024,
  };

  const multiplier = unitMultipliers[unit];
  if (multiplier === undefined) return 0;

  return Math.round(val * multiplier);
}

/**
 * 格式化运行时间（秒数）为易读字符串（如 2天3小时10分钟 或 2d 3h 10m）
 */
export function formatUptime(seconds: number | undefined | null, short = false): string {
  if (!seconds || seconds <= 0) return short ? '-' : '0分钟';
  const dur = dayjs.duration(seconds, 'seconds');
  const days = Math.floor(dur.asDays());
  const hours = dur.hours();
  const minutes = dur.minutes();

  if (short) {
    if (days > 0) return `${days}d ${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  }

  if (days > 0) return `${days}天${hours}小时${minutes}分钟`;
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  return `${minutes}分钟`;
}


/**
 * 格式化时间戳/字符串为 `YYYY-MM-DD HH:mm:ss`
 */
export function formatDateTime(
  value: string | number | Date | dayjs.Dayjs | null | undefined,
  template = 'YYYY-MM-DD HH:mm:ss'
): string {
  if (!value) return '-';
  const parsedVal = typeof value === 'string' && / [A-Z]{3,4}$/.test(value)
    ? value.replace(/ [A-Z]{3,4}$/, '')
    : value;
  const d = dayjs(parsedVal);
  return d.isValid() ? d.format(template) : String(value);
}
