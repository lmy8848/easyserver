-- Drop users 表中的两个死列：账号过期(expires_at)与 per-user IP 白名单(ip_whitelist)。
-- 这两条链路均已移除（见 auth 领域清理），列已无任何读写方。

ALTER TABLE users DROP COLUMN expires_at;
ALTER TABLE users DROP COLUMN ip_whitelist;
