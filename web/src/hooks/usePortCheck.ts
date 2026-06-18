import { useState, useCallback, useRef } from 'react';
import { systemApi } from '../services/api';

interface PortCheckResult {
  available: boolean;
  port: number;
  process?: string;
  message: string;
}

export function usePortCheck() {
  const [result, setResult] = useState<PortCheckResult | null>(null);
  const [checking, setChecking] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const checkPort = useCallback((port: number) => {
    if (timerRef.current) clearTimeout(timerRef.current);

    if (!port || port < 1 || port > 65535) {
      setResult(null);
      return;
    }

    // Debounce 500ms
    timerRef.current = setTimeout(async () => {
      setChecking(true);
      try {
        const res = await systemApi.checkPort(port);
        setResult(res.data?.data || null);
      } catch {
        setResult(null);
      } finally {
        setChecking(false);
      }
    }, 500);
  }, []);

  const clearResult = useCallback(() => {
    setResult(null);
  }, []);

  return { result, checking, checkPort, clearResult };
}
