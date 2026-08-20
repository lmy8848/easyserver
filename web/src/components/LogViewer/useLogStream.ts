import { useEffect, useRef, useState, useCallback } from 'react';
import type { LogEntry, LogStreamStatus, UseLogBufferReturn } from './types';

export interface UseLogStreamOptions {
  /** SSE path (e.g. '/api/runtime/logs/node@20.11.0') */
  path: string;
  /** Whether the stream is active/enabled. Defaults to true. */
  enabled?: boolean;
  /** Log buffer instance. If omitted, useLogStream can operate standalone or with provided buffer. */
  buffer?: UseLogBufferReturn;
  /** Auto reconnect on transient errors. Defaults to false for task/execution logs. */
  autoReconnect?: boolean;
  /** Custom transform handler. Return true or 'done' if stream reached terminal state. */
  onMessage?: (data: unknown, helpers: { buffer?: UseLogBufferReturn; close: () => void }) => boolean | void;
  /** Callback fired when stream reaches a completed or failed terminal state */
  onDone?: (result: { status: 'completed' | 'failed' | 'stopped'; error?: string; exitCode?: number }) => void;
}

export interface UseLogStreamReturn {
  status: LogStreamStatus;
  error: string | null;
  exitCode: number | null;
  elapsedMs: number;
  close: () => void;
  reconnect: () => void;
}

export function useLogStream(options: UseLogStreamOptions): UseLogStreamReturn {
  const { path, enabled = true, buffer, autoReconnect = false, onMessage, onDone } = options;

  const [status, setStatus] = useState<LogStreamStatus>(enabled && path ? 'connecting' : 'idle');
  const [error, setError] = useState<string | null>(null);
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number>(0);

  const startTimeRef = useRef<number>(0);
  const timerRef = useRef<number | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const disposedRef = useRef<boolean>(false);

  // Stable callback refs
  const onMessageRef = useRef(onMessage);
  const onDoneRef = useRef(onDone);
  const bufferRef = useRef(buffer);
  useEffect(() => {
    onMessageRef.current = onMessage;
    onDoneRef.current = onDone;
    bufferRef.current = buffer;
  });

  const stopTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const startTimer = useCallback(() => {
    stopTimer();
    startTimeRef.current = Date.now();
    setElapsedMs(0);
    timerRef.current = window.setInterval(() => {
      setElapsedMs(Date.now() - startTimeRef.current);
    }, 100);
  }, [stopTimer]);

  const close = useCallback(() => {
    disposedRef.current = true;
    stopTimer();
    if (startTimeRef.current > 0) {
      setElapsedMs(Date.now() - startTimeRef.current);
    }
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    setStatus((prev) => (prev === 'streaming' || prev === 'connecting' ? 'stopped' : prev));
  }, [stopTimer]);

  const connect = useCallback(() => {
    if (!enabled || !path) {
      setStatus('idle');
      return;
    }

    disposedRef.current = false;
    setStatus('connecting');
    setError(null);
    setExitCode(null);
    startTimer();

    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }

    const es = new EventSource(path);
    esRef.current = es;

    const handleTerminalDone = (isFailed: boolean, errMsg?: string, code?: number) => {
      stopTimer();
      setElapsedMs(Date.now() - startTimeRef.current);
      es.close();
      esRef.current = null;

      const finalStatus: LogStreamStatus = isFailed ? 'failed' : 'completed';
      setStatus(finalStatus);
      if (errMsg) setError(errMsg);
      if (code !== undefined) setExitCode(code);

      onDoneRef.current?.({
        status: finalStatus,
        error: errMsg,
        exitCode: code,
      });
    };

    es.onopen = () => {
      if (disposedRef.current) return;
      setStatus('streaming');
    };

    es.onmessage = (e) => {
      if (disposedRef.current) return;
      setStatus('streaming');

      let parsed: unknown;
      try {
        parsed = JSON.parse(e.data);
      } catch {
        parsed = e.data;
      }

      // If caller provided custom onMessage handler
      if (onMessageRef.current) {
        const isDone = onMessageRef.current(parsed, {
          buffer: bufferRef.current,
          close: () => handleTerminalDone(false),
        });
        if (isDone) {
          handleTerminalDone(false);
          return;
        }
      }

      // Built-in protocol heuristics
      if (parsed && typeof parsed === 'object') {
        const msg = parsed as Record<string, unknown>;
        const msgType = msg['type'];

        // Pattern 1: Task / Runtime / Database format: { type: 'line', text: '...' } / { type: 'done', error?: '...' }
        if (msgType === 'line' && typeof msg['text'] === 'string') {
          bufferRef.current?.appendLine(msg['text'] as string);
        } else if (msgType === 'done') {
          const errMsg = typeof msg['error'] === 'string' ? (msg['error'] as string) : undefined;
          handleTerminalDone(!!errMsg, errMsg);
          return;
        }

        // Pattern 2: Cron Script format: { type: 'log', data: { time, message, stream } } / { type: 'exit', code } / { type: 'error' }
        else if (msgType === 'log' && msg['data'] && typeof msg['data'] === 'object') {
          const logData = msg['data'] as Record<string, unknown>;
          const msgText = typeof logData['message'] === 'string' ? logData['message'] : '';
          if (msgText) {
            const entry: LogEntry = {
              text: msgText,
              time: typeof logData['time'] === 'string' ? logData['time'] : undefined,
              level: logData['stream'] === 'stderr' ? 'stderr' : 'stdout',
            };
            bufferRef.current?.appendEntry(entry);
          }
        } else if (msgType === 'exit') {
          const code = typeof msg['code'] === 'number' ? (msg['code'] as number) : 0;
          handleTerminalDone(code !== 0, code !== 0 ? `Exit code ${code}` : undefined, code);
          return;
        } else if (msgType === 'error') {
          const errMsg = typeof msg['error'] === 'string' ? (msg['error'] as string) : 'Execution error';
          handleTerminalDone(true, errMsg, -1);
          return;
        }
      } else if (typeof parsed === 'string') {
        bufferRef.current?.appendLine(parsed);
      }
    };

    es.onerror = () => {
      if (disposedRef.current) return;
      if (!autoReconnect) {
        // Stop on error / server disconnect
        stopTimer();
        setElapsedMs(Date.now() - startTimeRef.current);
        es.close();
        esRef.current = null;
        setStatus('completed');
        onDoneRef.current?.({ status: 'completed' });
      }
    };
  }, [enabled, path, autoReconnect, startTimer, stopTimer]);

  useEffect(() => {
    connect();
    return () => {
      disposedRef.current = true;
      stopTimer();
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    };
  }, [connect, stopTimer]);

  const reconnect = useCallback(() => {
    connect();
  }, [connect]);

  return {
    status,
    error,
    exitCode,
    elapsedMs,
    close,
    reconnect,
  };
}
