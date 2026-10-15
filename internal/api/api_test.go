package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harkole/skoll/internal/config"
	"github.com/Harkole/skoll/internal/database"
	"github.com/Harkole/skoll/internal/fenrir"
)

type mockOAuthExchanger struct {
	ExchangeCodeFunc func(ctx context.Context, code string) (string, error)
	FetchUserFunc    func(ctx context.Context, token string) (*DiscordUser, error)
}

func (m *mockOAuthExchanger) ExchangeCode(ctx context.Context, code string) (string, error) {
	if m.ExchangeCodeFunc != nil {
		return m.ExchangeCodeFunc(ctx, code)
	}
	return "mock_token_" + code, nil
}

func (m *mockOAuthExchanger) FetchUser(ctx context.Context, token string) (*DiscordUser, error) {
	if m.FetchUserFunc != nil {
		return m.FetchUserFunc(ctx, token)
	}
	return &DiscordUser{
		ID:         "1234567890",
		Username:   "TestUser",
		GlobalName: "Test User",
	}, nil
}

func setupTestAPIServer(t *testing.T) (*Server, *database.DB, *config.Config, *mockOAuthExchanger) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{
		Port:                ":8081",
		PublicURL:           "http://localhost:8081",
		AdminKey:            "super-secret-admin",
		DatabasePath:        dbPath,
		DiscordClientID:     "mock_client_id",
		DiscordClientSecret: "mock_client_secret",
		DiscordRedirectURI:  "http://localhost:8081/auth/discord/callback",
		FenrirAPIURL:        "http://localhost:8080",
		HTTPTimeout:         5 * time.Second,
	}

	oauth := &mockOAuthExchanger{}
	server := NewServer(cfg, db, nil, oauth)
	return server, db, cfg, oauth
}

func TestHealthEndpoint(t *testing.T) {
	server, _, _, _ := setupTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"status":"healthy"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestLinkRedirect(t *testing.T) {
	server, _, _, _ := setupTestAPIServer(t)

	// Missing state parameter
	reqBad := httptest.NewRequest(http.MethodGet, "/link", nil)
	wBad := httptest.NewRecorder()
	server.Handler().ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without state, got %d", wBad.Code)
	}

	// With state parameter
	req := httptest.NewRequest(http.MethodGet, "/link?state=token123", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "discord.com/oauth2/authorize") || !strings.Contains(loc, "state=token123") {
		t.Errorf("invalid redirect location: %s", loc)
	}
}

func TestDiscordOAuthCallback(t *testing.T) {
	server, db, _, oauth := setupTestAPIServer(t)
	ctx := context.Background()

	// 1. Create a valid pending link state
	token := "valid_link_state_xyz"
	if err := db.CreateLinkState(ctx, token, "1234567890", 10*time.Minute); err != nil {
		t.Fatalf("failed to create link state: %v", err)
	}

	// 2. Perform OAuth Callback
	req := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=good_code&state="+token, nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Identity Verified") {
		t.Errorf("expected success template, got: %s", w.Body.String())
	}

	// Verify user persisted
	user, err := db.GetUserByDiscordID(ctx, "1234567890")
	if err != nil {
		t.Fatalf("user was not persisted: %v", err)
	}
	if user.Username != "Test User" {
		t.Errorf("expected username 'Test User', got %s", user.Username)
	}
	if user.Role != database.RoleAdmin { // First user promoted to Admin
		t.Errorf("expected first user to be Admin, got %s", user.Role)
	}

	// Verify audit log
	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("expected audit log for oauth linking")
	}
	if logs[0].Action != "account_linked" || logs[0].Status != "SUCCESS" {
		t.Errorf("unexpected audit log: %+v", logs[0])
	}

	// 3. Attempt replay of same token (must fail 400)
	reqReplay := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=good_code&state="+token, nil)
	wReplay := httptest.NewRecorder()
	server.Handler().ServeHTTP(wReplay, reqReplay)
	if wReplay.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on consumed token replay, got %d", wReplay.Code)
	}

	// 4. Mismatched user ID should return 403
	token2 := "mismatch_state_abc"
	if err := db.CreateLinkState(ctx, token2, "9999999999", 10*time.Minute); err != nil {
		t.Fatalf("failed to create link state: %v", err)
	}

	oauth.FetchUserFunc = func(ctx context.Context, token string) (*DiscordUser, error) {
		return &DiscordUser{
			ID:       "1234567890", // Mismatch with 9999999999
			Username: "Imposter",
		}, nil
	}

	reqMismatch := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=code2&state="+token2, nil)
	wMismatch := httptest.NewRecorder()
	server.Handler().ServeHTTP(wMismatch, reqMismatch)
	if wMismatch.Code != http.StatusForbidden {
		t.Errorf("expected 403 on user mismatch, got %d", wMismatch.Code)
	}
}

