package database

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrInstanceNotFound 表示数据库实例不存在
	ErrInstanceNotFound = errx.NewSentinel(errx.KindNotFound, 40440, "数据库实例不存在")

	// ErrBackupNotFound 表示备份记录不存在
	ErrBackupNotFound = errx.NewSentinel(errx.KindNotFound, 40441, "备份不存在")

	// ErrBackupInProgress 表示备份任务进行中
	ErrBackupInProgress = errx.NewSentinel(errx.KindConflict, 40940, "备份进行中，请等待完成后再操作")

	// ErrRestoreInProgress 表示备份正在恢复中
	ErrRestoreInProgress = errx.NewSentinel(errx.KindConflict, 40941, "该备份正在恢复中，无法删除")

	// ErrBackupNotFinished 表示备份尚未完成
	ErrBackupNotFinished = errx.NewSentinel(errx.KindBadRequest, 40040, "备份不是已完成状态，无法恢复")

	// ErrRedisAOFEnabled 表示 Redis AOF 开启时不支持直接 RDB 恢复
	ErrRedisAOFEnabled = errx.NewSentinel(errx.KindBadRequest, 40041, "Redis 已开启 AOF（appendonly=yes），RDB 恢复会被忽略；请先在配置中关闭 AOF 再恢复")

	// ErrNotContainerManaged 表示数据库实例非容器受管
	ErrNotContainerManaged = errx.NewSentinel(errx.KindBadRequest, 40042, "该数据库实例非容器受管")

	// ErrInstallCancelled 表示安装已取消
	ErrInstallCancelled = errx.NewSentinel(errx.KindBadRequest, 40043, "安装已取消")

	// ErrUnsupportedDBType 表示不支持的数据库类型
	ErrUnsupportedDBType = errx.NewSentinel(errx.KindBadRequest, 40044, "不支持的数据库类型")

	// ErrInvalidDBName 表示无效的数据库名
	ErrInvalidDBName = errx.NewSentinel(errx.KindBadRequest, 40045, "无效的数据库名")

	// ErrInvalidTableName 表示无效的表名
	ErrInvalidTableName = errx.NewSentinel(errx.KindBadRequest, 40046, "无效的表名")

	// ErrInvalidContainerName 表示容器名格式无效
	ErrInvalidContainerName = errx.NewSentinel(errx.KindBadRequest, 40047, "容器名只能包含字母、数字以及 _ . -，且必须以字母或数字开头")
)
