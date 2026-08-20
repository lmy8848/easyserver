package systemd

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrUnitNotFound 表示系统服务单元不存在
	ErrUnitNotFound = errx.NewSentinel(errx.KindNotFound, 40495, "系统服务单元不存在")

	// ErrInvalidUnitName 表示托管服务或定时任务名称不合法
	ErrInvalidUnitName = errx.NewSentinel(errx.KindBadRequest, 40095, "名称只能包含小写字母、数字、连字符，且不能以连字符开头/结尾（1-60字符）")
)