func TestAdminDashboardAndRoleUpdate(t *testing.T) {
	server, db, cfg, _ := setupTestAPIServer(t)
	ctx := context.Background()

	// Seed users
	_, _ = db.UpsertUser(ctx, "d1", "Alice", database.RoleAdmin)
	u2, _ := db.UpsertUser(ctx, "d2", "Bob", database.RoleViewer)

	// 1. Unauthenticated request to /admin -> shows login form
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin", nil)
	wUnauth := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUnauth, reqUnauth)
	if !strings.Contains(wUnauth.Body.String(), "Sköll Admin Panel") {
		t.Errorf("expected login card for unauthenticated admin access")
	}

	// 2. Authenticated request with query parameter key
	reqAuth := httptest.NewRequest(http.MethodGet, "/admin?key="+cfg.AdminKey, nil)
	wAuth := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wAuth.Code)
	}
	bodyAuth := wAuth.Body.String()
	if !strings.Contains(bodyAuth, "Alice") || !strings.Contains(bodyAuth, "Bob") {
		t.Errorf("expected users to appear in admin dashboard")
	}

	// 3. Update Bob to Operator via POST /admin/role
	formData := url.Values{}
	formData.Set("user_id", fmt.Sprintf("%d", u2.ID))
	formData.Set("role", "Operator")
	formData.Set("key", cfg.AdminKey)

	reqPost := httptest.NewRequest(http.MethodPost, "/admin/role", strings.NewReader(formData.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wPost := httptest.NewRecorder()
	server.Handler().ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after role update, got %d", wPost.Code)
	}

	// Verify Bob's role changed
	bobUpdated, err := db.GetUserByID(ctx, u2.ID)
	if err != nil {
		t.Fatalf("failed to query bob: %v", err)
	}
	if bobUpdated.Role != database.RoleOperator {
		t.Errorf("expected Bob to be Operator, got %s", bobUpdated.Role)
	}

	// 4. REST API User role update
	apiReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/role", u2.ID), strings.NewReader(`{"role":"Admin"}`))
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("X-Admin-Key", cfg.AdminKey)
	wAPI := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAPI, apiReq)

	if wAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 on REST API role update, got %d: %s", wAPI.Code, wAPI.Body.String())
	}

	bobAdmin, _ := db.GetUserByID(ctx, u2.ID)
	if bobAdmin.Role != database.RoleAdmin {
		t.Errorf("expected Bob to be Admin after REST API update, got %s", bobAdmin.Role)
	}

	// 5. Audit logs endpoint
	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	auditReq.Header.Set("X-Admin-Key", cfg.AdminKey)
	wAudit := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAudit, auditReq)

	if wAudit.Code != http.StatusOK {
		t.Fatalf("expected 200 on audit logs API, got %d", wAudit.Code)
	}
	if !strings.Contains(wAudit.Body.String(), "role_update") {
		t.Errorf("expected role_update in audit logs json")
	}
}

