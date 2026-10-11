-- 端口转发规则（nftables nat DNAT）
CREATE TABLE IF NOT EXISTS port_forwards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    listen_port TEXT NOT NULL,
    dest_ip TEXT NOT NULL,
    dest_port TEXT NOT NULL DEFAULT '',
    source_cidr TEXT NOT NULL DEFAULT '',
    interface TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    remark TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_port_forwards_enabled ON port_forwards(enabled);
