package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// 结构化配置。面板是配置参数的唯一写入方：参数元数据定义在下方，Key 就是驱动
// 设置时的实际参数名（零转换）。配置的读写全走驱动直连（分支迁移）：读取用
// 数据库的变量查询，持久化交给数据库自身机制 —— MySQL SET PERSIST
// （mysqld-auto.cnf）/ PostgreSQL ALTER SYSTEM（postgresql.auto.conf）/ Redis
// CONFIG SET + CONFIG REWRITE（redis.conf）。GET 读到啥返回啥（运行时值）。

// configParams 返回某类型可编辑的参数元数据（供 GET 渲染编辑器、SAVE 白名单与
// 读配置过滤）。只含 label/type/options 等 UI 信息，无编译默认值。
func configParams(dbType DBType) []ParamMeta {
	switch dbType {
	case DBTypeMySQL:
		return mysqlConfigParams()
	case DBTypePostgreSQL:
		return postgresConfigParams()
	default:
		return redisConfigParams()
	}
}

func mysqlConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Description: "MySQL 服务监听端口。修改后保存即重建容器生效"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数"},
		{Key: "max_allowed_packet", Label: "最大数据包", Description: "单个数据包最大大小", Unit: "MB"},
		{Key: "default_storage_engine", Label: "默认存储引擎", Description: "默认存储引擎", Options: []string{"InnoDB", "MyISAM", "MEMORY"}},
		{Key: "innodb_buffer_pool_size", Label: "InnoDB 缓冲池", Description: "InnoDB 缓冲池大小，生产建议内存的 70-80%", Unit: "MB/GB"},
		{Key: "tmp_table_size", Label: "临时表大小", Description: "内存临时表最大大小", Unit: "MB"},
		{Key: "max_heap_table_size", Label: "堆表最大大小", Description: "用户内存表最大大小", Unit: "MB"},
		{Key: "sort_buffer_size", Label: "排序缓冲区", Description: "每个会话排序缓冲区", Unit: "KB"},
		{Key: "join_buffer_size", Label: "JOIN 缓冲区", Description: "每个会话 JOIN 缓冲区", Unit: "KB"},
		{Key: "read_buffer_size", Label: "读缓冲区", Description: "顺序扫描读缓冲区", Unit: "KB"},
		{Key: "wait_timeout", Label: "非交互超时", Description: "非交互连接空闲超时（秒）", Unit: "秒"},
		{Key: "interactive_timeout", Label: "交互超时", Description: "交互连接空闲超时（秒）", Unit: "秒"},
		{Key: "slow_query_log", Label: "慢查询日志", Description: "是否启用慢查询日志", Options: []string{"ON", "OFF"}},
		{Key: "long_query_time", Label: "慢查询阈值", Description: "超过此时间（秒）记入慢查询日志", Unit: "秒"},
	}
}

func postgresConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Description: "PostgreSQL 服务监听端口。修改后保存即重建容器生效"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数"},
		{Key: "shared_buffers", Label: "共享缓冲区", Description: "共享缓冲区大小，生产建议内存的 25%", Unit: "MB/GB"},
		{Key: "work_mem", Label: "工作内存", Description: "每个排序/哈希操作的内存", Unit: "MB/KB"},
		{Key: "maintenance_work_mem", Label: "维护工作内存", Description: "VACUUM/CREATE INDEX 等维护操作内存", Unit: "MB/GB"},
		{Key: "wal_level", Label: "WAL 级别", Description: "Write-Ahead 日志级别", Options: []string{"minimal", "replica", "logical"}},
		{Key: "max_wal_size", Label: "最大 WAL 大小", Description: "自动检查点之间最大 WAL 大小", Unit: "MB/GB"},
		{Key: "min_wal_size", Label: "最小 WAL 大小", Description: "WAL 回收的最小大小", Unit: "MB/GB"},
		{Key: "log_destination", Label: "日志目标", Description: "日志输出目标", Options: []string{"stderr", "csvlog", "syslog"}},
		{Key: "logging_collector", Label: "日志收集器", Description: "是否启用日志收集器", Options: []string{"on", "off"}},
		{Key: "ssl", Label: "SSL", Description: "是否启用 SSL", Options: []string{"on", "off"}},
	}
}

func redisConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Description: "Redis 服务监听端口。修改后保存即重建容器生效"},
		{Key: "protected-mode", Label: "保护模式", Description: "无密码时禁止外部访问", Options: []string{"yes", "no"}},
		{Key: "maxmemory", Label: "最大内存", Description: "0 表示不限制", Unit: "mb/gb"},
		{Key: "maxmemory-policy", Label: "内存淘汰策略", Description: "内存满时的 key 淘汰策略", Options: []string{"noeviction", "allkeys-lru", "volatile-lru", "allkeys-random", "volatile-random", "volatile-ttl"}},
		{Key: "appendonly", Label: "AOF 持久化", Description: "是否启用 AOF 持久化", Options: []string{"yes", "no"}},
		{Key: "appendfsync", Label: "AOF 同步策略", Description: "AOF 文件同步策略", Options: []string{"always", "everysec", "no"}},
		{Key: "timeout", Label: "空闲超时", Description: "客户端空闲断开时间（秒），0 表示不断开", Unit: "秒"},
		{Key: "databases", Label: "数据库数量", Description: "Redis 数据库数量"},
		{Key: "loglevel", Label: "日志级别", Description: "Redis 日志级别", Options: []string{"debug", "verbose", "notice", "warning"}},
	}
}

// configParamKeys 返回某类型可编辑参数的 key 列表（读配置时只取这些项）。
func configParamKeys(dbType DBType) []string {
	metas := configParams(dbType)
	keys := make([]string, 0, len(metas))
	for _, m := range metas {
		keys = append(keys, m.Key)
	}
	return keys
}

func stringSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

// --- 驱动读写 ---

// readConfigValues 用驱动读面板参数的当前值（运行时值，数据库是权威）。port 不在
// 其中 —— 端口由容器映射管理，见 GetInstanceConfig。
func (s *Service) readConfigValues(ctx context.Context, v *DBInstance) (map[string]string, error) {
	switch v.DBType {
	case DBTypeMySQL:
		return s.readMySQLConfig(ctx, v)
	case DBTypePostgreSQL:
		return s.readPostgresConfig(ctx, v)
	default:
		return s.readRedisConfig(ctx, v)
	}
}

func (s *Service) readMySQLConfig(ctx context.Context, v *DBInstance) (map[string]string, error) {
	res, err := s.driver.Query(ctx, v, systemDBName(v.DBType), "SHOW VARIABLES")
	if err != nil {
		return nil, err
	}
	wanted := stringSet(configParamKeys(v.DBType))
	params := make(map[string]string)
	for _, row := range res.Rows {
		if len(row) >= 2 {
			if name := str(row, 0); wanted[name] {
				params[name] = str(row, 1)
			}
		}
	}
	return params, nil
}

func (s *Service) readPostgresConfig(ctx context.Context, v *DBInstance) (map[string]string, error) {
	res, err := s.driver.Query(ctx, v, systemDBName(v.DBType),
		"SELECT name, setting FROM pg_settings WHERE name = ANY($1)", configParamKeys(v.DBType))
	if err != nil {
		return nil, err
	}
	params := make(map[string]string)
	for _, row := range res.Rows {
		if len(row) >= 2 {
			params[str(row, 0)] = str(row, 1)
		}
	}
	return params, nil
}

func (s *Service) readRedisConfig(ctx context.Context, v *DBInstance) (map[string]string, error) {
	raw, err := s.redisFor().ConfigGetAll(ctx, v)
	if err != nil {
		return nil, err
	}
	wanted := stringSet(configParamKeys(v.DBType))
	params := make(map[string]string)
	for k, val := range raw {
		if wanted[k] {
			params[k] = val
		}
	}
	return params, nil
}

// applyConfigValues 把本次覆盖值持久化到运行中的实例。返回 restart 表示修改涉及
// reload 不生效的参数（PG postmaster 级），调用方需重启容器；其余参数驱动已在线生效。
// 参数名来自 configParams 白名单（安全拼入语句）；空值已被上层过滤，不会到这里。
func (s *Service) applyConfigValues(ctx context.Context, v *DBInstance, params map[string]string) (restart bool, err error) {
	switch v.DBType {
	case DBTypeMySQL:
		return false, s.applyMySQLConfig(ctx, v, params)
	case DBTypePostgreSQL:
		return s.applyPostgresConfig(ctx, v, params)
	default:
		return false, s.applyRedisConfig(ctx, v, params)
	}
}

func (s *Service) applyMySQLConfig(ctx context.Context, v *DBInstance, params map[string]string) error {
	// SET PERSIST 是 MySQL 8.0+ 语法；先查版本给明确错误而非底层语法报错。
	res, err := s.driver.Query(ctx, v, systemDBName(v.DBType), "SELECT VERSION()")
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return fmt.Errorf("无法获取 MySQL 版本")
	}
	major, _ := strconv.Atoi(strings.Split(str(res.Rows[0], 0), ".")[0])
	if major < 8 {
		return fmt.Errorf("MySQL 8.0+ 才支持在线持久化配置（SET PERSIST）")
	}

	builder := NewSQLBuilder(DBTypeMySQL)
	sys := systemDBName(v.DBType)
	for key, value := range params {
		if _, err := s.driver.Exec(ctx, v, sys, "SET PERSIST "+builder.QuoteIdentifier(key)+" = '"+builder.EscapeString(value)+"'"); err != nil {
			return fmt.Errorf("设置参数 %s: %w", key, err)
		}
	}
	return nil
}

func (s *Service) applyPostgresConfig(ctx context.Context, v *DBInstance, params map[string]string) (bool, error) {
	builder := NewSQLBuilder(DBTypePostgreSQL)
	sys := systemDBName(v.DBType)
	for key, value := range params {
		if _, err := s.driver.Exec(ctx, v, sys, "ALTER SYSTEM SET "+builder.QuoteIdentifier(key)+" = '"+strings.ReplaceAll(value, "'", "''")+"'"); err != nil {
			return false, fmt.Errorf("设置参数 %s: %w", key, err)
		}
	}
	if _, err := s.driver.Exec(ctx, v, sys, "SELECT pg_reload_conf()"); err != nil {
		return false, fmt.Errorf("重载配置: %w", err)
	}

	// postmaster 级参数 reload 不生效，需要重启容器 —— 从 pg_settings 判断本次
	// 修改涉及哪些参数级别。
	names := make([]string, 0, len(params))
	for key := range params {
		names = append(names, key)
	}
	res, err := s.driver.Query(ctx, v, sys, "SELECT name FROM pg_settings WHERE name = ANY($1) AND context = 'postmaster'", names)
	if err != nil {
		return false, err
	}
	return len(res.Rows) > 0, nil
}

func (s *Service) applyRedisConfig(ctx context.Context, v *DBInstance, params map[string]string) error {
	for key, value := range params {
		if err := s.redisFor().ConfigSet(ctx, v, key, value); err != nil {
			return fmt.Errorf("设置参数 %s: %w", key, err)
		}
	}
	if len(params) > 0 {
		if err := s.redisFor().ConfigRewrite(ctx, v); err != nil {
			return fmt.Errorf("写回配置文件: %w", err)
		}
	}
	return nil
}
