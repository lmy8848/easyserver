package firewall

import (
	"context"
	"database/sql"
	"log"
)

// sqlitePortForwardRepo implements PortForwardRepository for SQLite.
type sqlitePortForwardRepo struct {
	db *sql.DB
}

// NewPortForwardSQLiteRepository creates a PortForwardRepository backed by SQLite.
func NewPortForwardSQLiteRepository(db *sql.DB) PortForwardRepository {
	return &sqlitePortForwardRepo{db: db}
}

const pfSelectColumns = "id, protocol, listen_port, dest_ip, dest_port, source_cidr, interface, enabled, remark, created_at, updated_at"

// List returns all port-forward rules ordered by id.
func (r *sqlitePortForwardRepo) List(ctx context.Context) ([]PortForwardRule, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+pfSelectColumns+" FROM port_forwards ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []PortForwardRule
	for rows.Next() {
		var rule PortForwardRule
		var enabled int
		var destPort, sourceCIDR, iface, remark sql.NullString
		if err := rows.Scan(&rule.ID, &rule.Protocol, &rule.ListenPort, &rule.DestIP, &destPort, &sourceCIDR, &iface, &enabled, &remark, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			log.Printf("firewall: scan port-forward row error: %v", err)
			continue
		}
		rule.Enabled = enabled != 0
		rule.DestPort = destPort.String
		rule.SourceCIDR = sourceCIDR.String
		rule.Interface = iface.String
		rule.Remark = remark.String
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// Get returns one port-forward rule; nil when absent.
func (r *sqlitePortForwardRepo) Get(ctx context.Context, id int64) (*PortForwardRule, error) {
	var rule PortForwardRule
	var enabled int
	var destPort, sourceCIDR, iface, remark sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT "+pfSelectColumns+" FROM port_forwards WHERE id = ?", id).
		Scan(&rule.ID, &rule.Protocol, &rule.ListenPort, &rule.DestIP, &destPort, &sourceCIDR, &iface, &enabled, &remark, &rule.CreatedAt, &rule.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rule.Enabled = enabled != 0
	rule.DestPort = destPort.String
	rule.SourceCIDR = sourceCIDR.String
	rule.Interface = iface.String
	rule.Remark = remark.String
	return &rule, nil
}

// Create inserts a rule and sets its generated ID.
func (r *sqlitePortForwardRepo) Create(ctx context.Context, rule *PortForwardRule) error {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO port_forwards (protocol, listen_port, dest_ip, dest_port, source_cidr, interface, enabled, remark) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		rule.Protocol, rule.ListenPort, rule.DestIP, rule.DestPort, rule.SourceCIDR, rule.Interface, boolToInt(rule.Enabled), rule.Remark)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rule.ID = id
	return nil
}

// Update rewrites an existing rule.
func (r *sqlitePortForwardRepo) Update(ctx context.Context, rule *PortForwardRule) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE port_forwards SET protocol = ?, listen_port = ?, dest_ip = ?, dest_port = ?, source_cidr = ?, interface = ?, enabled = ?, remark = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		rule.Protocol, rule.ListenPort, rule.DestIP, rule.DestPort, rule.SourceCIDR, rule.Interface, boolToInt(rule.Enabled), rule.Remark, rule.ID)
	return err
}

// Delete removes a rule.
func (r *sqlitePortForwardRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM port_forwards WHERE id = ?", id)
	return err
}

// SetEnabled toggles a rule's enabled flag.
func (r *sqlitePortForwardRepo) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, "UPDATE port_forwards SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", boolToInt(enabled), id)
	return err
}

// ListEnabled returns all enabled rules.
func (r *sqlitePortForwardRepo) ListEnabled(ctx context.Context) ([]PortForwardRule, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+pfSelectColumns+" FROM port_forwards WHERE enabled = 1 ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []PortForwardRule
	for rows.Next() {
		var rule PortForwardRule
		var enabled int
		var destPort, sourceCIDR, iface, remark sql.NullString
		if err := rows.Scan(&rule.ID, &rule.Protocol, &rule.ListenPort, &rule.DestIP, &destPort, &sourceCIDR, &iface, &enabled, &remark, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			log.Printf("firewall: scan enabled port-forward row error: %v", err)
			continue
		}
		rule.Enabled = enabled != 0
		rule.DestPort = destPort.String
		rule.SourceCIDR = sourceCIDR.String
		rule.Interface = iface.String
		rule.Remark = remark.String
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
