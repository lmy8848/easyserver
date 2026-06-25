package model

// DeployServer represents a remote server for deployment
type DeployServer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	AuthType  string `json:"auth_type"` // password or key
	AuthData  string `json:"auth_data"` // password or key path (encrypted in DB)
	Status    string `json:"status"`    // online, offline, unknown
	LastPing  string `json:"last_ping"`
	CreatedAt string `json:"created_at"`
}

// DeployTask represents a deployment task
type DeployTask struct {
	ID         int64  `json:"id"`
	ServerID   int64  `json:"server_id"`
	ServerName string `json:"server_name"`
	Name       string `json:"name"`
	Type       string `json:"type"` // sync, command, rollback
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`
	Command    string `json:"command"`
	Status     string `json:"status"` // pending, running, success, failed
	Result     string `json:"result"`
	CreatedAt  string `json:"created_at"`
}

// DeployVersion represents a deployment version record
type DeployVersion struct {
	ID         int64  `json:"id"`
	ServerID   int64  `json:"server_id"`
	ServerName string `json:"server_name"`
	TaskID     int64  `json:"task_id"`
	Version    string `json:"version"`
	Files      string `json:"files"` // JSON array of changed files
	BackupPath string `json:"backup_path"`
	CreatedAt  string `json:"created_at"`
}
