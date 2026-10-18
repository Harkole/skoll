package fenrir

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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

// Archive represents a backed-up Docker Compose stack on 90-day retention cooldown.
type Archive struct {
	ID                int64     `json:"id"`
	IDStr             string    `json:"id_str,omitempty"`
	ProjectName       string    `json:"projectName"`
	FilePath          string    `json:"filePath,omitempty"`
	FileSize          int64     `json:"fileSize"`
	CreatedAt         time.Time `json:"createdAt"`
	ExpiresAt         time.Time `json:"expiresAt"`
	DaysUntilDeletion int       `json:"daysUntilDeletion"`
}

// UnmarshalJSON implements custom JSON unmarshaling to flexibly accept camelCase,
// snake_case, string or numeric IDs, and calculate retention cooldown metrics.
func (a *Archive) UnmarshalJSON(data []byte) error {
	type Alias Archive
	aux := &struct {
		RawID              any        `json:"id"`
		ProjectNameAlt     string     `json:"project_name"`
		NameAlt            string     `json:"name"`
		ProjectAlt         string     `json:"project"`
		FileSizeAlt        *int64     `json:"file_size"`
		SizeAlt            *int64     `json:"size"`
		SizeBytesAlt       *int64     `json:"sizeBytes"`
		ArchiveSizeAlt     *int64     `json:"archive_file_size"`
		CreatedAtAlt       *time.Time `json:"created_at"`
		ExpiresAtAlt       *time.Time `json:"expires_at"`
		ExpirationDateAlt  *time.Time `json:"expirationDate"`
		ExpirationDateAlt2 *time.Time `json:"expiration_date"`
		DaysUntilDelAlt    *int       `json:"days_until_deletion"`
		DaysRemainingAlt   *int       `json:"daysRemaining"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.RawID != nil {
		switch v := aux.RawID.(type) {
		case float64:
			a.ID = int64(v)
			a.IDStr = strconv.FormatInt(a.ID, 10)
		case string:
			a.IDStr = v
			if idNum, err := strconv.ParseInt(v, 10, 64); err == nil {
				a.ID = idNum
			}
		case int64:
			a.ID = v
			a.IDStr = strconv.FormatInt(v, 10)
		}
	} else if a.ID != 0 {
		a.IDStr = strconv.FormatInt(a.ID, 10)
	}

	if a.ProjectName == "" {
		if aux.ProjectNameAlt != "" {
			a.ProjectName = aux.ProjectNameAlt
		} else if aux.NameAlt != "" {
			a.ProjectName = aux.NameAlt
		} else if aux.ProjectAlt != "" {
			a.ProjectName = aux.ProjectAlt
		}
	}

	if a.FileSize == 0 {
		if aux.FileSizeAlt != nil {
			a.FileSize = *aux.FileSizeAlt
		} else if aux.SizeAlt != nil {
			a.FileSize = *aux.SizeAlt
		} else if aux.SizeBytesAlt != nil {
			a.FileSize = *aux.SizeBytesAlt
		} else if aux.ArchiveSizeAlt != nil {
			a.FileSize = *aux.ArchiveSizeAlt
		}
	}

	if a.CreatedAt.IsZero() && aux.CreatedAtAlt != nil {
		a.CreatedAt = *aux.CreatedAtAlt
	}

	if a.ExpiresAt.IsZero() {
		if aux.ExpiresAtAlt != nil {
			a.ExpiresAt = *aux.ExpiresAtAlt
		} else if aux.ExpirationDateAlt != nil {
			a.ExpiresAt = *aux.ExpirationDateAlt
		} else if aux.ExpirationDateAlt2 != nil {
			a.ExpiresAt = *aux.ExpirationDateAlt2
		}
	}

	// Default to 90-day retention if CreatedAt is present and ExpiresAt was not explicitly set
	if a.ExpiresAt.IsZero() && !a.CreatedAt.IsZero() {
		a.ExpiresAt = a.CreatedAt.Add(90 * 24 * time.Hour)
	}

	if a.DaysUntilDeletion == 0 {
		if aux.DaysUntilDelAlt != nil {
			a.DaysUntilDeletion = *aux.DaysUntilDelAlt
		} else if aux.DaysRemainingAlt != nil {
			a.DaysUntilDeletion = *aux.DaysRemainingAlt
		}
	}

	// Calculate days remaining until auto-deletion if missing or 0 and ExpiresAt is set
	if a.DaysUntilDeletion <= 0 && !a.ExpiresAt.IsZero() {
		days := int(math.Ceil(time.Until(a.ExpiresAt).Hours() / 24.0))
		if days < 0 {
			days = 0
		}
		a.DaysUntilDeletion = days
	}

	return nil
}

// FormattedSize returns a clean human-readable representation of archive file size.
func (a *Archive) FormattedSize() string {
	b := a.FileSize
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// GetIDString returns the string representation of the archive ID.
func (a *Archive) GetIDString() string {
	if a.IDStr != "" {
		return a.IDStr
	}
	return strconv.FormatInt(a.ID, 10)
}

