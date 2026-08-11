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
		{Key: "port", Label: "监听端口", Type: "number", Description: "MySQL 服务监听端口。修改后保存即重建容器生效"},
		{Key: "max_connections", Label: "最大连接数", Type: "number", Description: "允许的最大并发连接数"},
		{Key: "max_allowed_packet", Label: "最大数据包", Type: "number", Unit: "MB", Description: "单个数据包最大大小"},
		{Key: "default_storage_engine", Label: "默认存储引擎", Type: "string", Description: "默认存储引擎", Options: []string{"InnoDB", "MyISAM", "MEMORY"}},
		{Key: "innodb_buffer_pool_size", Label: "InnoDB 缓冲池", Type: "number", Unit: "MB", Description: "InnoDB 缓冲池大小，生产建议内存的 70-80%"},
		{Key: "tmp_table_size", Label: "临时表大小", Type: "number", Unit: "MB", Description: "内存临时表最大大小"},
		{Key: "max_heap_table_size", Label: "堆表最大大小", Type: "number", Unit: "MB", Description: "用户内存表最大大小"},
		{Key: "sort_buffer_size", Label: "排序缓冲区", Type: "number", Unit: "KB", Description: "每个会话排序缓冲区"},
		{Key: "join_buffer_size", Label: "JOIN 缓冲区", Type: "number", Unit: "KB", Description: "每个会话 JOIN 缓冲区"},
		{Key: "read_buffer_size", Label: "读缓冲区", Type: "number", Unit: "KB", Description: "顺序扫描读缓冲区"},
		{Key: "wait_timeout", Label: "非交互超时", Type: "number", Unit: "秒", Description: "非交互连接空闲超时（秒）"},
		{Key: "interactive_timeout", Label: "交互超时", Type: "number", Unit: "秒", Description: "交互连接空闲超时（秒）"},
		{Key: "slow_query_log", Label: "慢查询日志", Type: "string", Description: "是否启用慢查询日志", Options: []string{"ON", "OFF"}},
		{Key: "long_query_time", Label: "慢查询阈值", Type: "number", Unit: "秒", Description: "超过此时间（秒）记入慢查询日志"},
	}
}

func postgresConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Type: "number", Description: "PostgreSQL 服务监听端口。修改后保存即重建容器生效"},
		{Key: "max_connections", Label: "最大连接数", Type: "number", Description: "允许的最大并发连接数"},
		// PG 内存参数按 string 输入：ALTER SYSTEM 接受带单位的引号串（'128MB'），
		// 无单位会被按 kB 解析（shared_buffers 为 8kB 块）——不能像 MySQL 那样裸
		// 字节转换，用户在输入框直接写"128MB"最不易错。
		{Key: "shared_buffers", Label: "共享缓冲区", Type: "string", Unit: "MB", Description: "共享缓冲区大小，生产建议内存的 25%"},
		{Key: "work_mem", Label: "工作内存", Type: "string", Unit: "KB", Description: "每个排序/哈希操作的内存"},
		{Key: "maintenance_work_mem", Label: "维护工作内存", Type: "string", Unit: "MB", Description: "VACUUM/CREATE INDEX 等维护操作内存"},
		{Key: "wal_level", Label: "WAL 级别", Type: "string", Options: []string{"minimal", "replica", "logical"}, Description: "Write-Ahead 日志级别"},
		{Key: "max_wal_size", Label: "最大 WAL 大小", Type: "string", Unit: "MB", Description: "自动检查点之间最大 WAL 大小"},
		{Key: "min_wal_size", Label: "最小 WAL 大小", Type: "string", Unit: "MB", Description: "WAL 回收的最小大小"},
		{Key: "log_destination", Label: "日志目标", Type: "string", Options: []string{"stderr", "csvlog", "syslog"}, Description: "日志输出目标"},
		{Key: "logging_collector", Label: "日志收集器", Type: "string", Options: []string{"on", "off"}, Description: "是否启用日志收集器"},
		{Key: "ssl", Label: "SSL", Type: "string", Options: []string{"on", "off"}, Description: "是否启用 SSL"},
	}
}

func redisConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Type: "number", Description: "Redis 服务监听端口。修改后保存即重建容器生效"},
		{Key: "protected-mode", Label: "保护模式", Type: "string", Options: []string{"yes", "no"}, Description: "无密码时禁止外部访问"},
		{Key: "maxmemory", Label: "最大内存", Type: "number", Unit: "MB", Description: "0 表示不限制"},
		{Key: "maxmemory-policy", Label: "内存淘汰策略", Type: "string", Options: []string{"noeviction", "allkeys-lru", "volatile-lru", "allkeys-random", "volatile-random", "volatile-ttl"}, Description: "内存满时的 key 淘汰策略"},
		{Key: "appendonly", Label: "AOF 持久化", Type: "string", Options: []string{"yes", "no"}, Description: "是否启用 AOF 持久化"},
		{Key: "appendfsync", Label: "AOF 同步策略", Type: "string", Options: []string{"always", "everysec", "no"}, Description: "AOF 文件同步策略"},
		{Key: "timeout", Label: "空闲超时", Type: "number", Unit: "秒", Description: "客户端空闲断开时间（秒），0 表示不断开"},
		{Key: "databases", Label: "数据库数量", Type: "number", Description: "Redis 数据库数量"},
		{Key: "loglevel", Label: "日志级别", Type: "string", Options: []string{"debug", "verbose", "notice", "warning"}, Description: "Redis 日志级别"},
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

// configParamTypes 返回 key → Type 的映射，供保存端按变量类型决定字面量拼法。
func configParamTypes(dbType DBType) map[string]string {
	metas := configParams(dbType)
	m := make(map[string]string, len(metas))
	for _, pm := range metas {
		m[pm.Key] = pm.Type
	}
	return m
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
	// 必须读 GLOBAL 作用域：SET PERSIST 写的是全局值，而 SHOW VARIABLES（会话作用域）
	// 在长连接上会返回会话建立时的旧值——保存后刷新"恢复原值"正是这个原因
	// （wait_timeout/sort_buffer_size 等会话级变量；max_connections 等纯全局变量
	// 不受影响，所以只有部分参数回退）。
	res, err := s.driver.Query(ctx, v, systemDBName(v.DBType), "SHOW GLOBAL VARIABLES")
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
	types := configParamTypes(DBTypeMySQL)
	for key, value := range params {
		// number 型变量必须裸字面量（可带 K/M/G 单位，如 innodb_buffer_pool_size=1G），
		// 带引号报 1232 Incorrect argument type；string 型才加引号转义。
		literal := "'" + builder.EscapeString(value) + "'"
		if types[key] == "number" {
			literal = value
		}
		if _, err := s.driver.Exec(ctx, v, sys, "SET PERSIST "+builder.QuoteIdentifier(key)+" = "+literal); err != nil {
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
