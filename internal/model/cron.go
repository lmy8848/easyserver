package model

import "easyserver/internal/cron"

// Domain types moved to easyserver/internal/cron.
// Kept as aliases for backward compatibility.

type CronTask = cron.CronTask
type CronLog = cron.CronLog
type CreateCronTaskRequest = cron.CreateCronTaskRequest
type UpdateCronTaskRequest = cron.UpdateCronTaskRequest
type Script = cron.Script
type CreateScriptRequest = cron.CreateScriptRequest
type UpdateScriptRequest = cron.UpdateScriptRequest
type CronDoc = cron.CronDoc
