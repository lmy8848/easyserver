package filemanager

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrPathNotFound 表示文件或目录不存在
	ErrPathNotFound = errx.NewSentinel(errx.KindNotFound, 40460, "文件或目录不存在")

	// ErrPathForbidden 表示无权访问或操作受限目录
	ErrPathForbidden = errx.NewSentinel(errx.KindForbidden, 40360, "无权访问受限目录")

	// ErrCannotDeleteRoot 表示不能删除根数据目录或其上级目录
	ErrCannotDeleteRoot = errx.NewSentinel(errx.KindForbidden, 40361, "不能删除根数据目录或其上级目录")

	// ErrPathExists 表示目标文件或目录已存在
	ErrPathExists = errx.NewSentinel(errx.KindConflict, 40960, "目标文件或目录已存在")

	// ErrSameSourceAndDest 表示源路径与目标路径相同
	ErrSameSourceAndDest = errx.NewSentinel(errx.KindBadRequest, 40060, "源路径与目标路径相同")

	// ErrDirCopyNotSupported 表示不支持直接复制目录
	ErrDirCopyNotSupported = errx.NewSentinel(errx.KindBadRequest, 40061, "不支持复制目录")

	// ErrInvalidPermissionBits 表示权限位包含不允许的设置
	ErrInvalidPermissionBits = errx.NewSentinel(errx.KindBadRequest, 40062, "不允许设置特殊权限位（setuid/setgid/sticky）或全局可写权限")

	// ErrShareNotFound 表示分享不存在
	ErrShareNotFound = errx.NewSentinel(errx.KindNotFound, 40461, "分享不存在")

	// ErrShareExpired 表示分享已过期
	ErrShareExpired = errx.NewSentinel(errx.KindNotFound, 40462, "分享已过期")

	// ErrSharePasswordRequired 表示需要提取码
	ErrSharePasswordRequired = errx.NewSentinel(errx.KindUnauthorized, 40160, "需要提取码")

	// ErrSharePasswordIncorrect 表示提取码错误
	ErrSharePasswordIncorrect = errx.NewSentinel(errx.KindUnauthorized, 40161, "提取码错误")

	// ErrInvalidTicket 表示下载凭证无效
	ErrInvalidTicket = errx.NewSentinel(errx.KindUnauthorized, 40162, "凭证无效")

	// ErrTicketExpired 表示下载凭证已过期
	ErrTicketExpired = errx.NewSentinel(errx.KindUnauthorized, 40163, "凭证已过期")
)
