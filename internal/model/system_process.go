package model

import "easyserver/internal/systemprocess"

// Domain types moved to easyserver/internal/systemprocess.
// Kept as aliases for backward compatibility.

type SystemProcess = systemprocess.SystemProcess
type SystemService = systemprocess.SystemService
type SystemServiceAction = systemprocess.SystemServiceAction
type ServiceWhitelistEntry = systemprocess.ServiceWhitelistEntry
type SystemOverview = systemprocess.SystemOverview
type SystemProcessListRequest = systemprocess.SystemProcessListRequest
