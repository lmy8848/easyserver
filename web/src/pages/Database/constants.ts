// Default config templates per database type
const DEFAULT_CONFIG_TEMPLATES: Record<string, string> = {
  mysql: `# MySQL 默认配置模板
# 保存后将创建此文件

[mysqld]
port = 3306
datadir = /var/lib/mysql
socket = /var/run/mysqld/mysqld.sock
max_connections = 151
innodb_buffer_pool_size = 128M
character-set-server = utf8mb4
collation-server = utf8mb4_general_ci
default-storage-engine = InnoDB
max_allowed_packet = 64M
tmp_table_size = 64M
max_heap_table_size = 64M
sort_buffer_size = 256K
read_buffer_size = 256K
join_buffer_size = 256K
log_error = /var/log/mysql/error.log
slow_query_log = OFF
long_query_time = 10
wait_timeout = 28800
interactive_timeout = 28800

[client]
default-character-set = utf8mb4
port = 3306
socket = /var/run/mysqld/mysqld.sock

[mysql]
default-character-set = utf8mb4

[mysqldump]
max_allowed_packet = 64M
default-character-set = utf8mb4
`,
  postgresql: `# PostgreSQL 默认配置模板

listen_addresses = 'localhost'
port = 5432
max_connections = 100
shared_buffers = 128MB
work_mem = 4MB
maintenance_work_mem = 64MB
log_destination = 'stderr'
logging_collector = on
log_directory = 'pg_log'
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'
`,
  redis: `# Redis 默认配置模板

bind 127.0.0.1
port 6379
maxmemory 256mb
maxmemory-policy allkeys-lru
save 900 1
save 300 10
save 60 10000
`,
};

export default DEFAULT_CONFIG_TEMPLATES;
