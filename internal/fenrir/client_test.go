package fenrir

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/metrics/host":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"cpu_percent":    15.4,
					"memory_percent": 42.1,
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
	if metrics.CPUPercent != 15.4 {
		t.Errorf("expected 15.4 cpu, got %f", metrics.CPUPercent)
	}

	// 6. Test Unauthorized
	badClient := NewClient(server.URL, "wrong-key", 5*time.Second)
	_, err = badClient.GetHealth(ctx)
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}
