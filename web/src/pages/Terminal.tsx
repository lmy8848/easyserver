import { useEffect, useRef, useState, useCallback } from 'react';
import { Card, Tabs, Button, Space, Tooltip, Badge, Input, type InputRef } from 'antd';
import {
  PlusOutlined, CloseOutlined,
  ZoomInOutlined, ZoomOutOutlined,
  FullscreenOutlined, FullscreenExitOutlined,
  ExpandOutlined, CompressOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

type ConnStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

interface TerminalTab {
  key: string;
  label: string;
  terminal: Terminal;
  fitAddon: FitAddon;
  ws: WebSocket | null;
  fontSize: number;
  reconnectTimer: number | null;
  status: ConnStatus;
  reconnectCount: number;
  onDataDisposable: { dispose: () => void } | null;
  disposed: boolean;
  /** Guards against concurrent WebSocket writes */
  writeLock: boolean;
}

const MIN_FONT_SIZE = 10;
const MAX_FONT_SIZE = 28;
const DEFAULT_FONT_SIZE = 16;

const StatusBadge = ({ status }: { status: ConnStatus }) => {
  switch (status) {
    case 'connected':
      return <Badge status="success" />;
    case 'connecting':
    case 'reconnecting':
      return <Badge status="processing" />;
    case 'disconnected':
      return <Badge status="error" />;
  }
};

export default function TerminalPage() {
  const [tabs, setTabs] = useState<TerminalTab[]>([]);
  const [activeKey, setActiveKey] = useState<string>('');
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');
  const [isWebFullscreen, setIsWebFullscreen] = useState(false);
  const [isNativeFullscreen, setIsNativeFullscreen] = useState(false);
  const inputRef = useRef<InputRef>(null);
  const lastClickRef = useRef<{ key: string; time: number }>({ key: '', time: 0 });
  const renameStartTimeRef = useRef(0);
  const cardContainerRef = useRef<HTMLDivElement>(null);
  const tabCounter = useRef(0);
  const tabsRef = useRef<TerminalTab[]>([]);
  const mountGenRef = useRef(0);
  const animFrameIdsRef = useRef<number[]>([]);

  useEffect(() => {
    tabsRef.current = tabs;
  }, [tabs]);

  const updateTabStatus = useCallback((key: string, status: ConnStatus) => {
    setTabs(prev => {
      const tab = prev.find(t => t.key === key);
      if (tab) {
        tab.status = status;
        return [...prev];
      }
      return prev;
    });
  }, []);

  // 通用 resize 发送（带写锁防并发）
  const sendResize = useCallback((tab: TerminalTab) => {
    if (tab.writeLock || tab.ws?.readyState !== WebSocket.OPEN) return;
    const dims = tab.fitAddon.proposeDimensions();
    if (dims) {
      tab.writeLock = true;
      try {
        tab.ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
      } catch (e) {
        console.debug('WebSocket send error:', e);
      }
      tab.writeLock = false;
    }
  }, []);

  // 连接 WebSocket
  const connectWsRef = useRef<(tab: TerminalTab, isReconnect?: boolean) => void>(() => {});

  const connectWs = useCallback((tab: TerminalTab, isReconnect = false) => {
    if (tab.disposed) return;

    // 清理可能已存在的重连定时器
    if (tab.reconnectTimer) {
      clearTimeout(tab.reconnectTimer);
      tab.reconnectTimer = null;
    }

    // 关闭旧连接前注销全部事件，防止旧连接 close 事件误触发重连定时器
    if (tab.ws) {
      const oldWs = tab.ws;
      oldWs.onopen = null;
      oldWs.onmessage = null;
      oldWs.onerror = null;
      oldWs.onclose = null;
      try { oldWs.close(); } catch (e) { console.debug('WebSocket close error:', e); }
      tab.ws = null;
    }

    updateTabStatus(tab.key, isReconnect ? 'reconnecting' : 'connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/terminal`;
    const ws = new WebSocket(wsUrl);
    tab.ws = ws;

    ws.onopen = () => {
      if (tab.ws !== ws || tab.disposed) return;
      tab.reconnectCount = 0;
      updateTabStatus(tab.key, 'connected');
      // 延迟发送 resize
      setTimeout(() => {
        if (!tab.disposed && tab.ws === ws && ws.readyState === WebSocket.OPEN) {
          tab.fitAddon.fit();
          sendResize(tab);
        }
      }, 200);
    };

    ws.onmessage = (event) => {
      if (tab.ws !== ws || tab.disposed) return;
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'output') {
          tab.terminal.write(msg.data);
        } else if (msg.type === 'exit') {
          tab.terminal.write('\r\n\x1b[31m[Process exited]\x1b[0m\r\n');
        }
      } catch {
        if (typeof event.data === 'string') {
          tab.terminal.write(event.data);
        }
      }
    };

    ws.onerror = (event) => {
      console.error('Terminal WebSocket error:', event);
    };

    ws.onclose = () => {
      if (tab.ws !== ws || tab.disposed) return;
      tab.ws = null;
      tab.reconnectCount++;
      if (tab.reconnectCount <= 3) {
        updateTabStatus(tab.key, 'reconnecting');
      } else {
        updateTabStatus(tab.key, 'disconnected');
      }
      if (tab.reconnectCount <= 5) {
        if (tab.reconnectTimer) clearTimeout(tab.reconnectTimer);
        tab.reconnectTimer = window.setTimeout(() => {
          if (!tab.disposed && tabsRef.current.includes(tab)) {
            connectWsRef.current(tab, true);
          }
        }, 3000);
      }
    };
  }, [updateTabStatus, sendResize]);

  useEffect(() => {
    connectWsRef.current = connectWs;
  }, [connectWs]);

  const createTerminal = useCallback((gen: number) => {
    tabCounter.current++;
    const id = tabCounter.current;
    const key = `terminal-${id}`;
    const label = `终端 ${id}`;

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: DEFAULT_FONT_SIZE,
      fontFamily: 'Menlo, Monaco, Consolas, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78',
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);

    const newTab: TerminalTab = {
      key, label, terminal, fitAddon,
      ws: null,
      fontSize: DEFAULT_FONT_SIZE,
      reconnectTimer: null,
      status: 'connecting',
      reconnectCount: 0,
      onDataDisposable: null,
      disposed: false,
      writeLock: false,
    };

    // Safe WS write helper to prevent concurrent writes
    const safeSend = (tab: TerminalTab, data: string) => {
      if (tab.writeLock || !tab.ws || tab.ws.readyState !== WebSocket.OPEN) return;
      tab.writeLock = true;
      try {
        tab.ws.send(data);
      } catch (e) {
        console.debug('WebSocket send error:', e);
      }
      tab.writeLock = false;
    };

    // 只注册一次 onData，引用 tab.ws（重连时 ws 会更新）
    newTab.onDataDisposable = terminal.onData((data) => {
      safeSend(newTab, JSON.stringify({ type: 'input', data }));
    });

    setTabs(prev => [...prev, newTab]);
    setActiveKey(key);

    // 等待 DOM 渲染完成后再打开终端
    const frameId1 = requestAnimationFrame(() => {
      if (mountGenRef.current !== gen) {
        newTab.disposed = true;
        newTab.terminal.dispose();
        return;
      }
      const frameId2 = requestAnimationFrame(() => {
        if (mountGenRef.current !== gen) {
          newTab.disposed = true;
          newTab.terminal.dispose();
          return;
        }
        const container = document.getElementById(key);
        if (container) {
          terminal.open(container);
          fitAddon.fit();
          terminal.focus();
        }
        connectWs(newTab);
      });
      animFrameIdsRef.current.push(frameId2);
    });
    animFrameIdsRef.current.push(frameId1);
  }, [connectWs]);

  const closeTab = useCallback((key: string) => {
    setTabs(prev => {
      const tab = prev.find(t => t.key === key);
      if (tab) {
        tab.disposed = true;
        if (tab.reconnectTimer) {
          clearTimeout(tab.reconnectTimer);
          tab.reconnectTimer = null;
        }
        tab.onDataDisposable?.dispose();
        tab.terminal.dispose();
        if (tab.ws) {
          tab.ws.onopen = null;
          tab.ws.onmessage = null;
          tab.ws.onerror = null;
          tab.ws.onclose = null;
          try { tab.ws.close(); } catch (e) { console.debug('WebSocket close error:', e); }
          tab.ws = null;
        }
      }
      const newTabs = prev.filter(t => t.key !== key);
      if (activeKey === key && newTabs.length > 0) {
        setActiveKey(newTabs[newTabs.length - 1]!.key);
      }
      return newTabs;
    });
  }, [activeKey]);

  const handleStartRename = useCallback((tab: TerminalTab) => {
    renameStartTimeRef.current = Date.now();
    setEditingKey(tab.key);
    setEditingTitle(tab.label);
  }, []);

  const handleSaveRename = useCallback((key: string, force = false) => {
    if (!force && Date.now() - renameStartTimeRef.current < 300) {
      return;
    }
    const trimmed = editingTitle.trim();
    if (trimmed) {
      setTabs(prev => prev.map(t => {
        if (t.key === key) {
          t.label = trimmed;
          return { ...t };
        }
        return t;
      }));
    }
    setEditingKey(null);
  }, [editingTitle]);

  const handleCancelRename = useCallback(() => {
    setEditingKey(null);
  }, []);

  const handleTabClick = useCallback((tab: TerminalTab) => {
    const now = Date.now();
    if (lastClickRef.current.key === tab.key && now - lastClickRef.current.time < 350) {
      handleStartRename(tab);
      lastClickRef.current = { key: '', time: 0 };
    } else {
      lastClickRef.current = { key: tab.key, time: now };
    }
  }, [handleStartRename]);

  useEffect(() => {
    if (!editingKey) return undefined;
    const timer = setTimeout(() => {
      inputRef.current?.focus({ cursor: 'all' });
    }, 50);
    return () => clearTimeout(timer);
  }, [editingKey]);

  const changeFontSize = useCallback((delta: number) => {
    const tab = tabsRef.current.find(t => t.key === activeKey);
    if (!tab) return;
    const oldSize = tab.fontSize;
    const newSize = Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, oldSize + delta));
    if (newSize === oldSize) return;

    // 若放大字号，先按比例预收缩行列数，避免在 fontSize 生效与 RAF fit 之间的 1 帧渲染中宽度瞬间撑满/溢出右侧
    if (newSize > oldSize && tab.terminal.cols > 0) {
      const approxCols = Math.max(2, Math.floor(tab.terminal.cols * (oldSize / newSize)));
      const approxRows = Math.max(1, Math.floor(tab.terminal.rows * (oldSize / newSize)));
      try {
        tab.terminal.resize(approxCols, approxRows);
      } catch (e) {
        console.debug('Pre-resize error:', e);
      }
    }

    tab.fontSize = newSize;
    tab.terminal.options.fontSize = newSize;
    setTabs(prev => [...prev]);

    // xterm 在 DOM 测量新字号后进行精确 fit 并同步给后端 PTY
    requestAnimationFrame(() => {
      if (tab.disposed) return;
      tab.fitAddon.fit();
      sendResize(tab);
    });
  }, [activeKey, sendResize]);

  const reconnect = useCallback(() => {
    const tab = tabsRef.current.find(t => t.key === activeKey);
    if (tab) {
      if (tab.reconnectTimer) {
        clearTimeout(tab.reconnectTimer);
        tab.reconnectTimer = null;
      }
      tab.reconnectCount = 0;
      tab.terminal.clear();
      connectWs(tab, true);
    }
  }, [activeKey, connectWs]);

  const toggleWebFullscreen = useCallback(() => {
    setIsWebFullscreen(prev => !prev);
    setTimeout(() => {
      tabsRef.current.forEach(tab => {
        tab.fitAddon.fit();
        sendResize(tab);
      });
    }, 100);
  }, [sendResize]);

  const toggleNativeFullscreen = useCallback(() => {
    if (!document.fullscreenElement) {
      cardContainerRef.current?.requestFullscreen().catch(err => {
        console.debug('Failed to enter fullscreen:', err);
      });
    } else {
      document.exitFullscreen().catch(err => {
        console.debug('Failed to exit fullscreen:', err);
      });
    }
  }, []);

  // 监听浏览器原生全屏状态变化（如按 Esc 键退出全屏）
  useEffect(() => {
    const handleNativeFullscreenChange = () => {
      const isFull = !!document.fullscreenElement;
      setIsNativeFullscreen(isFull);
      setTimeout(() => {
        tabsRef.current.forEach(tab => {
          tab.fitAddon.fit();
          sendResize(tab);
        });
      }, 100);
    };

    document.addEventListener('fullscreenchange', handleNativeFullscreenChange);
    document.addEventListener('webkitfullscreenchange', handleNativeFullscreenChange);
    return () => {
      document.removeEventListener('fullscreenchange', handleNativeFullscreenChange);
      document.removeEventListener('webkitfullscreenchange', handleNativeFullscreenChange);
    };
  }, [sendResize]);

  // 首次挂载
  useEffect(() => {
    const gen = ++mountGenRef.current;
    createTerminal(gen);
    return () => {
      // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional: increment generation to invalidate pending operations
      mountGenRef.current++;
      // Cancel any pending animation frames to prevent memory leaks
      animFrameIdsRef.current.forEach(id => cancelAnimationFrame(id));
      animFrameIdsRef.current = [];
      tabsRef.current.forEach(tab => {
        tab.disposed = true;
        if (tab.reconnectTimer) {
          clearTimeout(tab.reconnectTimer);
          tab.reconnectTimer = null;
        }
        tab.onDataDisposable?.dispose();
        tab.terminal.dispose();
        if (tab.ws) {
          tab.ws.onopen = null;
          tab.ws.onmessage = null;
          tab.ws.onerror = null;
          tab.ws.onclose = null;
          try { tab.ws.close(); } catch (e) { console.debug('WebSocket close error:', e); }
          tab.ws = null;
        }
      });
      tabsRef.current = [];
      // 必须同步清空 React state，否则 React StrictMode 双挂载会让旧标签残留
      setTabs([]);
      setActiveKey('');
      tabCounter.current = 0;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 窗口 resize
  useEffect(() => {
    const handleResize = () => {
      const tab = tabsRef.current.find(t => t.key === activeKey);
      if (tab) {
        tab.fitAddon.fit();
        sendResize(tab);
      }
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [activeKey, sendResize]);

  // 切换标签时聚焦终端
  useEffect(() => {
    const tab = tabs.find(t => t.key === activeKey);
    if (!tab) return;

    const timer = setTimeout(() => {
      tab.fitAddon.fit();
      tab.terminal.focus();
      sendResize(tab);
    }, 100);

    return () => clearTimeout(timer);
  }, [activeKey, tabs, sendResize]);

  // 侧边栏切换导致容器尺寸变化时 resize
  useEffect(() => {
    let prevWidth = 0;
    const ro = new ResizeObserver(() => {
      const container = document.getElementById(activeKey);
      if (!container) return;
      const w = container.clientWidth;
      if (w !== prevWidth && prevWidth > 0) {
        prevWidth = w;
        const tab = tabsRef.current.find(t => t.key === activeKey);
        if (tab) {
          tab.fitAddon.fit();
          sendResize(tab);
        }
      }
      prevWidth = container.clientWidth;
    });

    const container = document.getElementById(activeKey);
    if (container) {
      prevWidth = container.clientWidth;
      ro.observe(container);
    }

    return () => ro.disconnect();
  }, [activeKey, sendResize]);

  const currentTab = tabs.find(t => t.key === activeKey);

  const tabItems = tabs.map(tab => {
    const isEditing = editingKey === tab.key;
    return {
      key: tab.key,
      label: (
        <div
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            height: 24,
            maxHeight: 24,
            gap: 6,
          }}
          onMouseDown={(e) => {
            if (isEditing) e.stopPropagation();
          }}
          onClick={(e) => {
            if (isEditing) e.stopPropagation();
          }}
        >
          <StatusBadge status={tab.status} />
          {isEditing ? (
            <Input
              ref={inputRef}
              autoFocus
              size="small"
              value={editingTitle}
              maxLength={30}
              style={{
                width: 110,
                height: 22,
                maxHeight: 22,
                lineHeight: '20px',
                padding: '0 4px',
                fontSize: 13,
                margin: 0,
                verticalAlign: 'middle',
              }}
              onFocus={(e) => e.target.select()}
              onChange={(e) => setEditingTitle(e.target.value)}
              onBlur={() => handleSaveRename(tab.key)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleSaveRename(tab.key, true);
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  handleCancelRename();
                }
              }}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => e.stopPropagation()}
            />
          ) : (
            <span
              title="双击自定义标题"
              onClick={() => handleTabClick(tab)}
              onDoubleClick={(e) => {
                e.stopPropagation();
                handleStartRename(tab);
              }}
              style={{
                cursor: 'pointer',
                userSelect: 'none',
                height: 22,
                lineHeight: '22px',
                display: 'inline-block',
              }}
            >
              {tab.label}
            </span>
          )}
          <CloseOutlined
            style={{ fontSize: 10, color: '#999', marginLeft: 2 }}
            onClick={(e: React.MouseEvent) => {
              e.stopPropagation();
              if (editingKey === tab.key) setEditingKey(null);
              closeTab(tab.key);
            }}
          />
        </div>
      ),
      children: null, // 不使用 Tabs 的 children
    };
  });

  const isAnyFullscreen = isWebFullscreen || isNativeFullscreen;

  return (
    <div
      ref={cardContainerRef}
      style={isAnyFullscreen ? {
        position: isWebFullscreen && !isNativeFullscreen ? 'fixed' : 'relative',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: isWebFullscreen ? 1000 : undefined,
        height: '100vh',
        width: '100%',
        display: 'flex',
        flexDirection: 'column',
        background: '#ffffff',
      } : { display: 'flex', flexDirection: 'column', height: '100%' }}
    >
      <Card
        style={isAnyFullscreen
          ? { display: 'flex', flexDirection: 'column', height: '100vh', width: '100%', borderRadius: 0, border: 'none' }
          : { display: 'flex', flexDirection: 'column', height: 'calc(100vh - 96px)' }
        }
        styles={{
          body: {
            padding: '6px 12px 12px',
            display: 'flex',
            flexDirection: 'column',
            flex: 1,
            minHeight: 0,
            overflow: 'hidden',
          },
        }}
      >
        {tabs.length > 0 ? (
          <>
            <Tabs
              activeKey={activeKey}
              onChange={(key) => setActiveKey(key)}
              items={tabItems}
              hideAdd
              tabBarStyle={{ marginBottom: 8 }}
              tabBarExtraContent={
                <Space size={8}>
                  <Tooltip title="缩小字体">
                    <Button
                      icon={<ZoomOutOutlined />}
                      onClick={() => changeFontSize(-1)}
                      disabled={!currentTab || currentTab.fontSize <= MIN_FONT_SIZE}
                    />
                  </Tooltip>
                  <span style={{ fontSize: 16, color: '#666', minWidth: 24, textAlign: 'center' }}>
                    {currentTab?.fontSize || DEFAULT_FONT_SIZE}
                  </span>
                  <Tooltip title="放大字体">
                    <Button
                      icon={<ZoomInOutlined />}
                      onClick={() => changeFontSize(1)}
                      disabled={!currentTab || currentTab.fontSize >= MAX_FONT_SIZE}
                    />
                  </Tooltip>
                  <Tooltip title="重新连接">
                    <Button icon={<ReloadOutlined />} onClick={reconnect} />
                  </Tooltip>
                  <Tooltip title={isWebFullscreen ? '退出网页全屏' : '网页全屏'}>
                    <Button
                      icon={isWebFullscreen ? <CompressOutlined /> : <ExpandOutlined />}
                      onClick={toggleWebFullscreen}
                    />
                  </Tooltip>
                  <Tooltip title={isNativeFullscreen ? '退出屏幕全屏' : '屏幕全屏'}>
                    <Button
                      icon={isNativeFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                      onClick={toggleNativeFullscreen}
                    />
                  </Tooltip>
                  <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => createTerminal(++mountGenRef.current)}
                  >
                    新建终端
                  </Button>
                </Space>
              }
            />
            {/* 终端容器，用 CSS 控制显示 */}
            {tabs.map(tab => (
              <div
                key={tab.key}
                id={tab.key}
                style={{
                  flex: 1,
                  minHeight: 0,
                  minWidth: 0,
                  width: '100%',
                  overflow: 'hidden',
                  background: '#1e1e1e',
                  borderRadius: 6,
                  display: tab.key === activeKey ? 'block' : 'none',
                }}
              />
            ))}
          </>
        ) : (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', color: '#999', gap: 16 }}>
            <div>暂无活动终端</div>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => createTerminal(++mountGenRef.current)}
            >
              新建终端
            </Button>
          </div>
        )}
      </Card>
    </div>
  );
}
