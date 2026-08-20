// @vitest-environment jsdom
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LogViewer } from '../LogViewer';

describe('LogViewer Component', () => {
  afterEach(() => {
    cleanup();
  });

  it('should render empty state when no entries', () => {
    render(<LogViewer />);
    expect(screen.getByText('暂无日志输出')).toBeDefined();
  });

  it('should render direct string data via logs prop with line numbers', () => {
    render(<LogViewer logs={'Line 1: Server started\nLine 2: Ready for connections'} />);
    expect(screen.getByText('Line 1: Server started')).toBeDefined();
    expect(screen.getByText('Line 2: Ready for connections')).toBeDefined();
  });

  it('should render direct lines array via lines prop', () => {
    render(<LogViewer lines={['Array line 1', 'Array line 2']} />);
    expect(screen.getByText('Array line 1')).toBeDefined();
    expect(screen.getByText('Array line 2')).toBeDefined();
  });

  it('should filter logs with search input and highlight', () => {
    render(<LogViewer logs={'INFO starting\nERROR connection failed\nINFO retry'} />);
    const searchInput = screen.getByPlaceholderText('搜索日志...');
    fireEvent.change(searchInput, { target: { value: 'ERROR' } });
    expect(screen.getByText('ERROR')).toBeDefined();
    expect(screen.getByText(/connection failed/)).toBeDefined();
    expect(screen.queryByText('INFO starting')).toBeNull();
  });

  it('should render status tag for completed state', () => {
    render(<LogViewer logs="Build finished" status="completed" />);
    expect(screen.getByText('已完成')).toBeDefined();
  });
});
