package container

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrManagedContainer 表示受管数据库容器不可直接通过通用容器模块操作
	ErrManagedContainer = errx.NewSentinel(errx.KindForbidden, 40320, "受管数据库容器，请通过数据库模块操作")

	// ErrPodmanOnly 表示特定操作仅在 Podman 引擎下支持
	ErrPodmanOnly = errx.NewSentinel(errx.KindBadRequest, 40030, "socket 操作仅支持 Podman")

	// ErrInvalidProjectDir 表示 Compose 项目目录路径无效
	ErrInvalidProjectDir = errx.NewSentinel(errx.KindBadRequest, 40031, "无效的项目目录路径")

	// ErrInvalidContainerName 表示容器名称格式或长度无效
	ErrInvalidContainerName = errx.NewSentinel(errx.KindBadRequest, 40032, "容器名只能包含字母、数字以及 _ . -，且必须以字母或数字开头（1-128字符）")

	// ErrNullByteInCommand 表示执行命令包含非法空字节
	ErrNullByteInCommand = errx.NewSentinel(errx.KindBadRequest, 40033, "命令包含非法空字节")

	// ErrContainerNotFound 表示容器不存在
	ErrContainerNotFound = errx.NewSentinel(errx.KindNotFound, 40430, "容器不存在")

	// ErrImageNotFound 表示镜像不存在
	ErrImageNotFound = errx.NewSentinel(errx.KindNotFound, 40431, "镜像不存在")

	// ErrVolumeNotFound 表示数据卷不存在
	ErrVolumeNotFound = errx.NewSentinel(errx.KindNotFound, 40432, "数据卷不存在")

	// ErrNetworkNotFound 表示网络不存在
	ErrNetworkNotFound = errx.NewSentinel(errx.KindNotFound, 40433, "网络不存在")
)
