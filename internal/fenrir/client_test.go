package fenrir

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFenrirClient(t *testing.T) {
	expectedKey := "test-fenrir-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		authHeader := r.Header.Get("Authorization")
		apiKeyHeader := r.Header.Get("X-API-Key")
		if authHeader != "Bearer "+expectedKey && apiKeyHeader != expectedKey {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "healthy",
				"version": "1.0.0",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":   "minecraft-srv",
						"status": "running",
						"services": []map[string]any{
							{"name": "mc", "state": "running", "status": "Up 2 hours"},
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/minecraft-srv/restart":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "restarted"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var req CreateProjectRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name == "" || req.Compose == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "created"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/minecraft-srv/compose":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"compose": "version: '3.8'\nservices:\n  mc:\n    image: itzg/minecraft-server:latest\n",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/minecraft-srv/compose":
			// Verify query parameters if present
			dryRun := r.URL.Query().Get("dry_run")
			redeploy := r.URL.Query().Get("redeploy")
			contentType := r.Header.Get("Content-Type")

			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "empty body"})
				return
			}

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":      true,
				"content_type": contentType,
				"dry_run":      dryRun,
				"redeploy":     redeploy,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/minecraft-srv/yaml/config.yml":
			restart := r.URL.Query().Get("restart")
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "empty body"})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"restart": restart,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/minecraft-srv/yaml/config.yml":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": "server:\n  port: 25565\n",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/host":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"cpu": map[string]any{
						"totalPercent": 15.4,
					},
					"memory": map[string]any{
						"usedPercent": 42.1,
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, expectedKey, 5*time.Second)
	ctx := context.Background()

	// 1. Health
	health, err := client.GetHealth(ctx)
	if err != nil {
		t.Fatalf("unexpected health error: %v", err)
	}
	if health.Status != "healthy" {
		t.Errorf("expected healthy, got %s", health.Status)
	}

	// 2. List Projects
	projects, err := client.ListProjects(ctx)
	if err != nil {
		t.Fatalf("unexpected list projects error: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "minecraft-srv" {
		t.Errorf("unexpected projects response: %+v", projects)
	}

	// 3. Restart Project
	if err := client.RestartProject(ctx, "minecraft-srv"); err != nil {
		t.Fatalf("unexpected restart error: %v", err)
	}

	// 4. Deploy Project
	if err := client.DeployProject(ctx, "valheim-srv", "services:\n  valheim:\n    image: valheim"); err != nil {
		t.Fatalf("unexpected deploy error: %v", err)
	}

	// 5. Host Metrics
	metrics, err := client.GetHostMetrics(ctx)
	if err != nil {
		t.Fatalf("unexpected host metrics error: %v", err)
	}
	if metrics.CPU.TotalPercent != 15.4 {
		t.Errorf("expected 15.4 cpu, got %f", metrics.CPU.TotalPercent)
	}

	// 6. Get Project Compose
	composeStr, err := client.GetProjectCompose(ctx, "minecraft-srv")
	if err != nil {
		t.Fatalf("unexpected compose fetch error: %v", err)
	}
	if composeStr == "" {
		t.Errorf("expected non-empty compose content")
	}

	// 7. Update Project Compose with Raw YAML
	rawYAMLContent := "version: '3.8'\nservices:\n  mc:\n    image: itzg/minecraft-server:latest\n"
	falseVal := false
	trueVal := true
	err = client.UpdateProjectCompose(ctx, "minecraft-srv", rawYAMLContent, &ComposeUpdateOptions{
		RawYAML:  true,
		Redeploy: &falseVal,
		DryRun:   &trueVal,
	})
	if err != nil {
		t.Fatalf("unexpected raw compose update error: %v", err)
	}

	// 8. Update Project Compose with JSON payload (different field keys)
	for _, field := range []string{"compose", "yaml", "content", "composeContent"} {
		err = client.UpdateProjectCompose(ctx, "minecraft-srv", rawYAMLContent, &ComposeUpdateOptions{
			JSONField: field,
		})
		if err != nil {
			t.Fatalf("unexpected compose update error with JSON field %s: %v", field, err)
		}
	}

	// 9. Update Project YAML with optional restart
	configContent := "server:\n  port: 25565\n"
	err = client.UpdateProjectYAML(ctx, "minecraft-srv", "config.yml", configContent, &YAMLUpdateOptions{
		Restart: &trueVal,
	})
	if err != nil {
		t.Fatalf("unexpected yaml update error: %v", err)
	}

	// 10. Update Project YAML with raw YAML
	err = client.UpdateProjectYAML(ctx, "minecraft-srv", "config.yml", configContent, &YAMLUpdateOptions{
		RawYAML: true,
	})
	if err != nil {
		t.Fatalf("unexpected raw yaml update error: %v", err)
	}

	// 11. Get Project YAML
	yamlContent, err := client.GetProjectYAML(ctx, "minecraft-srv", "config.yml")
	if err != nil {
		t.Fatalf("unexpected yaml fetch error: %v", err)
	}
	if !strings.Contains(yamlContent, "port: 25565") {
		t.Errorf("unexpected yaml content: %s", yamlContent)
	}

	// 12. Reconstruct Compose fallback test
	fallbackYAML := ReconstructCompose(&projects[0])
	if fallbackYAML == "" {
		t.Errorf("expected reconstructed compose YAML")
	}

	// 13. Test Unauthorized
	badClient := NewClient(server.URL, "wrong-key", 5*time.Second)
	_, err = badClient.GetHealth(ctx)
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFenrirHostAndHealthV1Endpoints(t *testing.T) {
	expectedKey := "test-fenrir-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+expectedKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "healthy",
				"version":       "2.4.0",
				"uptime":        "3d 12h 45m",
				"uptimeSeconds": 305100.0,
				"docker": map[string]any{
					"connected": true,
					"pingMs":    1.5,
				},
				"database": map[string]any{
					"connected": true,
					"path":      "fenrir.db",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/host":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"timestamp": "2026-10-18T09:30:00Z",
					"host": map[string]any{
						"hostname":      "fenrir-node-01",
						"os":            "linux",
						"platform":      "ubuntu",
						"uptimeSeconds": 305100,
					},
					"cpu": map[string]any{
						"coresLogical":  16,
						"coresPhysical": 8,
						"totalPercent":  24.5,
					},
					"memory": map[string]any{
						"totalBytes":  17179869184, // 16 GB
						"usedBytes":   10737418240, // 10 GB
						"usedPercent": 68.2,
					},
					"disks": []map[string]any{
						{
							"device":      "/dev/nvme0n1p2",
							"mountPoint":  "/",
							"totalBytes":  1000204886016,
							"usedBytes":   450092198708,
							"usedPercent": 45.0,
						},
					},
					"load": map[string]any{
						"load1":  1.25,
						"load5":  1.10,
						"load15": 0.95,
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, expectedKey, 5*time.Second)
	ctx := context.Background()

	// 1. Test GetHealth via /api/v1/health
	health, err := client.GetHealth(ctx)
	if err != nil {
		t.Fatalf("unexpected error from GetHealth: %v", err)
	}
	if health.Status != "healthy" || health.Version != "2.4.0" || health.UptimeSeconds != 305100 {
		t.Errorf("unexpected health response: %+v", health)
	}

	// 2. Test GetHost via /api/v1/host
	host, err := client.GetHost(ctx)
	if err != nil {
		t.Fatalf("unexpected error from GetHost: %v", err)
	}
	if host.CPU.TotalPercent != 24.5 || host.Memory.UsedPercent != 68.2 {
		t.Errorf("unexpected cpu or memory: %+v", host)
	}
	if host.Load == nil || host.Load.Load1 != 1.25 {
		t.Errorf("unexpected load average: %+v", host.Load)
	}
	if host.Host.Hostname != "fenrir-node-01" || host.Host.OS != "linux" {
		t.Errorf("unexpected host metadata: %+v", host)
	}
}

func TestFenrirRealPayloadStrictMapping(t *testing.T) {
	// 1. Verbatim payload from Fenrir GET /api/v1/health
	healthJSON := `{
		"success": true,
		"data": {
			"status": "ok",
			"version": "1.0.0",
			"uptimeSeconds": 3600.5,
			"docker": {
				"connected": true,
				"pingMs": 1.25
			},
			"database": {
				"connected": true,
				"path": "fenrir.db"
			}
		}
	}`

	var healthEnvelope struct {
		Data HealthStatus `json:"data"`
	}
	if err := json.Unmarshal([]byte(healthJSON), &healthEnvelope); err != nil {
		t.Fatalf("failed to decode health payload: %v", err)
	}
	h := healthEnvelope.Data
	if h.Status != "ok" || h.Version != "1.0.0" || h.UptimeSeconds != 3600.5 {
		t.Errorf("unexpected health mapping: %+v", h)
	}
	if !h.Docker.Connected || h.Docker.PingMs != 1.25 {
		t.Errorf("unexpected docker status mapping: %+v", h.Docker)
	}
	if !h.Database.Connected || h.Database.Path != "fenrir.db" {
		t.Errorf("unexpected db status mapping: %+v", h.Database)
	}

	// 2. Verbatim payload from Fenrir GET /api/v1/host
	hostJSON := `{
		"success": true,
		"data": {
			"timestamp": "2026-10-18T09:30:00Z",
			"host": {
				"hostname": "fenrir-node-01",
				"os": "linux",
				"platform": "ubuntu",
				"platformFamily": "debian",
				"platformVersion": "24.04",
				"kernelVersion": "6.8.0-40-generic",
				"kernelArch": "x86_64",
				"uptimeSeconds": 864000,
				"bootTime": 1729200000,
				"hostId": "a1b2c3d4-e5f6-7890-abcd-ef0123456789"
			},
			"cpu": {
				"modelName": "AMD Ryzen 7 5700X 8-Core Processor",
				"coresLogical": 16,
				"coresPhysical": 8,
				"totalPercent": 14.5,
				"perCorePercent": [12.0, 15.2, 11.8, 18.1, 14.0, 13.5, 16.0, 15.4]
			},
			"memory": {
				"totalBytes": 33614299136,
				"availableBytes": 21474836480,
				"usedBytes": 12139462656,
				"usedPercent": 36.1,
				"swapTotalBytes": 8589934592,
				"swapUsedBytes": 0,
				"swapFreeBytes": 8589934592,
				"swapUsedPercent": 0.0
			},
			"disks": [
				{
					"device": "/dev/nvme0n1p2",
					"mountPoint": "/",
					"fsType": "ext4",
					"totalBytes": 1000204886016,
					"freeBytes": 650133176320,
					"usedBytes": 350071709696,
					"usedPercent": 35.0
				}
			],
			"load": {
				"load1": 0.45,
				"load5": 0.52,
				"load15": 0.48
			}
		}
	}`

	var hostEnvelope struct {
		Data HostMetrics `json:"data"`
	}
	if err := json.Unmarshal([]byte(hostJSON), &hostEnvelope); err != nil {
		t.Fatalf("failed to decode host payload: %v", err)
	}
	host := hostEnvelope.Data
	if host.CPU.TotalPercent != 14.5 || host.CPUPercent() != 14.5 {
		t.Errorf("expected 14.5 CPU percent, got %f", host.CPU.TotalPercent)
	}
	if host.Memory.UsedPercent != 36.1 || host.MemoryPercent() != 36.1 {
		t.Errorf("expected 36.1 Memory percent, got %f", host.Memory.UsedPercent)
	}
	if host.Memory.UsedBytes != 12139462656 || host.Memory.TotalBytes != 33614299136 {
		t.Errorf("expected exact RAM byte counts, got used=%d total=%d", host.Memory.UsedBytes, host.Memory.TotalBytes)
	}
	if host.Host.Hostname != "fenrir-node-01" || host.Host.UptimeSeconds != 864000 {
		t.Errorf("unexpected host metadata: %+v", host.Host)
	}
	if host.Load == nil || host.Load.Load1 != 0.45 || host.Load.Load5 != 0.52 || host.Load.Load15 != 0.48 {
		t.Errorf("unexpected load metrics: %+v", host.Load)
	}
	if len(host.Disks) != 1 || host.Disks[0].UsedPercent != 35.0 || host.PrimaryDiskPercent() != 35.0 {
		t.Errorf("unexpected disk metrics: %+v", host.Disks)
	}

	// 3. Verbatim payload from Fenrir GET /api/v1/projects
	projectsJSON := `{
		"success": true,
		"data": [
			{
				"id": 1,
				"name": "mc-survival",
				"directoryPath": "/var/lib/fenrir/stacks/mc-survival",
				"composeFile": "compose.yml",
				"description": "PaperMC Survival Server",
				"status": "running",
				"createdAt": "2026-10-18T06:00:00Z",
				"updatedAt": "2026-10-18T06:00:00Z"
			}
		]
	}`

	var projectsEnvelope struct {
		Data []Project `json:"data"`
	}
	if err := json.Unmarshal([]byte(projectsJSON), &projectsEnvelope); err != nil {
		t.Fatalf("failed to decode projects payload: %v", err)
	}
	if len(projectsEnvelope.Data) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projectsEnvelope.Data))
	}
	p := projectsEnvelope.Data[0]
	if p.ID != 1 || p.Name != "mc-survival" || p.DirectoryPath != "/var/lib/fenrir/stacks/mc-survival" || p.Status != "running" {
		t.Errorf("unexpected project mapping: %+v", p)
	}
	if p.CreatedAt.IsZero() {
		t.Errorf("expected non-zero CreatedAt timestamp")
	}

	// 4. Verbatim payload from Fenrir GET /api/v1/projects/{name}/status
	statusJSON := `{
		"success": true,
		"data": {
			"project": "mc-survival",
			"status": "running",
			"services": [
				{
					"id": "c1f2e3d4a5b6",
					"name": "mc-survival-minecraft-1",
					"service": "minecraft",
					"state": "running",
					"status": "Up 3 hours (healthy)",
					"health": "healthy",
					"publishers": "0.0.0.0:25565->25565/tcp"
				}
			]
		}
	}`

	var statusEnvelope struct {
		Data ProjectStatusResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(statusJSON), &statusEnvelope); err != nil {
		t.Fatalf("failed to decode project status payload: %v", err)
	}
	ps := statusEnvelope.Data
	if ps.Project != "mc-survival" || ps.Status != "running" || len(ps.Services) != 1 {
		t.Fatalf("unexpected project status response: %+v", ps)
	}
	svc := ps.Services[0]
	if svc.ID != "c1f2e3d4a5b6" || svc.Name != "mc-survival-minecraft-1" || svc.Service != "minecraft" || svc.State != "running" || svc.Health != "healthy" {
		t.Errorf("unexpected service status mapping: %+v", svc)
	}
}


