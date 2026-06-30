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
