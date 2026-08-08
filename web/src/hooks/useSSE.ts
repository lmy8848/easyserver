import { useEffect, useRef, useState } from 'react';

export type SSEStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

interface UseSSEOptions {
  /** SSE 相对路径，如 '/api/monitor' */
  path: string;
  /** 每个 SSE 事件（data: 行）解析后的回调 */
  onMessage?: (data: any) => void;
  /** 连接关闭（非主动）时回调 */
  onClose?: () => void;
  /** 是否连接（默认 true） */
  enabled?: boolean;
  /** 断线是否自动重连（默认 true）。EventSource 原生自动重连；false 时断开即 close。 */
  autoReconnect?: boolean;
}

interface UseSSEReturn {
  status: SSEStatus;
  /** 主动关闭连接 */
  close: () => void;
}

/**
 * 基于原生 EventSource 的 SSE 客户端。
 *
 * 登录态走 HttpOnly Cookie（同源自动携带），EventSource 无需也无法自定义 header，
 * 因此用二进制 fetch+Bearer 的旧方案整个退役。登录失效由 axios 拦截器/全局
 * loadUser 处理，SSE 不自行判 401（EventSource 的 onerror 拿不到状态码）。
 */
export function useSSE(options: UseSSEOptions): UseSSEReturn {
  const { path, onMessage, onClose, enabled = true, autoReconnect = true } = options;

  const [status, setStatus] = useState<SSEStatus>('disconnected');
  const esRef = useRef<EventSource | null>(null);
  const disposedRef = useRef(false);

  // Stable callback refs
  const onMessageRef = useRef(onMessage);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onMessageRef.current = onMessage;
    onCloseRef.current = onClose;
  });

  useEffect(() => {
    if (!enabled) return;
    disposedRef.current = false;
    setStatus('connecting');

    // 同源相对路径，cookie 自动携带
    const es = new EventSource(path);
    esRef.current = es;

    es.onopen = () => {
      setStatus('connected');
    };

    es.onmessage = (e) => {
      try {
        onMessageRef.current?.(JSON.parse(e.data));
      } catch {
        onMessageRef.current?.(e.data);
      }
    };

    es.onerror = () => {
      if (disposedRef.current) return;
      if (!autoReconnect) {
        // 主动断开（如脚本退出后不再续看）
        es.close();
        esRef.current = null;
        setStatus('disconnected');
        onCloseRef.current?.();
        return;
      }
      // EventSource 会自动重连，仅更新状态
      setStatus('reconnecting');
      onCloseRef.current?.();
    };

    return () => {
      disposedRef.current = true;
      es.close();
      esRef.current = null;
      setStatus('disconnected');
    };
  }, [path, enabled, autoReconnect]);

  const close = () => {
    disposedRef.current = true;
    esRef.current?.close();
    esRef.current = null;
    setStatus('disconnected');
  };

  return { status, close };
}