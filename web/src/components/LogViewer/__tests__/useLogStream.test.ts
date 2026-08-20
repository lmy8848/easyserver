// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLogStream } from '../useLogStream';
import { useLogBuffer } from '../useLogBuffer';

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  readyState = 0;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close = vi.fn();

  emitOpen() {
    this.readyState = 1;
    this.onopen?.();
  }

  emitMessage(data: string) {
    this.onmessage?.({ data });
  }

  emitError() {
    this.onerror?.();
  }
}

describe('useLogStream Hook', () => {
  beforeEach(() => {
    MockEventSource.instances = [];
    // @ts-expect-error Mock global EventSource
    global.EventSource = MockEventSource;
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should initialize and connect when enabled with path', () => {
    const { result } = renderHook(() =>
      useLogStream({ path: '/api/logs/test', enabled: true })
    );

    expect(result.current.status).toBe('connecting');
    expect(MockEventSource.instances).toHaveLength(1);

    const es = MockEventSource.instances[0]!;
    act(() => {
      es.emitOpen();
    });

    expect(result.current.status).toBe('streaming');
  });

  it('should handle task log lines and done event', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/logs/task',
        buffer,
        onDone,
      });
      return { buffer, stream };
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitOpen();
      es.emitMessage(JSON.stringify({ type: 'line', text: 'Compiling files...' }));
      es.emitMessage(JSON.stringify({ type: 'line', text: 'Build completed successfully.' }));
    });

    expect(result.current.buffer.entries).toHaveLength(2);
    expect(result.current.buffer.entries[0]?.text).toBe('Compiling files...');

    act(() => {
      es.emitMessage(JSON.stringify({ type: 'done' }));
    });

    expect(result.current.stream.status).toBe('completed');
    expect(es.close).toHaveBeenCalled();
    expect(onDone).toHaveBeenCalledWith({ status: 'completed', error: undefined, exitCode: undefined });
  });

  it('should handle error done event and record error message', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/logs/task',
        buffer,
        onDone,
      });
      return { buffer, stream };
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitOpen();
      es.emitMessage(JSON.stringify({ type: 'line', text: 'Fatal error occurred' }));
      es.emitMessage(JSON.stringify({ type: 'done', error: 'Package missing' }));
    });

    expect(result.current.stream.status).toBe('failed');
    expect(result.current.stream.error).toBe('Package missing');
    expect(onDone).toHaveBeenCalledWith({
      status: 'failed',
      error: 'Package missing',
      exitCode: undefined,
    });
  });

  it('should handle connection error with failed status and not completed', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/logs/task',
        buffer,
        onDone,
      });
      return { buffer, stream };
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitError();
    });

    expect(result.current.stream.status).toBe('failed');
    expect(onDone).toHaveBeenCalledWith({
      status: 'failed',
      error: '连接失败或已断开',
      exitCode: undefined,
    });
  });

  it('should report stopped status and trigger onDone once on close', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/logs/task',
        buffer,
        onDone,
      });
      return { buffer, stream };
    });

    act(() => {
      result.current.stream.close();
    });

    expect(result.current.stream.status).toBe('stopped');
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(onDone).toHaveBeenCalledWith({
      status: 'stopped',
      error: undefined,
      exitCode: undefined,
    });
  });

  it('should guard against duplicate terminal triggers', () => {
    const onDone = vi.fn();
    renderHook(() => {
      const buffer = useLogBuffer();
      return useLogStream({
        path: '/api/logs/custom',
        buffer,
        onDone,
        onMessage: (_data, helpers) => {
          helpers.close();
          return true; // returns true after calling close
        },
      });
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitOpen();
      es.emitMessage(JSON.stringify({ text: 'sample' }));
    });

    // onDone should only be triggered once
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(onDone).toHaveBeenCalledWith({
      status: 'stopped',
      error: undefined,
      exitCode: undefined,
    });
  });

  it('should handle cron script log format with exit code', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/cron/scripts/1/logs',
        buffer,
        onDone,
      });
      return { buffer, stream };
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitOpen();
      es.emitMessage(
        JSON.stringify({
          type: 'log',
          data: { time: '12:00:00', message: 'Script started', stream: 'stdout' },
        })
      );
      es.emitMessage(
        JSON.stringify({
          type: 'log',
          data: { time: '12:00:01', message: 'Warning in line 5', stream: 'stderr' },
        })
      );
      es.emitMessage(JSON.stringify({ type: 'exit', code: 0 }));
    });

    expect(result.current.buffer.entries).toHaveLength(2);
    expect(result.current.buffer.entries[0]?.level).toBe('stdout');
    expect(result.current.buffer.entries[1]?.level).toBe('stderr');
    expect(result.current.stream.status).toBe('completed');
    expect(result.current.stream.exitCode).toBe(0);
  });

  it('should support returning "done" string from onMessage handler', () => {
    const onDone = vi.fn();
    const { result } = renderHook(() => {
      const buffer = useLogBuffer();
      const stream = useLogStream({
        path: '/api/logs/custom',
        buffer,
        onDone,
        onMessage: (data, helpers) => {
          helpers.buffer?.appendLine(String(data));
          return 'done';
        },
      });
      return { buffer, stream };
    });

    const es = MockEventSource.instances[0]!;

    act(() => {
      es.emitOpen();
      es.emitMessage('finish signal');
    });

    expect(result.current.buffer.entries).toHaveLength(1);
    expect(result.current.stream.status).toBe('completed');
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(onDone).toHaveBeenCalledWith({
      status: 'completed',
      error: undefined,
      exitCode: undefined,
    });
  });
});

