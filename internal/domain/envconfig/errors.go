package envconfig

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrEnvConfigNotFound 表示环境变量配置不存在
	ErrEnvConfigNotFound = errx.NewSentinel(errx.KindNotFound, 40420, "环境变量配置不存在")
)
