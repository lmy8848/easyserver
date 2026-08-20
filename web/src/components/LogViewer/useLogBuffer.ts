/* eslint-disable no-control-regex */
import { useCallback, useMemo, useState } from 'react';
import { stripAnsi, splitLinesWithCrInfo } from './ansi';
import type { LogEntry, LogLevel, UseLogBufferOptions, UseLogBufferReturn } from './types';

const DEFAULT_MAX_LINES = 5000;

function trimOverflow(entries: LogEntry[], maxLines: number): LogEntry[] {
  if (entries.length <= maxLines) return entries;
  // Discard the oldest 25% to minimize frequent reallocations
  const keepCount = Math.floor(maxLines * 0.75);
  return entries.slice(entries.length - keepCount);
}

/**
 * Processes incoming log entries, handling:
 * 1. ANSI cursor up / clear line (\x1b[1A, \x1b[2K)
 * 2. Carriage return (\r) in-place overwrites across chunks
 */
export function processLogEntries(
  currentEntries: LogEntry[],
  incomingEntries: LogEntry[],
  maxLines: number = DEFAULT_MAX_LINES
): LogEntry[] {
  if (incomingEntries.length === 0) return currentEntries;

  const result = [...currentEntries];

  for (const entry of incomingEntries) {
    const rawText = entry.text ?? '';

    // 1. Check for ANSI cursor up (e.g. \x1b[1A or \x1b[2A)
    const cursorUpMatch = rawText.match(/\x1b\[(\d+)A/);
    if (cursorUpMatch && result.length > 0) {
      const count = parseInt(cursorUpMatch[1] ?? '1', 10);
      const targetIndex = result.length - count;
      if (targetIndex >= 0 && targetIndex < result.length) {
        const textWithoutCursor = rawText
          .replace(/\x1b\[\d+A/g, '')
          .replace(/\x1b\[\d+B/g, '')
          .replace(/\x1b\[2?K/g, '')
          .replace(/^\r+/, '');
        result[targetIndex] = {
          ...result[targetIndex],
          text: textWithoutCursor,
          time: entry.time ?? result[targetIndex]?.time,
          level: entry.level ?? result[targetIndex]?.level,
          meta: entry.meta ?? result[targetIndex]?.meta,
        };
        continue;
      }
    }

    // 2. Check for leading \r or \x1b[2K\r, or previous entry ended with \r (CR overwrite)
    const isLeadingCr = /^(?:\x1b\[2?K)?\r/.test(rawText);
    const prevEndsInCr = result.length > 0 && Boolean(result[result.length - 1]?.meta?.['endsInCr']);

    if ((isLeadingCr || prevEndsInCr) && result.length > 0) {
      const cleanLine = rawText.replace(/^(?:\x1b\[2?K)?\r+/, '');
      result[result.length - 1] = {
        ...result[result.length - 1],
        text: cleanLine,
        time: entry.time ?? result[result.length - 1]?.time,
        level: entry.level ?? result[result.length - 1]?.level,
        meta: entry.meta,
      };
      continue;
    }

    // Default: append as new entry
    result.push(entry);
  }

  return trimOverflow(result, maxLines);
}

export function useLogBuffer(options: UseLogBufferOptions = {}): UseLogBufferReturn {
  const { maxLines = DEFAULT_MAX_LINES, initialEntries = [] } = options;

  const [entries, setEntriesState] = useState<LogEntry[]>(() =>
    processLogEntries([], initialEntries, maxLines)
  );
  const [searchKeyword, setSearchKeyword] = useState<string>('');

  const appendEntries = useCallback(
    (newEntries: LogEntry[]) => {
      if (newEntries.length === 0) return;
      setEntriesState((prev) => processLogEntries(prev, newEntries, maxLines));
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
        const parsed = splitLinesWithCrInfo(line);
        parsed.forEach(({ text, endsInCr }) => {
          newEntries.push({ text, level, meta: endsInCr ? { endsInCr: true } : undefined });
        });
      });
      appendEntries(newEntries);
    },
    [appendEntries]
  );

  const appendLine = useCallback(
    (text: string, level?: LogLevel | string, time?: string) => {
      const parsed = splitLinesWithCrInfo(text);
      const newEntries: LogEntry[] = parsed.map(({ text: t, endsInCr }) => ({
        text: t,
        level,
        time,
        meta: endsInCr ? { endsInCr: true } : undefined,
      }));
      appendEntries(newEntries);
    },
    [appendEntries]
  );

  const setEntries = useCallback(
    (updater: LogEntry[] | ((prev: LogEntry[]) => LogEntry[])) => {
      setEntriesState((prev) => {
        const next = typeof updater === 'function' ? updater(prev) : updater;
        return processLogEntries([], next, maxLines);
      });
    },
    [maxLines]
  );

  const clear = useCallback(() => {
    setEntriesState([]);
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
