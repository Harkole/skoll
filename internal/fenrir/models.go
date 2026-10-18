package fenrir

import (
	"encoding/json"
	"time"
)

// DockerStatus indicates connection state to the Docker daemon.
type DockerStatus struct {
	Connected bool    `json:"connected"`
	PingMs    float64 `json:"pingMs,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// DBStatus indicates SQLite database connectivity and state.
type DBStatus struct {
	Connected bool   `json:"connected"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthStatus represents the health check response from Fenrir Daemon.
type HealthStatus struct {
	Status        string       `json:"status"` // "ok", "degraded", "healthy"
	Version       string       `json:"version,omitempty"`
	Uptime        string       `json:"uptime,omitempty"`
	UptimeSeconds float64      `json:"uptimeSeconds,omitempty"`
	Docker        DockerStatus `json:"docker,omitempty"`
	Database      DBStatus     `json:"database,omitempty"`
}

// HostInfo holds static and boot-time operating system information.
type HostInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformFamily  string `json:"platformFamily"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	KernelArch      string `json:"kernelArch"`
	UptimeSeconds   uint64 `json:"uptimeSeconds"`
	BootTime        uint64 `json:"bootTime"`
	HostID          string `json:"hostId"`
}

// CPUMetrics holds processor metrics.
type CPUMetrics struct {
	ModelName      string    `json:"modelName,omitempty"`
	CoresLogical   int       `json:"coresLogical"`
	CoresPhysical  int       `json:"coresPhysical"`
	TotalPercent   float64   `json:"totalPercent"`
	PerCorePercent []float64 `json:"perCorePercent,omitempty"`
}

// MemoryMetrics holds RAM and Swap statistics.
type MemoryMetrics struct {
	TotalBytes      uint64  `json:"totalBytes"`
	AvailableBytes  uint64  `json:"availableBytes"`
	UsedBytes       uint64  `json:"usedBytes"`
	UsedPercent     float64 `json:"usedPercent"`
	SwapTotalBytes  uint64  `json:"swapTotalBytes"`
	SwapUsedBytes   uint64  `json:"swapUsedBytes"`
	SwapFreeBytes   uint64  `json:"swapFreeBytes"`
	SwapUsedPercent float64 `json:"swapUsedPercent"`
}

// DiskPartitionMetrics holds storage stats for a single mounted filesystem.
type DiskPartitionMetrics struct {
	Device      string  `json:"device"`
	MountPoint  string  `json:"mountPoint"`
	FSType      string  `json:"fsType"`
	TotalBytes  uint64  `json:"totalBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

// LoadMetrics holds system load averages (available on Unix-like hosts).
type LoadMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// HostMetrics represents the full aggregate host hardware and system state mirroring Fenrir's schema.
type HostMetrics struct {
	Timestamp time.Time              `json:"timestamp"`
	Host      HostInfo               `json:"host"`
	CPU       CPUMetrics             `json:"cpu"`
	Memory    MemoryMetrics          `json:"memory"`
	Disks     []DiskPartitionMetrics `json:"disks"`
	Load      *LoadMetrics           `json:"load,omitempty"`
}

// CPUPercent returns overall CPU utilization percentage.
func (h *HostMetrics) CPUPercent() float64 {
	return h.CPU.TotalPercent
}

// MemoryPercent returns RAM utilization percentage.
func (h *HostMetrics) MemoryPercent() float64 {
	return h.Memory.UsedPercent
}

// PrimaryDiskPercent returns the disk utilization percentage of the root filesystem or first disk.
func (h *HostMetrics) PrimaryDiskPercent() float64 {
	for _, d := range h.Disks {
		if d.MountPoint == "/" {
			return d.UsedPercent
		}
	}
	if len(h.Disks) > 0 {
		return h.Disks[0].UsedPercent
	}
	return 0
}

// ComposeServiceStatus represents status of a single service in a compose stack.
type ComposeServiceStatus struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Service    string   `json:"service,omitempty"`
	Image      string   `json:"image,omitempty"`
	State      string   `json:"state"`  // running, exited, paused, etc.
	Status     string   `json:"status"` // e.g. "Up 2 hours"
	Health     string   `json:"health,omitempty"`
	Publishers string   `json:"publishers,omitempty"`
	Ports      []string `json:"ports,omitempty"`
}

// ServiceInfo is an alias for ComposeServiceStatus for backward compatibility.
type ServiceInfo = ComposeServiceStatus

// ProjectPort represents a host port mapping allocated to a project stack.
type ProjectPort struct {
	ID            int64  `json:"id,omitempty"`
	ProjectName   string `json:"projectName"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"` // "tcp" or "udp"
}

// Project represents a Docker Compose stack managed by Fenrir.
type Project struct {
	ID            int64                  `json:"id,omitempty"`
	Name          string                 `json:"name"`
	DirectoryPath string                 `json:"directoryPath,omitempty"`
	ComposeFile   string                 `json:"composeFile,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Status        string                 `json:"status"` // created, deploying, running, stopped, failed
	CreatedAt     time.Time              `json:"createdAt,omitempty"`
	UpdatedAt     time.Time              `json:"updatedAt,omitempty"`
	Services      []ComposeServiceStatus `json:"services,omitempty"`
	Compose       string                 `json:"compose,omitempty"`
	ComposeYAML   string                 `json:"compose_yaml,omitempty"`
}

// ProjectDetail provides extended details for a project including compose content, files, and allocated ports.
type ProjectDetail struct {
	Project
	ComposeContent string        `json:"composeContent,omitempty"`
	Files          []string      `json:"files,omitempty"`
	HasDataDir     bool          `json:"hasDataDir"`
	Ports          []ProjectPort `json:"ports,omitempty"`
}

// ProjectStatusResponse holds status check results for a project stack.
type ProjectStatusResponse struct {
	Project  string                 `json:"project"`
	Status   string                 `json:"status"`
	Services []ComposeServiceStatus `json:"services"`
	Output   string                 `json:"output,omitempty"`
}

// CreateProjectRequest is the payload for creating/deploying a stack.
type CreateProjectRequest struct {
	Name    string `json:"name"`
	Compose string `json:"compose"`
}

// ComposeUpdateOptions contains optional query parameters and formatting options for updating project compose.
type ComposeUpdateOptions struct {
	Redeploy  *bool  `json:"redeploy,omitempty"`
	DryRun    *bool  `json:"dry_run,omitempty"`
	RawYAML   bool   `json:"raw_yaml,omitempty"`
	JSONField string `json:"json_field,omitempty"` // "compose", "yaml", "content", or "composeContent"
}

// YAMLUpdateOptions contains optional query parameters and formatting options for updating auxiliary YAML files.
type YAMLUpdateOptions struct {
	Restart   *bool  `json:"restart,omitempty"`
	RawYAML   bool   `json:"raw_yaml,omitempty"`
	JSONField string `json:"json_field,omitempty"` // "content", "yaml", "compose", or "composeContent"
}

// APIResponse wraps standardized JSON envelopes returned by Fenrir Daemon.
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
}
