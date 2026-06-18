import { useEffect, useRef, useState, useCallback } from 'react';
import { Card, Tabs, Button, Space, message, Tooltip, Badge } from 'antd';
import {
  PlusOutlined, CloseOutlined,
  ZoomInOutlined, ZoomOutOutlined,
  FullscreenOutlined, FullscreenExitOutlined,
  ReloadOutlined,
  CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined,
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
}

const MIN_FONT_SIZE = 10;
const MAX_FONT_SIZE = 24;
const DEFAULT_FONT_SIZE = 14;

const StatusIcon = ({ status }: { status: ConnStatus }) => {
  switch (status) {
    case 'connected':
      return <CheckCircleOutlined style={{ color: '#52c41a', fontSize: 12 }} />;
    case 'connecting':
    case 'reconnecting':
      return <LoadingOutlined style={{ color: '#faad14', fontSize: 12 }} spin />;
    case 'disconnected':
      return <CloseCircleOutlined style={{ color: '#ff4d4f', fontSize: 12 }} />;
  }
};

export default function TerminalPage() {
  const [tabs, setTabs] = useState<TerminalTab[]>([]);
  const [activeKey, setActiveKey] = useState<string>('');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const tabCounter = useRef(0);
  const tabsRef = useRef<TerminalTab[]>([]);
  const mountGenRef = useRef(0);

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

  // 连接 WebSocket
  const connectWs = useCallback((tab: TerminalTab, isReconnect = false) => {
    const token = localStorage.getItem('token');
    if (!token) return;

    if (tab.ws) {
      try { tab.ws.close(); } catch {}
    }

    updateTabStatus(tab.key, isReconnect ? 'reconnecting' : 'connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/terminal/${tab.key.replace('terminal-', '')}?token=${token}`;
    const ws = new WebSocket(wsUrl);
    tab.ws = ws;

    ws.onopen = () => {
      tab.reconnectCount = 0;
      updateTabStatus(tab.key, 'connected');
      // 延迟发送 resize
      setTimeout(() => {
        const dims = tab.fitAddon.proposeDimensions();
        if (dims && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
        }
      }, 200);
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'output') {
          tab.terminal.write(msg.data);
        } else if (msg.type === 'exit') {
          tab.terminal.write('\r\n\x1b[31m[Process exited]\x1b[0m\r\n');
        } else if (msg.type !== 'pong') {
          // 非预期消息，忽略
        }
      } catch {
        // 如果不是 JSON，直接作为终端输出写入
        if (typeof event.data === 'string') {
          tab.terminal.write(event.data);
        }
      }
    };

    ws.onerror = () => {};

    ws.onclose = () => {
      if (tab.disposed) return;
      tab.reconnectCount++;
      if (tab.reconnectCount <= 3) {
        updateTabStatus(tab.key, 'reconnecting');
      } else {
        updateTabStatus(tab.key, 'disconnected');
      }
      if (tab.reconnectCount <= 10) {
        if (tab.reconnectTimer) clearTimeout(tab.reconnectTimer);
        tab.reconnectTimer = window.setTimeout(() => {
          if (!tab.disposed && tabsRef.current.includes(tab)) {
            connectWs(tab, true);
          }
        }, 3000);
      }
    };
  }, [updateTabStatus]);

  const createTerminal = useCallback((gen: number) => {
    const token = localStorage.getItem('token');
    if (!token) {
      message.error('请先登录');
      return;
    }

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
    };

    // 只注册一次 onData，引用 tab.ws（重连时 ws 会更新）
    newTab.onDataDisposable = terminal.onData((data) => {
      if (newTab.ws?.readyState === WebSocket.OPEN) {
        newTab.ws.send(JSON.stringify({ type: 'input', data }));
      }
    });

    setTabs(prev => [...prev, newTab]);
    setActiveKey(key);

    // 等待 DOM 渲染完成后再打开终端
    requestAnimationFrame(() => {
      if (mountGenRef.current !== gen) {
        newTab.disposed = true;
        newTab.terminal.dispose();
        return;
      }
      requestAnimationFrame(() => {
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
    });
  }, [connectWs]);

  const closeTab = useCallback((key: string) => {
    setTabs(prev => {
      const tab = prev.find(t => t.key === key);
      if (tab) {
        tab.disposed = true;
        if (tab.reconnectTimer) clearTimeout(tab.reconnectTimer);
        tab.onDataDisposable?.dispose();
        tab.terminal.dispose();
        if (tab.ws) tab.ws.close();
      }
      const newTabs = prev.filter(t => t.key !== key);
      if (activeKey === key && newTabs.length > 0) {
        setActiveKey(newTabs[newTabs.length - 1].key);
      }
      return newTabs;
    });
  }, [activeKey]);

  const changeFontSize = useCallback((delta: number) => {
    setTabs(prev => {
      const tab = prev.find(t => t.key === activeKey);
      if (tab) {
        const newSize = Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, tab.fontSize + delta));
        tab.fontSize = newSize;
        tab.terminal.options.fontSize = newSize;
        tab.fitAddon.fit();
        return [...prev];
      }
      return prev;
    });
  }, [activeKey]);

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

  const toggleFullscreen = useCallback(() => {
    setIsFullscreen(prev => !prev);
    setTimeout(() => {
      tabsRef.current.forEach(tab => tab.fitAddon.fit());
    }, 100);
  }, []);

  // 首次挂载
  useEffect(() => {
    const gen = ++mountGenRef.current;
    createTerminal(gen);
    return () => {
      mountGenRef.current++;
      tabsRef.current.forEach(tab => {
        tab.disposed = true;
        if (tab.reconnectTimer) clearTimeout(tab.reconnectTimer);
        tab.onDataDisposable?.dispose();
        tab.terminal.dispose();
        if (tab.ws) tab.ws.close();
      });
      tabsRef.current = [];
      // 必须同步清空 React state，否则 React StrictMode 双挂载会让旧标签残留
      setTabs([]);
      setActiveKey('');
      tabCounter.current = 0;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 通用 resize 发送
  const sendResize = useCallback((tab: TerminalTab) => {
    if (tab.ws?.readyState === WebSocket.OPEN) {
      const dims = tab.fitAddon.proposeDimensions();
      if (dims) {
        tab.ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
      }
    }
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

  const tabItems = tabs.map(tab => ({
    key: tab.key,
    label: (
      <Space size={4}>
        <StatusIcon status={tab.status} />
        {tab.label}
        <CloseOutlined
          style={{ fontSize: 10 }}
          onClick={(e) => {
            e.stopPropagation();
            closeTab(tab.key);
          }}
        />
      </Space>
    ),
    children: null, // 不使用 Tabs 的 children
  }));

  return (
    <Card
      title="Web 终端"
      style={isFullscreen ? { position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 1000, borderRadius: 0 } : { display: 'flex', flexDirection: 'column', height: 'calc(100vh - 112px)' }}
      styles={{ body: { display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0, overflow: 'hidden' } }}
      extra={
        <Space>
          {currentTab && (
            <Tooltip title={
              currentTab.status === 'connected' ? '已连接' :
              currentTab.status === 'connecting' ? '连接中...' :
              currentTab.status === 'reconnecting' ? '重连中...' :
              '已断开'
            }>
              <Badge
                status={
                  currentTab.status === 'connected' ? 'success' :
                  currentTab.status === 'connecting' || currentTab.status === 'reconnecting' ? 'processing' :
                  'error'
                }
                text={
                  <span style={{ fontSize: 12 }}>
                    {currentTab.status === 'connected' ? '已连接' :
                     currentTab.status === 'connecting' ? '连接中' :
                     currentTab.status === 'reconnecting' ? '重连中' :
                     '已断开'}
                  </span>
                }
              />
            </Tooltip>
          )}
          <Tooltip title="缩小字体">
            <Button
              icon={<ZoomOutOutlined />}
              onClick={() => changeFontSize(-1)}
              disabled={!currentTab || currentTab.fontSize <= MIN_FONT_SIZE}
            />
          </Tooltip>
          <span style={{ fontSize: 12, color: '#666', minWidth: 30, textAlign: 'center' }}>
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
          <Tooltip title={isFullscreen ? '退出全屏' : '全屏'}>
            <Button
              icon={isFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
              onClick={toggleFullscreen}
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
    >
      {tabs.length > 0 ? (
        <>
          <Tabs
            activeKey={activeKey}
            onChange={(key) => setActiveKey(key)}
            items={tabItems}
            hideAdd
          />
          {/* 终端容器，用 CSS 控制显示 */}
          {tabs.map(tab => (
            <div
              key={tab.key}
              id={tab.key}
              style={{
                flex: 1,
                minHeight: 0,
                background: '#1e1e1e',
                borderRadius: 4,
                padding: 8,
                display: tab.key === activeKey ? 'block' : 'none',
              }}
            />
          ))}
        </>
      ) : (
        <div style={{ textAlign: 'center', padding: 100, color: '#999' }}>
          点击"新建终端"开始
        </div>
      )}
    </Card>
  );
}
