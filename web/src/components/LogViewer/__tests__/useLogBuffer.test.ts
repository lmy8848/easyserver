// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLogBuffer } from '../useLogBuffer';

describe('useLogBuffer Hook', () => {
  it('should initialize with empty entries', () => {
    const { result } = renderHook(() => useLogBuffer());
    expect(result.current.entries).toEqual([]);
    expect(result.current.totalCount).toBe(0);
    expect(result.current.getPlainText()).toBe('');
  });

  it('should append single line and split multiline texts', () => {
    const { result } = renderHook(() => useLogBuffer());
    act(() => {
      result.current.appendLine('Line 1\nLine 2');
    });

    expect(result.current.entries).toHaveLength(2);
    expect(result.current.entries[0]?.text).toBe('Line 1');
    expect(result.current.entries[1]?.text).toBe('Line 2');
    expect(result.current.totalCount).toBe(2);
  });

  it('should trim oldest 25% when exceeding maxLines capacity', () => {
    const { result } = renderHook(() => useLogBuffer({ maxLines: 10 }));

    act(() => {
      // Append 10 lines
      for (let i = 1; i <= 10; i++) {
        result.current.appendLine(`Line ${i}`);
      }
    });
    expect(result.current.entries).toHaveLength(10);
    expect(result.current.entries[0]?.text).toBe('Line 1');

    act(() => {
      // Append 11th line - should trigger trim
      result.current.appendLine('Line 11');
    });

    // Capacity is 10, trimmed by 25% (dropping ~2-3 oldest lines), new length <= 10
    expect(result.current.entries.length).toBeLessThanOrEqual(10);
    // Line 1 should have been evicted
    expect(result.current.entries.some((e) => e.text === 'Line 1')).toBe(false);
    // Line 11 should be at the end
    expect(result.current.entries[result.current.entries.length - 1]?.text).toBe('Line 11');
  });

  it('should filter entries with searchKeyword', () => {
    const { result } = renderHook(() => useLogBuffer());

    act(() => {
      result.current.appendLines([
        'Error: connection refused',
        'Info: listening on port 8080',
        'Warning: deprecated API',
        'error: second failure',
      ]);
    });

    expect(result.current.totalCount).toBe(4);
    expect(result.current.filteredEntries).toHaveLength(4);

    act(() => {
      result.current.setSearchKeyword('error');
    });

    expect(result.current.filteredEntries).toHaveLength(2);
    expect(result.current.matchCount).toBe(2);
    expect(result.current.filteredEntries[0]?.text).toBe('Error: connection refused');
    expect(result.current.filteredEntries[1]?.text).toBe('error: second failure');
  });

  it('should extract plain text with ANSI stripped', () => {
    const { result } = renderHook(() => useLogBuffer());

    act(() => {
      result.current.appendLine('\x1b[31mError\x1b[0m: disk full');
      result.current.appendLine('\x1b[32mSuccess\x1b[0m');
    });

    expect(result.current.getPlainText()).toBe('Error: disk full\nSuccess');
  });

  it('should clear entries without wiping searchKeyword', () => {
    const { result } = renderHook(() => useLogBuffer());

    act(() => {
      result.current.appendLine('Line 1');
      result.current.setSearchKeyword('test keyword');
    });
    expect(result.current.searchKeyword).toBe('test keyword');

    act(() => {
      result.current.clear();
    });

    expect(result.current.entries).toHaveLength(0);
    expect(result.current.totalCount).toBe(0);
    expect(result.current.searchKeyword).toBe('test keyword');
  });
});
