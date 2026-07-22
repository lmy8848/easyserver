package cron

import "context"

// Repository defines the interface for cron task data access
type Repository interface {
	// Task CRUD
	ListTasks(ctx context.Context) ([]CronTask, error)
	GetTask(ctx context.Context, id int64) (*CronTask, error)
	CreateTask(ctx context.Context, task *CronTask) error
	UpdateTask(ctx context.Context, task *CronTask) error
	DeleteTask(ctx context.Context, id int64) error

	// Runtime version validation — Service.Create refuses to bind cron tasks
	// to a runtime_version row that isn't 'installed', mirroring process.
	GetRuntimeVersionStatus(ctx context.Context, runtimeVersionID int64) (string, error)

	// GetRuntime 返回 runtime_version 行的 lang/exact/status。
	// 供 systemd 包的 ServiceManager.fillRuntime 调用，补全托管 unit 的 mise 包裹参数。
	GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error)

	// Task status management
	ListEnabledTasks(ctx context.Context) ([]CronTask, error)
	EnableTask(ctx context.Context, id int64) error
	DisableTask(ctx context.Context, id int64) error
	SetTaskRunning(ctx context.Context, id int64) (bool, error)
	UpdateTaskResult(ctx context.Context, id int64, status string, lastResult string) error

	// Logs
	CreateLog(ctx context.Context, taskID int64, status string, output string, duration int) error
	GetLogs(ctx context.Context, taskID int64, limit int) ([]CronLog, error)

	// Scripts
	ListScripts(ctx context.Context) ([]Script, error)
	GetScript(ctx context.Context, id int64) (*Script, error)
	CreateScript(ctx context.Context, script *Script) error
	UpdateScript(ctx context.Context, script *Script) error
	DeleteScript(ctx context.Context, id int64) error

	// Documentation
	ListDocs(ctx context.Context) ([]CronDoc, error)
	GetDoc(ctx context.Context, id int64) (*CronDoc, error)
	CreateDoc(ctx context.Context, doc *CronDoc) error
	UpdateDoc(ctx context.Context, doc *CronDoc) error
	DeleteDoc(ctx context.Context, id int64) error
	CountDocs(ctx context.Context) (int, error)
	BatchCreateDocs(ctx context.Context, docs []CronDoc) error
}
