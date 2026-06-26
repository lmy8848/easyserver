package model

import "easyserver/internal/deploy"

// Deploy types are now defined in internal/deploy.
// Kept as aliases for backward compatibility.

type DeployServer = deploy.Server
type DeployTask = deploy.Task
type DeployVersion = deploy.Version
