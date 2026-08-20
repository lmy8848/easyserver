package cron

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrTaskNotFound 表示定时任务不存在
	ErrTaskNotFound = errx.NewSentinel(errx.KindNotFound, 40450, "定时任务不存在")

	// ErrScriptNotFound 表示脚本不存在
	ErrScriptNotFound = errx.NewSentinel(errx.KindNotFound, 40451, "脚本不存在")

	// ErrInvalidTaskName 表示任务名称格式无效
	ErrInvalidTaskName = errx.NewSentinel(errx.KindBadRequest, 40050, "无效的任务名称")

	// ErrInvalidWorkDir 表示工作目录必须是绝对路径
	ErrInvalidWorkDir = errx.NewSentinel(errx.KindBadRequest, 40051, "工作目录必须是绝对路径")
)
