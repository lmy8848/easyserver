package firewall

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// FirewallRepository implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewFirewallRepository creates a new FirewallRepository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// CountEnabledRules returns the count of enabled firewall rules
func (r *sqliteRepo) CountEnabledRules(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM firewall_rules WHERE enabled = 1").Scan(&count)
	return count, err
}

// ListRules returns all firewall rules ordered by priority
func (r *sqliteRepo) ListRules(ctx context.Context) ([]FirewallRule, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, chain, protocol, port, action, source, target, enabled, priority, ip_version, remark, created_at FROM firewall_rules ORDER BY priority ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []FirewallRule
	for rows.Next() {
		var rule FirewallRule
		var enabled int
		var port, source, target, ipVersion, remark sql.NullString
		if err := rows.Scan(&rule.ID, &rule.Chain, &rule.Protocol, &port, &rule.Action, &source, &target, &enabled, &rule.Priority, &ipVersion, &remark, &rule.CreatedAt); err != nil {
			log.Printf("firewall: scan rule row error: %v", err)
			continue
		}
		rule.Enabled = enabled != 0
		if port.Valid {
			rule.Port = port.String
		}
		if source.Valid {
			rule.Source = source.String
		}
		if target.Valid {
			rule.Target = target.String
		}
		if ipVersion.Valid && ipVersion.String != "" {
			rule.IPVersion = ipVersion.String
		} else {
			rule.IPVersion = "ipv4"
		}
		if remark.Valid {
			rule.Remark = remark.String
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating firewall rules: %w", err)
	}
	return rules, nil
}

// GetRule returns a firewall rule by ID
func (r *sqliteRepo) GetRule(ctx context.Context, id int64) (*FirewallRule, error) {
	var rule FirewallRule
	var enabled int
	var port, source, target, ipVersion, remark sql.NullString

	err := r.db.QueryRowContext(ctx,
		"SELECT id, chain, protocol, port, action, source, target, enabled, priority, ip_version, remark, created_at FROM firewall_rules WHERE id = ?", id,
	).Scan(&rule.ID, &rule.Chain, &rule.Protocol, &port, &rule.Action, &source, &target, &enabled, &rule.Priority, &ipVersion, &remark, &rule.CreatedAt)
	if err != nil {
		return nil, err
	}
	rule.Enabled = enabled != 0
	if port.Valid {
		rule.Port = port.String
	}
	if source.Valid {
		rule.Source = source.String
	}
	if target.Valid {
		rule.Target = target.String
	}
	if ipVersion.Valid && ipVersion.String != "" {
		rule.IPVersion = ipVersion.String
	} else {
		rule.IPVersion = "ipv4"
	}
	if remark.Valid {
		rule.Remark = remark.String
	}
	return &rule, nil
}

// CreateRule inserts a new firewall rule and sets the rule.ID
func (r *sqliteRepo) CreateRule(ctx context.Context, rule *FirewallRule) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO firewall_rules (chain, protocol, port, action, source, target, enabled, ip_version, remark)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Chain, rule.Protocol, rule.Port, rule.Action, rule.Source, rule.Target, boolToInt(rule.Enabled), rule.IPVersion, rule.Remark)
	if err != nil {
		return err
	}
	rule.ID, _ = result.LastInsertId()
	return nil
}

// UpdateRule updates an existing firewall rule's fields
func (r *sqliteRepo) UpdateRule(ctx context.Context, rule *FirewallRule) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firewall_rules SET chain=?, protocol=?, port=?, action=?, source=?, target=?, ip_version=?, remark=? WHERE id=?`,
		rule.Chain, rule.Protocol, rule.Port, rule.Action, rule.Source, rule.Target, rule.IPVersion, rule.Remark, rule.ID)
	return err
}

// DeleteRule deletes a firewall rule by ID
func (r *sqliteRepo) DeleteRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM firewall_rules WHERE id = ?", id)
	return err
}

// EnableRule marks a firewall rule as enabled
func (r *sqliteRepo) EnableRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE firewall_rules SET enabled=1 WHERE id=?", id)
	return err
}

// DisableRule marks a firewall rule as disabled
func (r *sqliteRepo) DisableRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE firewall_rules SET enabled=0 WHERE id=?", id)
	return err
}

// SwapPriorities swaps the priorities of two rules in a transaction
func (r *sqliteRepo) SwapPriorities(ctx context.Context, id1 int64, priority1 int64, id2 int64, priority2 int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", priority1, id1); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", priority2, id2); err != nil {
		return err
	}

	return tx.Commit()
}

// boolToInt converts a bool to int (0 or 1)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
