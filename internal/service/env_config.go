package service

import (
	"database/sql"
	"fmt"
	"strings"

	"easyserver/internal/model"
)

type EnvConfigService struct {
	db *sql.DB
}

func NewEnvConfigService(db *sql.DB) *EnvConfigService {
	return &EnvConfigService{db: db}
}

// InitTables creates environment config tables if they don't exist
func (s *EnvConfigService) InitTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS env_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			value TEXT NOT NULL,
			runtime_id INTEGER DEFAULT 0,
			is_global INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(name, runtime_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_env_configs_runtime ON env_configs(runtime_id)`,
		`CREATE TABLE IF NOT EXISTS path_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL,
			runtime_id INTEGER DEFAULT 0,
			is_global INTEGER DEFAULT 0,
			order_num INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(path, runtime_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_path_entries_runtime ON path_entries(runtime_id)`,
		`CREATE TABLE IF NOT EXISTS global_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(category, key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_global_configs_category ON global_configs(category)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	// Initialize default global configs
	s.InitDefaultGlobalConfigs()

	return nil
}

// ListEnvConfigs returns all environment configurations
func (s *EnvConfigService) ListEnvConfigs(runtimeID int64) ([]model.EnvConfig, error) {
	rows, err := s.db.Query(
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ? ORDER BY name",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.EnvConfig
	for rows.Next() {
		var c model.EnvConfig
		var isGlobal int
		err := rows.Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		c.IsGlobal = isGlobal != 0
		configs = append(configs, c)
	}

	return configs, nil
}

// GetEnvConfig returns a specific environment configuration
func (s *EnvConfigService) GetEnvConfig(id int64) (*model.EnvConfig, error) {
	c := &model.EnvConfig{}
	var isGlobal int
	err := s.db.QueryRow(
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IsGlobal = isGlobal != 0
	return c, nil
}

// CreateEnvConfig creates a new environment configuration
func (s *EnvConfigService) CreateEnvConfig(c *model.EnvConfig) error {
	// Validate name
	if !isValidEnvName(c.Name) {
		return fmt.Errorf("invalid environment variable name: %s", c.Name)
	}

	result, err := s.db.Exec(
		"INSERT INTO env_configs (name, value, runtime_id, is_global) VALUES (?, ?, ?, ?)",
		c.Name, c.Value, c.RuntimeID, c.IsGlobal,
	)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	return nil
}

// UpdateEnvConfig updates an environment configuration
func (s *EnvConfigService) UpdateEnvConfig(c *model.EnvConfig) error {
	// Validate name
	if !isValidEnvName(c.Name) {
		return fmt.Errorf("invalid environment variable name: %s", c.Name)
	}

	_, err := s.db.Exec(
		"UPDATE env_configs SET name = ?, value = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Name, c.Value, c.ID,
	)
	return err
}

// DeleteEnvConfig deletes an environment configuration
func (s *EnvConfigService) DeleteEnvConfig(id int64) error {
	_, err := s.db.Exec("DELETE FROM env_configs WHERE id = ?", id)
	return err
}

// ListPathEntries returns all PATH entries
func (s *EnvConfigService) ListPathEntries(runtimeID int64) ([]model.PathEntry, error) {
	rows, err := s.db.Query(
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ? ORDER BY order_num",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.PathEntry
	for rows.Next() {
		var e model.PathEntry
		var isGlobal int
		err := rows.Scan(&e.ID, &e.Path, &e.RuntimeID, &isGlobal, &e.Order, &e.CreatedAt)
		if err != nil {
			continue
		}
		e.IsGlobal = isGlobal != 0
		entries = append(entries, e)
	}

	return entries, nil
}

// CreatePathEntry creates a new PATH entry
func (s *EnvConfigService) CreatePathEntry(e *model.PathEntry) error {
	// Validate path
	if !isValidPath(e.Path) {
		return fmt.Errorf("invalid path: %s", e.Path)
	}

	// Get max order
	var maxOrder int
	s.db.QueryRow("SELECT COALESCE(MAX(order_num), 0) FROM path_entries WHERE runtime_id = ?", e.RuntimeID).Scan(&maxOrder)

	result, err := s.db.Exec(
		"INSERT INTO path_entries (path, runtime_id, is_global, order_num) VALUES (?, ?, ?, ?)",
		e.Path, e.RuntimeID, e.IsGlobal, maxOrder+1,
	)
	if err != nil {
		return err
	}
	e.ID, _ = result.LastInsertId()
	e.Order = maxOrder + 1
	return nil
}

// DeletePathEntry deletes a PATH entry
func (s *EnvConfigService) DeletePathEntry(id int64) error {
	_, err := s.db.Exec("DELETE FROM path_entries WHERE id = ?", id)
	return err
}

// ReorderPathEntries reorders PATH entries
func (s *EnvConfigService) ReorderPathEntries(runtimeID int64, ids []int64) error {
	for i, id := range ids {
		_, err := s.db.Exec(
			"UPDATE path_entries SET order_num = ? WHERE id = ? AND runtime_id = ?",
			i+1, id, runtimeID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GenerateEnvScript generates a shell script to set environment variables
func (s *EnvConfigService) GenerateEnvScript(runtimeID int64) (string, error) {
	var script strings.Builder

	// Get environment variables
	configs, err := s.ListEnvConfigs(runtimeID)
	if err != nil {
		return "", err
	}

	for _, c := range configs {
		// Escape value for safe use inside double-quoted shell string
		escaped := shellEscapeDoubleQuote(c.Value)
		script.WriteString(fmt.Sprintf("export %s=\"%s\"\n", c.Name, escaped))
	}

	// Get PATH entries
	entries, err := s.ListPathEntries(runtimeID)
	if err != nil {
		return "", err
	}

	if len(entries) > 0 {
		script.WriteString("export PATH=\"")
		for i, e := range entries {
			if i > 0 {
				script.WriteString(":")
			}
			// PATH entries are already validated by isValidPath, but escape for safety
			script.WriteString(shellEscapeDoubleQuote(e.Path))
		}
		script.WriteString(":$PATH\"\n")
	}

	return script.String(), nil
}

// shellEscapeDoubleQuote escapes special characters for use inside a double-quoted shell string.
// Characters escaped: \ " $ ` !
func shellEscapeDoubleQuote(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '\\', '"', '$', '`', '!':
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// isValidEnvName validates environment variable name
func isValidEnvName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for i, c := range name {
		if i == 0 {
			// First character must be letter or underscore
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
		} else {
			// Subsequent characters can be letters, digits, or underscores
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// isValidPath validates PATH entry — must be a safe absolute path with no shell metacharacters
func isValidPath(path string) bool {
	if len(path) == 0 || len(path) > 4096 {
		return false
	}
	// Must start with / (absolute path)
	if path[0] != '/' {
		return false
	}
	// Reject paths with .. for security
	if strings.Contains(path, "..") {
		return false
	}
	// Reject shell metacharacters that could enable command injection
	shellMeta := "|&;()`${}<>'\"\\!#~"
	for _, c := range path {
		if strings.ContainsRune(shellMeta, c) {
			return false
		}
		// Reject control characters
		if c < 32 {
			return false
		}
	}
	return true
}

// ListGlobalConfigs returns all global configurations
func (s *EnvConfigService) ListGlobalConfigs(category string) ([]model.GlobalConfig, error) {
	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = s.db.Query(
			"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs WHERE category = ? ORDER BY category, key",
			category,
		)
	} else {
		rows, err = s.db.Query(
			"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs ORDER BY category, key",
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.GlobalConfig
	for rows.Next() {
		var c model.GlobalConfig
		err := rows.Scan(&c.ID, &c.Category, &c.Key, &c.Value, &c.Description, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		configs = append(configs, c)
	}

	return configs, nil
}

// GetGlobalConfig returns a specific global configuration
func (s *EnvConfigService) GetGlobalConfig(id int64) (*model.GlobalConfig, error) {
	c := &model.GlobalConfig{}
	err := s.db.QueryRow(
		"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Category, &c.Key, &c.Value, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CreateGlobalConfig creates a new global configuration
func (s *EnvConfigService) CreateGlobalConfig(c *model.GlobalConfig) error {
	result, err := s.db.Exec(
		"INSERT INTO global_configs (category, key, value, description) VALUES (?, ?, ?, ?)",
		c.Category, c.Key, c.Value, c.Description,
	)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	return nil
}

// UpdateGlobalConfig updates a global configuration
func (s *EnvConfigService) UpdateGlobalConfig(c *model.GlobalConfig) error {
	_, err := s.db.Exec(
		"UPDATE global_configs SET value = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Value, c.Description, c.ID,
	)
	return err
}

// DeleteGlobalConfig deletes a global configuration
func (s *EnvConfigService) DeleteGlobalConfig(id int64) error {
	_, err := s.db.Exec("DELETE FROM global_configs WHERE id = ?", id)
	return err
}

// InitDefaultGlobalConfigs initializes default global configurations
func (s *EnvConfigService) InitDefaultGlobalConfigs() error {
	defaults := []model.GlobalConfig{
		// Maven
		{Category: "maven", Key: "mirror_url", Value: "https://maven.aliyun.com/repository/public", Description: "Maven 镜像地址"},
		{Category: "maven", Key: "local_repo", Value: "${user.home}/.m2/repository", Description: "Maven 本地仓库路径"},

		// npm
		{Category: "npm", Key: "registry", Value: "https://registry.npmmirror.com", Description: "npm 镜像源"},
		{Category: "npm", Key: "cache", Value: "${HOME}/.npm", Description: "npm 缓存目录"},

		// pip
		{Category: "pip", Key: "index_url", Value: "https://pypi.tuna.tsinghua.edu.cn/simple", Description: "pip 镜像源"},
		{Category: "pip", Key: "trusted_host", Value: "pypi.tuna.tsinghua.edu.cn", Description: "pip 可信主机"},

		// Go
		{Category: "go", Key: "goproxy", Value: "https://goproxy.cn,direct", Description: "Go 模块代理"},
		{Category: "go", Key: "gonosumcheck", Value: "", Description: "Go 不校验 checksum 的模块"},

		// Composer (PHP)
		{Category: "composer", Key: "repo_url", Value: "https://mirrors.aliyun.com/composer/", Description: "Composer 镜像地址"},

		// Ruby
		{Category: "ruby", Key: "source", Value: "https://gems.ruby-china.com/", Description: "RubyGems 镜像源"},
	}

	for _, c := range defaults {
		// Only insert if not exists
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM global_configs WHERE category = ? AND key = ?", c.Category, c.Key).Scan(&count)
		if count == 0 {
			s.db.Exec(
				"INSERT INTO global_configs (category, key, value, description) VALUES (?, ?, ?, ?)",
				c.Category, c.Key, c.Value, c.Description,
			)
		}
	}

	return nil
}
