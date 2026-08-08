// 调度纯函数：预设频率 → OnCalendar 表达式、中文描述、下次执行时间计算。
// 全部本地计算，不请求后端（与后端 internal/cron/converter.go 逻辑一致）。

export interface ScheduleForm {
  frequency: 'secondly' | 'minutely' | 'hourly' | 'daily' | 'weekly' | 'monthly' | 'every_n_days';
  every_n?: number;
  time?: string;
  weekdays?: string[];
  day_of_month?: number;
}

// buildOnCalendar 把预设频率表单转为 systemd OnCalendar 表达式。
export function buildOnCalendar(f: ScheduleForm): string {
  switch (f.frequency) {
    case 'secondly':
      return `*:*:0/${f.every_n || 1}`;
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
    case 'every_n_days':
      // 从每月 1 号起每隔 N 天（1、N+1、2N+1...）。systemd 无跨月绝对间隔日，此为最接近近似。
      return `*-*-01/${f.every_n || 1} ${f.time || '03:00'}:00`;
    default:
      return '';
  }
}

// describeSchedule 返回表达式 + 中文描述（表单预览用）。
export function describeSchedule(f: ScheduleForm): { on_calendar: string; description: string } {
  const on_calendar = buildOnCalendar(f);
  let description = '';
  switch (f.frequency) {
    case 'secondly':
      description = `每 ${f.every_n || 1} 秒执行`;
      break;
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
    case 'every_n_days':
      description = `每 ${f.every_n || 1} 天（${f.time || '03:00'}）执行`;
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
    const dayTok = parts[parts.length - 1]!;
    let month: number | null = null;
    if (parts.length >= 2) {
      const m = parts[parts.length - 2]!;
      if (m !== '*') month = parseInt(m, 10); // `*-*-01` 的月份为任意
    }
    const dayStep = dayTok.match(/^(\d+)\/(\d+)$/);
    if (dayStep) {
      // 每 N 天：*-*-01/N（从每月 01 号起每隔 N 天取一天）
      const start = parseInt(dayStep[1]!, 10);
      const n = parseInt(dayStep[2]!, 10);
      if (t.getDate() < start || (t.getDate() - start) % n !== 0) return false;
    } else {
      const day = parseInt(dayTok, 10);
      if (day !== t.getDate()) return false;
    }
    if (month !== null && month !== t.getMonth() + 1) return false;
  }

  // 秒级步长：*:*:0/N（每 N 秒）
  const secStep = timeRule.match(/^\*:\*:(\d+)\/(\d+)/);
  if (secStep) {
    const start = parseInt(secStep[1]!, 10);
    const n = parseInt(secStep[2]!, 10);
    return t.getSeconds() >= start && (t.getSeconds() - start) % n === 0;
  }

  // 分钟步长：*:00/N（每分钟）
  const minStep = timeRule.match(/^\*:(\d+)\/(\d+)/);
  if (minStep) {
    const start = parseInt(minStep[1]!, 10);
    const n = parseInt(minStep[2]!, 10);
    return t.getSeconds() === 0 && t.getMinutes() >= start && (t.getMinutes() - start) % n === 0;
  }

  // 小时步长：0/N:00:00（每小时）
  const hourStep = timeRule.match(/^(\d+)\/(\d+):/);
  if (hourStep) {
    const start = parseInt(hourStep[1]!, 10);
    const n = parseInt(hourStep[2]!, 10);
    return t.getSeconds() === 0 && t.getMinutes() === 0 &&
      t.getHours() >= start && (t.getHours() - start) % n === 0;
  }

  // 固定时间：HH:MM:SS
  const hm = timeRule.split(':');
  const hh = parseInt(hm[0]!, 10);
  const mm = parseInt(hm[1]!, 10);
  const ss = hm.length > 2 ? parseInt(hm[2]!, 10) : 0;
  return t.getHours() === hh && t.getMinutes() === mm && t.getSeconds() === ss;
}

function fmt(t: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  const wd = ['日', '一', '二', '三', '四', '五', '六'][t.getDay()];
  return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())} ${p(t.getHours())}:${p(t.getMinutes())}:${p(t.getSeconds())} (周${wd})`;
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

  const isSeconds = /^\*:\*:/.test(timeRule);
  const t = new Date(from);
  if (isSeconds) {
    // 秒级：从当前下一秒开始找，避免命中当前秒。
    t.setSeconds(t.getSeconds() + 1, 0);
  } else {
    t.setSeconds(0, 0);
    t.setMinutes(t.getMinutes() + 1);
  }
  const maxIter = isSeconds ? 366 * 24 * 3600 : 366 * 24 * 60; // 兜底：最多扫一年
  for (let i = 0; i < maxIter; i++) {
    if (matchesOnCalendar(t, dowRule, dateRule, timeRule)) return fmt(t);
    if (isSeconds) t.setSeconds(t.getSeconds() + 1);
    else t.setMinutes(t.getMinutes() + 1);
  }
  return '';
}

const DOW_CN: Record<string, string> = {
  Mon: '周一', Tue: '周二', Wed: '周三', Thu: '周四', Fri: '周五', Sat: '周六', Sun: '周日',
};

// describeOnCalendar 把 systemd OnCalendar 表达式转成可读中文描述。
// 覆盖预设频率生成的表达式（`*:*:0/N`、`*:00/N`、`*-*-* H/N:00:00`、
// `*-*-* HH:MM:00`、`Mon,... *-*-* HH:MM:00`、`*-*-DD HH:MM:00`、
// `*-*-01/N HH:MM:00`）。无法识别时回退为原表达式。
export function describeOnCalendar(expr: string): string {
  const e = expr.trim();
  if (!e) return e;
  const tokens = e.split(/\s+/).filter(Boolean);
  const timeTok = tokens.find(t => t.includes(':'));

  // 秒级：*:*:0/N
  const secStep = expr.match(/^\*:\*:(\d+)\/(\d+)/);
  if (secStep) return `每 ${secStep[2]} 秒执行`;

  // 分钟级：*:00/N
  const minStep = expr.match(/^\*:(\d+)\/(\d+)/);
  if (minStep) return `每 ${minStep[2]} 分钟执行`;

  // 小时级：*-*-* H/N:00:00
  const hourStep = timeTok?.match(/^(\d+)\/(\d+):00:00/);
  if (hourStep) return `每 ${hourStep[2]} 小时执行`;

  // 确定时间：HH:MM:00
  const hm = timeTok?.match(/^(\d{2}):(\d{2}):00$/);
  if (hm) {
    const time = `${hm[1]}:${hm[2]}`;
    const dowTok = tokens.find(t => /^(Mon|Tue|Wed|Thu|Fri|Sat|Sun)/i.test(t));
    const dateTok = tokens.find(t => !t.includes(':') && !/^(Mon|Tue|Wed|Thu|Fri|Sat|Sun)/i.test(t));
    if (dowTok) {
      const dows = dowTok.split(',').map(d => DOW_CN[d] || d).join('、');
      return `每周 ${dows} ${time}`;
    }
    if (dateTok) {
      // 每 N 天：*-*-01/N
      const last = dateTok.split('-').pop();
      const dayStep = last?.match(/^(\d{2})\/(\d+)$/);
      if (dayStep) return `每 ${dayStep[2]} 天（${time}）`;
      if (last && /^\d+$/.test(last)) return `每月 ${parseInt(last, 10)} 号 ${time}`;
    }
    // 默认每天（`*-*-* HH:MM:00`）
    return `每天 ${time}`;
  }

  return e;
}