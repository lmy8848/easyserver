package cron

// CronTask 表示一条定时任务，承载为 systemd 的一对 .timer + .service（ADR-0004）。
// 任务以 Name（unit 名，不含前缀）为唯一标识，无 DB 记录；状态读 systemctl，
// 日志走 journald。Schedule 是 OnCalendar 表达式（前端预设频率或手写均转为它）。
type CronTask struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Schedule         string `json:"schedule"` // OnCalendar 表达式
	Persistent       bool   `json:"persistent"`
	Enabled          bool   `json:"enabled"`
	Status           string `json:"status"` // active / inactive / failed
	LastRun          string `json:"last_run"`
	LastResult       string `json:"last_result"`
	NextRun          string `json:"next_run"`
	ScriptID         int    `json:"script_id"` // 0 = 无脚本
	Command          string `json:"command"`   // 用户命令或脚本路径
	Timeout          int    `json:"timeout"`   // 秒，0 = 不超时
	MaxRetry         int    `json:"max_retry"` // 0 = 不重试
	EnvVars          string `json:"env_vars"`  // "KEY=VALUE\n..." 每行一个
	WorkDir          string `json:"work_dir"`
	RuntimeVersionID int64  `json:"runtime_version_id"`
	RuntimeLang      string `json:"runtime_lang"`
	RuntimeExact     string `json:"runtime_exact"`
}

// CreateCronTaskRequest 是创建定时任务的请求体。Schedule 为 OnCalendar 表达式
// （前端预设频率或手写均转为它），后端只负责解析校验。
type CreateCronTaskRequest struct {
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	Schedule         string `json:"schedule" binding:"required"` // OnCalendar 表达式
	Persistent       bool   `json:"persistent"`
	ScriptID         int    `json:"script_id"`
	Command          string `json:"command"`
	Timeout          int    `json:"timeout"`
	MaxRetry         int    `json:"max_retry"`
	EnvVars          string `json:"env_vars"`
	WorkDir          string `json:"work_dir"`
	RuntimeVersionID int64  `json:"runtime_version_id" binding:"min=1"`
}

// UpdateCronTaskRequest 是更新定时任务的请求体（指针字段 = 部分更新）。
type UpdateCronTaskRequest struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Schedule         *string `json:"schedule"` // OnCalendar 表达式（可选）
	Persistent       *bool   `json:"persistent"`
	Enabled          *bool   `json:"enabled"`
	ScriptID         *int    `json:"script_id"`
	Command          *string `json:"command"`
	Timeout          *int    `json:"timeout"`
	MaxRetry         *int    `json:"max_retry"`
	EnvVars          *string `json:"env_vars"`
	WorkDir          *string `json:"work_dir"`
	RuntimeVersionID *int64  `json:"runtime_version_id"`
}

// Script 表示可被 Cron Task 引用的可复用脚本。内容落盘 /opt/easyserver/scripts/，
// DB 仅存元数据（name/description/language）。Content 仅由 GetService 从文件填充
// （List 不加载全部文件内容）。
type Script struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"` // sh, bash, python
	Content     string `json:"content,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateScriptRequest 是创建脚本的请求体。
type CreateScriptRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Content     string `json:"content" binding:"required"`
	Language    string `json:"language"`
}

// UpdateScriptRequest 是更新脚本的请求体。
type UpdateScriptRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Content     *string `json:"content"`
	Language    *string `json:"language"`
}
