package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ConfigSection represents a section in a config file (e.g., [mysqld])
type ConfigSection struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}

// DBConfig represents a parsed database config file (generic for MySQL/PostgreSQL/Redis)
type DBConfig struct {
	FilePath string          `json:"file_path"`
	Sections []ConfigSection `json:"sections"`
}

// MySQLConfig is an alias for backward compatibility
type MySQLConfig = DBConfig

// CommonMySQLParams defines the UI metadata for common MySQL parameters
type ParamMeta struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"` // text, number, select, boolean
	Unit        string `json:"unit"` // MB, GB, etc.
	Options     []string `json:"options,omitempty"` // for select type
	Default     string `json:"default"`
}

// GetCommonParams returns metadata for common MySQL configuration parameters
func GetCommonParams(section string) []ParamMeta {
	switch section {
	case "mysqld":
		return []ParamMeta{
			{Key: "port", Label: "监听端口", Description: "MySQL 服务监听的端口号", Type: "number", Default: "3306"},
			{Key: "datadir", Label: "数据目录", Description: "MySQL 数据文件存储路径", Type: "text", Default: "/var/lib/mysql"},
			{Key: "socket", Label: "Socket 文件", Description: "Unix Socket 文件路径", Type: "text", Default: "/var/run/mysqld/mysqld.sock"},
			{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "151"},
			{Key: "innodb_buffer_pool_size", Label: "InnoDB 缓冲池", Description: "InnoDB 缓冲池大小，建议设为内存的 70-80%", Type: "text", Unit: "MB/GB", Default: "128M"},
			{Key: "character-set-server", Label: "服务器字符集", Description: "默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
			{Key: "collation-server", Label: "排序规则", Description: "默认排序规则", Type: "text", Default: "utf8mb4_general_ci"},
			{Key: "default-storage-engine", Label: "默认存储引擎", Description: "默认存储引擎", Type: "select", Options: []string{"InnoDB", "MyISAM", "MEMORY"}, Default: "InnoDB"},
			{Key: "max_allowed_packet", Label: "最大数据包", Description: "单个数据包最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "tmp_table_size", Label: "临时表大小", Description: "内存临时表最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "max_heap_table_size", Label: "堆表最大大小", Description: "用户创建的内存表最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "sort_buffer_size", Label: "排序缓冲区", Description: "每个会话的排序缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "read_buffer_size", Label: "读缓冲区", Description: "顺序扫描的读缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "join_buffer_size", Label: "JOIN 缓冲区", Description: "JOIN 操作的缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "log_error", Label: "错误日志路径", Description: "错误日志文件路径", Type: "text", Default: "/var/log/mysql/error.log"},
			{Key: "slow_query_log", Label: "慢查询日志", Description: "是否启用慢查询日志", Type: "select", Options: []string{"ON", "OFF"}, Default: "OFF"},
			{Key: "slow_query_log_file", Label: "慢查询日志路径", Description: "慢查询日志文件路径", Type: "text", Default: "/var/log/mysql/slow.log"},
			{Key: "long_query_time", Label: "慢查询阈值", Description: "超过此时间的查询记录到慢查询日志", Type: "number", Unit: "秒", Default: "10"},
			{Key: "wait_timeout", Label: "空闲超时", Description: "非交互连接的空闲超时时间", Type: "number", Unit: "秒", Default: "28800"},
			{Key: "interactive_timeout", Label: "交互超时", Description: "交互连接的空闲超时时间", Type: "number", Unit: "秒", Default: "28800"},
		}
	case "client":
		return []ParamMeta{
			{Key: "default-character-set", Label: "默认字符集", Description: "客户端默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
			{Key: "port", Label: "端口", Description: "连接端口", Type: "number", Default: "3306"},
			{Key: "socket", Label: "Socket", Description: "Unix Socket 文件路径", Type: "text", Default: "/var/run/mysqld/mysqld.sock"},
		}
	case "mysql":
		return []ParamMeta{
			{Key: "default-character-set", Label: "默认字符集", Description: "mysql 客户端默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
		}
	case "mysqldump":
		return []ParamMeta{
			{Key: "max_allowed_packet", Label: "最大数据包", Description: "导出数据包最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "default-character-set", Label: "默认字符集", Description: "导出默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1"}, Default: "utf8mb4"},
		}
	}
	return nil
}

// CommonConfigFilePaths returns common MySQL config file locations
func CommonConfigFilePaths() []string {
	return []string{
		"/etc/mysql/my.cnf",
		"/etc/mysql/mysql.conf.d/mysqld.cnf",
		"/etc/my.cnf",
		"/usr/etc/my.cnf",
	}
}

// FindMySQLConfig finds the active MySQL config file
func FindMySQLConfig() string {
	for _, path := range CommonConfigFilePaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParseMySQLConfig parses a my.cnf file into structured sections
func ParseMySQLConfig(filePath string) (*MySQLConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	config := &MySQLConfig{
		FilePath: filePath,
		Sections: []ConfigSection{},
	}

	var currentSection *ConfigSection
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := line[1 : len(line)-1]
			config.Sections = append(config.Sections, ConfigSection{
				Name:   sectionName,
				Params: make(map[string]string),
			})
			currentSection = &config.Sections[len(config.Sections)-1]
			continue
		}

		// Key = Value
		if currentSection != nil {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// Remove inline comments
				if idx := strings.Index(value, " #"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				if idx := strings.Index(value, " //"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				currentSection.Params[key] = value
			}
		}
	}

	return config, nil
}

// SaveMySQLConfig saves the structured config back to file
func SaveMySQLConfig(config *MySQLConfig) error {
	// Backup original file
	backupPath := config.FilePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(config.FilePath); err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated MySQL configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	for _, section := range config.Sections {
		sb.WriteString(fmt.Sprintf("[%s]\n", section.Name))
		keys := make([]string, 0, len(section.Params))
		for key := range section.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("%s = %s\n", key, section.Params[key]))
		}
		sb.WriteString("\n")
	}

	// Ensure directory exists
	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)

	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// UpdateConfigParam updates a single parameter in a config section
func UpdateConfigParam(config *MySQLConfig, section, key, value string) {
	for i, s := range config.Sections {
		if s.Name == section {
			config.Sections[i].Params[key] = value
			return
		}
	}
	// Section doesn't exist, create it
	config.Sections = append(config.Sections, ConfigSection{
		Name:   section,
		Params: map[string]string{key: value},
	})
}

// ============================================================
// PostgreSQL config management
// ============================================================

// FindPostgreSQLConfig finds the active PostgreSQL config file
func FindPostgreSQLConfig() string {
	paths := []string{
		"/etc/postgresql/16/main/postgresql.conf",
		"/etc/postgresql/15/main/postgresql.conf",
		"/etc/postgresql/14/main/postgresql.conf",
		"/etc/postgresql/13/main/postgresql.conf",
		"/var/lib/pgsql/data/postgresql.conf",
		"/var/lib/postgresql/16/main/postgresql.conf",
		"/var/lib/postgresql/15/main/postgresql.conf",
		"/var/lib/postgresql/14/main/postgresql.conf",
		"/var/lib/postgresql/13/main/postgresql.conf",
	}
	// Also glob for any version
	matches, _ := filepath.Glob("/etc/postgresql/*/main/postgresql.conf")
	paths = append(matches, paths...)
	matches, _ = filepath.Glob("/var/lib/postgresql/*/main/postgresql.conf")
	paths = append(matches, paths...)

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParsePostgreSQLConfig parses a postgresql.conf file
// PostgreSQL uses flat key = value format (no section headers)
func ParsePostgreSQLConfig(filePath string) (*MySQLConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	params := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Skip include directives
		if strings.HasPrefix(line, "include") || strings.HasPrefix(line, "include_if_exists") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove inline comments
			if idx := strings.Index(value, " #"); idx != -1 {
				value = strings.TrimSpace(value[:idx])
			}
			// Remove surrounding quotes (single or double)
			if len(value) >= 2 {
				if (value[0] == '\'' && value[len(value)-1] == '\'') ||
					(value[0] == '"' && value[len(value)-1] == '"') {
					value = value[1 : len(value)-1]
				}
			}
			params[key] = value
		}
	}

	return &MySQLConfig{
		FilePath: filePath,
		Sections: []ConfigSection{
			{Name: "main", Params: params},
		},
	}, nil
}

// pgNeedsQuote returns true if a PostgreSQL config value needs single quotes
func pgNeedsQuote(value string) bool {
	// Numeric values don't need quotes
	if _, err := fmt.Sscanf(value, "%d", new(int)); err == nil {
		return false
	}
	// Boolean-like values don't need quotes
	switch strings.ToLower(value) {
	case "on", "off", "true", "false", "yes", "no":
		return false
	}
	// Values with units like 128MB, 1GB don't need quotes
	if len(value) > 0 {
		lastChar := value[len(value)-1]
		if lastChar >= 'A' && lastChar <= 'Z' || lastChar >= 'a' && lastChar <= 'z' {
			// Could be a unit suffix - check if the numeric part parses
			numPart := value[:len(value)-1]
			if _, err := fmt.Sscanf(numPart, "%f", new(float64)); err == nil {
				return false
			}
		}
	}
	return true
}

// SavePostgreSQLConfig saves PostgreSQL config back to file
func SavePostgreSQLConfig(config *MySQLConfig) error {
	backupPath := config.FilePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(config.FilePath); err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated PostgreSQL configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	if len(config.Sections) > 0 {
		// Sort keys for deterministic output
		keys := make([]string, 0, len(config.Sections[0].Params))
		for key := range config.Sections[0].Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := config.Sections[0].Params[key]
			if pgNeedsQuote(value) {
				sb.WriteString(fmt.Sprintf("%s = '%s'\n", key, value))
			} else {
				sb.WriteString(fmt.Sprintf("%s = %s\n", key, value))
			}
		}
	}

	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// GetPostgreSQLCommonParams returns metadata for common PostgreSQL parameters
func GetPostgreSQLCommonParams() []ParamMeta {
	return []ParamMeta{
		{Key: "listen_addresses", Label: "监听地址", Description: "监听的 IP 地址，'*' 表示所有", Type: "text", Default: "localhost"},
		{Key: "port", Label: "端口", Description: "PostgreSQL 服务监听端口", Type: "number", Default: "5432"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "100"},
		{Key: "shared_buffers", Label: "共享缓冲区", Description: "共享缓冲区大小，建议设为内存的 25%", Type: "text", Unit: "MB/GB", Default: "128MB"},
		{Key: "work_mem", Label: "工作内存", Description: "每个排序/哈希操作的内存", Type: "text", Unit: "MB/KB", Default: "4MB"},
		{Key: "maintenance_work_mem", Label: "维护工作内存", Description: "VACUUM/CREATE INDEX 等维护操作的内存", Type: "text", Unit: "MB/GB", Default: "64MB"},
		{Key: "wal_level", Label: "WAL 级别", Description: "Write-Ahead 日志级别", Type: "select", Options: []string{"minimal", "replica", "logical"}, Default: "replica"},
		{Key: "max_wal_size", Label: "最大 WAL 大小", Description: "自动检查点之间的最大 WAL 大小", Type: "text", Unit: "MB/GB", Default: "1GB"},
		{Key: "min_wal_size", Label: "最小 WAL 大小", Description: "WAL 文件回收的最小大小", Type: "text", Unit: "MB/GB", Default: "80MB"},
		{Key: "log_destination", Label: "日志目标", Description: "日志输出目标", Type: "select", Options: []string{"stderr", "csvlog", "syslog"}, Default: "stderr"},
		{Key: "logging_collector", Label: "日志收集器", Description: "是否启用日志收集器", Type: "select", Options: []string{"on", "off"}, Default: "on"},
		{Key: "log_directory", Label: "日志目录", Description: "日志文件存储目录", Type: "text", Default: "pg_log"},
		{Key: "log_filename", Label: "日志文件名", Description: "日志文件名模式", Type: "text", Default: "postgresql-%Y-%m-%d_%H%M%S.log"},
		{Key: "password_encryption", Label: "密码加密", Description: "密码加密方式", Type: "select", Options: []string{"scram-sha-256", "md5"}, Default: "scram-sha-256"},
		{Key: "ssl", Label: "SSL", Description: "是否启用 SSL", Type: "select", Options: []string{"on", "off"}, Default: "off"},
	}
}

// ============================================================
// Redis config management
// ============================================================

// FindRedisConfig finds the active Redis config file
func FindRedisConfig() string {
	paths := []string{
		"/etc/redis/redis.conf",
		"/etc/redis.conf",
		"/opt/redis/redis.conf",
		"/usr/local/etc/redis.conf",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParseRedisConfig parses a redis.conf file
// Redis uses space-separated key value format
func ParseRedisConfig(filePath string) (*MySQLConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	params := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Skip includes and special directives
		if strings.HasPrefix(line, "include ") {
			continue
		}
		idx := strings.Index(line, " ")
		if idx == -1 {
			continue
		}
		key := line[:idx]
		value := strings.TrimSpace(line[idx+1:])
		// Remove inline comments
		if cidx := strings.Index(value, " #"); cidx != -1 {
			value = strings.TrimSpace(value[:cidx])
		}
		// Handle multiple save directives - keep the last one for simplicity
		// or concatenate them
		if key == "save" {
			if existing, ok := params["save"]; ok {
				params["save"] = existing + "\n" + value
			} else {
				params["save"] = value
			}
		} else {
			params[key] = value
		}
	}

	return &MySQLConfig{
		FilePath: filePath,
		Sections: []ConfigSection{
			{Name: "main", Params: params},
		},
	}, nil
}

// SaveRedisConfig saves Redis config back to file
func SaveRedisConfig(config *MySQLConfig) error {
	backupPath := config.FilePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(config.FilePath); err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated Redis configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	if len(config.Sections) > 0 {
		keys := make([]string, 0, len(config.Sections[0].Params))
		for key := range config.Sections[0].Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := config.Sections[0].Params[key]
			if key == "save" {
				// Handle multi-line save directives
				lines := strings.Split(value, "\n")
				for _, l := range lines {
					l = strings.TrimSpace(l)
					if l != "" {
						sb.WriteString(fmt.Sprintf("save %s\n", l))
					}
				}
			} else {
				sb.WriteString(fmt.Sprintf("%s %s\n", key, value))
			}
		}
	}

	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// GetRedisCommonParams returns metadata for common Redis parameters
func GetRedisCommonParams() []ParamMeta {
	return []ParamMeta{
		{Key: "bind", Label: "绑定地址", Description: "监听的 IP 地址", Type: "text", Default: "127.0.0.1"},
		{Key: "port", Label: "端口", Description: "Redis 服务监听端口", Type: "number", Default: "6379"},
		{Key: "protected-mode", Label: "保护模式", Description: "无密码时禁止外部访问", Type: "select", Options: []string{"yes", "no"}, Default: "yes"},
		{Key: "requirepass", Label: "访问密码", Description: "Redis 访问密码", Type: "text", Default: ""},
		{Key: "maxmemory", Label: "最大内存", Description: "Redis 最大内存使用量", Type: "text", Unit: "mb/gb", Default: "256mb"},
		{Key: "maxmemory-policy", Label: "内存淘汰策略", Description: "内存满时的 key 淘汰策略", Type: "select", Options: []string{"noeviction", "allkeys-lru", "volatile-lru", "allkeys-random", "volatile-random", "volatile-ttl"}, Default: "allkeys-lru"},
		{Key: "save", Label: "RDB 持久化", Description: "RDB 快照策略（秒 数据变更次数），多个策略用换行分隔", Type: "text", Default: "900 1"},
		{Key: "appendonly", Label: "AOF 持久化", Description: "是否启用 AOF 持久化", Type: "select", Options: []string{"yes", "no"}, Default: "no"},
		{Key: "appendfsync", Label: "AOF 同步策略", Description: "AOF 文件同步策略", Type: "select", Options: []string{"always", "everysec", "no"}, Default: "everysec"},
		{Key: "timeout", Label: "空闲超时", Description: "客户端空闲断开时间（秒），0 表示不断开", Type: "number", Unit: "秒", Default: "0"},
		{Key: "databases", Label: "数据库数量", Description: "Redis 数据库数量", Type: "number", Default: "16"},
		{Key: "tcp-backlog", Label: "TCP 积压", Description: "TCP 连接积压队列大小", Type: "number", Default: "511"},
		{Key: "tcp-keepalive", Label: "TCP 保活", Description: "TCP 保活探测间隔（秒）", Type: "number", Unit: "秒", Default: "300"},
		{Key: "loglevel", Label: "日志级别", Description: "Redis 日志级别", Type: "select", Options: []string{"debug", "verbose", "notice", "warning"}, Default: "notice"},
		{Key: "logfile", Label: "日志文件", Description: "日志文件路径，空表示标准输出", Type: "text", Default: ""},
	}
}
