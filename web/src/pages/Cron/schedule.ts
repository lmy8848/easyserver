// 调度纯函数：预设频率 → OnCalendar 表达式、中文描述、下次执行时间计算。
// 全部本地计算，不请求后端（与后端 internal/cron/converter.go 逻辑一致）。

export interface ScheduleForm {
  frequency: 'minutely' | 'hourly' | 'daily' | 'weekly' | 'monthly';
  every_n?: number;
  time?: string;
  weekdays?: string[];
  day_of_month?: number;
}

// buildOnCalendar 把预设频率表单转为 systemd OnCalendar 表达式。
export function buildOnCalendar(f: ScheduleForm): string {
  switch (f.frequency) {
    case 'minutely':
      return `*:00/${f.every_n || 5}`;
    case 'hourly':
      return `*-*-* 0/${f.every_n || 1}:00:00`;
    case 'daily':
      return `*-*-* ${f.time || '03:00'}:00`;
    case 'weekly':
      return `${f.weekdays?.length ? f.weekdays.join(',') : 'Mon'} *-*-* ${f.time || '03:00'}:00`;
    case 'monthly':
      return `*-*-${String(f.day_of_month || 1).padStart(2, '0')} ${f.time || '03:00'}:00`;
    default:
      return '';
  }
}

// describeSchedule 返回表达式 + 中文描述（表单预览用）。
export function describeSchedule(f: ScheduleForm): { on_calendar: string; description: string } {
  const on_calendar = buildOnCalendar(f);
  let description = '';
  switch (f.frequency) {
    case 'minutely':
      description = `每 ${f.every_n || 5} 分钟执行`;
      break;
    case 'hourly':
      description = `每 ${f.every_n || 1} 小时执行`;
      break;
    case 'daily':
      description = `每天 ${f.time || '03:00'} 执行`;
      break;
    case 'weekly':
      description = `每周 ${(f.weekdays || ['Mon']).join('、')} 的 ${f.time || '03:00'} 执行`;
      break;
    case 'monthly':
      description = `每月 ${f.day_of_month || 1} 号 ${f.time || '03:00'} 执行`;
      break;
  }
  return { on_calendar, description };
}

const DOW_MAP: Record<string, number> = {
  Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6, Sun: 0,
};

function parseDow(tok: string): number[] {
  const set = new Set<number>();
  for (const part of tok.split(',')) {
    if (part.includes('..')) {
      const [a, b] = part.split('..');
      let x = DOW_MAP[a!]!;
      const y = DOW_MAP[b!]!;
      while (true) {
        set.add(x);
        if (x === y) break;
        x = (x + 1) % 7;
      }
    } else if (DOW_MAP[part] !== undefined) {
      set.add(DOW_MAP[part]);
    }
  }
  return [...set];
}

function matchesOnCalendar(t: Date, dowRule: number[] | null, dateRule: string | null, timeRule: string): boolean {
  if (dowRule && dowRule.length && !dowRule.includes(t.getDay())) return false;

  if (dateRule && dateRule !== '*' && dateRule !== '*-*' && dateRule !== '*-*-*') {
    const parts = dateRule.split('-');
    const day = parseInt(parts[parts.length - 1]!, 10);
    let month: number | null = null;
    if (parts.length >= 2) {
      const m = parts[parts.length - 2]!;
      if (m !== '*') month = parseInt(m, 10); // `*-*-01` 的月份为任意
    }
    if (day !== t.getDate()) return false;
    if (month !== null && month !== t.getMonth() + 1) return false;
  }

  // 步长：*:00/N（每分钟）或 0/N:00:00（每小时）
  const minStep = timeRule.match(/^\*:(\d+)\/(\d+)/);
  if (minStep) {
    const start = parseInt(minStep[1]!, 10);
    const n = parseInt(minStep[2]!, 10);
    if (t.getMinutes() < start || (t.getMinutes() - start) % n !== 0) return false;
  } else {
    const hourStep = timeRule.match(/^(\d+)\/(\d+):/);
    if (hourStep) {
      const start = parseInt(hourStep[1]!, 10);
      const n = parseInt(hourStep[2]!, 10);
      if (t.getHours() < start || (t.getHours() - start) % n !== 0 || t.getMinutes() !== 0) return false;
    } else {
      const hm = timeRule.split(':');
      const hh = parseInt(hm[0]!, 10);
      const mm = parseInt(hm[1]!, 10);
      if (t.getHours() !== hh || t.getMinutes() !== mm) return false;
    }
  }
  return true;
}

function fmt(t: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  const wd = ['日', '一', '二', '三', '四', '五', '六'][t.getDay()];
  return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())} ${p(t.getHours())}:${p(t.getMinutes())} (周${wd})`;
}

// computeNextRun 计算 OnCalendar 表达式的下次执行时间。无法解析或找不到返回空串。
export function computeNextRun(expr: string, from: Date = new Date()): string {
  const tokens = expr.trim().split(/\s+/).filter(Boolean);
  if (!tokens.length) return '';

  let dowRule: number[] | null = null;
  let dateRule: string | null = null;
  let timeRule: string | null = null;
  for (const tok of tokens) {
    if (tok.includes(':')) timeRule = tok;
    else if (/^(Mon|Tue|Wed|Thu|Fri|Sat|Sun)/i.test(tok)) dowRule = parseDow(tok);
    else dateRule = tok;
  }
  if (!timeRule) return '';

  const t = new Date(from);
  t.setSeconds(0, 0);
  t.setMinutes(t.getMinutes() + 1);
  const maxIter = 366 * 24 * 60; // 兜底：最多扫一年
  for (let i = 0; i < maxIter; i++) {
    if (matchesOnCalendar(t, dowRule, dateRule, timeRule)) return fmt(t);
    t.setMinutes(t.getMinutes() + 1);
  }
  return '';
}