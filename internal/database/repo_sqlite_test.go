package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"easyserver/internal/infra/database"
	_ "modernc.org/sqlite"
)

func TestSQLiteRepositoryUsesDatabaseInstanceSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewSQLiteRepository(db)
	ctx := context.Background()
	id, err := repo.CreateInstance(ctx, &DBInstance{
		DBType:          DBTypeMySQL,
		Version:         "8.0",
		ContainerEngine: "docker",
		Image:           "mysql:8.0",
		ContainerName:   "easyserver-db-mysql-8",
		VolumeName:      "easyserver-db-mysql-8-data",
		BindAddress:     "127.0.0.1",
		Port:            3306,
		AdminPassword:   "ciphertext",
		Status:          "running",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	instance, err := repo.GetInstance(ctx, id)
	if err != nil || instance == nil || instance.ContainerEngine != "docker" || instance.DBType != DBTypeMySQL {
		t.Fatalf("get instance: %v %#v", err, instance)
	}
}
