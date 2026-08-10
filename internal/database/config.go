package database

import (
	"fmt"
	"sort"
	"strings"
)

// 结构化配置。面板是配置参数的唯一写入方：参数元数据与编译默认值定义在下方，
// 用户覆盖值以面板生成的配置文件形式存于容器配置卷（generateConfigFile 只写覆盖
// 项），文件是唯一持久化，无存储漂移。默认值取自各数据库编译默认值（researched
// 2026-08：MySQL 8.0+ / PostgreSQL 16 / Redis 7+ 官方文档）。

// configSectionName 是各类型结构化配置的段名：MySQL 的 [mysqld] 段（my.cnf 的
// !includedir conf.d 合并进来），PostgreSQL/Redis 是隐式主段。
func configSectionName(dbType DBType) string {
	if dbType == DBTypeMySQL {
		return "mysqld"
	}
	return "main"
}

// configParams 返回某类型可编辑的常用参数（编译默认值），供 GET 渲染与生成文件。
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
		{Key: "port", Label: "监听端口", Description: "MySQL 服务监听端口。修改后保存即重建容器生效", Type: "number", Default: "3306"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "151"},
		{Key: "max_allowed_packet", Label: "最大数据包", Description: "单个数据包最大大小", Type: "text", Unit: "MB", Default: "64M"},
		{Key: "default-storage-engine", Label: "默认存储引擎", Description: "默认存储引擎", Type: "select", Options: []string{"InnoDB", "MyISAM", "MEMORY"}, Default: "InnoDB"},
		{Key: "innodb_buffer_pool_size", Label: "InnoDB 缓冲池", Description: "InnoDB 缓冲池大小，生产建议内存的 70-80%", Type: "text", Unit: "MB/GB", Default: "128M"},
		{Key: "tmp_table_size", Label: "临时表大小", Description: "内存临时表最大大小", Type: "text", Unit: "MB", Default: "16M"},
		{Key: "max_heap_table_size", Label: "堆表最大大小", Description: "用户内存表最大大小", Type: "text", Unit: "MB", Default: "16M"},
		{Key: "sort_buffer_size", Label: "排序缓冲区", Description: "每个会话排序缓冲区", Type: "text", Unit: "KB", Default: "256K"},
		{Key: "join_buffer_size", Label: "JOIN 缓冲区", Description: "每个会话 JOIN 缓冲区", Type: "text", Unit: "KB", Default: "256K"},
		{Key: "read_buffer_size", Label: "读缓冲区", Description: "顺序扫描读缓冲区", Type: "text", Unit: "KB", Default: "128K"},
		{Key: "wait_timeout", Label: "非交互超时", Description: "非交互连接空闲超时（秒）", Type: "number", Unit: "秒", Default: "28800"},
		{Key: "interactive_timeout", Label: "交互超时", Description: "交互连接空闲超时（秒）", Type: "number", Unit: "秒", Default: "28800"},
		{Key: "slow_query_log", Label: "慢查询日志", Description: "是否启用慢查询日志", Type: "select", Options: []string{"ON", "OFF"}, Default: "OFF"},
		{Key: "long_query_time", Label: "慢查询阈值", Description: "超过此时间（秒）记入慢查询日志", Type: "number", Unit: "秒", Default: "10"},
	}
}

func postgresConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Description: "PostgreSQL 服务监听端口。修改后保存即重建容器生效", Type: "number", Default: "5432"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "100"},
		{Key: "shared_buffers", Label: "共享缓冲区", Description: "共享缓冲区大小，生产建议内存的 25%", Type: "text", Unit: "MB/GB", Default: "128MB"},
		{Key: "work_mem", Label: "工作内存", Description: "每个排序/哈希操作的内存", Type: "text", Unit: "MB/KB", Default: "4MB"},
		{Key: "maintenance_work_mem", Label: "维护工作内存", Description: "VACUUM/CREATE INDEX 等维护操作内存", Type: "text", Unit: "MB/GB", Default: "64MB"},
		{Key: "wal_level", Label: "WAL 级别", Description: "Write-Ahead 日志级别", Type: "select", Options: []string{"minimal", "replica", "logical"}, Default: "replica"},
		{Key: "max_wal_size", Label: "最大 WAL 大小", Description: "自动检查点之间最大 WAL 大小", Type: "text", Unit: "MB/GB", Default: "1GB"},
		{Key: "min_wal_size", Label: "最小 WAL 大小", Description: "WAL 回收的最小大小", Type: "text", Unit: "MB/GB", Default: "80MB"},
		{Key: "log_destination", Label: "日志目标", Description: "日志输出目标", Type: "select", Options: []string{"stderr", "csvlog", "syslog"}, Default: "stderr"},
		{Key: "logging_collector", Label: "日志收集器", Description: "是否启用日志收集器（编译默认 off）", Type: "select", Options: []string{"on", "off"}, Default: "off"},
		{Key: "ssl", Label: "SSL", Description: "是否启用 SSL", Type: "select", Options: []string{"on", "off"}, Default: "off"},
	}
}

