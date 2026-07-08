package monitor

import (
	"context"
	"database/sql"
	"time"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed monitor Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// EnsureIndexes creates necessary indexes for monitor data
func (r *sqliteRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_monitor_data_timestamp ON monitor_data(timestamp)")
	return err
}

// Save saves a single monitor point
func (r *sqliteRepo) Save(ctx context.Context, point *MonitorPoint) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO monitor_data
		(cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		 mem_total, mem_used, mem_available, mem_usage,
		 disk_total, disk_used, disk_free, disk_usage,
		 net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		point.CPUPercent, point.CPULoad1m, point.CPULoad5m, point.CPULoad15m,
		point.MemTotal, point.MemUsed, point.MemAvailable, point.MemPercent,
		point.DiskTotal, point.DiskUsed, point.DiskFree, point.DiskPercent,
		point.NetBytesSent, point.NetBytesRecv, point.NetPktsSent, point.NetPktsRecv, point.Timestamp,
	)
	return err
}

// SaveBatch saves multiple monitor points in a transaction
func (r *sqliteRepo) SaveBatch(ctx context.Context, points []*MonitorPoint) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO monitor_data
		(cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		 mem_total, mem_used, mem_available, mem_usage,
		 disk_total, disk_used, disk_free, disk_usage,
		 net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, point := range points {
		if _, err := stmt.ExecContext(ctx,
			point.CPUPercent, point.CPULoad1m, point.CPULoad5m, point.CPULoad15m,
			point.MemTotal, point.MemUsed, point.MemAvailable, point.MemPercent,
			point.DiskTotal, point.DiskUsed, point.DiskFree, point.DiskPercent,
			point.NetBytesSent, point.NetBytesRecv, point.NetPktsSent, point.NetPktsRecv, point.Timestamp,
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
		        mem_total, mem_used, mem_available, mem_usage,
		        disk_total, disk_used, disk_free, disk_usage,
		        net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp
		 FROM monitor_data ORDER BY id DESC LIMIT 1`,
	).Scan(
		&p.CPUPercent, &p.CPULoad1m, &p.CPULoad5m, &p.CPULoad15m,
		&p.MemTotal, &p.MemUsed, &p.MemAvailable, &p.MemPercent,
		&p.DiskTotal, &p.DiskUsed, &p.DiskFree, &p.DiskPercent,
		&p.NetBytesSent, &p.NetBytesRecv, &p.NetPktsSent, &p.NetPktsRecv, &p.Timestamp,
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
// Caps the SQL fetch at maxHistoryPoints*2 rows so a long-running server
// (86k+ rows in monitor_data) does NOT load the full table into memory —
// the original query without this cap was the cause of the slow post-login
// Dashboard render. The caller (service layer) downsamples the result to
// maxHistoryPoints with even stride.
func (r *sqliteRepo) GetHistory(ctx context.Context, start, end time.Time) ([]MonitorPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_available, mem_usage,
		        disk_total, disk_used, disk_free, disk_usage,
		        net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp
		 FROM monitor_data
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC
		 LIMIT ?`,
		start.Format(time.RFC3339), end.Format(time.RFC3339), maxHistoryPoints*2,
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
			&p.MemTotal, &p.MemUsed, &p.MemAvailable, &p.MemPercent,
			&p.DiskTotal, &p.DiskUsed, &p.DiskFree, &p.DiskPercent,
			&p.NetBytesSent, &p.NetBytesRecv, &p.NetPktsSent, &p.NetPktsRecv, &p.Timestamp,
		); err != nil {
			continue
		}
		points = append(points, p)
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
