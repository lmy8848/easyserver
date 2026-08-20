package notification

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrChannelNotFound 表示通知通道不存在
	ErrChannelNotFound = errx.NewSentinel(errx.KindNotFound, 40425, "通知通道不存在")
)
