package sqlite

import (
	"context"
	"database/sql"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// MonitorRepository implements repository.MonitorRepository for SQLite
type MonitorRepository struct {
	db *sql.DB
}

// NewMonitorRepository creates a new MonitorRepository
func NewMonitorRepository(db *sql.DB) repository.MonitorRepository {
	return &MonitorRepository{db: db}
}

// Save saves a single monitor point
func (r *MonitorRepository) Save(ctx context.Context, point *model.MonitorPoint) error {
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
func (r *MonitorRepository) SaveBatch(ctx context.Context, points []*model.MonitorPoint) error {
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
func (r *MonitorRepository) GetLatest(ctx context.Context) (*model.MonitorPoint, error) {
	p := &model.MonitorPoint{}
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

// GetHistory returns monitor points in the given time range
func (r *MonitorRepository) GetHistory(ctx context.Context, start, end time.Time) ([]model.MonitorPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_available, mem_usage,
		        disk_total, disk_used, disk_free, disk_usage,
		        net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp
		 FROM monitor_data
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []model.MonitorPoint
	for rows.Next() {
		var p model.MonitorPoint
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
func (r *MonitorRepository) Clean(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM monitor_data WHERE timestamp < ?", before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
