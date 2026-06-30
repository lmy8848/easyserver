package audit

// ResourceCategory 定义了审计日志中的资源分类
type ResourceCategory string

const (
	ResourceDatabase     ResourceCategory = "数据库"
	ResourceRuntime      ResourceCategory = "运行环境"
	ResourcePackage      ResourceCategory = "软件包"
	ResourceContainer    ResourceCategory = "容器"
	ResourceCloud        ResourceCategory = "云服务器"
	ResourceCron         ResourceCategory = "定时任务"
	ResourceFirewall     ResourceCategory = "防火墙"
	ResourceSSH          ResourceCategory = "SSH"
	ResourceTerminal     ResourceCategory = "终端"
	ResourceDaemon       ResourceCategory = "守护进程"
	ResourceFile         ResourceCategory = "文件"
	ResourceWebsite      ResourceCategory = "网站"
	ResourceWebServer    ResourceCategory = "Web服务"
	ResourceDeploy       ResourceCategory = "发布"
	ResourceSetting      ResourceCategory = "面板设置"
	ResourceEnvVar       ResourceCategory = "环境变量"
	ResourceNotification ResourceCategory = "通知"
	ResourceAudit        ResourceCategory = "审计"
	ResourceSystem       ResourceCategory = "系统服务"
	ResourceAuth         ResourceCategory = "认证"
	ResourceOther        ResourceCategory = "其他"
)

// ActionCategory 定义了审计日志中的动作分类
type ActionCategory string

const (
	ActionCreate  ActionCategory = "创建"
	ActionDelete  ActionCategory = "删除"
	ActionUpdate  ActionCategory = "修改"
	ActionExecute ActionCategory = "执行"
	ActionAuth    ActionCategory = "认证"
	ActionOther   ActionCategory = "其他"
)
