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

  it('should render rawLogs text with line numbers', () => {
    const raw = 'Line 1: Server started\nLine 2: Ready for connections';
    render(<LogViewer rawLogs={raw} />);

    expect(screen.getByText('Line 1: Server started')).toBeDefined();
    expect(screen.getByText('Line 2: Ready for connections')).toBeDefined();
    expect(screen.getByText('1')).toBeDefined();
    expect(screen.getByText('2')).toBeDefined();
  });

  it('should filter logs with search input and highlight', () => {
    const raw = 'INFO starting\nERROR connection failed\nINFO retry';
    render(<LogViewer rawLogs={raw} />);

    const searchInput = screen.getByPlaceholderText('搜索日志...');
    fireEvent.change(searchInput, { target: { value: 'ERROR' } });

    expect(screen.getByText('ERROR')).toBeDefined();
    expect(screen.getByText(/connection failed/)).toBeDefined();
    expect(screen.queryByText('INFO starting')).toBeNull();
  });

  it('should render status tag for completed state', () => {
    render(<LogViewer rawLogs="Build finished" status="completed" />);
    expect(screen.getByText('已完成')).toBeDefined();
  });

  it('should render exit code tag', () => {
    render(<LogViewer rawLogs="Process exited" status="failed" exitCode={1} />);
    expect(screen.getByText('退出码 1')).toBeDefined();
  });
});
