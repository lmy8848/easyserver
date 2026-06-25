package model

// Container represents a Docker container (from docker ps --format json)
type Container struct {
	ID         string `json:"ID"`
	Name       string `json:"Names"`
	Image      string `json:"Image"`
	Status     string `json:"Status"`
	State      string `json:"State"`
	Ports      string `json:"Ports"`
	CreatedAt  string `json:"CreatedAt"`
	Command    string `json:"Command"`
	Labels     string `json:"Labels"`
	Mounts     string `json:"Mounts"`
	Networks   string `json:"Networks"`
	Size       string `json:"Size"`
	RunningFor string `json:"RunningFor"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// Mount represents a volume mount
type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	Type        string `json:"type"`
}

// Image represents a Docker image
type Image struct {
	ID         string            `json:"ID"`
	Repository string            `json:"Repository"`
	Tag        string            `json:"Tag"`
	Size       string            `json:"Size"`
	CreatedAt  string            `json:"CreatedAt"`
	Labels     map[string]string `json:"Labels"`
}

// CreateContainerRequest represents a request to create a container
type CreateContainerRequest struct {
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

// DockerStatus represents Docker installation and running status
type DockerStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	ComposeVersion string `json:"compose_version"`
	Running        bool   `json:"running"`
	OS             string `json:"os"`
}

// ContainerStats represents real-time container resource usage
type ContainerStats struct {
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

// ContainerProcessInfo represents a process running inside a container
type ContainerProcessInfo struct {
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

// UpdateContainerRequest represents a request to update container resources
type UpdateContainerRequest struct {
	Memory  int64   `json:"memory"`
	CPUs    float64 `json:"cpus"`
	Restart string  `json:"restart"`
}

// ComposeProject represents a Docker Compose project
type ComposeProject struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	ConfigFile string   `json:"config_file"`
	Services   []string `json:"services"`
	CreatedAt  string   `json:"created_at"`
}

// Volume represents a Docker volume
type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	CreatedAt  string `json:"created_at"`
	Size       int64  `json:"size"`
}

// Network represents a Docker network
type Network struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Scope   string `json:"scope"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
}
