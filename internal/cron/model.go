package cron

// CronTask represents a scheduled cron job
type CronTask struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Command          string `json:"command"`
	Schedule         string `json:"schedule"`
	Description      string `json:"description"`
	Enabled          bool   `json:"enabled"`
	Status           string `json:"status"` // idle, running, success, failed
	LastRun          string `json:"last_run"`
	LastResult       string `json:"last_result"`
	NextRun          string `json:"next_run"`
	ScriptID         int    `json:"script_id"`          // 0 = no script
	Timeout          int    `json:"timeout"`            // seconds, 0 = no timeout
	MaxRetry         int    `json:"max_retry"`          // 0 = no retry
	EnvVars          string `json:"env_vars"`           // KEY=VALUE format, one per line
	WorkDir          string `json:"work_dir"`           // working directory
	RuntimeVersionID int64  `json:"runtime_version_id"` // FK → runtime_version.id; NOT NULL since Issue 02
	RuntimeLang      string `json:"runtime_lang"`       // joined: runtime_version.lang (read-only)
	RuntimeExact     string `json:"runtime_exact"`      // joined: runtime_version.exact (read-only)
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// CronLog represents a cron task execution log
type CronLog struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	Status    string `json:"status"` // success, failed
	Output    string `json:"output"`
	Duration  int    `json:"duration"` // milliseconds
	CreatedAt string `json:"created_at"`
}

// CreateCronTaskRequest is the request body for creating a cron task
type CreateCronTaskRequest struct {
	Name             string `json:"name" binding:"required"`
	Command          string `json:"command"`
	Schedule         string `json:"schedule" binding:"required"`
	Description      string `json:"description"`
	ScriptID         int    `json:"script_id"`
	Timeout          int    `json:"timeout"`
	MaxRetry         int    `json:"max_retry"`
	EnvVars          string `json:"env_vars"`
	WorkDir          string `json:"work_dir"`
	RuntimeVersionID int64  `json:"runtime_version_id" binding:"required,min=1"`
}

// UpdateCronTaskRequest is the request body for updating a cron task
type UpdateCronTaskRequest struct {
	Name             *string `json:"name"`
	Command          *string `json:"command"`
	Schedule         *string `json:"schedule"`
	Description      *string `json:"description"`
	ScriptID         *int    `json:"script_id"`
	Timeout          *int    `json:"timeout"`
	MaxRetry         *int    `json:"max_retry"`
	EnvVars          *string `json:"env_vars"`
	WorkDir          *string `json:"work_dir"`
	RuntimeVersionID *int64  `json:"runtime_version_id"`
}

// Script represents a reusable script
type Script struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Language    string `json:"language"` // sh, bash, python
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateScriptRequest is the request body for creating a script
type CreateScriptRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Content     string `json:"content" binding:"required"`
	Language    string `json:"language"`
}

// UpdateScriptRequest is the request body for updating a script
type UpdateScriptRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Content     *string `json:"content"`
	Language    *string `json:"language"`
}

// CronDoc represents a cron documentation section
type CronDoc struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
