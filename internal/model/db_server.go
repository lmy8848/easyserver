package model

import (
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
)

// DBServer is now defined in internal/dbserver. Alias for backward compatibility.
type DBServer = dbserver.DBServer

// DBVersion is now defined in internal/dbserver. Alias for backward compatibility.
type DBVersion = dbserver.DBVersion

// CreateDBVersionRequest is now defined in internal/dbserver. Alias for backward compatibility.
type CreateDBVersionRequest = dbserver.CreateDBVersionRequest

// VersionTemplate is now defined in internal/dbserver. Alias for backward compatibility.
type VersionTemplate = dbserver.VersionTemplate

// GetVersionTemplates delegates to dbserver.GetVersionTemplates.
var GetVersionTemplates = dbserver.GetVersionTemplates

// PredefinedDBServers delegates to dbserver.PredefinedDBServers.
var PredefinedDBServers = dbserver.PredefinedDBServers

// Domain types moved to easyserver/internal/database_mgmt.
// Kept as aliases for backward compatibility.

type DBBackup = database_mgmt.DBBackup
type Database = database_mgmt.Database
type DBUser = database_mgmt.DBUser
type CreateDatabaseRequest = database_mgmt.CreateDatabaseRequest
type CreateDBUserRequest = database_mgmt.CreateDBUserRequest
type GrantRequest = database_mgmt.GrantRequest
