-- 暴露端口访问控制（PortACL）：面板管控内网穿透/服务暴露的端口，
-- 只允许授权源 IP/CIDR 访问，其余来源 DROP。
-- 供插件经 Host API port.expose/port.revoke 维护（权限 firewall.manage）。
CREATE TABLE IF NOT EXISTS port_acl (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    port INTEGER NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    cidrs TEXT NOT NULL,           -- 逗号分隔的授权源 CIDR 列表
    remark TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(port, protocol)
);
CREATE INDEX idx_port_acl_port ON port_acl(port);
