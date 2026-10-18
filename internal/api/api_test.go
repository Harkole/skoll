package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	server, db, _, _ := setupTestAPIServer(t)
	ctx := context.Background()

	// Seed users
	_, _ = db.UpsertUser(ctx, "d1", "Alice", database.RoleAdmin)
	u2, _ := db.UpsertUser(ctx, "d2", "Bob", database.RoleViewer)

	// 1. Unauthenticated request to /admin -> redirects to /login
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin", nil)
	wUnauth := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect to /login for unauthenticated admin access, got %d", wUnauth.Code)
	}
	if loc := wUnauth.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect location /login, got %s", loc)
	}

	// Clean login page renders with "Login with Discord" button
	reqLogin := httptest.NewRequest(http.MethodGet, "/login", nil)
	wLogin := httptest.NewRecorder()
	server.Handler().ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Errorf("expected 200 on /login, got %d", wLogin.Code)
	}
	if !strings.Contains(wLogin.Body.String(), "Login with Discord") {
		t.Errorf("expected Login with Discord button on login page, got: %s", wLogin.Body.String())
	}

	// 2. Authenticated request with session cookie
	aliceUser, _ := db.GetUserByDiscordID(ctx, "d1")
	sessionEnc, err := server.sessions.Encrypt(&SessionData{
		DiscordUserID: aliceUser.DiscordID,
		Username:      aliceUser.Username,
		Role:          aliceUser.Role,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("failed to encrypt session: %v", err)
	}

	reqAuth := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAuth.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: sessionEnc,
	})
	wAuth := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wAuth.Code)
	}
	bodyAuth := wAuth.Body.String()
	if !strings.Contains(bodyAuth, "Alice") || !strings.Contains(bodyAuth, "Bob") {
		t.Errorf("expected users to appear in admin dashboard")
	}

	// 3. Update Bob to Operator via POST /admin/role with Alice's session cookie
	formData := url.Values{}
	formData.Set("user_id", fmt.Sprintf("%d", u2.ID))
	formData.Set("role", "Operator")

	reqPost := httptest.NewRequest(http.MethodPost, "/admin/role", strings.NewReader(formData.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: sessionEnc,
	})
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

	// 4. REST API User role update using session cookie
	apiReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/role", u2.ID), strings.NewReader(`{"role":"Admin"}`))
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: sessionEnc,
	})
	wAPI := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAPI, apiReq)

	if wAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 on REST API role update, got %d: %s", wAPI.Code, wAPI.Body.String())
	}

	bobAdmin, _ := db.GetUserByID(ctx, u2.ID)
	if bobAdmin.Role != database.RoleAdmin {
		t.Errorf("expected Bob to be Admin after REST API update, got %s", bobAdmin.Role)
	}

	// 5. Audit logs endpoint and verify unified audit trail contains Alice's Discord info
	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	auditReq.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: sessionEnc,
	})
	wAudit := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAudit, auditReq)

	if wAudit.Code != http.StatusOK {
		t.Fatalf("expected 200 on audit logs API, got %d", wAudit.Code)
	}
	auditBody := wAudit.Body.String()
	if !strings.Contains(auditBody, "role_update") {
		t.Errorf("expected role_update in audit logs json")
	}
	if !strings.Contains(auditBody, "d1") || !strings.Contains(auditBody, "Alice") {
		t.Errorf("expected Alice's Discord ID and username in audit logs, got: %s", auditBody)
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
			_, _ = w.Write(fmt.Appendf(nil, `{"success":true,"dry_run":%s,"redeploy":%s}`,
				strconvBool(dryRun == "true"),
				strconvBool(redeploy != "false")))

		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/minecraft-srv/yaml/config.yml":
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"empty yaml body"}`))
				return
			}
			restart := r.URL.Query().Get("restart")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fmt.Appendf(nil, `{"success":true,"restart":%s}`, strconvBool(restart == "true")))

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

func TestDiscordOAuth2LoginEndpoint(t *testing.T) {
	server, _, cfg, _ := setupTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 temporary redirect, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "discord.com/oauth2/authorize") {
		t.Errorf("expected Discord authorization URL, got: %s", loc)
	}
	if !strings.Contains(loc, "client_id="+cfg.DiscordClientID) {
		t.Errorf("expected client_id in OAuth URL: %s", loc)
	}
	if !strings.Contains(loc, "scope=identify") {
		t.Errorf("expected identify scope in OAuth URL: %s", loc)
	}
	if !strings.Contains(loc, "response_type=code") {
		t.Errorf("expected response_type=code: %s", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Errorf("expected state token in OAuth URL: %s", loc)
	}

	// Verify temporary oauth state cookie was set
	cookies := w.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "skoll_oauth_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Errorf("expected skoll_oauth_state cookie to be set")
	}
}

func TestDiscordOAuth2CallbackWebLoginAndBootstrap(t *testing.T) {
	server, db, _, oauth := setupTestAPIServer(t)
	ctx := context.Background()

	// Configure mock user for first login
	oauth.FetchUserFunc = func(ctx context.Context, token string) (*DiscordUser, error) {
		return &DiscordUser{
			ID:         "snowflake_admin_1",
			Username:   "InitialAdmin",
			GlobalName: "InitialAdmin",
		}, nil
	}

	// 1. Initiate login
	reqLogin := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	wLogin := httptest.NewRecorder()
	server.Handler().ServeHTTP(wLogin, reqLogin)

	parsedURL, err := url.Parse(wLogin.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	stateToken := parsedURL.Query().Get("state")
	if stateToken == "" {
		t.Fatalf("state token missing from login redirect URL")
	}

	// 2. Discord calls /auth/callback with code and state
	reqCallback := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mock_code_1&state="+stateToken, nil)
	wCallback := httptest.NewRecorder()
	server.Handler().ServeHTTP(wCallback, reqCallback)

	if wCallback.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect to /admin, got %d: %s", wCallback.Code, wCallback.Body.String())
	}
	if loc := wCallback.Header().Get("Location"); loc != "/admin" {
		t.Errorf("expected redirect to /admin, got %s", loc)
	}

	// Verify session cookie was set
	cookies := wCallback.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected %s cookie to be set", SessionCookieName)
	}
	if !sessionCookie.HttpOnly {
		t.Errorf("expected session cookie to be HttpOnly")
	}

	// Verify first user bootstrap: InitialAdmin MUST be RoleAdmin
	user1, err := db.GetUserByDiscordID(ctx, "snowflake_admin_1")
	if err != nil {
		t.Fatalf("failed to find user1: %v", err)
	}
	if user1.Role != database.RoleAdmin {
		t.Errorf("expected first user to receive RoleAdmin, got %s", user1.Role)
	}

	// 3. Second user logs in
	oauth.FetchUserFunc = func(ctx context.Context, token string) (*DiscordUser, error) {
		return &DiscordUser{
			ID:         "snowflake_user_2",
			Username:   "SecondUser",
			GlobalName: "Second User",
		}, nil
	}

	reqLogin2 := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	wLogin2 := httptest.NewRecorder()
	server.Handler().ServeHTTP(wLogin2, reqLogin2)
	parsedURL2, _ := url.Parse(wLogin2.Header().Get("Location"))
	stateToken2 := parsedURL2.Query().Get("state")

	reqCallback2 := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mock_code_2&state="+stateToken2, nil)
	wCallback2 := httptest.NewRecorder()
	server.Handler().ServeHTTP(wCallback2, reqCallback2)

	if wCallback2.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for second user, got %d", wCallback2.Code)
	}

	var sessionCookie2 *http.Cookie
	for _, c := range wCallback2.Result().Cookies() {
		if c.Name == SessionCookieName {
			sessionCookie2 = c
			break
		}
	}
	if sessionCookie2 == nil {
		t.Fatalf("expected session cookie for second user")
	}

	// Second user MUST receive RoleViewer (not admin)
	user2, err := db.GetUserByDiscordID(ctx, "snowflake_user_2")
	if err != nil {
		t.Fatalf("failed to find user2: %v", err)
	}
	if user2.Role != database.RoleViewer {
		t.Errorf("expected second user to have RoleViewer, got %s", user2.Role)
	}

	// 4. Second user (RoleViewer) visits /admin -> MUST be blocked with 403 Forbidden!
	reqAdminViewer := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAdminViewer.AddCookie(sessionCookie2)
	wAdminViewer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminViewer, reqAdminViewer)

	if wAdminViewer.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer accessing /admin, got %d", wAdminViewer.Code)
	}
	if !strings.Contains(wAdminViewer.Body.String(), "403 Forbidden") {
		t.Errorf("expected 403 Forbidden page, got: %s", wAdminViewer.Body.String())
	}

	// 5. First user (RoleAdmin) visits /admin -> MUST succeed with 200 OK!
	reqAdminFirst := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAdminFirst.AddCookie(sessionCookie)
	wAdminFirst := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminFirst, reqAdminFirst)

	if wAdminFirst.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Admin accessing /admin, got %d", wAdminFirst.Code)
	}
	if !strings.Contains(wAdminFirst.Body.String(), "InitialAdmin") {
		t.Errorf("expected InitialAdmin in dashboard view")
	}
}

func TestWebRBACRoleHierarchy(t *testing.T) {
	server, db, _, _ := setupTestAPIServer(t)
	ctx := context.Background()

	admin, _ := db.UpsertUser(ctx, "d_admin", "AdminUser", database.RoleAdmin)
	op, _ := db.UpsertUser(ctx, "d_op", "OperatorUser", database.RoleOperator)
	viewer, _ := db.UpsertUser(ctx, "d_viewer", "ViewerUser", database.RoleViewer)

	createCookie := func(u *database.User) *http.Cookie {
		enc, _ := server.sessions.Encrypt(&SessionData{
			DiscordUserID: u.DiscordID,
			Username:      u.Username,
			Role:          u.Role,
			CreatedAt:     time.Now().Unix(),
			ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
		})
		return &http.Cookie{
			Name:  SessionCookieName,
			Value: enc,
		}
	}

	// 1. Viewer cannot view admin dashboard (403)
	reqViewer := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqViewer.AddCookie(createCookie(viewer))
	wViewer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewer, reqViewer)
	if wViewer.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer on /admin, got %d", wViewer.Code)
	}

	// 2. Operator CAN view admin dashboard (200 OK)
	reqOp := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqOp.AddCookie(createCookie(op))
	wOp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOp, reqOp)
	if wOp.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Operator on /admin, got %d", wOp.Code)
	}
	if !strings.Contains(wOp.Body.String(), "OperatorUser") {
		t.Errorf("expected OperatorUser in header")
	}

	// 3. Operator CANNOT update user roles (403 Forbidden)
	formData := url.Values{}
	formData.Set("user_id", fmt.Sprintf("%d", viewer.ID))
	formData.Set("role", "Operator")
	reqOpRole := httptest.NewRequest(http.MethodPost, "/admin/role", strings.NewReader(formData.Encode()))
	reqOpRole.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOpRole.AddCookie(createCookie(op))
	wOpRole := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpRole, reqOpRole)
	if wOpRole.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator updating roles, got %d", wOpRole.Code)
	}

	// 4. Admin CAN update user roles (303 Redirect)
	reqAdminRole := httptest.NewRequest(http.MethodPost, "/admin/role", strings.NewReader(formData.Encode()))
	reqAdminRole.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAdminRole.AddCookie(createCookie(admin))
	wAdminRole := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminRole, reqAdminRole)
	if wAdminRole.Code != http.StatusSeeOther {
		t.Errorf("expected 303 Redirect for Admin updating roles, got %d", wAdminRole.Code)
	}

	// 5. Verify audit log has Admin's Discord identity
	logs, err := db.ListAuditLogs(ctx, 5)
	if err != nil || len(logs) == 0 {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	latest := logs[0]
	if latest.Action != "role_update" || latest.DiscordUserID != admin.DiscordID || latest.Username != admin.Username {
		t.Errorf("expected role_update log with admin identity, got: %+v", latest)
	}
}

func TestWebUnifiedAuditTrailIntegration(t *testing.T) {
	// Setup mock Fenrir server
	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/compose"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/yaml/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/restart"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_audit_trail.db")
	db, _ := database.Open(dbPath)
	defer db.Close()

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)
	ctx := context.Background()

	adminUser, _ := db.UpsertUser(ctx, "discord_admin_999", "AdminSarah", database.RoleAdmin)
	adminCookieVal, _ := server.sessions.Encrypt(&SessionData{
		DiscordUserID: adminUser.DiscordID,
		Username:      adminUser.Username,
		Role:          adminUser.Role,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	})
	adminCookie := &http.Cookie{Name: SessionCookieName, Value: adminCookieVal}

	opUser, _ := db.UpsertUser(ctx, "discord_op_888", "OperatorMike", database.RoleOperator)
	opCookieVal, _ := server.sessions.Encrypt(&SessionData{
		DiscordUserID: opUser.DiscordID,
		Username:      opUser.Username,
		Role:          opUser.Role,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	})
	opCookie := &http.Cookie{Name: SessionCookieName, Value: opCookieVal}

	// 1. Compose Update via Web Session -> Must record AdminSarah's Discord ID & Username
	composeYAML := "version: '3.8'\nservices:\n  app:\n    image: nginx:latest\n"
	reqCompose := httptest.NewRequest(http.MethodPut, "/api/v1/projects/production-stack/compose", strings.NewReader(composeYAML))
	reqCompose.Header.Set("Content-Type", "text/yaml")
	reqCompose.AddCookie(adminCookie)
	wCompose := httptest.NewRecorder()
	server.Handler().ServeHTTP(wCompose, reqCompose)
	if wCompose.Code != http.StatusOK {
		t.Fatalf("expected 200 on compose update, got %d: %s", wCompose.Code, wCompose.Body.String())
	}

	// 2. YAML Edit via Web Session -> Must record AdminSarah's Discord ID & Username
	yamlContent := "setting: enabled\n"
	reqYAML := httptest.NewRequest(http.MethodPut, "/api/v1/projects/production-stack/yaml/app.yaml", strings.NewReader(yamlContent))
	reqYAML.Header.Set("Content-Type", "text/yaml")
	reqYAML.AddCookie(adminCookie)
	wYAML := httptest.NewRecorder()
	server.Handler().ServeHTTP(wYAML, reqYAML)
	if wYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 on yaml update, got %d: %s", wYAML.Code, wYAML.Body.String())
	}

	// 3. Stack Restart via Web Session (OperatorMike) -> Must record "server_restart" matching Discord bot format!
	reqRestart := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/production-stack/restart", nil)
	reqRestart.AddCookie(opCookie)
	wRestart := httptest.NewRecorder()
	server.Handler().ServeHTTP(wRestart, reqRestart)
	if wRestart.Code != http.StatusOK {
		t.Fatalf("expected 200 on stack restart, got %d: %s", wRestart.Code, wRestart.Body.String())
	}

	// 4. Verify all audit log entries
	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}

	var foundRestart, foundCompose, foundYAML bool
	for _, l := range logs {
		// Verify none of them fell back to generic "administrator"
		if l.Username == "administrator" {
			t.Errorf("unexpected generic 'administrator' in audit log: %+v", l)
		}

		if l.Action == "server_restart" && l.TargetProject == "production-stack" {
			foundRestart = true
			if l.DiscordUserID != opUser.DiscordID || l.Username != opUser.Username {
				t.Errorf("server_restart had wrong user: %+v", l)
			}
			if l.Status != "SUCCESS" {
				t.Errorf("server_restart status not SUCCESS: %+v", l)
			}
		}

		if l.Action == "compose_update" && l.TargetProject == "production-stack" {
			foundCompose = true
			if l.DiscordUserID != adminUser.DiscordID || l.Username != adminUser.Username {
				t.Errorf("compose_update had wrong user: %+v", l)
			}
			if l.Status != "SUCCESS" {
				t.Errorf("compose_update status not SUCCESS: %+v", l)
			}
		}

		if l.Action == "yaml_update" && l.TargetProject == "production-stack/app.yaml" {
			foundYAML = true
			if l.DiscordUserID != adminUser.DiscordID || l.Username != adminUser.Username {
				t.Errorf("yaml_update had wrong user: %+v", l)
			}
			if l.Status != "SUCCESS" {
				t.Errorf("yaml_update status not SUCCESS: %+v", l)
			}
		}
	}

	if !foundRestart {
		t.Errorf("server_restart audit entry not found")
	}
	if !foundCompose {
		t.Errorf("compose_update audit entry not found")
	}
	if !foundYAML {
		t.Errorf("yaml_update audit entry not found")
	}
}

func TestSessionManagerEncryptionAndTampering(t *testing.T) {
	sm := NewSessionManager("test_secret_key_1234567890_abcdef")

	orig := &SessionData{
		DiscordUserID: "user_123",
		Username:      "TestUser",
		Role:          database.RoleAdmin,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     time.Now().Add(1 * time.Hour).Unix(),
	}

	// 1. Encryption & Decryption roundtrip
	encrypted, err := sm.Encrypt(orig)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	decrypted, err := sm.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}
	if decrypted.DiscordUserID != orig.DiscordUserID || decrypted.Username != orig.Username || decrypted.Role != orig.Role {
		t.Errorf("decrypted session does not match original: %+v", decrypted)
	}

	// 2. Tampering detection
	rawBytes, _ := base64.URLEncoding.DecodeString(encrypted)
	rawBytes[len(rawBytes)-1] ^= 0xFF // Flip last bit
	tampered := base64.URLEncoding.EncodeToString(rawBytes)

	if _, err := sm.Decrypt(tampered); err == nil {
		t.Errorf("expected decryption to fail on tampered ciphertext")
	}

	// 3. Expiration detection
	expired := &SessionData{
		DiscordUserID: "user_123",
		Username:      "TestUser",
		Role:          database.RoleAdmin,
		CreatedAt:     time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt:     time.Now().Add(-1 * time.Hour).Unix(),
	}
	encExpired, _ := sm.Encrypt(expired)
	if _, err := sm.Decrypt(encExpired); err == nil {
		t.Errorf("expected decryption to fail on expired session")
	}
}

func TestAuthLogoutEndpoint(t *testing.T) {
	server, db, _, _ := setupTestAPIServer(t)
	ctx := context.Background()

	user, _ := db.UpsertUser(ctx, "d_logout", "LogoutUser", database.RoleOperator)
	enc, _ := server.sessions.Encrypt(&SessionData{
		DiscordUserID: user.DiscordID,
		Username:      user.Username,
		Role:          user.Role,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: enc,
	})
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on logout, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}

	// Verify cookie was cleared
	var clearedCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			clearedCookie = c
			break
		}
	}
	if clearedCookie == nil || clearedCookie.MaxAge != -1 {
		t.Errorf("expected session cookie to have MaxAge -1")
	}

	// Verify audit log
	logs, err := db.ListAuditLogs(ctx, 5)
	if err != nil || len(logs) == 0 {
		t.Fatalf("expected audit log for logout")
	}
	if logs[0].Action != "logout" || logs[0].Username != "LogoutUser" {
		t.Errorf("unexpected audit log: %+v", logs[0])
	}
}

func TestDestructiveAndControlEndpointsStrictAdminRBAC(t *testing.T) {
	// 1. Mock Fenrir Daemon with delete, deploy, and container endpoints
	deletedStacks := make(map[string]bool)
	deployedStacks := make(map[string]string)
	containerActions := make(map[string]string)

	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
			deletedStacks[name] = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "deleted", "project": name})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			deployedStacks[req["name"]] = req["compose"]
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "created"})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/projects/") && strings.HasSuffix(r.URL.Path, "/restart"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "restarted"})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/projects/") && strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "started"})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/projects/") && strings.HasSuffix(r.URL.Path, "/stop"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "stopped"})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/containers/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/containers/"), "/")
			if len(parts) == 2 {
				containerActions[parts[0]] = parts[1] // container: action
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_destructive_rbac.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	adminUser, _ := db.UpsertUser(ctx, "discord_admin_1", "AdminAlice", database.RoleAdmin)
	operatorUser, _ := db.UpsertUser(ctx, "discord_op_1", "OperatorBob", database.RoleOperator)
	viewerUser, _ := db.UpsertUser(ctx, "discord_viewer_1", "ViewerCharlie", database.RoleViewer)

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		AdminKey:     "master-admin-secret",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)

	// --- 1. Stack Deletion (DELETE /api/v1/stacks/{name} & /projects/{name}) ---
	// A. Viewer attempt -> 403
	reqViewerDel := httptest.NewRequest(http.MethodDelete, "/api/v1/stacks/prod-minecraft", nil)
	reqViewerDel.Header.Set("X-Discord-User-ID", viewerUser.DiscordID)
	wViewerDel := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewerDel, reqViewerDel)
	if wViewerDel.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer stack delete, got %d", wViewerDel.Code)
	}

	// B. Operator attempt -> 403 Forbidden (Destructive deletion is strictly Admin-only!)
	reqOpDel := httptest.NewRequest(http.MethodDelete, "/api/v1/stacks/prod-minecraft", nil)
	reqOpDel.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpDel := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpDel, reqOpDel)
	if wOpDel.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator stack delete, got %d", wOpDel.Code)
	}

	// C. Admin attempt via /api/v1/stacks/{name} -> 200 OK
	reqAdminDel := httptest.NewRequest(http.MethodDelete, "/api/v1/stacks/prod-minecraft", nil)
	reqAdminDel.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminDel := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminDel, reqAdminDel)
	if wAdminDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Admin stack delete, got %d: %s", wAdminDel.Code, wAdminDel.Body.String())
	}
	if !deletedStacks["prod-minecraft"] {
		t.Errorf("expected stack prod-minecraft to be deleted in Fenrir")
	}

	// D. Admin attempt via /api/v1/projects/{name} alias -> 200 OK
	reqAdminProjDel := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/valheim-srv", nil)
	reqAdminProjDel.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminProjDel := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminProjDel, reqAdminProjDel)
	if wAdminProjDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Admin project delete, got %d: %s", wAdminProjDel.Code, wAdminProjDel.Body.String())
	}
	if !deletedStacks["valheim-srv"] {
		t.Errorf("expected stack valheim-srv to be deleted in Fenrir")
	}

	// --- 2. Stack Deletion via Web Form (POST /admin/stacks/action with action=delete) ---
	// Operator attempt -> 403
	formOp := url.Values{"name": {"valheim-srv"}, "action": {"delete"}}
	reqOpForm := httptest.NewRequest(http.MethodPost, "/admin/stacks/action", strings.NewReader(formOp.Encode()))
	reqOpForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOpForm.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpForm := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpForm, reqOpForm)
	if wOpForm.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator web delete action, got %d", wOpForm.Code)
	}

	// Admin attempt -> 303 redirect
	formAdmin := url.Values{"name": {"valheim-srv"}, "action": {"delete"}}
	reqAdminForm := httptest.NewRequest(http.MethodPost, "/admin/stacks/action", strings.NewReader(formAdmin.Encode()))
	reqAdminForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAdminForm.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminForm := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminForm, reqAdminForm)
	if wAdminForm.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 Redirect for Admin web delete action, got %d", wAdminForm.Code)
	}

	// --- 3. Stack Provisioning (POST /api/v1/stacks & POST /api/v1/projects) ---
	deployBody := `{"name":"new-game","compose":"services:\n  game:\n    image: game:latest\n"}`
	// Operator attempt -> 403 Forbidden (Admin only)
	reqOpDeploy := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", strings.NewReader(deployBody))
	reqOpDeploy.Header.Set("Content-Type", "application/json")
	reqOpDeploy.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpDeploy := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpDeploy, reqOpDeploy)
	if wOpDeploy.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator stack deploy, got %d", wOpDeploy.Code)
	}

	// Admin attempt -> 202 Accepted
	reqAdminDeploy := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", strings.NewReader(deployBody))
	reqAdminDeploy.Header.Set("Content-Type", "application/json")
	reqAdminDeploy.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminDeploy := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminDeploy, reqAdminDeploy)
	if wAdminDeploy.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for Admin stack deploy, got %d: %s", wAdminDeploy.Code, wAdminDeploy.Body.String())
	}
	if deployedStacks["new-game"] == "" {
		t.Errorf("expected stack new-game to be deployed")
	}

	// --- 4. Stack Operational Controls (POST /api/v1/stacks/{name}/restart & Web Form) ---
	// Viewer attempt -> 403 Forbidden
	reqViewerRestart := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/prod-minecraft/restart", nil)
	reqViewerRestart.Header.Set("X-Discord-User-ID", viewerUser.DiscordID)
	wViewerRestart := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewerRestart, reqViewerRestart)
	if wViewerRestart.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer stack restart, got %d", wViewerRestart.Code)
	}

	// Operator attempt -> 200 OK (Operators are permitted)
	reqOpRestart := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/prod-minecraft/restart", nil)
	reqOpRestart.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpRestart := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpRestart, reqOpRestart)
	if wOpRestart.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Operator stack restart, got %d: %s", wOpRestart.Code, wOpRestart.Body.String())
	}

	// Operator web form action -> 303 Redirect
	formOpRestart := url.Values{"name": {"prod-minecraft"}, "action": {"restart"}}
	reqOpFormRestart := httptest.NewRequest(http.MethodPost, "/admin/stacks/action", strings.NewReader(formOpRestart.Encode()))
	reqOpFormRestart.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOpFormRestart.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpFormRestart := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpFormRestart, reqOpFormRestart)
	if wOpFormRestart.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 Redirect for Operator web restart action, got %d", wOpFormRestart.Code)
	}

	// --- 5. Container Controls (POST /api/v1/containers/{name}/start, stop, restart) ---
	// Viewer attempt -> 403 Forbidden
	reqViewerContainer := httptest.NewRequest(http.MethodPost, "/api/v1/containers/mc-container/restart", nil)
	reqViewerContainer.Header.Set("X-Discord-User-ID", viewerUser.DiscordID)
	wViewerContainer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewerContainer, reqViewerContainer)
	if wViewerContainer.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer container restart, got %d", wViewerContainer.Code)
	}

	// Operator attempt -> 200 OK (Operators are permitted)
	for _, act := range []string{"start", "stop", "restart"} {
		reqOpContainer := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/containers/mc-container/%s", act), nil)
		reqOpContainer.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
		wOpContainer := httptest.NewRecorder()
		server.Handler().ServeHTTP(wOpContainer, reqOpContainer)
		if wOpContainer.Code != http.StatusOK {
			t.Errorf("expected 200 OK for Operator container %s, got %d: %s", act, wOpContainer.Code, wOpContainer.Body.String())
		}
		if containerActions["mc-container"] != act {
			t.Errorf("expected container action %s in Fenrir, got %s", act, containerActions["mc-container"])
		}
	}

	// Admin attempt -> 200 OK (Admins are permitted)
	reqAdminContainer := httptest.NewRequest(http.MethodPost, "/api/v1/containers/mc-container/restart", nil)
	reqAdminContainer.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminContainer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminContainer, reqAdminContainer)
	if wAdminContainer.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Admin container restart, got %d", wAdminContainer.Code)
	}

	// --- 6. Verify SQLite Audit Logs ---
	logs, err := db.ListAuditLogs(ctx, 50)
	if err != nil {
		t.Fatalf("failed to list audit logs: %v", err)
	}

	foundUnauthorizedDelete := false
	foundSuccessDelete := false
	foundUnauthorizedDeploy := false
	foundSuccessDeploy := false
	foundOperatorContainerControl := false
	foundOperatorStackControl := false
	foundViewerUnauthorizedContainer := false

	for _, l := range logs {
		if l.Action == "stack_delete" && l.Status == "UNAUTHORIZED" && l.Username == operatorUser.Username {
			foundUnauthorizedDelete = true
		}
		if l.Action == "stack_delete" && l.Status == "SUCCESS" && l.Username == adminUser.Username {
			foundSuccessDelete = true
		}
		if l.Action == "stack_deploy" && l.Status == "UNAUTHORIZED" && l.Username == operatorUser.Username {
			foundUnauthorizedDeploy = true
		}
		if l.Action == "stack_deploy" && l.Status == "SUCCESS" && l.Username == adminUser.Username {
			foundSuccessDeploy = true
		}
		if strings.HasPrefix(l.Action, "container_") && l.Status == "SUCCESS" && l.Username == operatorUser.Username {
			foundOperatorContainerControl = true
		}
		if strings.HasPrefix(l.Action, "server_") && l.Status == "SUCCESS" && l.Username == operatorUser.Username {
			foundOperatorStackControl = true
		}
		if strings.HasPrefix(l.Action, "container_") && l.Status == "UNAUTHORIZED" && l.Username == viewerUser.Username {
			foundViewerUnauthorizedContainer = true
		}
	}

	if !foundUnauthorizedDelete {
		t.Errorf("missing audit log for UNAUTHORIZED stack_delete by operator")
	}
	if !foundSuccessDelete {
		t.Errorf("missing audit log for SUCCESS stack_delete by admin")
	}
	if !foundUnauthorizedDeploy {
		t.Errorf("missing audit log for UNAUTHORIZED stack_deploy by operator")
	}
	if !foundSuccessDeploy {
		t.Errorf("missing audit log for SUCCESS stack_deploy by admin")
	}
	if !foundOperatorContainerControl {
		t.Errorf("missing audit log for SUCCESS container control by operator")
	}
	if !foundOperatorStackControl {
		t.Errorf("missing audit log for SUCCESS stack control by operator")
	}
	if !foundViewerUnauthorizedContainer {
		t.Errorf("missing audit log for UNAUTHORIZED container control by viewer")
	}
}

func TestAdminDashboardArchives(t *testing.T) {
	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/api/v1/health"):
			_, _ = w.Write([]byte(`{"status":"healthy","version":"1.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/archives":
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": 101,
						"projectName": "production-app",
						"fileSize": 10485760,
						"createdAt": "2026-10-01T12:00:00Z",
						"expiresAt": "2026-12-30T12:00:00Z",
						"daysUntilDeletion": 73
					}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_admin_archives.db")
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

	// 1. Verify /admin/archives redirects to /admin?tab=archives
	reqRedir := httptest.NewRequest(http.MethodGet, "/admin/archives?key=admin-secret", nil)
	wRedir := httptest.NewRecorder()
	server.Handler().ServeHTTP(wRedir, reqRedir)
	if wRedir.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect from /admin/archives, got %d", wRedir.Code)
	}
	if loc := wRedir.Header().Get("Location"); !strings.Contains(loc, "tab=archives") {
		t.Errorf("expected location to contain tab=archives, got %s", loc)
	}

	// 2. Fetch /admin dashboard with tab=archives
	req := httptest.NewRequest(http.MethodGet, "/admin?key=admin-secret&tab=archives", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /admin, got %d", w.Code)
	}

	body := w.Body.String()

	// Check Tab Button
	if !strings.Contains(body, `id="tab-btn-archives"`) {
		t.Errorf("missing tab-btn-archives in admin UI")
	}
	if !strings.Contains(body, "Archived Stacks (1)") {
		t.Errorf("missing archive count in tab button")
	}

	// Check Tab Panel
	if !strings.Contains(body, `id="tab-panel-archives"`) {
		t.Errorf("missing tab-panel-archives in admin UI")
	}

	// Check Archive Metadata Rendering
	if !strings.Contains(body, "production-app") {
		t.Errorf("missing archive project name in UI")
	}
	if !strings.Contains(body, "10.00 MB") {
		t.Errorf("missing formatted file size in UI")
	}
	if !strings.Contains(body, "73 days remaining") {
		t.Errorf("missing days remaining badge in UI")
	}

	// Check Actions: Restore button and Purge Now button
	if !strings.Contains(body, "action=\"/admin/archives/action\"") {
		t.Errorf("missing form action for archives")
	}
	if !strings.Contains(body, "openPurgeModal('101', 'production-app')") {
		t.Errorf("missing openPurgeModal trigger on Purge Now button")
	}

	// Check Purge Confirmation Modal
	if !strings.Contains(body, `id="purgeModal"`) {
		t.Errorf("missing purge confirmation modal in UI")
	}
	if !strings.Contains(body, "Confirm Permanent Archive Purge") {
		t.Errorf("missing confirmation modal title in UI")
	}
	if !strings.Contains(body, "PERMANENT &amp; IRREVERSIBLE ACTION") {
		t.Errorf("missing permanent warning in confirmation modal")
	}
}

