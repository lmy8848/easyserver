package ssh

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrKeyNotFound 表示 SSH 密钥不存在
	ErrKeyNotFound = errx.NewSentinel(errx.KindNotFound, 40490, "SSH 密钥不存在")

	// ErrInvalidPublicKey 表示无效的公钥格式
	ErrInvalidPublicKey = errx.NewSentinel(errx.KindBadRequest, 40090, "无效的公钥格式")
)
