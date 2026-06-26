package deploy

import "context"

// Repository defines the interface for deploy data access
type Repository interface {
	// Server CRUD
	ListServers(ctx context.Context) ([]Server, error)
	GetServer(ctx context.Context, id int64) (*Server, error)
	GetServerAuthData(ctx context.Context, id int64) (string, error)
	CreateServer(ctx context.Context, srv *Server) error
	UpdateServer(ctx context.Context, srv *Server) error
	DeleteServer(ctx context.Context, id int64) error
	UpdateServerStatus(ctx context.Context, id int64, status string, lastPing string) error
	CountServerTasks(ctx context.Context, serverID int64) (int, error)
	CountServerVersions(ctx context.Context, serverID int64) (int, error)

	// Task CRUD
	ListTasks(ctx context.Context) ([]Task, error)
	GetTask(ctx context.Context, id int64) (*Task, error)
	ServerExists(ctx context.Context, id int64) (bool, error)
	CreateTask(ctx context.Context, task *Task) error
	DeleteTask(ctx context.Context, id int64) error
	UpdateTaskStatus(ctx context.Context, id int64, status string, result string) error

	// Version CRUD
	ListVersions(ctx context.Context, serverID int64) ([]Version, error)
	GetVersion(ctx context.Context, id int64) (*Version, error)
	CreateVersion(ctx context.Context, ver *Version) error
}