func TestAPIArchivesEndpoints(t *testing.T) {
	var restoreCalledWith string
	var deleteCalledWith string

	fenrirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/archives":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"id":                55,
						"projectName":       "analytics-stack",
						"fileSize":          1024,
						"createdAt":         time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339),
						"expiresAt":         time.Now().Add(80 * 24 * time.Hour).Format(time.RFC3339),
						"daysUntilDeletion": 80,
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/archives/55/restore":
			restoreCalledWith = "55"
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/archives/55":
			deleteCalledWith = "55"
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer fenrirServer.Close()

	fenrirClient := fenrir.NewClient(fenrirServer.URL, "test-key", 5*time.Second)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_archives.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	adminUser, _ := db.UpsertUser(ctx, "discord_admin", "AdminUser", database.RoleAdmin)
	operatorUser, _ := db.UpsertUser(ctx, "discord_operator", "OperatorUser", database.RoleOperator)
	viewerUser, _ := db.UpsertUser(ctx, "discord_viewer", "ViewerUser", database.RoleViewer)

	cfg := &config.Config{
		Port:         ":8081",
		PublicURL:    "http://localhost:8081",
		AdminKey:     "admin-secret",
		DatabasePath: dbPath,
		HTTPTimeout:  5 * time.Second,
	}

	server := NewServer(cfg, db, fenrirClient, nil)

	// 1. GET /api/v1/archives RBAC
	// Unlinked / unauthorized:
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/archives", nil)
	wUnauth := httptest.NewRecorder()
	server.Handler().ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unlinked caller on GET /api/v1/archives, got %d", wUnauth.Code)
	}

	// Viewer (forbidden - Operator required)
	reqViewer := httptest.NewRequest(http.MethodGet, "/api/v1/archives", nil)
	reqViewer.Header.Set("X-Discord-User-ID", viewerUser.DiscordID)
	wViewer := httptest.NewRecorder()
	server.Handler().ServeHTTP(wViewer, reqViewer)
	if wViewer.Code != http.StatusForbidden {
		t.Errorf("expected 403 for Viewer on GET /api/v1/archives, got %d", wViewer.Code)
	}

	// Operator (allowed)
	reqOp := httptest.NewRequest(http.MethodGet, "/api/v1/archives", nil)
	reqOp.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOp, reqOp)
	if wOp.Code != http.StatusOK {
		t.Errorf("expected 200 for Operator on GET /api/v1/archives, got %d", wOp.Code)
	}
	var archives []fenrir.Archive
	if err := json.Unmarshal(wOp.Body.Bytes(), &archives); err != nil {
		t.Fatalf("failed to decode archives response: %v", err)
	}
	if len(archives) != 1 || archives[0].ProjectName != "analytics-stack" {
		t.Errorf("unexpected archives list: %+v", archives)
	}

	// 2. POST /api/v1/archives/{id}/restore RBAC
	// Operator cannot restore (Admin required)
	reqOpRestore := httptest.NewRequest(http.MethodPost, "/api/v1/archives/55/restore", nil)
	reqOpRestore.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpRestore := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpRestore, reqOpRestore)
	if wOpRestore.Code != http.StatusForbidden {
		t.Errorf("expected 403 for Operator on restore, got %d", wOpRestore.Code)
	}

	// Admin can restore
	reqAdminRestore := httptest.NewRequest(http.MethodPost, "/api/v1/archives/55/restore", nil)
	reqAdminRestore.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminRestore := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminRestore, reqAdminRestore)
	if wAdminRestore.Code != http.StatusOK {
		t.Errorf("expected 200 for Admin on restore, got %d", wAdminRestore.Code)
	}
	if restoreCalledWith != "55" {
		t.Errorf("expected restore called for '55', got '%s'", restoreCalledWith)
	}

	// 3. DELETE /api/v1/archives/{id} RBAC
	// Operator cannot delete (Admin required)
	reqOpDelete := httptest.NewRequest(http.MethodDelete, "/api/v1/archives/55", nil)
	reqOpDelete.Header.Set("X-Discord-User-ID", operatorUser.DiscordID)
	wOpDelete := httptest.NewRecorder()
	server.Handler().ServeHTTP(wOpDelete, reqOpDelete)
	if wOpDelete.Code != http.StatusForbidden {
		t.Errorf("expected 403 for Operator on delete, got %d", wOpDelete.Code)
	}

	// Admin can delete / purge
	reqAdminDelete := httptest.NewRequest(http.MethodDelete, "/api/v1/archives/55", nil)
	reqAdminDelete.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAdminDelete := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAdminDelete, reqAdminDelete)
	if wAdminDelete.Code != http.StatusOK {
		t.Errorf("expected 200 for Admin on delete, got %d", wAdminDelete.Code)
	}
	if deleteCalledWith != "55" {
		t.Errorf("expected delete called for '55', got '%s'", deleteCalledWith)
	}

	// 4. POST /admin/archives/action form submissions
	// Test Web Form Restore Action
	formRestore := url.Values{}
	formRestore.Set("id", "55")
	formRestore.Set("action", "restore")
	reqFormRestore := httptest.NewRequest(http.MethodPost, "/admin/archives/action", strings.NewReader(formRestore.Encode()))
	reqFormRestore.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqFormRestore.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wFormRestore := httptest.NewRecorder()
	server.Handler().ServeHTTP(wFormRestore, reqFormRestore)
	if wFormRestore.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect on form restore, got %d", wFormRestore.Code)
	}
	if loc := wFormRestore.Header().Get("Location"); !strings.Contains(loc, "tab=archives") || !strings.Contains(loc, "notification") {
		t.Errorf("unexpected redirect location on form restore: %s", loc)
	}

	// Test Web Form Purge Action
	formPurge := url.Values{}
	formPurge.Set("id", "55")
	formPurge.Set("action", "purge")
	reqFormPurge := httptest.NewRequest(http.MethodPost, "/admin/archives/action", strings.NewReader(formPurge.Encode()))
	reqFormPurge.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqFormPurge.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wFormPurge := httptest.NewRecorder()
	server.Handler().ServeHTTP(wFormPurge, reqFormPurge)
	if wFormPurge.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect on form purge, got %d", wFormPurge.Code)
	}
	if loc := wFormPurge.Header().Get("Location"); !strings.Contains(loc, "tab=archives") || !strings.Contains(loc, "purged") {
		t.Errorf("unexpected redirect location on form purge: %s", loc)
	}

	// Test AJAX Purge Action
	formAjax := url.Values{}
	formAjax.Set("id", "55")
	formAjax.Set("action", "purge")
	reqAjax := httptest.NewRequest(http.MethodPost, "/admin/archives/action", strings.NewReader(formAjax.Encode()))
	reqAjax.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAjax.Header.Set("Accept", "application/json")
	reqAjax.Header.Set("X-Discord-User-ID", adminUser.DiscordID)
	wAjax := httptest.NewRecorder()
	server.Handler().ServeHTTP(wAjax, reqAjax)
	if wAjax.Code != http.StatusOK {
		t.Errorf("expected 200 on AJAX purge, got %d", wAjax.Code)
	}
	var ajaxResp map[string]any
	if err := json.Unmarshal(wAjax.Body.Bytes(), &ajaxResp); err != nil || ajaxResp["success"] != true {
		t.Errorf("unexpected AJAX response: %v", wAjax.Body.String())
	}

	// 5. Verify Unified Audit Logs recorded
	logs, err := db.ListAuditLogs(ctx, 20)
	if err != nil {
		t.Fatalf("failed to list audit logs: %v", err)
	}
	foundRestore := false
	foundPurge := false
	for _, l := range logs {
		if l.Action == "archive_restore" && l.Status == "SUCCESS" {
			foundRestore = true
		}
		if l.Action == "archive_purge" && l.Status == "SUCCESS" {
			foundPurge = true
		}
	}
	if !foundRestore {
		t.Errorf("expected audit log for archive_restore")
	}
	if !foundPurge {
		t.Errorf("expected audit log for archive_purge")
	}
}