func TestAdminStacksAndComposeViewer(t *testing.T) {
	// 1. Setup mock Fenrir daemon
	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"healthy","version":"1.2.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"name": "valheim-srv",
						"status": "running",
						"services": [
							{"name": "valheim", "image": "lloesche/valheim-server:latest", "state": "running", "status": "Up 5 hours", "ports": ["2456-2457/udp"]}
						]
					},
					{
						"name": "redis-cache",
						"status": "stopped",
						"services": [
							{"name": "redis", "image": "redis:alpine", "state": "exited", "status": "Exited (0) 10m ago"}
						]
					}
				]
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/valheim-srv/compose":
			_, _ = w.Write([]byte(`{
				"compose": "version: '3.8'\nservices:\n  valheim:\n    image: lloesche/valheim-server:latest\n    ports:\n      - \"2456-2457/udp\"\n"
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/redis-cache/compose":
			_, _ = w.Write([]byte(`{
				"compose": "version: '3.8'\nservices:\n  redis:\n    image: redis:alpine\n"
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_stacks_api.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		AdminKey:     "admin-secret",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)

	// Test 1: Authenticated request to /admin with tab=stacks
	req := httptest.NewRequest(http.MethodGet, "/admin?key=admin-secret&tab=stacks", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /admin, got %d", w.Code)
	}

	body := w.Body.String()
	// Must contain stack names
	if !strings.Contains(body, "valheim-srv") || !strings.Contains(body, "redis-cache") {
		t.Errorf("expected valheim-srv and redis-cache in HTML response")
	}
	// Must contain service details
	if !strings.Contains(body, "lloesche/valheim-server:latest") {
		t.Errorf("expected image details in HTML response")
	}
	if !strings.Contains(body, "2456-2457/udp") {
		t.Errorf("expected port in HTML response")
	}
	// Must contain compose viewer button and modal elements
	if !strings.Contains(body, "showComposeModal") || !strings.Contains(body, "composeModal") {
		t.Errorf("expected compose modal trigger in HTML response")
	}
	// Must contain compose definition inside hidden textarea
	if !strings.Contains(body, "compose-data-valheim-srv") {
		t.Errorf("expected compose textarea in HTML response")
	}

	// Test 2: Redirect /admin/stacks to /admin?tab=stacks
	reqRedirect := httptest.NewRequest(http.MethodGet, "/admin/stacks?key=admin-secret", nil)
	wRedirect := httptest.NewRecorder()
	server.Handler().ServeHTTP(wRedirect, reqRedirect)

	if wRedirect.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected 307 redirect from /admin/stacks, got %d", wRedirect.Code)
	}
	if loc := wRedirect.Header().Get("Location"); !strings.Contains(loc, "/admin?tab=stacks") {
		t.Errorf("expected redirect to /admin?tab=stacks, got %s", loc)
	}

	// Test 3: AJAX /admin/compose?name=valheim-srv
	reqCompose := httptest.NewRequest(http.MethodGet, "/admin/compose?name=valheim-srv&key=admin-secret", nil)
	wCompose := httptest.NewRecorder()
	server.Handler().ServeHTTP(wCompose, reqCompose)

	if wCompose.Code != http.StatusOK {
		t.Fatalf("expected 200 from /admin/compose, got %d: %s", wCompose.Code, wCompose.Body.String())
	}
	if !strings.Contains(wCompose.Body.String(), "lloesche/valheim-server") {
		t.Errorf("expected compose YAML in JSON response, got: %s", wCompose.Body.String())
	}

	// Test 4: REST API /api/v1/stacks
	reqAPIStacks := httptest.NewRequest(http.MethodGet, "/api/v1/stacks", nil)
	reqAPIStacks.Header.Set("X-Admin-Key", "admin-secret")
	wAPIStacks := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAPIStacks, reqAPIStacks)

	if wAPIStacks.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/v1/stacks, got %d: %s", wAPIStacks.Code, wAPIStacks.Body.String())
	}
	if !strings.Contains(wAPIStacks.Body.String(), "valheim-srv") {
		t.Errorf("expected valheim-srv in /api/v1/stacks, got: %s", wAPIStacks.Body.String())
	}

	// Test 5: REST API /api/v1/stacks/{name}/compose
	reqAPICompose := httptest.NewRequest(http.MethodGet, "/api/v1/stacks/valheim-srv/compose", nil)
	reqAPICompose.Header.Set("X-Admin-Key", "admin-secret")
	wAPICompose := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAPICompose, reqAPICompose)

	if wAPICompose.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/v1/stacks/valheim-srv/compose, got %d: %s", wAPICompose.Code, wAPICompose.Body.String())
	}
	if !strings.Contains(wAPICompose.Body.String(), "lloesche/valheim-server") {
		t.Errorf("expected compose in /api/v1/stacks/{name}/compose, got: %s", wAPICompose.Body.String())
	}
}

