package process

import "context"

// Repository defines the interface for process/process-group/process-log data access
type Repository interface {
	// Process CRUD
	ListProcesses(ctx context.Context) ([]Process, error)
	GetProcessByID(ctx context.Context, id int64) (*Process, error)
	CreateProcess(ctx context.Context, p *Process) (int64, error)
	UpdateProcess(ctx context.Context, id int64, req *UpdateProcessRequest) error
	DeleteProcess(ctx context.Context, id int64) error
	GetAutoStartIDs(ctx context.Context) ([]int64, error)

	// Runtime version validation — Service.Create refuses to bind processes
	// to a runtime_version row that isn't 'installed'.
	GetRuntimeVersionStatus(ctx context.Context, runtimeVersionID int64) (string, error)

	// Process status
	UpsertStatus(ctx context.Context, processID int64, status string, pid int, exitCode int, lastError string) error
	GetStatus(ctx context.Context, processID int64) (*ProcessStatus, error)
	IncrementRestarts(ctx context.Context, processID int64) error
	ClearExitInfo(ctx context.Context, processID int64) error

	// Process logs
	AppendLog(ctx context.Context, processID int64, logType, content string) error
	ListLogs(ctx context.Context, processID int64, limit, offset int) ([]ProcessLog, int, error)

	// Process groups
	ListGroups(ctx context.Context) ([]ProcessGroup, error)
	GetGroup(ctx context.Context, id int64) (*ProcessGroup, error)
	CreateGroup(ctx context.Context, name, description string) (int64, error)
	UpdateGroup(ctx context.Context, id int64, req *UpdateProcessGroupRequest) error
	DeleteGroup(ctx context.Context, id int64) error
}
