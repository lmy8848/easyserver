package firewall

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
)

// sqlitePortACLRepo implements PortACLRepository for SQLite.
type sqlitePortACLRepo struct {
	db *sql.DB
}

// NewPortACLSQLiteRepository creates a PortACLRepository backed by SQLite.
func NewPortACLSQLiteRepository(db *sql.DB) PortACLRepository {
	return &sqlitePortACLRepo{db: db}
}

const aclSelectColumns = "id, port, protocol, cidrs, remark, created_at"

func scanACL(scan func(dest ...any) error) (*PortACL, error) {
	var acl PortACL
	var cidrs string
	var remark sql.NullString
	if err := scan(&acl.ID, &acl.Port, &acl.Protocol, &cidrs, &remark, &acl.CreatedAt); err != nil {
		return nil, err
	}
	acl.Remark = remark.String
	if cidrs != "" {
		acl.CIDRs = strings.Split(cidrs, ",")
	}
	return &acl, nil
}

// List 返回全部白名单条目。
func (r *sqlitePortACLRepo) List(ctx context.Context) ([]PortACL, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+aclSelectColumns+" FROM port_acl ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PortACL
	for rows.Next() {
		acl, err := scanACL(rows.Scan)
		if err != nil {
			log.Printf("firewall: scan port_acl row error: %v", err)
			continue
		}
		out = append(out, *acl)
	}
	return out, rows.Err()
}

// Get 按端口+协议查条目（不存在返回 nil）。
func (r *sqlitePortACLRepo) Get(ctx context.Context, port int, protocol string) (*PortACL, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+aclSelectColumns+" FROM port_acl WHERE port = ? AND protocol = ?", port, protocol)
	acl, err := scanACL(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return acl, nil
}

// Upsert 插入或按 (port, protocol) 覆盖（含 ID 回填，供内核规则标签使用）。
func (r *sqlitePortACLRepo) Upsert(ctx context.Context, acl *PortACL) error {
	if acl.ID == 0 {
		if existing, err := r.Get(ctx, acl.Port, acl.Protocol); err == nil && existing != nil {
			acl.ID = existing.ID
		}
	}
	if acl.ID == 0 {
		res, err := r.db.ExecContext(ctx,
			"INSERT INTO port_acl (port, protocol, cidrs, remark) VALUES (?, ?, ?, ?)",
			acl.Port, acl.Protocol, strings.Join(acl.CIDRs, ","), acl.Remark)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		acl.ID = id
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		"UPDATE port_acl SET cidrs = ?, remark = ? WHERE id = ?",
		strings.Join(acl.CIDRs, ","), acl.Remark, acl.ID)
	return err
}

// Delete 删除指定端口+协议的条目。
func (r *sqlitePortACLRepo) Delete(ctx context.Context, port int, protocol string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM port_acl WHERE port = ? AND protocol = ?", port, protocol)
	return err
}
