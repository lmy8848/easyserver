-- 脚本去掉 language 字段：执行依赖 shebang，语言字段仅展示用，移除。
-- SQLite 3.35+ 支持 ALTER TABLE DROP COLUMN（沿用 000002 的做法）。

ALTER TABLE scripts DROP COLUMN language;