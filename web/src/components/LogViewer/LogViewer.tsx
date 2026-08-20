import {
  type ReactNode,
  type CSSProperties,
  useEffect,
  useRef,
  useState,
  useCallback,
  useMemo,
} from 'react';
import {
  Button,
  Input,
  Tag,
  Tooltip,
  Space,
  Checkbox,
  message,
  theme,
} from 'antd';
import {
  SearchOutlined,
  CopyOutlined,
  DownloadOutlined,
  ClearOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  ArrowDownOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons';
import { parseAnsi, splitLinesWithCrInfo } from './ansi';
import { useLogBuffer, processLogEntries } from './useLogBuffer';
import { useLogStream } from './useLogStream';
import { copyToClipboard } from '../../utils/clipboard';
import type {
  LogEntry,
  LogStreamStatus,
  UseLogBufferReturn,
  LogStreamMessageHandler,
} from './types';
import './LogViewer.css';

export interface LogViewerProps {
  // ==================== 接入方式一：直接传入数据模式 ====================
  /** 直接传入字符串行数组（支持多行文本与 ANSI 颜色，自动分行规整） */
  lines?: string[];
  /** 直接传入结构化日志对象数组 */
  entries?: LogEntry[];

  // ==================== 接入方式二：SSE 实时流式模式 ====================
  /** SSE 服务端流式接口相对路径（例如 '/api/runtime/logs/node@20.11.0'） */
  streamUrl?: string;
  /** 是否开启 SSE 连接（默认在 streamUrl 存在时为 true） */
  streamEnabled?: boolean;
  /** 流完成或终止时的回调 */
  onDone?: (result: { status: 'completed' | 'failed' | 'stopped'; error?: string; exitCode?: number }) => void;
  /** 自定义 SSE 消息解析适配器（如需拦截特殊协议帧，返回 true 或 'done' 标志流结束） */
  onStreamMessage?: LogStreamMessageHandler;

  // ==================== 方式三（高级）：外部 Buffer 受控 ====================
  /** 外部传入的 useLogBuffer 实例 */
  buffer?: UseLogBufferReturn;

  // ==================== 状态与元信息展示 ====================
  /** 运行状态（直接数据模式下可显式指定，如 'completed'、'failed'） */
  status?: LogStreamStatus;
  /** 错误信息提示文案 */
  error?: string | null;
  /** 任务退出码 */
  exitCode?: number | null;
  /** 耗时（毫秒） */
  elapsedMs?: number;
  /** 工具栏左侧标题 */
  title?: ReactNode;
  /** 工具栏左侧额外自定义区域 */
  headerExtra?: ReactNode;
  /** 工具栏右侧额外操作按钮区域 */
  extraActions?: ReactNode;

  // ==================== 视图与交互控制 ====================
  /** 视口高度（如 400, '100%', 'calc(100vh - 200px)'） */
  height?: number | string;
  /** 视口最大高度 */
  maxHeight?: number | string;
  /** 是否显示行号（默认 true） */
  showLineNumbers?: boolean;
  /** 是否显示单行时间戳（默认 true） */
  showTimestamps?: boolean;
  /** 是否显示搜索框（默认 true） */
  showSearch?: boolean;
  /** 是否显示复制按钮（默认 true） */
  showCopy?: boolean;
  /** 是否显示下载按钮（默认 true） */
  showDownload?: boolean;
  /** 是否显示清空按钮（默认 false） */
  showClear?: boolean;
  /** 是否显示自动换行切换按钮（默认 true） */
  showWrapToggle?: boolean;
  /** 是否显示自动滚动吸底开关（默认 true） */
  showFollowToggle?: boolean;
  /** 是否允许全屏切换（默认 true） */
  allowFullscreen?: boolean;
  /** 导出日志的文件名前缀 */
  downloadFileName?: string;
  /** 自定义空状态文案 */
  emptyText?: ReactNode;

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
  lines,
  entries: externalEntries,
  streamUrl,
  streamEnabled = true,
  onDone,
  onStreamMessage,
  buffer: externalBuffer,
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
  className = '',
  style = {},
}: LogViewerProps) {
  const { token } = theme.useToken();
  const internalBuffer = useLogBuffer();
  const buffer = externalBuffer || internalBuffer;

  const bufferRef = useRef(buffer);
  useEffect(() => {
    bufferRef.current = buffer;
  });

  // SSE Stream integration
  const stream = useLogStream({
    path: streamUrl || '',
    enabled: Boolean(streamUrl && streamEnabled),
    buffer,
    onDone,
    onMessage: onStreamMessage,
  });

  // Convert lines array to LogEntry[] (single-pass, memoized)
  const parsedLineEntries = useMemo(() => {
    if (!lines) return null;
    const rawList: LogEntry[] = [];
    lines.forEach((line) => {
      const subLines = splitLinesWithCrInfo(line);
      subLines.forEach(({ text, endsInCr }) => {
        rawList.push({ text, meta: endsInCr ? { endsInCr: true } : undefined });
      });
    });
    return processLogEntries([], rawList);
  }, [lines]);

  // Direct data integration (lines / entries)
  useEffect(() => {
    if (externalEntries) {
      bufferRef.current.setEntries(externalEntries);
    } else if (parsedLineEntries) {
      bufferRef.current.setEntries(parsedLineEntries);
    }
  }, [externalEntries, parsedLineEntries]);

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
  }, [buffer.filteredEntries, follow, isScrolledUp]);

  const handleCopy = useCallback(() => {
    const text = buffer.getPlainText();
    if (!text) {
      message.info('日志内容为空');
      return;
    }
    copyToClipboard(text, '日志已复制到剪贴板');
  }, [buffer]);

  const handleDownload = useCallback(() => {
    const text = buffer.getPlainText();
    if (!text) {
      message.info('日志内容为空');
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
        return <Tag color="processing">运行中</Tag>;
      case 'completed':
        return <Tag color="success">已完成</Tag>;
      case 'failed':
        return <Tag color="error">失败</Tag>;
      case 'stopped':
        return <Tag color="warning">已停止</Tag>;
      default:
        return null;
    }
  }, [status]);

  const containerStyle: CSSProperties = {
    height: isFullscreen ? '100vh' : height,
    maxHeight: isFullscreen ? '100vh' : maxHeight,
    border: `1px solid ${token.colorBorderSecondary}`,
    backgroundColor: token.colorBgContainer,
    ...style,
  };

  return (
    <div
      className={`log-viewer-wrapper ${isFullscreen ? 'fullscreen' : ''} ${className}`}
      style={containerStyle}
    >
      {/* 顶部原生控制栏（默认尺寸） */}
      <div
        style={{
          padding: '10px 16px',
          background: token.colorFillAlter,
          borderBottom: `1px solid ${token.colorBorderSecondary}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: 12,
        }}
      >
        <Space size="middle" wrap>
          {title && (
            <span style={{ fontWeight: 600, color: token.colorText, fontSize: 14 }}>
              {title}
            </span>
          )}
          {renderedStatusTag}
          {exitCode !== null && exitCode !== undefined && (
            <Tag color={exitCode === 0 ? 'green' : 'red'}>退出码 {exitCode}</Tag>
          )}
          {elapsedMs > 0 && (
            <span style={{ color: token.colorTextTertiary, fontSize: 13 }}>
              耗时 {(elapsedMs / 1000).toFixed(1)}s
            </span>
          )}
          <span style={{ color: token.colorTextTertiary, fontSize: 13 }}>
            共 {buffer.totalCount} 条
          </span>
          {headerExtra}
        </Space>

        <Space size="middle" wrap>
          {showSearch && (
            <Input
              placeholder="搜索日志..."
              prefix={<SearchOutlined style={{ color: token.colorTextTertiary }} />}
              value={buffer.searchKeyword}
              onChange={(e) => buffer.setSearchKeyword(e.target.value)}
              allowClear
              style={{ width: 180 }}
              suffix={
                buffer.searchKeyword ? (
                  <span style={{ fontSize: 12, color: token.colorPrimary }}>
                    {buffer.matchCount} 项
                  </span>
                ) : null
              }
            />
          )}

          {showWrapToggle && (
            <Checkbox checked={wrap} onChange={(e) => setWrap(e.target.checked)}>
              换行
            </Checkbox>
          )}

          {showFollowToggle && (
            <Checkbox checked={follow} onChange={(e) => setFollow(e.target.checked)}>
              跟踪
            </Checkbox>
          )}

          {showCopy && (
            <Button icon={<CopyOutlined />} onClick={handleCopy}>
              复制
            </Button>
          )}

          {showDownload && (
            <Button icon={<DownloadOutlined />} onClick={handleDownload}>
              下载
            </Button>
          )}

          {showClear && (
            <Button icon={<ClearOutlined />} onClick={buffer.clear}>
              清空
            </Button>
          )}

          {allowFullscreen && (
            <Tooltip title={isFullscreen ? '退出全屏' : '全屏显示'}>
              <Button
                icon={isFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                onClick={() => setIsFullscreen((prev) => !prev)}
              />
            </Tooltip>
          )}

          {extraActions}
        </Space>
      </div>

      {/* 日志内容主体容器 */}
      <div className="log-viewer-body">
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
        </div>

        {/* 浮动吸底按钮：挂载在 non-scrolling body 容器右下角 */}
        {isScrolledUp && (
          <Button
            type="primary"
            icon={<ArrowDownOutlined />}
            onClick={scrollToBottom}
            className="log-viewer-resume-float"
          >
            回到底部
          </Button>
        )}
      </div>

      {/* 错误提示栏 */}
      {error && (
        <div className="log-viewer-error-banner">
          <CloseCircleOutlined />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
}

