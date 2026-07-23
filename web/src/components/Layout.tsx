import { useState, useEffect, useCallback, useRef } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { useAuthStore } from '../store/useAuthStore';
import { notificationApi, settingsApi } from '../services/api';
import type { Notification } from '../types';
import CommandPalette from './CommandPalette';
import { COLORS } from '../utils/theme';
import { message } from 'antd';
import './Layout.css';

const MENU_GROUPS = [
  {
    label: '监控',
    items: [
      { key: '/', icon: 'dashboard', label: '系统概览' },
      { key: '/processes', icon: 'cpu', label: '进程守护' },
      { key: '/system-monitor', icon: 'monitor', label: '系统监控' },
    ],
  },
  {
    label: '管理',
    items: [
      { key: '/terminal', icon: 'terminal', label: '终端访问' },
      { key: '/files', icon: 'folder', label: '文件管理' },
      { key: '/file-shares', icon: 'link', label: '文件外链' },
      { key: '/deploy', icon: 'rocket', label: '部署同步' },
      { key: '/runtime', icon: 'server', label: '运行环境' },
      { key: '/env-config', icon: 'control', label: '环境配置' },
    ],
  },
  {
    label: '业务',
    items: [
      { key: '/websites', icon: 'global', label: '网站管理' },
      { key: '/databases', icon: 'database', label: '数据库管理' },
      { key: '/cron', icon: 'clock', label: '计划任务' },
      { key: '/scripts', icon: 'code', label: '脚本库' },
      { key: '/firewall', icon: 'shield', label: '防火墙' },
      { key: '/ssh', icon: 'key', label: 'SSH 管理' },
      { key: '/containers', icon: 'box', label: '容器管理' },
    ],
  },
  {
    label: '系统',
    items: [
      { key: '/cloud', icon: 'cloud', label: '腾讯云' },
      { key: '/audit', icon: 'file-text', label: '审计日志' },
      { key: '/settings', icon: 'tool', label: '面板设置' },
      { key: '/security', icon: 'lock', label: '安全设置' },
      { key: '/vulnerabilities', icon: 'alert', label: '漏洞扫描' },
    ],
  },
];

const ICONS: Record<string, string> = {
  dashboard: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
  cluster: '<path d="M12 20V10M18 20V4M6 20v-4"/>',
  monitor: '<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
  terminal: '<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>',
  folder: '<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>',
  rocket: '<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z"/><line x1="4" y1="22" x2="4" y2="15"/>',
  api: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
  control: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
  global: '<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
  database: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/>',
  clock: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
  code: '<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>',
  shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
  key: '<path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>',
  cloud: '<path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/>',
  'file-text': '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/>',
  tool: '<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>',
  bell: '<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>',
  search: '<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>',
  user: '<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>',
  'menu-fold': '<polyline points="11 17 6 12 11 7"/><polyline points="18 17 13 12 18 7"/>',
  'menu-unfold': '<polyline points="13 17 18 12 13 7"/><polyline points="6 17 11 12 6 7"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
  check: '<polyline points="20 6 9 17 4 12"/>',
  'chevron-down': '<polyline points="6 9 12 15 18 9"/>',
  'log-out': '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>',
  cpu: '<rect x="4" y="4" width="16" height="16" rx="2" ry="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/><line x1="20" y1="9" x2="23" y2="9"/><line x1="20" y1="14" x2="23" y2="14"/><line x1="1" y1="9" x2="4" y2="9"/><line x1="1" y1="14" x2="4" y2="14"/>',
  box: '<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>',
  server: '<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>',
  lock: '<rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
};

function Icon({ name, size = 18 }: { name: string; size?: number }) {
  const path = ICONS[name];
  if (!path) return null;
  // Hardcoded SVG paths from ICONS constant (not user input) — safe for dangerouslySetInnerHTML
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" dangerouslySetInnerHTML={{ __html: path }} />
  );
}

const PAGE_TITLES: Record<string, string> = {
  '/': '系统概览',
  '/processes': '进程守护',
  '/system-monitor': '系统监控',
  '/terminal': '终端访问',
  '/files': '文件管理',
  '/file-shares': '文件外链',
  '/deploy': '部署同步',
  '/runtime': '运行环境',
  '/env-config': '环境配置',
  '/websites': '网站管理',
  '/databases': '数据库管理',
  '/cron': '计划任务',
  '/scripts': '脚本库',
  '/firewall': '防火墙',
  '/ssh': 'SSH 管理',
  '/containers': '容器管理',
  '/cloud': '腾讯云',
  '/audit': '审计日志',
  '/settings': '面板设置',
  '/security': '安全设置',
  '/vulnerabilities': '漏洞扫描',
};

