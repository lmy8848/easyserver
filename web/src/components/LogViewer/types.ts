export type LogLevel = 'info' | 'warn' | 'error' | 'debug' | 'stderr' | 'stdout';

export interface LogEntry {
  id?: string | number;
  time?: string;
  level?: LogLevel | string;
  text: string;
  meta?: Record<string, unknown>;
}

export type LogStreamStatus =
  | 'idle'
  | 'connecting'
  | 'streaming'
  | 'completed'
  | 'failed'
  | 'stopped';

export interface UseLogBufferOptions {
  /** Maximum lines to retain in memory. Defaults to 5000. */
  maxLines?: number;
  /** Initial log entries. */
  initialEntries?: LogEntry[];
}

export interface UseLogBufferReturn {
  entries: LogEntry[];
  filteredEntries: LogEntry[];
  totalCount: number;
  matchCount: number;
  searchKeyword: string;
  setSearchKeyword: (keyword: string) => void;
  appendLine: (text: string, level?: LogLevel | string, time?: string) => void;
  appendLines: (lines: string[], level?: LogLevel | string) => void;
  appendEntry: (entry: LogEntry) => void;
  appendEntries: (newEntries: LogEntry[]) => void;
  setEntries: (entries: LogEntry[] | ((prev: LogEntry[]) => LogEntry[])) => void;
  clear: () => void;
  getPlainText: () => string;
}
