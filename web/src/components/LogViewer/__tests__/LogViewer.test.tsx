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

  it('should preserve search input value on re-render with new lines array reference', () => {
    const { rerender } = render(<LogViewer lines={['INFO starting', 'ERROR test']} />);
    const searchInput = screen.getByPlaceholderText('搜索日志...') as HTMLInputElement;
    fireEvent.change(searchInput, { target: { value: 'ERROR' } });
    expect(searchInput.value).toBe('ERROR');

    // Re-render with new array reference
    rerender(<LogViewer lines={['INFO starting', 'ERROR test', 'ERROR second']} />);
    expect(searchInput.value).toBe('ERROR');
    expect(screen.getByText(/second/)).toBeDefined();
  });

  it('should render status tag for completed state', () => {
    render(<LogViewer lines={['Build finished']} status="completed" />);
    expect(screen.getByText('已完成')).toBeDefined();
  });

  it('should render exit code tag', () => {
    render(<LogViewer lines={['Process exited']} status="failed" exitCode={1} />);
    expect(screen.getByText('退出码 1')).toBeDefined();
  });

  it('should collapse progress lines in LogViewer when ANSI cursor up codes are used', () => {
    const { container } = render(
      <LogViewer
        lines={[
          'Starting build...',
          'Compiling module A...',
          '\x1b[1A\x1b[2KCompiling module A... Done',
          'Build complete',
        ]}
      />
    );

    expect(screen.getByText('Starting build...')).toBeDefined();
    expect(screen.getByText('Compiling module A... Done')).toBeDefined();
    expect(screen.getByText('Build complete')).toBeDefined();
    const lines = container.querySelectorAll('.log-viewer-line');
    expect(lines.length).toBe(3);
  });

  it('should render log-viewer-body and viewport structure for fixed floating button anchoring', () => {
    const { container } = render(<LogViewer lines={['Line 1']} />);
    const body = container.querySelector('.log-viewer-body');
    const viewport = container.querySelector('.log-viewer-viewport');
    expect(body).toBeDefined();
    expect(viewport).toBeDefined();
    expect(body?.contains(viewport)).toBe(true);
  });
});
