package container

// Container represents a Docker container exposed over the API.
// Docker CLI outputs uppercase keys; service.go unmarshals into a private
// shim and maps into this lowercase shape before returning.
type Container struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Image      string        `json:"image"`
	Status     string        `json:"status"`
	State      string        `json:"state"`
	Ports      []PortMapping `json:"ports"`
	CreatedAt  string        `json:"created_at"`
	Command    string        `json:"command"`
	Labels     string        `json:"labels"`
	Mounts     string        `json:"mounts"`
	Networks   string        `json:"networks"`
	Size       string        `json:"size"`
	RunningFor string        `json:"running_for"`
}

// PortMapping represents a port mapping.
type PortMapping struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// Mount represents a volume mount.
type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	Type        string `json:"type"`
}

// Image represents a Docker image exposed over the API. See Container note.
type Image struct {
	ID         string            `json:"id"`
	Repository string            `json:"repository"`
	Tag        string            `json:"tag"`
	Size       string            `json:"size"`
	CreatedAt  string            `json:"created_at"`
	Labels     map[string]string `json:"labels"`
}

// CreateRequest represents a request to create a container.
type CreateRequest struct {
	Name          string            `json:"name"`
	Image         string            `json:"image" binding:"required"`
	Command       string            `json:"command"`
	Ports         []PortMapping     `json:"ports"`
	EnvVars       map[string]string `json:"env_vars"`
	Volumes       []Mount           `json:"volumes"`
	Networks      []string          `json:"networks"`
	RestartPolicy string            `json:"restart_policy"`
	Labels        map[string]string `json:"labels"`
	AutoRemove    bool              `json:"auto_remove"`
	Memory        int64             `json:"memory"`
	CPUs          float64           `json:"cpus"`
}

// DockerStatus represents Docker installation and running status.
type DockerStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	ComposeVersion string `json:"compose_version"`
	Running        bool   `json:"running"`
	OS             string `json:"os"`
}

// Stats represents real-time container resource usage.
type Stats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   int64   `json:"mem_usage"`
	MemLimit   int64   `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
	NetRx      int64   `json:"net_rx"`
	NetTx      int64   `json:"net_tx"`
	BlockRead  int64   `json:"block_read"`
	BlockWrite int64   `json:"block_write"`
	PIDs       int     `json:"pids"`
}

// ProcessInfo represents a process running inside a container.
type ProcessInfo struct {
	User    string `json:"user"`
	PID     string `json:"pid"`
	PPID    string `json:"ppid"`
	CPU     string `json:"cpu"`
	MEM     string `json:"mem"`
	VSZ     string `json:"vsz"`
	RSS     string `json:"rss"`
	TTY     string `json:"tty"`
	Stat    string `json:"stat"`
	Start   string `json:"start"`
	Time    string `json:"time"`
	Command string `json:"command"`
}

// UpdateRequest represents a request to update container resources.
type UpdateRequest struct {
	Memory  int64   `json:"memory"`
	CPUs    float64 `json:"cpus"`
	Restart string  `json:"restart"`
}

// ComposeProject represents a Docker Compose project.
type ComposeProject struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	ConfigFile string   `json:"config_file"`
	Services   []string `json:"services"`
	CreatedAt  string   `json:"created_at"`
}

// Volume represents a Docker volume.
type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	CreatedAt  string `json:"created_at"`
	Size       int64  `json:"size"`
}

// Network represents a Docker network.
type Network struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Scope   string `json:"scope"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
}
