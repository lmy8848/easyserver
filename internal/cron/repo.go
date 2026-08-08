package cron

import "context"

// Repository 定义脚本/文档/运行时元数据的访问接口。
// 定时任务不复存在 DB（timer unit 是唯一权威，见 ADR-0004），由 TimerManager
// 直接读 systemd；因此任务 CRUD 不在此接口。
type Repository interface {
	// Runtime version validation — Service.Create refuses to bind cron tasks
	// to a runtime_version row that isn't 'installed'.
	GetRuntimeVersionStatus(ctx context.Context, runtimeVersionID int64) (string, error)

	// GetRuntime 返回 runtime_version 行的 lang/exact/status。
	GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error)

	// Scripts（内容落盘 /opt/easyserver/scripts/，DB 仅存元数据）
	ListScripts(ctx context.Context) ([]Script, error)
	GetScript(ctx context.Context, id int64) (*Script, error)
	CreateScript(ctx context.Context, script *Script) error
	UpdateScript(ctx context.Context, script *Script) error
	DeleteScript(ctx context.Context, id int64) error

	// 脚本内容落盘文件的读写（元数据与文件分离的补齐）
	ReadScriptFile(id int64) (string, error)
	WriteScriptFile(id int64, content string) error
	DeleteScriptFile(id int64) error

	// Documentation
	ListDocs(ctx context.Context) ([]CronDoc, error)
	GetDoc(ctx context.Context, id int64) (*CronDoc, error)
	CreateDoc(ctx context.Context, doc *CronDoc) error
	UpdateDoc(ctx context.Context, doc *CronDoc) error
	DeleteDoc(ctx context.Context, id int64) error
	CountDocs(ctx context.Context) (int, error)
	BatchCreateDocs(ctx context.Context, docs []CronDoc) error
}
