package fenrir

import "time"

// HealthStatus represents the health check response from Fenrir Daemon.
type HealthStatus struct {
	Status    string    `json:"status"`
	Version   string    `json:"version,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ServiceInfo represents a container or service managed within a project stack.
type ServiceInfo struct {
	Name   string   `json:"name"`
	Image  string   `json:"image,omitempty"`
	State  string   `json:"state"`  // running, exited, paused, etc.
	Status string   `json:"status"` // e.g. "Up 2 hours"
	Ports  []string `json:"ports,omitempty"`
}

// Project represents a Docker Compose stack managed by Fenrir.
type Project struct {
	Name        string        `json:"name"`
	Status      string        `json:"status"` // running, stopped, degraded, unknown
	CreatedAt   time.Time     `json:"created_at,omitempty"`
	Services    []ServiceInfo `json:"services,omitempty"`
	Compose     string        `json:"compose,omitempty"`
	ComposeYAML string        `json:"compose_yaml,omitempty"`
}

// HostMetrics represents system metrics collected by Fenrir.
type HostMetrics struct {
	CPUPercent      float64 `json:"cpu_percent"`
	MemoryPercent   float64 `json:"memory_percent"`
	MemoryUsedBytes uint64  `json:"memory_used_bytes,omitempty"`
	MemoryTotal     uint64  `json:"memory_total_bytes,omitempty"`
	DiskPercent     float64 `json:"disk_used_percent,omitempty"`
	UptimeSeconds   uint64  `json:"uptime_seconds,omitempty"`
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
	Success bool   `json:"success,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

