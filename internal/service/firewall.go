package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/firewall"
)

// FirewallService is kept as a compatibility alias for the firewall module.
type FirewallService = firewall.Service

func NewFirewallService(repo firewall.Repository, exec executor.CommandExecutor) *FirewallService {
	return firewall.NewService(repo, exec)
}
