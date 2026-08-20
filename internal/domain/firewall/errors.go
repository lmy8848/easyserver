package firewall

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrRuleNotFound 表示防火墙规则不存在
	ErrRuleNotFound = errx.NewSentinel(errx.KindNotFound, 40470, "防火墙规则不存在")

	// ErrFirewallDisabled 表示防火墙服务未启用或未运行
	ErrFirewallDisabled = errx.NewSentinel(errx.KindBadRequest, 40070, "防火墙服务未启用或未运行")
)
