package web

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrServerNotFound 表示 Web 服务器不存在
	ErrServerNotFound = errx.NewSentinel(errx.KindNotFound, 40480, "Web 服务器不存在")

	// ErrWebsiteNotFound 表示网站不存在
	ErrWebsiteNotFound = errx.NewSentinel(errx.KindNotFound, 40481, "网站不存在")

	// ErrDomainExists 表示域名已存在
	ErrDomainExists = errx.NewSentinel(errx.KindConflict, 40980, "域名已存在")

	// ErrInvalidDomain 表示无效的域名格式
	ErrInvalidDomain = errx.NewSentinel(errx.KindBadRequest, 40080, "无效的域名格式")

	// ErrCertMismatch 表示证书与私钥不匹配或格式错误
	ErrCertMismatch = errx.NewSentinel(errx.KindBadRequest, 40081, "证书与私钥不匹配或格式错误")
)
