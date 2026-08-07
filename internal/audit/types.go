package audit

import "easyserver/internal/httpx/middleware"

// ResourceCategory 定义了审计日志中的资源分类
type ResourceCategory = middleware.ResourceCategory

const (
	ResourceDatabase     = middleware.ResourceDatabase
	ResourceRuntime      = middleware.ResourceRuntime
	ResourcePackage      = middleware.ResourcePackage
	ResourceContainer    = middleware.ResourceContainer
	ResourceCloud        = middleware.ResourceCloud
	ResourceCron         = middleware.ResourceCron
	ResourceFirewall     = middleware.ResourceFirewall
	ResourceSSH          = middleware.ResourceSSH
	ResourceTerminal     = middleware.ResourceTerminal
	ResourceDaemon       = middleware.ResourceDaemon
	ResourceFile         = middleware.ResourceFile
	ResourceWebsite      = middleware.ResourceWebsite
	ResourceWebServer    = middleware.ResourceWebServer
	ResourceDeploy       = middleware.ResourceDeploy
	ResourceSetting      = middleware.ResourceSetting
	ResourceEnvVar       = middleware.ResourceEnvVar
	ResourceNotification = middleware.ResourceNotification
	ResourceAudit        = middleware.ResourceAudit
	ResourceSystem       = middleware.ResourceSystem
	ResourceAuth         = middleware.ResourceAuth
	ResourceOther        = middleware.ResourceOther
)

// ActionCategory 定义了审计日志中的动作分类
type ActionCategory = middleware.ActionCategory

const (
	ActionCreate  = middleware.ActionCreate
	ActionDelete  = middleware.ActionDelete
	ActionUpdate  = middleware.ActionUpdate
	ActionExecute = middleware.ActionExecute
	ActionAuth    = middleware.ActionAuth
	ActionOther   = middleware.ActionOther
)
