package container

// Engine identifies a container runtime engine.
type Engine string

const (
	EngineDocker Engine = "docker"
	EnginePodman Engine = "podman"
)

// Default Unix Socket paths for Docker and Podman.
const (
	DefaultDockerHost = "/var/run/docker.sock"
	DefaultPodmanHost = "/run/podman/podman.sock"
)

// --- Container Types ---

type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type PortMapping struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

type ContainerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

type ContainerSummary struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Ports           []PortMapping     `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	SizeRw          int64             `json:"SizeRw"`
	SizeRootFs      int64             `json:"SizeRootFs"`
	Mounts          []ContainerMount  `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
			Gateway   string `json:"Gateway"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type ContainerInspect struct {
	ID      string   `json:"Id"`
	Created string   `json:"Created"`
	Path    string   `json:"Path"`
	Args    []string `json:"Args"`
	State   struct {
		Status     string           `json:"Status"`
		Running    bool             `json:"Running"`
		Paused     bool             `json:"Paused"`
		Restarting bool             `json:"Restarting"`
		OOMKilled  bool             `json:"OOMKilled"`
		Dead       bool             `json:"Dead"`
		Pid        int              `json:"Pid"`
		ExitCode   int              `json:"ExitCode"`
		Error      string           `json:"Error"`
		StartedAt  string           `json:"StartedAt"`
		FinishedAt string           `json:"FinishedAt"`
		Health     *ContainerHealth `json:"Health,omitempty"`
	} `json:"State"`
	Image           string           `json:"Image"`
	Name            string           `json:"Name"`
	RestartCount    int              `json:"RestartCount"`
	Driver          string           `json:"Driver"`
	Platform        string           `json:"Platform"`
	Mounts          []ContainerMount `json:"Mounts"`
	Config          ContainerConfig  `json:"Config"`
	HostConfig      HostConfig       `json:"HostConfig"`
	NetworkSettings struct {
		Ports    map[string][]PortBinding `json:"Ports"`
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
			Gateway   string `json:"Gateway"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type ContainerHealth struct {
	Status        string `json:"Status"`
	FailingStreak int    `json:"FailingStreak"`
}

type HealthcheckConfig struct {
	Test        []string `json:"Test,omitempty"`
	Interval    int64    `json:"Interval,omitempty"`
	Timeout     int64    `json:"Timeout,omitempty"`
	StartPeriod int64    `json:"StartPeriod,omitempty"`
	Retries     int      `json:"Retries,omitempty"`
}

type ContainerConfig struct {
	Hostname     string              `json:"Hostname,omitempty"`
	User         string              `json:"User,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Image        string              `json:"Image,omitempty"`
	WorkingDir   string              `json:"WorkingDir,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Healthcheck  *HealthcheckConfig  `json:"Healthcheck,omitempty"`
}

type RestartPolicy struct {
	Name              string `json:"Name,omitempty"`
	MaximumRetryCount int    `json:"MaximumRetryCount,omitempty"`
}

type HostConfig struct {
	Binds         []string                 `json:"Binds,omitempty"`
	PortBindings  map[string][]PortBinding `json:"PortBindings,omitempty"`
	RestartPolicy *RestartPolicy           `json:"RestartPolicy,omitempty"`
	AutoRemove    bool                     `json:"AutoRemove,omitempty"`
	NetworkMode   string                   `json:"NetworkMode,omitempty"`
	Memory        int64                    `json:"Memory,omitempty"`
	NanoCPUs      int64                    `json:"NanoCpus,omitempty"`
}

type ContainerCreateRequest struct {
	ContainerConfig
	HostConfig *HostConfig `json:"HostConfig,omitempty"`
}

type ContainerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// --- Image Types ---

type ImageSummary struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	VirtualSize int64             `json:"VirtualSize"`
	SharedSize  int64             `json:"SharedSize"`
	Labels      map[string]string `json:"Labels"`
	Containers  int64             `json:"Containers"`
}

type ImageInspect struct {
	ID            string            `json:"Id"`
	RepoTags      []string          `json:"RepoTags"`
	RepoDigests   []string          `json:"RepoDigests"`
	Parent        string            `json:"Parent"`
	Comment       string            `json:"Comment"`
	Created       string            `json:"Created"`
	DockerVersion string            `json:"DockerVersion"`
	Author        string            `json:"Author"`
	Architecture  string            `json:"Architecture"`
	Os            string            `json:"Os"`
	Size          int64             `json:"Size"`
	VirtualSize   int64             `json:"VirtualSize"`
	Labels        map[string]string `json:"Labels"`
}

type ImageDeleteResponseItem struct {
	Untagged string `json:"Untagged,omitempty"`
	Deleted  string `json:"Deleted,omitempty"`
}

type ImagesPruneReport struct {
	ImagesDeleted  []ImageDeleteResponseItem `json:"ImagesDeleted"`
	SpaceReclaimed uint64                    `json:"SpaceReclaimed"`
}

// --- Volume Types ---

type VolumeUsageData struct {
	Size     int64 `json:"Size"`
	RefCount int64 `json:"RefCount"`
}

type Volume struct {
	CreatedAt  string            `json:"CreatedAt,omitempty"`
	Driver     string            `json:"Driver"`
	Labels     map[string]string `json:"Labels"`
	Mountpoint string            `json:"Mountpoint"`
	Name       string            `json:"Name"`
	Options    map[string]string `json:"Options"`
	Scope      string            `json:"Scope"`
	Status     map[string]any    `json:"Status,omitempty"`
	UsageData  *VolumeUsageData  `json:"UsageData,omitempty"`
}

