package model

import "easyserver/internal/web"

// Types migrated to internal/web; kept as aliases for backward compatibility.

type WebServer = web.WebServer
type Website = web.Website
type CreateWebsiteRequest = web.CreateWebsiteRequest
type UpdateWebsiteRequest = web.UpdateWebsiteRequest
type CreateWebServerRequest = web.CreateWebServerRequest
type ProjectTypeConfig = web.ProjectTypeConfig

// Functions migrated to internal/web; kept as aliases for backward compatibility.

var FindPredefinedWebServer = web.FindPredefinedWebServer
var GetPredefinedWebServerNames = web.GetPredefinedWebServerNames
var PredefinedWebServers = web.PredefinedWebServers
var GetProjectTypes = web.GetProjectTypes
