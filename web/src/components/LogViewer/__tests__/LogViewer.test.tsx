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

  it('should render direct lines array via lines prop with line numbers', () => {
    render(<LogViewer lines={['Line 1: Server started', 'Line 2: Ready for connections']} />);
    expect(screen.getByText('Line 1: Server started')).toBeDefined();
    expect(screen.getByText('Line 2: Ready for connections')).toBeDefined();
    expect(screen.getByText('1')).toBeDefined();
    expect(screen.getByText('2')).toBeDefined();
  });

  it('should render structured entries array with timestamp and level', () => {
    render(
      <LogViewer
        entries={[
          { text: 'Critical error', time: '14:00:01', level: 'stderr' },
          { text: 'Standard info', time: '14:00:02', level: 'stdout' },
        ]}
      />
    );

    expect(screen.getByText('Critical error')).toBeDefined();
    expect(screen.getByText('14:00:01')).toBeDefined();
    expect(screen.getByText('stderr')).toBeDefined();
    expect(screen.getByText('Standard info')).toBeDefined();
    expect(screen.getByText('stdout')).toBeDefined();
  });

  it('should filter logs with search input and highlight', () => {
    render(<LogViewer lines={['INFO starting', 'ERROR connection failed', 'INFO retry']} />);
    const searchInput = screen.getByPlaceholderText('搜索日志...');
    fireEvent.change(searchInput, { target: { value: 'ERROR' } });
    expect(screen.getByText('ERROR')).toBeDefined();
    expect(screen.getByText(/connection failed/)).toBeDefined();
    expect(screen.queryByText('INFO starting')).toBeNull();
  });

  it('should render status tag for completed state', () => {
    render(<LogViewer lines={['Build finished']} status="completed" />);
    expect(screen.getByText('已完成')).toBeDefined();
  });

  it('should render exit code tag', () => {
    render(<LogViewer lines={['Process exited']} status="failed" exitCode={1} />);
    expect(screen.getByText('退出码 1')).toBeDefined();
  });
});
