import { useState, useCallback } from 'react';

/**
 * Wraps the repetitive "set loading true → await fn → set loading false"
 * pattern shared by every write operation. Returns a boolean loading flag and
 * a `run` that wraps an async fn. The wrapped promise still rejects, so the
 * caller keeps its own try/catch for error messages.
 *
 * Prefer this for single-button loading (modal confirm, a lone action). For
 * per-row actions (start/stop/delete in a table) use one keyed state instead
 * so all rows share a single loading flag.
 */
export function useAsyncRun() {
  const [loading, setLoading] = useState(false);
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T> => {
    setLoading(true);
    try {
      return await fn();
    } finally {
      setLoading(false);
    }
  }, []);
  return [loading, run] as const;
}