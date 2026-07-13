package fileshare

import "context"

// Repository defines the interface for file share data access
type Repository interface {
	Create(ctx context.Context, share *FileShare) (int64, error)
	GetByID(ctx context.Context, id int64) (*FileShare, error)
	GetByToken(ctx context.Context, token string) (*FileShare, error)
	List(ctx context.Context, createdBy int64) ([]FileShare, error)
	Update(ctx context.Context, id int64, req *UpdateShareRequest) error
	Delete(ctx context.Context, id int64) error
	IncrementDownloads(ctx context.Context, id int64) error
	// IncrementDownloadsIfUnderLimit atomically bumps download_count only if the
	// cap hasn't been reached, returning whether the increment happened. Prevents
	// the check-then-increment race that could exceed MaxDownloads.
	IncrementDownloadsIfUnderLimit(ctx context.Context, id int64) (bool, error)
	DeleteExpired(ctx context.Context) (int64, error)
}
