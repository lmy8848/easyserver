import { useRef, useCallback, useEffect, useState } from 'react';

export type SSEStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

interface UseSSEOptions {
  /** SSE 相对路径，如 '/api/monitor/stream' */
  path: string;
  /** 每个 SSE 事件（data: 行）解析后的回调 */
  onMessage?: (data: any) => void;
  /** 连接关闭（非主动）时回调 */
  onClose?: () => void;
  /** 是否连接（默认 true） */
  enabled?: boolean;
  /** 断线自动重连（默认 true） */
  autoReconnect?: boolean;
  /** 最大重连次数（默认 10） */
  maxReconnectAttempts?: number;
  /** 基础重连延迟 ms（默认 3000，指数退避） */
  reconnectDelay?: number;
}

interface UseSSEReturn {
  status: SSEStatus;
  /** 主动关闭连接（清 timer + abort fetch） */
  close: () => void;
}

/**
 * 基于 fetch + ReadableStream 的 SSE 客户端。
 *
 * 用 fetch 而非原生 EventSource：EventSource 无法自定义请求头，而本项目鉴权走
 * Authorization: Bearer，token 在 localStorage。fetch 可带 header，且无跨域 cookie 之扰。
 * 每次事件按 "data: <payload>\n\n" 解析，payload 为 JSON 字符串。
 */
export function useSSE(options: UseSSEOptions): UseSSEReturn {
  const {
    path,
    onMessage,
    onClose,
    enabled = true,
    autoReconnect = true,
    maxReconnectAttempts = 10,
    reconnectDelay = 3000,
  } = options;

  const [status, setStatus] = useState<SSEStatus>('disconnected');
  const ctrlRef = useRef<AbortController | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptRef = useRef(0);
  const disposedRef = useRef(false);

  // Stable callback refs
  const onMessageRef = useRef(onMessage);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onMessageRef.current = onMessage;
    onCloseRef.current = onClose;
  });

  const stop = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (ctrlRef.current) {
      ctrlRef.current.abort();
      ctrlRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    if (disposedRef.current) return;
    const token = localStorage.getItem('token');
    if (!token) return;

    stop();

    const isReconnect = attemptRef.current > 0;
    setStatus(isReconnect ? 'reconnecting' : 'connecting');

    const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:';
    const url = `${protocol}//${window.location.host}${path}`;
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;

    const handleEnd = () => {
      setStatus('disconnected');
      onCloseRef.current?.();
      if (!autoReconnect) return;
      if (disposedRef.current) return;
      if (!localStorage.getItem('token')) return;
      if (attemptRef.current >= maxReconnectAttempts) {
        setStatus('disconnected');
        return;
      }
      const delay = reconnectDelay * Math.pow(2, attemptRef.current);
      attemptRef.current++;
      setStatus('reconnecting');
      console.log(`SSE reconnecting in ${delay}ms (attempt ${attemptRef.current}/${maxReconnectAttempts})`);
      timerRef.current = setTimeout(() => connect(), delay);
    };

    fetch(url, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then(async (res) => {
        if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
        setStatus('connected');
        attemptRef.current = 0;
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          // 按事件分隔符 \n\n 切分
          let idx: number;
          while ((idx = buffer.indexOf('\n\n')) >= 0) {
            const raw = buffer.slice(0, idx);
            buffer = buffer.slice(idx + 2);
            const dataLine = raw.split('\n').find((l) => l.startsWith('data:'));
            if (dataLine) {
              const payload = dataLine.slice(5).trim();
              if (payload) {
                try {
                  onMessageRef.current?.(JSON.parse(payload));
                } catch {
                  onMessageRef.current?.(payload);
                }
              }
            }
          }
        }
        handleEnd();
      })
      .catch((err) => {
        if (err?.name === 'AbortError') return; // 主动关闭
        handleEnd();
      });
  }, [path, autoReconnect, maxReconnectAttempts, reconnectDelay, stop]);

  const connectRef = useRef(connect);
  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  const close = useCallback(() => {
    disposedRef.current = true;
    stop();
    setStatus('disconnected');
  }, [stop]);

  useEffect(() => {
    if (!enabled) return;
    disposedRef.current = false;
    connect();
    return () => {
      disposedRef.current = true;
      stop();
    };
  }, [enabled, connect, stop]);

  return { status, close };
}