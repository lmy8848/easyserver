package runtimeenv

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrRuntimeNotFound 表示运行环境不存在
	ErrRuntimeNotFound = errx.NewSentinel(errx.KindNotFound, 40498, "运行环境不存在")

	// ErrPackageNotFound 表示软件包不存在
	ErrPackageNotFound = errx.NewSentinel(errx.KindNotFound, 40499, "软件包不存在")
)
