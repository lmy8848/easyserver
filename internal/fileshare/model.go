package fileshare

// FileShare represents an external file share link
type FileShare struct {
	ID            int64  `json:"id"`
	FilePath      string `json:"file_path"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	Token         string `json:"token"`
	Password      string `json:"password,omitempty"`
	ExpiresAt     string `json:"expires_at"`
	MaxDownloads  int    `json:"max_downloads"`
	DownloadCount int    `json:"download_count"`
	CreatedBy     int64  `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// CreateShareRequest is the request body for creating a file share
type CreateShareRequest struct {
	FilePath     string `json:"file_path" binding:"required"`
	Password     string `json:"password"`
	ExpiresAt    string `json:"expires_at"`
	MaxDownloads int    `json:"max_downloads"`
}

// UpdateShareRequest is the request body for updating a share's metadata.
// File path and token are intentionally excluded — only access-control
// fields (password / expiry / download cap) may be modified after creation.
// Password is a pointer so we can distinguish "clear password" (empty string)
// from "leave unchanged" (nil). SetPassword=true means the provided Password
// (possibly empty) should replace the stored value.
type UpdateShareRequest struct {
	Password     *string `json:"password"`
	ExpiresAt    string  `json:"expires_at"`
	MaxDownloads *int    `json:"max_downloads"`
	ClearExpiry  bool    `json:"clear_expiry"`
}
