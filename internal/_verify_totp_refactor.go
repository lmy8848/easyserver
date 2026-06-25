//go:build ignore

// This file exists solely to verify that TOTPRepository interface is
// satisfied by the SQLite implementation and consumed correctly by the
// service layer. It is never compiled into the final binary.
package main

import (
	"database/sql"

	"easyserver/internal/repository/sqlite"
	"easyserver/internal/service"
)

func main() {
	var db *sql.DB
	_ = service.NewTOTPService(sqlite.NewTOTPRepository(db))
}
