package sqlite

import (
	"database/sql"

	"easyserver/internal/firewall"
)

// NewFirewallRepository creates a new FirewallRepository
// Deprecated: use firewall.NewSQLiteRepository directly
func NewFirewallRepository(db *sql.DB) firewall.Repository {
	return firewall.NewSQLiteRepository(db)
}
