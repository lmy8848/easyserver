package web

import "context"

// ServerRepository defines the interface for web server data access.
type ServerRepository interface {
	List(ctx context.Context) ([]WebServer, error)
	Get(ctx context.Context, id int64) (*WebServer, error)
	Create(ctx context.Context, ws *WebServer) error
	Delete(ctx context.Context, id int64) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateStatusAndVersion(ctx context.Context, id int64, status, version string) error
	CountWebsitesByServerID(ctx context.Context, serverID int64) (int, error)
}

// WebsiteRepository defines the interface for website data access.
type WebsiteRepository interface {
	List(ctx context.Context, webServerID int64) ([]Website, error)
	Get(ctx context.Context, webServerID, id int64) (*Website, error)
	Create(ctx context.Context, w *Website) (int64, error)
	Update(ctx context.Context, w *Website) error
	Delete(ctx context.Context, webServerID, id int64) error
	UpdateStatus(ctx context.Context, webServerID, id int64, status string) error
	UpdateSSL(ctx context.Context, id int64, certPath, keyPath string) error
	CountByDomain(ctx context.Context, domain string) (int, error)
	CountByDomainExcludingID(ctx context.Context, domain string, excludeID int64) (int, error)
}
