package api

import (
	"context"
	"fmt"
	"io"
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

func TestComposeAndYAMLEditingIntegration(t *testing.T) {
	// 1. Mock Fenrir Daemon with compose and yaml update endpoints
	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/minecraft-srv/compose":
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"empty compose body"}`))
				return
			}
			dryRun := r.URL.Query().Get("dry_run")
			redeploy := r.URL.Query().Get("redeploy")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"dry_run":%s,"redeploy":%s}`,
				strconvBool(dryRun == "true"),
				strconvBool(redeploy != "false"))))

		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/minecraft-srv/yaml/config.yml":
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"empty yaml body"}`))
				return
			}
			restart := r.URL.Query().Get("restart")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"restart":%s}`, strconvBool(restart == "true"))))

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/minecraft-srv/yaml/config.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"content":"server:\n  port: 25565\n"}`))

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/minecraft-srv/compose":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"compose":"version: '3.8'\nservices:\n  mc:\n    image: itzg/minecraft\n"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_editing_api.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		AdminKey:     "secret-admin-key",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)

	// 1. Raw YAML to PUT /api/v1/projects/{name}/compose
	rawYAML := "version: '3.8'\nservices:\n  mc:\n    image: itzg/minecraft:latest\n"
	reqRaw := httptest.NewRequest(http.MethodPut, "/api/v1/projects/minecraft-srv/compose?redeploy=false&dry_run=true", strings.NewReader(rawYAML))
	reqRaw.Header.Set("Content-Type", "text/yaml")
	reqRaw.Header.Set("X-Admin-Key", "secret-admin-key")
	wRaw := httptest.NewRecorder()
	server.Handler().ServeHTTP(wRaw, reqRaw)

	if wRaw.Code != http.StatusOK {
		t.Fatalf("expected 200 on raw compose PUT, got %d: %s", wRaw.Code, wRaw.Body.String())
	}
	if !strings.Contains(wRaw.Body.String(), `"success":true`) {
		t.Errorf("expected success in response: %s", wRaw.Body.String())
	}

	// 2. Test JSON body variations with /api/v1/stacks/{name}/compose
	for _, key := range []string{"compose", "yaml", "content", "composeContent"} {
		jsonPayload := fmt.Sprintf(`{"%s":"version: '3.8'\nservices:\n  mc:\n    image: itzg/minecraft\n"}`, key)
		reqJSON := httptest.NewRequest(http.MethodPut, "/api/v1/stacks/minecraft-srv/compose", strings.NewReader(jsonPayload))
		reqJSON.Header.Set("Content-Type", "application/json")
		reqJSON.Header.Set("X-Admin-Key", "secret-admin-key")
		wJSON := httptest.NewRecorder()
		server.Handler().ServeHTTP(wJSON, reqJSON)

		if wJSON.Code != http.StatusOK {
			t.Fatalf("expected 200 on compose JSON key '%s', got %d: %s", key, wJSON.Code, wJSON.Body.String())
		}
	}

	// 3. Test auxiliary YAML update: PUT /api/v1/projects/{name}/yaml/{filename}
	rawConfig := "server:\n  port: 25565\n"
	reqYAML := httptest.NewRequest(http.MethodPut, "/api/v1/projects/minecraft-srv/yaml/config.yml?restart=true", strings.NewReader(rawConfig))
	reqYAML.Header.Set("Content-Type", "text/yaml")
	reqYAML.Header.Set("X-Admin-Key", "secret-admin-key")
	wYAML := httptest.NewRecorder()
	server.Handler().ServeHTTP(wYAML, reqYAML)

	if wYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 on auxiliary yaml update, got %d: %s", wYAML.Code, wYAML.Body.String())
	}
	if !strings.Contains(wYAML.Body.String(), `"restart":true`) {
		t.Errorf("expected restart:true in response: %s", wYAML.Body.String())
	}

	// 4. Test auxiliary YAML fetch: GET /api/v1/projects/{name}/yaml/{filename}
	reqGetYAML := httptest.NewRequest(http.MethodGet, "/api/v1/projects/minecraft-srv/yaml/config.yml", nil)
	reqGetYAML.Header.Set("X-Admin-Key", "secret-admin-key")
	wGetYAML := httptest.NewRecorder()
	server.Handler().ServeHTTP(wGetYAML, reqGetYAML)

	if wGetYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 on auxiliary yaml fetch, got %d: %s", wGetYAML.Code, wGetYAML.Body.String())
	}
	if !strings.Contains(wGetYAML.Body.String(), "port: 25565") {
		t.Errorf("expected port in fetched yaml: %s", wGetYAML.Body.String())
	}

	// 5. Test Admin UI endpoint POST /admin/compose
	uiForm := url.Values{}
	uiForm.Set("name", "minecraft-srv")
	uiForm.Set("compose", rawYAML)
	uiForm.Set("key", "secret-admin-key")
	reqUICompose := httptest.NewRequest(http.MethodPost, "/admin/compose", strings.NewReader(uiForm.Encode()))
	reqUICompose.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wUICompose := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUICompose, reqUICompose)

	if wUICompose.Code != http.StatusOK && wUICompose.Code != http.StatusSeeOther {
		t.Fatalf("expected 200 or 303 from /admin/compose, got %d: %s", wUICompose.Code, wUICompose.Body.String())
	}

	// 6. Test Admin UI endpoint GET /admin/yaml
	reqAdminGetYAML := httptest.NewRequest(http.MethodGet, "/admin/yaml?name=minecraft-srv&filename=config.yml&key=secret-admin-key", nil)
	wAdminGetYAML := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminGetYAML, reqAdminGetYAML)

	if wAdminGetYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 from /admin/yaml, got %d: %s", wAdminGetYAML.Code, wAdminGetYAML.Body.String())
	}
	if !strings.Contains(wAdminGetYAML.Body.String(), "port: 25565") {
		t.Errorf("expected yaml content in /admin/yaml response: %s", wAdminGetYAML.Body.String())
	}
}

func strconvBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestComposeAndYAMLRBACStrictAdminOnly(t *testing.T) {
	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_rbac_strict.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	// Seed users with different roles
	adminUser, _ := db.UpsertUser(ctx, "discord_admin", "AdminBob", database.RoleAdmin)
	opUser, _ := db.UpsertUser(ctx, "discord_op", "OperatorAlice", database.RoleOperator)
	viewerUser, _ := db.UpsertUser(ctx, "discord_viewer", "ViewerCharlie", database.RoleViewer)
	unlinkedUser, _ := db.UpsertUser(ctx, "discord_unlinked", "UnlinkedDavid", database.RoleUnlinked)

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		AdminKey:     "system-admin-secret",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)
	validYAML := "version: '3.8'\nservices:\n  app:\n    image: nginx\n"

	// 1. Operator calling PUT compose -> 403 Forbidden
	reqOp := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqOp.Header.Set("Content-Type", "text/yaml")
	reqOp.Header.Set("X-Discord-User-ID", opUser.DiscordID)
	wOp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOp, reqOp)

	if wOp.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator on compose update, got %d: %s", wOp.Code, wOp.Body.String())
	}

	// 2. Viewer calling PUT compose -> 403 Forbidden
	reqViewer := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqViewer.Header.Set("Content-Type", "text/yaml")
	reqViewer.Header.Set("X-Discord-User-ID", viewerUser.DiscordID)
	wViewer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewer, reqViewer)

	if wViewer.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer on compose update, got %d: %s", wViewer.Code, wViewer.Body.String())
	}

	// 3. User with explicit RoleUnlinked -> 403 Forbidden
	reqUnlinkedRole := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqUnlinkedRole.Header.Set("Content-Type", "text/yaml")
	reqUnlinkedRole.Header.Set("X-Discord-User-ID", unlinkedUser.DiscordID)
	wUnlinkedRole := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUnlinkedRole, reqUnlinkedRole)

	if wUnlinkedRole.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Unlinked role on compose update, got %d: %s", wUnlinkedRole.Code, wUnlinkedRole.Body.String())
	}

	// 4. Completely unlinked / unknown user ID -> 403 Forbidden
	reqUnknown := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqUnknown.Header.Set("Content-Type", "text/yaml")
	reqUnknown.Header.Set("X-Discord-User-ID", "unknown_user_999999")
	wUnknown := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUnknown, reqUnknown)

	if wUnknown.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for unlinked user on compose update, got %d: %s", wUnknown.Code, wUnknown.Body.String())
	}

	// 5. Explicit X-Role: Operator or Viewer header -> 403 Forbidden
	reqRoleOp := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqRoleOp.Header.Set("Content-Type", "text/yaml")
	reqRoleOp.Header.Set("X-Role", "Operator")
	wRoleOp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wRoleOp, reqRoleOp)

	if wRoleOp.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for X-Role: Operator, got %d", wRoleOp.Code)
	}

	// 6. Anonymous request without credentials -> 403 Forbidden
	reqAnon := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqAnon.Header.Set("Content-Type", "text/yaml")
	wAnon := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAnon, reqAnon)

	if wAnon.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for anonymous request, got %d", wAnon.Code)
	}

	// 7. Auxiliary YAML editing routes strictly forbidden for non-admins
	for _, u := range []*database.User{opUser, viewerUser, unlinkedUser} {
		reqYAMLForbidden := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/yaml/config.yml", strings.NewReader("config: true"))
		reqYAMLForbidden.Header.Set("Content-Type", "text/yaml")
		reqYAMLForbidden.Header.Set("X-Discord-User-ID", u.DiscordID)
		wYF := httptest.NewRecorder()
		server.Handler().ServeHTTP(wYF, reqYAMLForbidden)

		if wYF.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden on YAML route for user %s (role %s), got %d", u.Username, u.Role, wYF.Code)
		}
	}

	// 8. Admin Discord User -> 200 OK
	reqAdmin := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqAdmin.Header.Set("Content-Type", "text/yaml")
	reqAdmin.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdmin := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdmin, reqAdmin)

	if wAdmin.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Admin user, got %d: %s", wAdmin.Code, wAdmin.Body.String())
	}

	// 9. Admin via X-Admin-Key -> 200 OK
	reqAdminKey := httptest.NewRequest(http.MethodPut, "/api/v1/projects/my-app/compose", strings.NewReader(validYAML))
	reqAdminKey.Header.Set("Content-Type", "text/yaml")
	reqAdminKey.Header.Set("X-Admin-Key", "system-admin-secret")
	wAdminKey := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminKey, reqAdminKey)

	if wAdminKey.Code != http.StatusOK {
		t.Errorf("expected 200 OK for AdminKey auth, got %d: %s", wAdminKey.Code, wAdminKey.Body.String())
	}

	// 10. Verify Audit Log was recorded
	logs, err := db.ListAuditLogs(ctx, 20)
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	hasUnauthorized := false
	hasSuccess := false
	for _, l := range logs {
		if l.Status == "UNAUTHORIZED" {
			hasUnauthorized = true
		}
		if l.Status == "SUCCESS" {
			hasSuccess = true
		}
	}
	if !hasUnauthorized {
		t.Errorf("expected UNAUTHORIZED entry in audit logs")
	}
	if !hasSuccess {
		t.Errorf("expected SUCCESS entry in audit logs")
	}
}


