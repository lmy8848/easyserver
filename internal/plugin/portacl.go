package plugin

import "context"

// PortACLInfo 是暴露端口访问控制的只读视图（Host API port/list 返回）。
type PortACLInfo struct {
	Port     int      `json:"port"`
	Protocol string   `json:"protocol"`
	CIDRs    []string `json:"cidrs"`
	Remark   string   `json:"remark"`
}

// PortExposer 是面板提供的暴露端口访问控制能力，由 router 注入防火墙
// PortACLService 的适配实现。插件声明 firewall.manage 权限后经 Host API
// port.expose / port.revoke / port.list 使用，确保暴露端口只对授权源开放。
type PortExposer interface {
	Expose(ctx context.Context, port int, protocol string, cidrs []string, remark string) error
	Revoke(ctx context.Context, port int, protocol string) error
	List(ctx context.Context) ([]PortACLInfo, error)
}
