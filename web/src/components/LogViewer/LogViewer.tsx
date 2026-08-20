import { type ReactNode, type CSSProperties, useEffect, useRef, useState, useCallback, useMemo } from 'react';
import {
  Button,
  Input,
  Space,
  Tag,
  Switch,
  Tooltip,
  message,
} from 'antd';
import {
  SearchOutlined,
  CopyOutlined,
  DownloadOutlined,
  ClearOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  ArrowDownOutlined,
  LoadingOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { parseAnsi } from './ansi';
import { useLogBuffer } from './useLogBuffer';
import { useLogStream } from './useLogStream';
import type {
  LogEntry,
  LogStreamStatus,
  UseLogBufferReturn,
} from './types';
import './LogViewer.css';

export interface LogViewerProps {
  /** Custom buffer, or default buffer will be created internally */
  buffer?: UseLogBufferReturn;
  /** Raw entries array (if controlled externally) */
  entries?: LogEntry[];
  /** Raw string (convenience for static logs) */
  rawLogs?: string;
  /** SSE Stream URL to connect and consume logs automatically */
  streamUrl?: string;
  /** Whether SSE stream is active */
  streamEnabled?: boolean;
  /** Stream lifecycle status override */
  status?: LogStreamStatus;
  /** Error message to display */
  error?: string | null;
  /** Exit code if execution completed */
  exitCode?: number | null;
  /** Elapsed execution time in ms */
  elapsedMs?: number;
  /** Title displayed on header */
  title?: ReactNode;
  /** Header left extra controls */
  headerExtra?: ReactNode;
  /** Header right extra actions */
  extraActions?: ReactNode;
  /** Max height of the log viewport */
  height?: number | string;
  maxHeight?: number | string;
  /** Show line numbers (default true) */
  showLineNumbers?: boolean;
  /** Show timestamps if present (default true) */
  showTimestamps?: boolean;
  /** Show search bar in toolbar (default true) */
  showSearch?: boolean;
  /** Show copy button (default true) */
  showCopy?: boolean;
  /** Show download button (default true) */
  showDownload?: boolean;
  /** Show clear button (default false) */
  showClear?: boolean;
  /** Show wrap toggle (default true) */
  showWrapToggle?: boolean;
  /** Show auto-scroll follow toggle (default true) */
  showFollowToggle?: boolean;
  /** Allow fullscreen toggle (default true) */
  allowFullscreen?: boolean;
  /** Filename prefix for download */
  downloadFileName?: string;
  /** Custom empty text */
  emptyText?: ReactNode;
  /** Callback when stream finishes */
  onDone?: (result: { status: 'completed' | 'failed' | 'stopped'; error?: string; exitCode?: number }) => void;
  /** Custom onMessage adapter for streamUrl */
  onStreamMessage?: (data: unknown, helpers: { buffer?: UseLogBufferReturn; close: () => void }) => boolean | void;
  className?: string;
  style?: CSSProperties;
}

function renderHighlightedText(text: string, keyword: string) {
  if (!keyword.trim()) return text;
  const lowerText = text.toLowerCase();
  const lowerKeyword = keyword.toLowerCase();
  const parts: ReactNode[] = [];
  let start = 0;
  let index = lowerText.indexOf(lowerKeyword, start);

  while (index !== -1) {
    if (index > start) {
      parts.push(text.slice(start, index));
    }
    parts.push(
      <mark key={index} className="log-viewer-highlight">
        {text.slice(index, index + lowerKeyword.length)}
      </mark>
    );
    start = index + lowerKeyword.length;
    index = lowerText.indexOf(lowerKeyword, start);
  }

  if (start < text.length) {
    parts.push(text.slice(start));
  }
  return parts;
}

function renderAnsiSpans(rawText: string, keyword: string) {
  const spans = parseAnsi(rawText);
  return spans.map((span, idx) => {
    const style: CSSProperties = {};
    if (span.color) style.color = span.color;
    if (span.backgroundColor) style.backgroundColor = span.backgroundColor;
    if (span.bold) style.fontWeight = 'bold';
    if (span.dim) style.opacity = 0.6;
    if (span.italic) style.fontStyle = 'italic';
    if (span.underline) style.textDecoration = 'underline';

    const content = keyword ? renderHighlightedText(span.text, keyword) : span.text;
    return (
      <span key={idx} style={style}>
        {content}
      </span>
    );
  });
}

export function LogViewer({
  buffer: externalBuffer,
  entries: externalEntries,
  rawLogs,
  streamUrl,
  streamEnabled = true,
  status: externalStatus,
  error: externalError,
  exitCode: externalExitCode,
  elapsedMs: externalElapsedMs,
  title,
  headerExtra,
  extraActions,
  height,
  maxHeight,
  showLineNumbers = true,
  showTimestamps = true,
  showSearch = true,
  showCopy = true,
  showDownload = true,
  showClear = false,
  showWrapToggle = true,
  showFollowToggle = true,
  allowFullscreen = true,
  downloadFileName = 'easyserver_log',
  emptyText,
  onDone,
  onStreamMessage,
  className = '',
  style = {},
}: LogViewerProps) {
  const internalBuffer = useLogBuffer();
  const buffer = externalBuffer || internalBuffer;

  // Stream integration
  const stream = useLogStream({
    path: streamUrl || '',
    enabled: Boolean(streamUrl && streamEnabled),
    buffer,
    onDone,
    onMessage: onStreamMessage,
  });

  // Handle external static entries or rawLogs
  useEffect(() => {
    if (externalEntries) {
      buffer.setEntries(externalEntries);
    } else if (rawLogs) {
      buffer.clear();
      buffer.appendLine(rawLogs);
    }
  }, [externalEntries, rawLogs, buffer]);

  const [follow, setFollow] = useState<boolean>(true);
  const [wrap, setWrap] = useState<boolean>(true);
  const [isFullscreen, setIsFullscreen] = useState<boolean>(false);
  const [isScrolledUp, setIsScrolledUp] = useState<boolean>(false);

  const viewportRef = useRef<HTMLDivElement>(null);

  // Status & timing resolution
  const status: LogStreamStatus = externalStatus || (streamUrl ? stream.status : 'idle');
  const error = externalError || stream.error;
  const exitCode = externalExitCode !== undefined ? externalExitCode : stream.exitCode;
  const elapsedMs = externalElapsedMs !== undefined ? externalElapsedMs : stream.elapsedMs;

  const scrollToBottom = useCallback(() => {
    if (viewportRef.current) {
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight;
      setIsScrolledUp(false);
    }
  }, []);

  // Handle scroll detection
  const handleScroll = useCallback(() => {
    if (!viewportRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = viewportRef.current;
    const distanceFromBottom = scrollHeight - scrollTop - clientHeight;
    if (distanceFromBottom > 40) {
      setIsScrolledUp(true);
    } else {
      setIsScrolledUp(false);
    }
  }, []);

  // Auto-scroll on new entries if follow is active and not scrolled up
  useEffect(() => {
    if (follow && !isScrolledUp && viewportRef.current) {
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight;
    }
  }, [buffer.filteredEntries.length, follow, isScrolledUp]);

  const handleCopy = useCallback(async () => {
    const text = buffer.getPlainText();
    if (!text) {
      message.warning('日志为空');
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      message.success('日志已复制到剪贴板');
    } catch {
      message.error('复制失败');
    }
  }, [buffer]);

  const handleDownload = useCallback(() => {
    const text = buffer.getPlainText();
    if (!text) {
      message.warning('日志为空');
      return;
    }
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    a.href = url;
    a.download = `${downloadFileName}_${timestamp}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, [buffer, downloadFileName]);

  const renderedStatusTag = useMemo(() => {
    switch (status) {
      case 'connecting':
      case 'streaming':
        return <Tag icon={<LoadingOutlined />} color="processing">运行中</Tag>;
      case 'completed':
        return <Tag icon={<CheckCircleOutlined />} color="success">已完成</Tag>;
      case 'failed':
        return <Tag icon={<CloseCircleOutlined />} color="error">失败</Tag>;
      case 'stopped':
        return <Tag icon={<StopOutlined />} color="warning">已停止</Tag>;
      default:
        return null;
    }
  }, [status]);

  const containerStyle: CSSProperties = {
    height: isFullscreen ? '100vh' : height,
    maxHeight: isFullscreen ? '100vh' : maxHeight,
    ...style,
  };

  return (
    <div
      className={`log-viewer-wrapper ${isFullscreen ? 'fullscreen' : ''} ${className}`}
      style={containerStyle}
    >
      {/* Toolbar */}
      <div className="log-viewer-toolbar">
        <div className="log-viewer-toolbar-left">
          {title && <span className="log-viewer-title">{title}</span>}
          {renderedStatusTag}
          {exitCode !== null && exitCode !== undefined && (
            <Tag color={exitCode === 0 ? 'green' : 'red'}>退出码 {exitCode}</Tag>
          )}
          {elapsedMs > 0 && (
            <span className="log-viewer-meta">
              {(elapsedMs / 1000).toFixed(1)}s
            </span>
          )}
          <span className="log-viewer-meta">共 {buffer.totalCount} 行</span>
          {headerExtra}
        </div>

        <div className="log-viewer-toolbar-right">
          {showSearch && (
            <Input
              size="small"
              placeholder="搜索日志..."
              prefix={<SearchOutlined style={{ color: '#8c8c8c' }} />}
              value={buffer.searchKeyword}
              onChange={(e) => buffer.setSearchKeyword(e.target.value)}
              allowClear
              style={{ width: 160 }}
              suffix={
                buffer.searchKeyword && (
                  <span style={{ fontSize: 11, color: '#8c8c8c' }}>
                    {buffer.matchCount} 项
                  </span>
                )
              }
            />
          )}

          {showWrapToggle && (
            <Tooltip title="自动换行">
              <Space size={4}>
                <Switch size="small" checked={wrap} onChange={setWrap} />
                <span className="log-viewer-meta">换行</span>
              </Space>
            </Tooltip>
          )}

          {showFollowToggle && (
            <Tooltip title="自动滚动到底部">
              <Space size={4}>
                <Switch size="small" checked={follow} onChange={setFollow} />
                <span className="log-viewer-meta">跟踪</span>
              </Space>
            </Tooltip>
          )}

          {showCopy && (
            <Tooltip title="复制全部日志">
              <Button
                size="small"
                type="text"
                icon={<CopyOutlined style={{ color: '#d4d4d4' }} />}
                onClick={handleCopy}
              />
            </Tooltip>
          )}

          {showDownload && (
            <Tooltip title="导出为 .log 文件">
              <Button
                size="small"
                type="text"
                icon={<DownloadOutlined style={{ color: '#d4d4d4' }} />}
                onClick={handleDownload}
              />
            </Tooltip>
          )}

          {showClear && (
            <Tooltip title="清空日志">
              <Button
                size="small"
                type="text"
                icon={<ClearOutlined style={{ color: '#d4d4d4' }} />}
                onClick={buffer.clear}
              />
            </Tooltip>
          )}

          {allowFullscreen && (
            <Tooltip title={isFullscreen ? '退出全屏' : '全屏显示'}>
              <Button
                size="small"
                type="text"
                icon={
                  isFullscreen ? (
                    <FullscreenExitOutlined style={{ color: '#d4d4d4' }} />
                  ) : (
                    <FullscreenOutlined style={{ color: '#d4d4d4' }} />
                  )
                }
                onClick={() => setIsFullscreen((prev) => !prev)}
              />
            </Tooltip>
          )}

          {extraActions}
        </div>
      </div>

      {/* Viewport */}
      <div
        ref={viewportRef}
        onScroll={handleScroll}
        className={`log-viewer-viewport ${wrap ? 'wrap' : 'no-wrap'}`}
      >
        {buffer.filteredEntries.length === 0 ? (
          <div className="log-viewer-empty">
            {emptyText ||
              (status === 'streaming' || status === 'connecting'
                ? '等待日志输出…'
                : '暂无日志输出')}
          </div>
        ) : (
          buffer.filteredEntries.map((entry, index) => (
            <div key={entry.id ?? index} className="log-viewer-line">
              {showLineNumbers && (
                <span className="log-viewer-line-num">{index + 1}</span>
              )}
              {showTimestamps && entry.time && (
                <span className="log-viewer-time">{entry.time}</span>
              )}
              {entry.level && (
                <span
                  className={`log-viewer-level log-viewer-level-${String(
                    entry.level
                  ).toLowerCase()}`}
                >
                  {entry.level}
                </span>
              )}
              <span className="log-viewer-line-text">
                {renderAnsiSpans(entry.text, buffer.searchKeyword)}
              </span>
            </div>
          ))
        )}

        {/* Floating resume auto-scroll button when scrolled up */}
        {isScrolledUp && (
          <Button
            size="small"
            type="primary"
            icon={<ArrowDownOutlined />}
            onClick={scrollToBottom}
            className="log-viewer-resume-float"
          >
            回到底部
          </Button>
        )}
      </div>

      {/* Error Banner */}
      {error && (
        <div className="log-viewer-error-banner">
          <CloseCircleOutlined />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
}
