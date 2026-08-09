package dbserver

// DBServer is the API representation of a database engine catalog entry.
type DBServer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`         // mysql, postgresql, redis
	DisplayName string `json:"display_name"` // MySQL, PostgreSQL, Redis
	Description string `json:"description"`
	DefaultPort int    `json:"default_port"`
	Status      string `json:"status"`  // not_installed, running, stopped, partial
	Version     string `json:"version"` // summary of installed versions
	CreatedAt   string `json:"created_at"`
}

// DBVersion is retained as a wire name for a container-backed Database Instance.
type DBVersion struct {
	ID                 int64  `json:"id"`
	DBServerID         int64  `json:"db_server_id"`
	Version            string `json:"version"`      // 5.7, 8.0, 13, 15, etc.
	ServiceName        string `json:"service_name"` // managed container name (legacy key)
	ConfigFile         string `json:"config_file"`
	DataDir            string `json:"data_dir"`
	Port               int    `json:"port"`
	Status             string `json:"status"` // running, stopped
	CreatedAt          string `json:"created_at"`
	Runtime            string `json:"runtime"`
	Image              string `json:"image"`
	ContainerID        string `json:"container_id"`
	VolumeName         string `json:"volume_name"`
	BindAddress        string `json:"bind_address"`
	AdminUser          string `json:"admin_user"`
	AdminPassword      string `json:"-"`
	AdminPasswordPlain string `json:"admin_password,omitempty"`
	HealthStatus       string `json:"health_status"`
}

// CreateDBVersionRequest is the request for installing a new database version
type CreateDBVersionRequest struct {
	Version     string `json:"version" binding:"required"`
	Port        int    `json:"port"`
	Runtime     string `json:"runtime"`
	BindAddress string `json:"bind_address"`
}

// VersionTemplate describes a package template for a database version
type VersionTemplate struct {
	Version     string `json:"version"`
	Package     string `json:"package"`
	Image       string `json:"image"`
	Description string `json:"description"`
}

// GetVersionTemplates returns available version templates for a database type
func GetVersionTemplates(dbName string) []VersionTemplate {
	switch dbName {
	case "mysql":
		return []VersionTemplate{
			{Version: "8.0", Image: "mysql:8.0", Description: "MySQL 8.0"},
			{Version: "8.4", Image: "mysql:8.4", Description: "MySQL 8.4 LTS"},
		}
	case "postgresql":
		return []VersionTemplate{
			{Version: "15", Image: "postgres:15", Description: "PostgreSQL 15"},
			{Version: "16", Image: "postgres:16", Description: "PostgreSQL 16"},
		}
	case "redis":
		return []VersionTemplate{
			{Version: "7", Image: "redis:7-alpine", Description: "Redis 7"},
			{Version: "6", Image: "redis:6-alpine", Description: "Redis 6"},
		}
	}
	return nil
}

// PredefinedDBServers returns the default database entries
func PredefinedDBServers() []DBServer {
	return []DBServer{
		{
			Name:        "mysql",
			DisplayName: "MySQL",
			Description: "最流行的关系型数据库，广泛用于 Web 应用",
			DefaultPort: 3306,
		},
		{
			Name:        "postgresql",
			DisplayName: "PostgreSQL",
			Description: "功能强大的开源关系型数据库",
			DefaultPort: 5432,
		},
		{
			Name:        "redis",
			DisplayName: "Redis",
			Description: "高性能内存数据库，用于缓存和消息队列",
			DefaultPort: 6379,
		},
	}
}