func redisConfigParams() []ParamMeta {
	return []ParamMeta{
		{Key: "port", Label: "监听端口", Description: "Redis 服务监听端口。修改后保存即重建容器生效", Type: "number", Default: "6379"},
		{Key: "protected-mode", Label: "保护模式", Description: "无密码时禁止外部访问", Type: "select", Options: []string{"yes", "no"}, Default: "yes"},
		{Key: "maxmemory", Label: "最大内存", Description: "0 表示不限制", Type: "text", Unit: "mb/gb", Default: "0"},
		{Key: "maxmemory-policy", Label: "内存淘汰策略", Description: "内存满时的 key 淘汰策略", Type: "select", Options: []string{"noeviction", "allkeys-lru", "volatile-lru", "allkeys-random", "volatile-random", "volatile-ttl"}, Default: "noeviction"},
		{Key: "appendonly", Label: "AOF 持久化", Description: "是否启用 AOF 持久化", Type: "select", Options: []string{"yes", "no"}, Default: "no"},
		{Key: "appendfsync", Label: "AOF 同步策略", Description: "AOF 文件同步策略", Type: "select", Options: []string{"always", "everysec", "no"}, Default: "everysec"},
		{Key: "timeout", Label: "空闲超时", Description: "客户端空闲断开时间（秒），0 表示不断开", Type: "number", Unit: "秒", Default: "0"},
		{Key: "databases", Label: "数据库数量", Description: "Redis 数据库数量", Type: "number", Default: "16"},
		{Key: "loglevel", Label: "日志级别", Description: "Redis 日志级别", Type: "select", Options: []string{"debug", "verbose", "notice", "warning"}, Default: "notice"},
	}
}

// effectiveParams 把结构化配置合并进编译默认值：无覆盖项 → 全部默认。
func effectiveParams(dbType DBType, stored map[string]string) map[string]string {
	params := make(map[string]string)
	for _, meta := range configParams(dbType) {
		params[meta.Key] = meta.Default
	}
	for key, value := range stored {
		if value != "" {
			params[key] = value
		}
	}
	return params
}

// generateConfigFile 根据传入的参数生成数据库配置文件内容。只写出传入的项
// （覆盖值；不含 key=="port"，端口由容器映射管理）——文件是面板写入的覆盖项
// 持久化，未写出的项由服务端编译默认值兜底。
func generateConfigFile(dbType DBType, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "port" || params[key] == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("# EasyServer managed configuration — 修改后保存并重启实例生效。\n")
	switch dbType {
	case DBTypeMySQL:
		sb.WriteString("[mysqld]\n")
		for _, key := range keys {
			sb.WriteString(key + " = " + params[key] + "\n")
		}
	case DBTypePostgreSQL:
		for _, key := range keys {
			value := params[key]
			if pgNeedsQuote(value) {
				escaped := strings.ReplaceAll(value, "'", "''")
				value = "'" + escaped + "'"
			}
			sb.WriteString(key + " = " + value + "\n")
		}
	default: // redis
		for _, key := range keys {
			if key == "save" {
				for _, line := range strings.Split(params[key], "\n") {
					if l := strings.TrimSpace(line); l != "" {
						sb.WriteString("save " + l + "\n")
					}
				}
				continue
			}
			sb.WriteString(key + " " + params[key] + "\n")
		}
	}
	return sb.String()
}

// parseConfigFile 解析面板生成的配置文件，还原出写入的参数（覆盖项）。行内
// 值去引号（PG）、Redis 的 save 多行合并回 \n 分隔，与 generateConfigFile 互逆。
func parseConfigFile(dbType DBType, content string) map[string]string {
	params := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		switch dbType {
		case DBTypeMySQL, DBTypePostgreSQL:
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
			if value != "" {
				params[key] = value
			}
		default: // redis
			idx := strings.Index(line, " ")
			if idx == -1 {
				continue
			}
			key := line[:idx]
			value := strings.TrimSpace(line[idx+1:])
			if key == "save" {
				if existing, ok := params["save"]; ok {
					params["save"] = existing + "\n" + value
				} else {
					params["save"] = value
				}
				continue
			}
			if value != "" {
				params[key] = value
			}
		}
	}
	return params
}

// pgNeedsQuote 判断 postgresql.conf 中该值是否需加单引号（数字与 on/off 等
// 关键字不加，其余字符串加）。
func pgNeedsQuote(value string) bool {
	if _, err := fmt.Sscanf(value, "%d", new(int)); err == nil {
		return false
	}
	switch strings.ToLower(value) {
	case "on", "off", "true", "false", "yes", "no":
		return false
	}
	if len(value) > 0 {
		lastChar := value[len(value)-1]
		if lastChar >= 'A' && lastChar <= 'Z' || lastChar >= 'a' && lastChar <= 'z' {
			numPart := value[:len(value)-1]
			if _, err := fmt.Sscanf(numPart, "%f", new(float64)); err == nil {
				return false
			}
		}
	}
	return true
}
