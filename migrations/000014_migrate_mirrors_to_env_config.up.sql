-- Migrate enabled mirror rows from runtime_mirror into env_configs.
-- Each (env_key) maps to at most one env_configs row; env_configs.name has a
-- UNIQUE constraint so only one row per env var name survives — we take the
-- enabled=1 row, preferring the lowest id if multiple were enabled (shouldn't
-- happen, but defensive). Rows where no mirror is enabled are skipped: that
-- means the user wants mise's default source, i.e. the env var should be
-- absent, which is exactly the "no env_configs row" state.
--
-- mise default source URLs (nodejs.org/dist, go.dev/dl) are NOT migrated:
-- those are the URLs mise uses when the env var is unset, so writing them
-- back would be a no-op that clutters the env-config UI. We filter them out.

INSERT OR IGNORE INTO env_configs (name, value, enabled, created_at, updated_at)
SELECT env_key, env_value, 1, updated_at, updated_at
FROM runtime_mirror m
WHERE m.enabled = 1
  AND NOT (
    (m.env_key = 'MISE_NODE_MIRROR_URL'     AND m.env_value = 'https://nodejs.org/dist')
    OR (m.env_key = 'MISE_GO_DOWNLOAD_MIRROR' AND m.env_value = 'https://go.dev/dl')
  )
  AND m.id = (
    SELECT MIN(id) FROM runtime_mirror
    WHERE env_key = m.env_key AND enabled = 1
  );

DROP TABLE IF EXISTS runtime_mirror;
