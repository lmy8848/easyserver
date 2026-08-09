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
	if err := repo.SeedServer(ctx, "mysql", "MySQL", "", 3306); err != nil {
		t.Fatal(err)
	}
	engines, err := repo.ListServers(ctx)
	if err != nil || len(engines) != 1 {
		t.Fatalf("list engines: %v %#v", err, engines)
	}
	id, err := repo.CreateContainerVersion(ctx, &DBInstance{
		DBServerID:    engines[0].ID,
		Version:       "8.0",
		Runtime:       "docker",
		Image:         "mysql:8.0",
		ContainerName: "easyserver-db-mysql-8",
		ContainerID:   "container-id",
		VolumeName:    "easyserver-db-mysql-8-data",
		BindAddress:   "127.0.0.1",
		Port:          3306,
		AdminUser:     "root",
		AdminPassword: "ciphertext",
		Status:        "running",
		HealthStatus:  "healthy",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	instance, err := repo.GetVersion(ctx, id)
	if err != nil || instance == nil || instance.Runtime != "docker" || instance.HealthStatus != "healthy" {
		t.Fatalf("get instance: %v %#v", err, instance)
	}
}
