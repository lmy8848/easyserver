import { useRef, useCallback, useEffect, useState } from 'react';

export type WSStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

interface UseWebSocketOptions {
  /** WebSocket path (without token query param). E.g. '/ws/monitor' */
  path: string;
  /** Called when connection opens */
  onOpen?: (ws: WebSocket) => void;
  /** Called for each message. Receives parsed JSON or raw string. */
  onMessage?: (data: any, raw: string) => void;
  /** Called when connection closes. Return true to prevent auto-reconnect. */
  onClose?: (event: CloseEvent) => boolean | void;
  /** Called on error */
  onError?: (event: Event) => void;
  /** Status change callback */
  onStatusChange?: (status: WSStatus) => void;
  /** Whether to auto-reconnect (default: true) */
  autoReconnect?: boolean;
  /** Max reconnect attempts (default: 10) */
  maxReconnectAttempts?: number;
  /** Base reconnect delay in ms (default: 3000) */
  reconnectDelay?: number;
  /** Ping interval in ms (default: 30000, 0 to disable) */
  pingInterval?: number;
  /** Custom ping message (default: { type: 'ping' }) */
  pingMessage?: string;
  /** Whether to connect immediately (default: true) */
  enabled?: boolean;
}

interface UseWebSocketReturn {
  /** Send a message (string or object to JSON.stringify) */
  send: (data: string | object) => void;
  /** Close the connection */
  close: () => void;
  /** Reconnect manually */
  reconnect: () => void;
  /** Current status */
  status: WSStatus;
  /** The underlying WebSocket (null if not connected) */
  ws: WebSocket | null;
}

/**
 * Shared WebSocket hook with secure auth via Sec-WebSocket-Protocol header.
 * Extracted from Dashboard, Terminal, Services duplicated logic.
 */
export function useWebSocket(options: UseWebSocketOptions): UseWebSocketReturn {
  const {
    path,
    onOpen,
    onMessage,
    onClose,
    onError,
    onStatusChange,
    autoReconnect = true,
    maxReconnectAttempts = 10,
    reconnectDelay = 3000,
    pingInterval = 30000,
    pingMessage = JSON.stringify({ type: 'ping' }),
    enabled = true,
  } = options;

  const wsRef = useRef<WebSocket | null>(null);
  const [ws, setWs] = useState<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pingTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const [status, setStatus] = useState<WSStatus>('disconnected');
  const disposedRef = useRef(false);

  // Stable callback refs
  const onOpenRef = useRef(onOpen);
  const onMessageRef = useRef(onMessage);
  const onCloseRef = useRef(onClose);
  const onErrorRef = useRef(onError);
  const onStatusChangeRef = useRef(onStatusChange);

  useEffect(() => {
    onOpenRef.current = onOpen;
    onMessageRef.current = onMessage;
    onCloseRef.current = onClose;
    onErrorRef.current = onError;
    onStatusChangeRef.current = onStatusChange;
  });

  const setStatusInternal = useCallback((newStatus: WSStatus) => {
    setStatus(newStatus);
    onStatusChangeRef.current?.(newStatus);
  }, []);

  const cleanupTimers = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (pingTimerRef.current) {
      clearInterval(pingTimerRef.current);
      pingTimerRef.current = null;
    }
  }, []);

  const connectRef = useRef<() => void>(() => {});

  const connect = useCallback(() => {
    if (disposedRef.current) return;
    const token = localStorage.getItem('token');
    if (!token) return;

    cleanupTimers();

    // Close existing connection
    if (wsRef.current) {
      try { wsRef.current.close(); } catch (e) { console.debug('WebSocket close error:', e); }
    }

    const isReconnect = reconnectAttemptsRef.current > 0;
    setStatusInternal(isReconnect ? 'reconnecting' : 'connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}${path}`;

    // Pass token via Sec-WebSocket-Protocol header (server already supports this)
    const ws = new WebSocket(wsUrl, ['token', token]);
    wsRef.current = ws;
    setWs(ws);

    ws.onopen = () => {
      reconnectAttemptsRef.current = 0;
      setStatusInternal('connected');

      // Start ping interval
      if (pingInterval > 0) {
        pingTimerRef.current = setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(pingMessage);
          }
        }, pingInterval);
      }

      onOpenRef.current?.(ws);
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        onMessageRef.current?.(msg, event.data);
      } catch {
        onMessageRef.current?.(event.data, event.data);
      }
    };

    ws.onerror = (event) => {
      onErrorRef.current?.(event);
    };

    ws.onclose = (event) => {
      cleanupTimers();

      if (disposedRef.current) return;

      // Let caller handle close (e.g., auth failure -> redirect)
      const preventReconnect = onCloseRef.current?.(event);

      if (preventReconnect) return;

      // Don't reconnect if token is gone
      if (!localStorage.getItem('token')) return;

      if (!autoReconnect) {
        setStatusInternal('disconnected');
        return;
      }

      // Reconnect with exponential backoff
      if (reconnectAttemptsRef.current >= maxReconnectAttempts) {
        console.error('Max WebSocket reconnect attempts reached');
        setStatusInternal('disconnected');
        return;
      }

      const delay = reconnectDelay * Math.pow(2, reconnectAttemptsRef.current);
      reconnectAttemptsRef.current++;

      console.log(`WebSocket reconnecting in ${delay}ms (attempt ${reconnectAttemptsRef.current}/${maxReconnectAttempts})`);
      setStatusInternal('reconnecting');
      reconnectTimerRef.current = setTimeout(() => connectRef.current(), delay);
    };
  }, [path, autoReconnect, maxReconnectAttempts, reconnectDelay, pingInterval, pingMessage, cleanupTimers, setStatusInternal]);

  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  const send = useCallback((data: string | object) => {
    const ws = wsRef.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(typeof data === 'string' ? data : JSON.stringify(data));
    }
  }, []);

  const close = useCallback(() => {
    disposedRef.current = true;
    cleanupTimers();
    if (wsRef.current) {
      try { wsRef.current.close(); } catch (e) { console.debug('WebSocket close error:', e); }
      wsRef.current = null;
      setWs(null);
    }
    setStatusInternal('disconnected');
  }, [cleanupTimers, setStatusInternal]);

  const reconnect = useCallback(() => {
    reconnectAttemptsRef.current = 0;
    disposedRef.current = false;
    connect();
  }, [connect]);

  // Connect on mount / when enabled changes
  useEffect(() => {
    if (!enabled) return;
    disposedRef.current = false;
    connect();
    return () => {
      disposedRef.current = true;
      cleanupTimers();
      if (wsRef.current) {
        try { wsRef.current.close(); } catch { /* ignore close error */ }
        wsRef.current = null;
        setWs(null);
      }
    };
  }, [connect, enabled, cleanupTimers]);

  return { send, close, reconnect, status, ws };
}
