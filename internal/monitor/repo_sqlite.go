package monitor

import (
	"context"
	"database/sql"

	"easyserver/internal/infra/database"
	"time"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db database.DB
}

// NewSQLiteRepository creates a new SQLite-backed monitor Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: database.Wrap(db)}
}

// EnsureIndexes creates necessary indexes and columns for monitor data
func (r *sqliteRepo) EnsureIndexes(ctx context.Context) error {
	_, _ = r.db.ExecContext(ctx, "ALTER TABLE monitor_data ADD COLUMN disk_read_bytes INTEGER DEFAULT 0")
	_, _ = r.db.ExecContext(ctx, "ALTER TABLE monitor_data ADD COLUMN disk_write_bytes INTEGER DEFAULT 0")
	_, err := r.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_monitor_data_timestamp ON monitor_data(timestamp)")
	return err
}

// Save saves a single monitor point
func (r *sqliteRepo) Save(ctx context.Context, point *MonitorPoint) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO monitor_data
		(cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		 mem_total, mem_used, mem_usage,
		 disk_total, disk_used, disk_usage, disk_read_bytes, disk_write_bytes,
		 net_bytes_sent, net_bytes_recv, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		point.CPUPercent, point.CPULoad1m, point.CPULoad5m, point.CPULoad15m,
		point.MemTotal, point.MemUsed, point.MemPercent,
		point.DiskTotal, point.DiskUsed, point.DiskPercent, point.DiskReadBytes, point.DiskWriteBytes,
		point.NetBytesSent, point.NetBytesRecv, point.Timestamp,
	)
	return err
}

// SaveBatch saves multiple monitor points in a transaction
func (r *sqliteRepo) SaveBatch(ctx context.Context, points []*MonitorPoint) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO monitor_data
		(cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		 mem_total, mem_used, mem_usage,
		 disk_total, disk_used, disk_usage, disk_read_bytes, disk_write_bytes,
		 net_bytes_sent, net_bytes_recv, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, point := range points {
		if _, err := stmt.ExecContext(ctx,
			point.CPUPercent, point.CPULoad1m, point.CPULoad5m, point.CPULoad15m,
			point.MemTotal, point.MemUsed, point.MemPercent,
			point.DiskTotal, point.DiskUsed, point.DiskPercent, point.DiskReadBytes, point.DiskWriteBytes,
			point.NetBytesSent, point.NetBytesRecv, point.Timestamp,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLatest returns the latest monitor point
func (r *sqliteRepo) GetLatest(ctx context.Context) (*MonitorPoint, error) {
	p := &MonitorPoint{}
	err := r.db.QueryRowContext(ctx,
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_usage,
		        disk_total, disk_used, disk_usage,
		        COALESCE(disk_read_bytes, 0), COALESCE(disk_write_bytes, 0),
		        net_bytes_sent, net_bytes_recv, timestamp
		 FROM monitor_data ORDER BY id DESC LIMIT 1`,
	).Scan(
		&p.CPUPercent, &p.CPULoad1m, &p.CPULoad5m, &p.CPULoad15m,
		&p.MemTotal, &p.MemUsed, &p.MemPercent,
		&p.DiskTotal, &p.DiskUsed, &p.DiskPercent,
		&p.DiskReadBytes, &p.DiskWriteBytes,
		&p.NetBytesSent, &p.NetBytesRecv, &p.Timestamp,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// GetHistory returns monitor points in the given time range.
func (r *sqliteRepo) GetHistory(ctx context.Context, start, end time.Time) ([]MonitorPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_usage,
		        disk_total, disk_used, disk_usage,
		        COALESCE(disk_read_bytes, 0), COALESCE(disk_write_bytes, 0),
		        net_bytes_sent, net_bytes_recv, timestamp
		 FROM monitor_data
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []MonitorPoint
	for rows.Next() {
		var p MonitorPoint
		if err := rows.Scan(
			&p.CPUPercent, &p.CPULoad1m, &p.CPULoad5m, &p.CPULoad15m,
			&p.MemTotal, &p.MemUsed, &p.MemPercent,
			&p.DiskTotal, &p.DiskUsed, &p.DiskPercent,
			&p.DiskReadBytes, &p.DiskWriteBytes,
			&p.NetBytesSent, &p.NetBytesRecv, &p.Timestamp,
		); err != nil {
			continue
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, nil
}

// Clean deletes monitor data older than the specified time
func (r *sqliteRepo) Clean(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM monitor_data WHERE timestamp < ?", before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
