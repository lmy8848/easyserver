package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func performHardCutoverBackup(tx *sql.Tx, db *sql.DB, migrationsDir string) error {
	log.Printf("migrate: performing hard cutover backup for version 6")

	// 1. Export processes and cron_tasks to JSON
	timestamp := time.Now().Format("20060102150405")
	backupDir := fmt.Sprintf("/var/lib/easyserver/migrations/%s-pre-mise", timestamp)

	// Create backup directory. Fallback to local ./migrations if permission denied
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Printf("migrate: failed to create %s, falling back to local migrations directory: %v", backupDir, err)
		backupDir = filepath.Join(migrationsDir, fmt.Sprintf("%s-pre-mise", timestamp))
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return fmt.Errorf("create fallback backup dir: %w", err)
		}
	}

	if err := backupTableToJSON(tx, "processes", filepath.Join(backupDir, "processes.json")); err != nil {
		return err
	}
	if err := backupTableToJSON(tx, "cron_tasks", filepath.Join(backupDir, "cron_tasks.json")); err != nil {
		return err
	}

	// 2. Write audit log
	auditBodyBytes, _ := json.Marshal(map[string]string{"backup_path": backupDir})
	auditBody := string(auditBodyBytes)
	if _, err := tx.Exec("INSERT INTO audit_logs (action, resource, detail) VALUES (?, ?, ?)", "runtime.migration", "system", auditBody); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	// 4. DELETE data (保留表结构后再 ALTER 加 FK)
	if _, err := tx.Exec("DELETE FROM processes"); err != nil {
		return fmt.Errorf("delete from processes: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM cron_tasks"); err != nil {
		return fmt.Errorf("delete from cron_tasks: %w", err)
	}

	return nil
}

func backupTableToJSON(tx *sql.Tx, tableName, filePath string) error {
	rows, err := tx.Query("SELECT * FROM " + tableName)
	if err != nil {
		return fmt.Errorf("query %s: %w", tableName, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create %s: %w", filePath, err)
	}
	defer file.Close()

	// Use a streaming JSON encoder
	if _, err := file.WriteString("[\n"); err != nil {
		return err
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("  ", "  ")

	first := true
	for rows.Next() {
		if !first {
			if _, err := file.WriteString(",\n"); err != nil {
				return err
			}
		}
		first = false

		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}

		if err := encoder.Encode(rowMap); err != nil {
			return fmt.Errorf("encode json row: %w", err)
		}
	}

	if _, err := file.WriteString("\n]\n"); err != nil {
		return err
	}

	return nil
}
