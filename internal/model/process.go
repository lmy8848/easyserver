package model

import "easyserver/internal/process"

// Domain types are defined in internal/process; kept as aliases for backward compatibility.

type Process = process.Process
type ProcessStatus = process.ProcessStatus
type ProcessLog = process.ProcessLog
type ProcessGroup = process.ProcessGroup
type ProcessWithStatus = process.ProcessWithStatus
type CreateProcessRequest = process.CreateProcessRequest
type UpdateProcessRequest = process.UpdateProcessRequest
type CreateProcessGroupRequest = process.CreateProcessGroupRequest
type UpdateProcessGroupRequest = process.UpdateProcessGroupRequest
type BatchProcessIDs = process.BatchProcessIDs
type ProcessStats = process.ProcessStats
