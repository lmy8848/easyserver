package firewall

import (
	"context"
)

// Repository defines the interface for firewall rule data access
type Repository interface {
	CountEnabledRules(ctx context.Context) (int, error)
	ListRules(ctx context.Context) ([]FirewallRule, error)
	GetRule(ctx context.Context, id int64) (*FirewallRule, error)
	CreateRule(ctx context.Context, rule *FirewallRule) error
	UpdateRule(ctx context.Context, rule *FirewallRule) error
	DeleteRule(ctx context.Context, id int64) error
	EnableRule(ctx context.Context, id int64) error
	DisableRule(ctx context.Context, id int64) error
	SwapPriorities(ctx context.Context, id1 int64, priority1 int64, id2 int64, priority2 int64) error
}
