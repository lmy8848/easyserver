package cron

import "context"

// Repository 定义脚本/文档的访问接口。
// 定时任务不复存在 DB（timer unit 是唯一权威，见 ADR-0004），由 TimerManager
// 直接读 systemd；因此任务 CRUD 不在此接口。
type Repository interface {
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
}
