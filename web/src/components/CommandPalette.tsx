import { useState, useEffect, useRef } from 'react';
import './CommandPalette.css';

interface CommandItem {
  id: string;
  label: string;
  hint?: string;
  group: string;
  action: () => void;
}

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onSelect: (path: string) => void;
}

export default function CommandPalette({ open, onClose, onSelect }: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const commands: CommandItem[] = [
    // Navigation
    { id: 'nav-overview', label: '系统概览', hint: '/', group: '导航', action: () => onSelect('/') },
    { id: 'nav-processes', label: '进程守护', hint: '/processes', group: '导航', action: () => onSelect('/processes') },
    { id: 'nav-services', label: '服务管理', hint: '/services', group: '导航', action: () => onSelect('/services') },
    { id: 'nav-terminal', label: '终端访问', hint: '/terminal', group: '导航', action: () => onSelect('/terminal') },
    { id: 'nav-files', label: '文件管理', hint: '/files', group: '导航', action: () => onSelect('/files') },
    { id: 'nav-deploy', label: '部署同步', hint: '/deploy', group: '导航', action: () => onSelect('/deploy') },
    { id: 'nav-websites', label: '网站管理', hint: '/websites', group: '导航', action: () => onSelect('/websites') },
    { id: 'nav-databases', label: '数据库管理', hint: '/databases', group: '导航', action: () => onSelect('/databases') },
    { id: 'nav-cron', label: '计划任务', hint: '/cron', group: '导航', action: () => onSelect('/cron') },
    { id: 'nav-audit', label: '操作日志', hint: '/audit', group: '导航', action: () => onSelect('/audit') },
    { id: 'nav-settings', label: '面板设置', hint: '/settings', group: '导航', action: () => onSelect('/settings') },
    { id: 'nav-security', label: '安全设置', hint: '/security', group: '导航', action: () => onSelect('/security') },
    // Quick actions
    { id: 'act-restart', label: '重启 EasyServer', group: '操作', action: () => { onClose(); } },
    { id: 'act-update', label: '检查更新', group: '操作', action: () => { onClose(); } },
    { id: 'act-logs', label: '查看系统日志', group: '操作', action: () => onSelect('/audit') },
  ];

  const filtered = query
    ? commands.filter(c => c.label.toLowerCase().includes(query.toLowerCase()))
    : commands;

  // Group filtered results
  const grouped = filtered.reduce<Record<string, CommandItem[]>>((acc, cmd) => {
    (acc[cmd.group] = acc[cmd.group] || []).push(cmd);
    return acc;
  }, {});

  const [prevOpen, setPrevOpen] = useState(open);
  if (open && !prevOpen) {
    setQuery('');
    setSelectedIndex(0);
  }
  if (prevOpen !== open) setPrevOpen(open);

  useEffect(() => {
    if (open) {
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  const [prevQuery, setPrevQuery] = useState(query);
  if (prevQuery !== query) {
    setPrevQuery(query);
    setSelectedIndex(0);
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex(i => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex(i => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      filtered[selectedIndex]?.action();
    } else if (e.key === 'Escape') {
      onClose();
    }
  };

  if (!open) return null;

  let flatIndex = -1;

  return (
    <div className="cmd-overlay" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="cmd-palette">
        <div className="cmd-input-wrap">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
          <input
            ref={inputRef}
            className="cmd-input"
            placeholder="输入命令或搜索..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          <kbd className="cmd-esc">ESC</kbd>
        </div>
        <div className="cmd-results">
          {Object.entries(grouped).map(([group, items]) => (
            <div key={group}>
              <div className="cmd-group-label">{group}</div>
              {items.map(item => {
                flatIndex++;
                const idx = flatIndex;
                return (
                  <div
                    key={item.id}
                    className={`cmd-item ${idx === selectedIndex ? 'selected' : ''}`}
                    onClick={item.action}
                    onMouseEnter={() => setSelectedIndex(idx)}
                  >
                    <span className="cmd-item-text">{item.label}</span>
                    {item.hint && <span className="cmd-item-hint">{item.hint}</span>}
                  </div>
                );
              })}
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="cmd-empty">没有匹配的命令</div>
          )}
        </div>
      </div>
    </div>
  );
}