type VolumeListResponse struct {
	Volumes  []Volume `json:"Volumes"`
	Warnings []string `json:"Warnings"`
}

type VolumeCreateRequest struct {
	Name       string            `json:"Name,omitempty"`
	Driver     string            `json:"Driver,omitempty"`
	DriverOpts map[string]string `json:"DriverOpts,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
}

type VolumesPruneReport struct {
	VolumesDeleted []string `json:"VolumesDeleted"`
	SpaceReclaimed uint64   `json:"SpaceReclaimed"`
}

// --- Network Types ---

type IPAMConfig struct {
	Subnet     string            `json:"Subnet,omitempty"`
	IPRange    string            `json:"IPRange,omitempty"`
	Gateway    string            `json:"Gateway,omitempty"`
	AuxAddress map[string]string `json:"AuxiliaryAddresses,omitempty"`
}

type IPAM struct {
	Driver  string            `json:"Driver,omitempty"`
	Config  []IPAMConfig      `json:"Config,omitempty"`
	Options map[string]string `json:"Options,omitempty"`
}

type NetworkSummary struct {
	Name       string            `json:"Name"`
	ID         string            `json:"Id"`
	Created    string            `json:"Created"`
	Scope      string            `json:"Scope"`
	Driver     string            `json:"Driver"`
	EnableIPv6 bool              `json:"EnableIPv6"`
	Internal   bool              `json:"Internal"`
	Attachable bool              `json:"Attachable"`
	Ingress    bool              `json:"Ingress"`
	IPAM       IPAM              `json:"IPAM"`
	Labels     map[string]string `json:"Labels"`
}

type NetworkCreateRequest struct {
	Name           string            `json:"Name"`
	CheckDuplicate bool              `json:"CheckDuplicate,omitempty"`
	Driver         string            `json:"Driver,omitempty"`
	Internal       bool              `json:"Internal,omitempty"`
	Attachable     bool              `json:"Attachable,omitempty"`
	EnableIPv6     bool              `json:"EnableIPv6,omitempty"`
	IPAM           *IPAM             `json:"IPAM,omitempty"`
	Labels         map[string]string `json:"Labels,omitempty"`
}

type NetworkCreateResponse struct {
	ID      string `json:"Id"`
	Warning string `json:"Warning"`
}

type NetworksPruneReport struct {
	NetworksDeleted []string `json:"NetworksDeleted"`
}

// --- Stats & Exec Types ---

type StatsJSON struct {
	Read      string `json:"read"`
	Preread   string `json:"preread"`
	PidsStats struct {
		Current uint64 `json:"current"`
		Limit   uint64 `json:"limit"`
	} `json:"pids_stats"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage        uint64   `json:"total_usage"`
			PercpuUsage       []uint64 `json:"percpu_usage"`
			UsageInKernelmode uint64   `json:"usage_in_kernelmode"`
			UsageInUsermode   uint64   `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage        uint64   `json:"total_usage"`
			PercpuUsage       []uint64 `json:"percpu_usage"`
			UsageInKernelmode uint64   `json:"usage_in_kernelmode"`
			UsageInUsermode   uint64   `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage    uint64            `json:"usage"`
		MaxUsage uint64            `json:"max_usage"`
		Limit    uint64            `json:"limit"`
		Stats    map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes   uint64 `json:"rx_bytes"`
		RxPackets uint64 `json:"rx_packets"`
		RxErrors  uint64 `json:"rx_errors"`
		RxDropped uint64 `json:"rx_dropped"`
		TxBytes   uint64 `json:"tx_bytes"`
		TxPackets uint64 `json:"tx_packets"`
		TxErrors  uint64 `json:"tx_errors"`
		TxDropped uint64 `json:"tx_dropped"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Major uint64 `json:"major"`
			Minor uint64 `json:"minor"`
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

type ExecCreateRequest struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Env          []string `json:"Env,omitempty"`
	Cmd          []string `json:"Cmd"`
	User         string   `json:"User,omitempty"`
	WorkingDir   string   `json:"WorkingDir,omitempty"`
}

type ExecCreateResponse struct {
	ID string `json:"Id"`
}

type ExecStartRequest struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

type ExecInspectResponse struct {
	ID            string `json:"ID"`
	Running       bool   `json:"Running"`
	ExitCode      int    `json:"ExitCode"`
	ProcessConfig struct {
		Tty        bool     `json:"tty"`
		Entrypoint string   `json:"entrypoint"`
		Arguments  []string `json:"arguments"`
	} `json:"ProcessConfig"`
	OpenStdin   bool   `json:"OpenStdin"`
	OpenStderr  bool   `json:"OpenStderr"`
	OpenStdout  bool   `json:"OpenStdout"`
	CanRemove   bool   `json:"CanRemove"`
	ContainerID string `json:"ContainerID"`
	DetachKeys  string `json:"DetachKeys"`
	Pid         int    `json:"Pid"`
}

// PingResponse represents /_ping response header data.
type PingResponse struct {
	APIVersion string
	OSType     string
}

// VersionResponse represents /version response data.
type VersionResponse struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
}
