package util

import (
	"context"
	"fmt"
	"net"
)

// GuardOutboundHost 阻断出站到云元数据地址（169.254.169.254 等
// link-local 网段）——面板侧的探活/webhook 是管理员可配的出站请求，
// DNS rebinding 或误配置不应能窃取云厂商临时凭据。
// 回环与内网网段不拦：面板运行在服务器本体上，探测本机应用是合法场景。
func GuardOutboundHost(ctx context.Context, host string) error {
	// 字面 IP 直接判；域名解析后逐 IP 判（覆盖 DNS rebinding 的首跳）
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else {
		resolved, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return fmt.Errorf("cannot resolve host %q: %w", host, err)
		}
		for _, s := range resolved {
			if ip := net.ParseIP(s); ip != nil {
				ips = append(ips, ip)
			}
		}
	}
	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("link-local address %s is not allowed (cloud metadata)", ip)
		}
	}
	return nil
}