const NOTIFICATION_LEVEL_COLORS: Record<string, string> = {
  info: COLORS.PRIMARY,
  warning: COLORS.WARNING,
  error: COLORS.ERROR,
};

export default function Layout() {
  const [collapsed, setCollapsed] = useState(false);
  const [cmdOpen, setCmdOpen] = useState(false);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [showNotifications, setShowNotifications] = useState(false);
  const [showUserMenu, setShowUserMenu] = useState(false);
  const [sysVersion, setSysVersion] = useState<string>('');
  const notifRef = useRef<HTMLDivElement>(null);
  const userMenuRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, loadUser } = useAuthStore();

  useEffect(() => { loadUser(); }, [loadUser]);

  // Fetch version
  useEffect(() => {
    let mounted = true;
    settingsApi.getSystem().then(res => {
      if (mounted) setSysVersion(res.data?.data?.version || 'dev');
    }).catch(err => console.debug('Failed to fetch system version:', err));
    return () => { mounted = false; };
  }, []);

  // Fetch notifications
  const fetchNotifications = useCallback(async () => {
    try {
      const [listRes, countRes] = await Promise.all([
        notificationApi.list(false, 20),
        notificationApi.unreadCount(),
      ]);
      setNotifications(listRes.data?.data || []);
      setUnreadCount(countRes.data?.data?.count || 0);
    } catch (err: unknown) {
      console.debug('Failed to fetch notifications:', err);
    }
  }, []);

  useEffect(() => {
    fetchNotifications();
    const timer = setInterval(fetchNotifications, 30000);
    return () => clearInterval(timer);
  }, [fetchNotifications]);

  // Cmd+K
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setCmdOpen(prev => !prev);
      }
      if (e.key === 'Escape') {
        setCmdOpen(false);
        setShowNotifications(false);
        setShowUserMenu(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  // Close dropdowns on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (notifRef.current && !notifRef.current.contains(e.target as Node)) {
        setShowNotifications(false);
      }
      if (userMenuRef.current && !userMenuRef.current.contains(e.target as Node)) {
        setShowUserMenu(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const handleLogout = useCallback(() => {
    logout();
    navigate('/login');
  }, [logout, navigate]);

  const handleCmdSelect = useCallback((path: string) => {
    setCmdOpen(false);
    navigate(path);
  }, [navigate]);

  const handleMarkAllRead = async () => {
    try {
      await notificationApi.markAllAsRead();
      setUnreadCount(0);
      setNotifications(prev => prev.map(n => ({ ...n, is_read: true })));
    } catch (err: unknown) {
      console.error('Failed to mark all as read:', err);
      message.error('标记已读失败');
    }
  };

  const handleMarkRead = async (id: number) => {
    try {
      await notificationApi.markAsRead(id);
      setUnreadCount(prev => Math.max(0, prev - 1));
      setNotifications(prev => prev.map(n => n.id === id ? { ...n, is_read: true } : n));
    } catch (err: unknown) {
      console.error('Failed to mark as read:', err);
      message.error('标记已读失败');
    }
  };

  const currentTitle = PAGE_TITLES[location.pathname] || '未知页面';

  return (
    <div className="app-layout">
      {/* Sidebar */}
      <aside className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
        <div className="sidebar-logo">
          <div className="logo-icon">ES</div>
          {!collapsed && <span className="logo-text">EasyServer</span>}
        </div>
        <nav className="sidebar-nav">
          {MENU_GROUPS.map(group => (
            <div key={group.label} className="nav-group">
              {!collapsed && <div className="nav-group-label">{group.label}</div>}
              {group.items.map(item => (
                <div
                  key={item.key}
                  className={`nav-item ${location.pathname === item.key ? 'active' : ''}`}
                  onClick={() => navigate(item.key)}
                  title={collapsed ? item.label : undefined}
                >
                  <Icon name={item.icon} />
                  {!collapsed && <span>{item.label}</span>}
                </div>
              ))}
            </div>
          ))}
        </nav>
        <div className="sidebar-footer" style={{ display: 'flex', alignItems: 'center', justifyContent: collapsed ? 'center' : 'space-between', width: '100%' }}>
          {!collapsed && <div className="sidebar-version" style={{ fontSize: '12px', color: '#888', paddingLeft: '16px' }}>{sysVersion || '...'}</div>}
          <button className="sidebar-collapse-btn" onClick={() => setCollapsed(!collapsed)}>
            <Icon name={collapsed ? 'menu-unfold' : 'menu-fold'} />
          </button>
        </div>
      </aside>

      {/* Main */}
      <div className={`main-area ${collapsed ? 'sidebar-collapsed' : ''}`}>
        <header className="header">
          <div className="header-left">
            <div className="header-breadcrumb">
              <span>首页</span>
              <span className="breadcrumb-sep">/</span>
              <span className="current">{currentTitle}</span>
            </div>
          </div>
          <div className="header-center">
            <div className="search-trigger" onClick={() => setCmdOpen(true)}>
              <Icon name="search" size={14} />
              <span>搜索命令...</span>
              <kbd>⌘K</kbd>
            </div>
          </div>
          <div className="header-right">
            {/* Notification Bell */}
            <div className="notif-wrapper" ref={notifRef}>
              <button className="header-btn" title="通知" onClick={() => { setShowNotifications(!showNotifications); setShowUserMenu(false); }}>
                <Icon name="bell" />
                {unreadCount > 0 && <span className="notif-badge">{unreadCount > 99 ? '99+' : unreadCount}</span>}
              </button>
              {showNotifications && (
                <div className="notif-dropdown">
                  <div className="notif-header">
                    <span className="notif-title">通知</span>
                    {unreadCount > 0 && (
                      <button className="notif-mark-all" onClick={handleMarkAllRead}>全部已读</button>
                    )}
                  </div>
                  <div className="notif-list">
                    {notifications.length === 0 ? (
                      <div className="notif-empty">暂无通知</div>
                    ) : (
                      notifications.map(n => (
                        <div key={n.id} className={`notif-item ${n.is_read ? '' : 'unread'}`} onClick={() => !n.is_read && handleMarkRead(n.id)}>
                          <div className="notif-dot" style={{ background: NOTIFICATION_LEVEL_COLORS[n.level] || '#6366f1' }} />
                          <div className="notif-content">
                            <div className="notif-item-title">{n.title}</div>
                            <div className="notif-item-msg">{n.message}</div>
                            <div className="notif-item-time">{n.created_at}</div>
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                  <div className="notif-footer" onClick={() => { setShowNotifications(false); navigate('/notifications'); }}>
                    查看全部
                  </div>
                </div>
              )}
            </div>

            <div className="header-divider" />

            {/* User Menu */}
            <div className="user-menu-wrapper" ref={userMenuRef}>
              <div className="user-info" onClick={() => { setShowUserMenu(!showUserMenu); setShowNotifications(false); }}>
                <span className="user-role">管理员</span>
                <div className="user-avatar">{user?.username?.[0]?.toUpperCase() || 'A'}</div>
              </div>
              {showUserMenu && (
                <div className="user-dropdown">
                  <div className="user-dropdown-header">
                    <div className="user-avatar-lg">{user?.username?.[0]?.toUpperCase() || 'A'}</div>
                    <div>
                      <div className="user-dropdown-name">{user?.username || '用户'}</div>
                      <div className="user-dropdown-role">管理员</div>
                    </div>
                  </div>
                  <div className="user-dropdown-divider" />
                  <div className="user-dropdown-item" onClick={() => { setShowUserMenu(false); navigate('/settings'); }}>
                    <Icon name="user" size={16} />
                    <span>个人信息</span>
                  </div>
                  <div className="user-dropdown-item" onClick={() => { setShowUserMenu(false); navigate('/change-password'); }}>
                    <Icon name="key" size={16} />
                    <span>修改密码</span>
                  </div>
                  <div className="user-dropdown-item" onClick={() => { setShowUserMenu(false); navigate('/security'); }}>
                    <Icon name="shield" size={16} />
                    <span>两步验证</span>
                  </div>
                  <div className="user-dropdown-item" onClick={() => { setShowUserMenu(false); navigate('/audit'); }}>
                    <Icon name="file-text" size={16} />
                    <span>审计日志</span>
                  </div>
                  <div className="user-dropdown-divider" />
                  <div className="user-dropdown-item danger" onClick={() => { setShowUserMenu(false); handleLogout(); }}>
                    <Icon name="log-out" size={16} />
                    <span>退出登录</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </header>
        <main className="content"><Outlet /></main>
      </div>

      <CommandPalette open={cmdOpen} onClose={() => setCmdOpen(false)} onSelect={handleCmdSelect} />
    </div>
  );
}
