import { useCallback, useMemo, useState } from 'react';
import { stripAnsi, splitLinesWithCr } from './ansi';
import type { LogEntry, LogLevel, UseLogBufferOptions, UseLogBufferReturn } from './types';

const DEFAULT_MAX_LINES = 5000;

function trimOverflow(entries: LogEntry[], maxLines: number): LogEntry[] {
  if (entries.length <= maxLines) return entries;
  // Discard the oldest 25% to minimize frequent reallocations
  const keepCount = Math.floor(maxLines * 0.75);
  return entries.slice(entries.length - keepCount);
}

export function useLogBuffer(options: UseLogBufferOptions = {}): UseLogBufferReturn {
  const { maxLines = DEFAULT_MAX_LINES, initialEntries = [] } = options;

  const [entries, setEntriesState] = useState<LogEntry[]>(() =>
    trimOverflow(initialEntries, maxLines)
  );
  const [searchKeyword, setSearchKeyword] = useState<string>('');

  const appendEntries = useCallback(
    (newEntries: LogEntry[]) => {
      if (newEntries.length === 0) return;
      setEntriesState((prev) => trimOverflow([...prev, ...newEntries], maxLines));
    },
    [maxLines]
  );

  const appendEntry = useCallback(
    (entry: LogEntry) => {
      appendEntries([entry]);
    },
    [appendEntries]
  );

  const appendLines = useCallback(
    (lines: string[], level?: LogLevel | string) => {
      const newEntries: LogEntry[] = [];
      lines.forEach((line) => {
        const subLines = splitLinesWithCr(line);
        subLines.forEach((text) => {
          newEntries.push({ text, level });
        });
      });
      appendEntries(newEntries);
    },
    [appendEntries]
  );

  const appendLine = useCallback(
    (text: string, level?: LogLevel | string, time?: string) => {
      const subLines = splitLinesWithCr(text);
      const newEntries: LogEntry[] = subLines.map((t) => ({ text: t, level, time }));
      appendEntries(newEntries);
    },
    [appendEntries]
  );

  const setEntries = useCallback(
    (updater: LogEntry[] | ((prev: LogEntry[]) => LogEntry[])) => {
      setEntriesState((prev) => {
        const next = typeof updater === 'function' ? updater(prev) : updater;
        return trimOverflow(next, maxLines);
      });
    },
    [maxLines]
  );

  const clear = useCallback(() => {
    setEntriesState([]);
    setSearchKeyword('');
  }, []);

  const totalCount = entries.length;

  const filteredEntries = useMemo(() => {
    const trimmed = searchKeyword.trim().toLowerCase();
    if (!trimmed) return entries;
    return entries.filter((e) => {
      const clean = stripAnsi(e.text).toLowerCase();
      return (
        clean.includes(trimmed) ||
        (e.level && String(e.level).toLowerCase().includes(trimmed)) ||
        (e.time && e.time.toLowerCase().includes(trimmed))
      );
    });
  }, [entries, searchKeyword]);

  const matchCount = useMemo(() => {
    if (!searchKeyword.trim()) return 0;
    return filteredEntries.length;
  }, [filteredEntries.length, searchKeyword]);

  const getPlainText = useCallback(() => {
    return entries.map((e) => stripAnsi(e.text)).join('\n');
  }, [entries]);

  return {
    entries,
    filteredEntries,
    totalCount,
    matchCount,
    searchKeyword,
    setSearchKeyword,
    appendLine,
    appendLines,
    appendEntry,
    appendEntries,
    setEntries,
    clear,
    getPlainText,
  };
}
