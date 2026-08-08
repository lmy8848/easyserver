-- 回滚 000003：还原 scripts.language 列。面板无部署，回滚仅用于开发环境重建。

ALTER TABLE scripts ADD COLUMN language TEXT DEFAULT 'sh';