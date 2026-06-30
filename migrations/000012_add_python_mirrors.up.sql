INSERT INTO runtime_mirror (lang, env_key, env_value, enabled, source)
SELECT 'python', 'PYTHON_BUILD_MIRROR_URL', 'https://npmmirror.com/mirrors/python', 1, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM runtime_mirror WHERE lang = 'python' AND env_key = 'PYTHON_BUILD_MIRROR_URL' AND env_value = 'https://npmmirror.com/mirrors/python'
);

INSERT INTO runtime_mirror (lang, env_key, env_value, enabled, source)
SELECT 'python', 'PYTHON_BUILD_MIRROR_URL', 'https://mirrors.aliyun.com/python', 0, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM runtime_mirror WHERE lang = 'python' AND env_key = 'PYTHON_BUILD_MIRROR_URL' AND env_value = 'https://mirrors.aliyun.com/python'
);
