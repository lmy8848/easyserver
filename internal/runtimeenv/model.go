package runtimeenv

// RuntimeEnvironment represents a runtime environment (Java, Node.js, PHP, Python, Go).
// 权威来源是 installs/ 目录扫描（ADR-0009）；lang@exact 是绑定键，无数字 id。
type RuntimeEnvironment struct {
	Name    string `json:"name"`    // java, node, php, python, go
	Version string `json:"version"` // 17, 18.17.0, 8.2, 3.11, 1.21
	Path    string `json:"path"`    // Installation path
	Status  string `json:"status"`  // installed（扫描恒为已安装）
}

// RuntimeInstallRequest represents a request to install a runtime environment
type RuntimeInstallRequest struct {
	Name    string `json:"name" binding:"required"`    // java, node, php, python, go
	Version string `json:"version" binding:"required"` // 17, 18.17.0, 8.2, 3.11, 1.21
}

// RuntimeUninstallRequest represents a request to uninstall a runtime environment
type RuntimeUninstallRequest struct {
	Name    string `json:"name" binding:"required"`    // java, node, php, python, go
	Version string `json:"version" binding:"required"` // 17, 18.17.0, 8.2, 3.11, 1.21
}

// RuntimeDetectResult represents detected runtime environments on the system
