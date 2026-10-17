package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Harkole/skoll/internal/config"
	"github.com/Harkole/skoll/internal/database"
	"github.com/Harkole/skoll/internal/fenrir"
)

// Server coordinates the HTTP web server, OAuth2 flows, and Admin dashboard.
type Server struct {
	cfg          *config.Config
	db           *database.DB
	fenrirClient *fenrir.Client
	oauth        OAuthExchanger
	mux          *http.ServeMux
	httpServer   *http.Server
}

// NewServer constructs a new API and Web server instance.
func NewServer(cfg *config.Config, db *database.DB, fenrirClient *fenrir.Client, oauth OAuthExchanger) *Server {
	if oauth == nil {
		oauth = NewDefaultDiscordOAuth(cfg.DiscordClientID, cfg.DiscordClientSecret, cfg.DiscordRedirectURI, cfg.HTTPTimeout)
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		fenrirClient: fenrirClient,
		oauth:        oauth,
		mux:          http.NewServeMux(),
	}

	s.routes()

	s.httpServer = &http.Server{
		Addr:         cfg.Port,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Handler returns the root HTTP handler (for testing).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start listens and serves on configured port.
func (s *Server) Start() error {
	log.Printf("[HTTP] Sköll Web & Admin server starting on %s", s.cfg.Port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully terminates HTTP connections.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// OAuth2 Linking Flow
	s.mux.HandleFunc("GET /link", s.handleLinkRedirect)
	s.mux.HandleFunc("GET /auth/discord/callback", s.handleDiscordCallback)

	// Admin Control Panel
	s.mux.HandleFunc("GET /admin", s.handleAdminDashboard)
	s.mux.HandleFunc("GET /admin/stacks", s.handleAdminStacksRedirect)
	s.mux.HandleFunc("GET /admin/compose", s.handleAdminGetCompose)
	s.mux.HandleFunc("POST /admin/compose", s.handleAdminUpdateCompose)
	s.mux.HandleFunc("PUT /admin/compose", s.handleAdminUpdateCompose)
	s.mux.HandleFunc("GET /admin/yaml", s.handleAdminGetYAML)
	s.mux.HandleFunc("POST /admin/yaml", s.handleAdminUpdateYAML)
	s.mux.HandleFunc("PUT /admin/yaml", s.handleAdminUpdateYAML)
	s.mux.HandleFunc("POST /admin/role", s.handleAdminRoleUpdate)

	// Admin REST API
	s.mux.HandleFunc("GET /api/v1/users", s.handleAPIListUsers)
	s.mux.HandleFunc("POST /api/v1/users/{id}/role", s.handleAPIUpdateRole)
	s.mux.HandleFunc("GET /api/v1/audit-logs", s.handleAPIListAuditLogs)
	s.mux.HandleFunc("GET /api/v1/stacks", s.handleAPIListStacks)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}/compose", s.handleAPIGetStackCompose)
	s.mux.HandleFunc("PUT /api/v1/stacks/{name}/compose", s.handleAPIUpdateStackCompose)
	s.mux.HandleFunc("PUT /api/v1/projects/{name}/compose", s.handleAPIUpdateStackCompose)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}/yaml/{filename}", s.handleAPIGetStackYAML)
	s.mux.HandleFunc("GET /api/v1/projects/{name}/yaml/{filename}", s.handleAPIGetStackYAML)
	s.mux.HandleFunc("PUT /api/v1/stacks/{name}/yaml/{filename}", s.handleAPIUpdateStackYAML)
	s.mux.HandleFunc("PUT /api/v1/projects/{name}/yaml/{filename}", s.handleAPIUpdateStackYAML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"service":   "skoll",
		"timestamp": time.Now().UTC(),
	})
}

// handleLinkRedirect initiates Discord OAuth authorization.
func (s *Server) handleLinkRedirect(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		s.renderError(w, http.StatusBadRequest, "Missing or invalid link state token. Please run /link in Discord.")
		return
	}

	// Build Discord OAuth2 URL
	discordAuthURL := fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&response_type=code&scope=identify&state=%s&redirect_uri=%s",
		url.QueryEscape(s.cfg.DiscordClientID),
		url.QueryEscape(state),
		url.QueryEscape(s.cfg.DiscordRedirectURI),
	)

	http.Redirect(w, r, discordAuthURL, http.StatusTemporaryRedirect)
}

// handleDiscordCallback processes Discord OAuth2 callback, exchanges code, and upserts user.
func (s *Server) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		s.renderError(w, http.StatusBadRequest, fmt.Sprintf("Discord authentication denied: %s (%s)", errParam, errDesc))
		return
	}

	stateToken := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))

	if stateToken == "" || code == "" {
		s.renderError(w, http.StatusBadRequest, "Missing state token or authorization code from Discord callback.")
		return
	}

	// 1. Consume state token atomically (validates expiry and prevents replay)
	linkState, err := s.db.ConsumeLinkState(ctx, stateToken)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "Invalid or expired link session. Please request a new link with /link in Discord.")
		return
	}

	// 2. Exchange code for OAuth2 Access Token
	accessToken, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		log.Printf("[OAuth] Exchange failed: %v", err)
		s.renderError(w, http.StatusInternalServerError, "Failed to exchange authorization code with Discord.")
		return
	}

	// 3. Fetch Discord user identity
	discordUser, err := s.oauth.FetchUser(ctx, accessToken)
	if err != nil {
		log.Printf("[OAuth] Fetch user failed: %v", err)
		s.renderError(w, http.StatusInternalServerError, "Failed to retrieve user profile from Discord.")
		return
	}

	// If linkState was targeted to a specific user, verify they match
	if linkState.DiscordUserID != "" && linkState.DiscordUserID != discordUser.ID {
		_ = s.db.RecordAuditLog(ctx, discordUser.ID, discordUser.DisplayName(), "oauth_link_attempt", "security", "UNAUTHORIZED")
		s.renderError(w, http.StatusForbidden, "Discord user mismatch: this link token was generated for a different Discord user.")
		return
	}

	// 4. Upsert user in database
	user, err := s.db.UpsertUser(ctx, discordUser.ID, discordUser.DisplayName(), database.RoleViewer)
	if err != nil {
		log.Printf("[OAuth] Upsert failed: %v", err)
		s.renderError(w, http.StatusInternalServerError, "Failed to persist user identity in database.")
		return
	}

	// 5. Record audit log entry
	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "account_linked", "system", "SUCCESS")

	// 6. Render success confirmation page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = linkSuccessTemplate.Execute(w, map[string]any{
		"Username": user.Username,
		"Role":     user.Role,
	})
}

// verifyAdminAuth checks admin credentials via query, header, cookie, or basic auth.
func (s *Server) verifyAdminAuth(r *http.Request) bool {
	if s.cfg.AdminKey == "" {
		return true // If not configured, allow access
	}

	// 1. Check Query parameter or Form Value
	if key := r.FormValue("key"); key != "" {
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.AdminKey)) == 1 {
			return true
		}
	}

	// 2. Check X-Admin-Key header
	if key := r.Header.Get("X-Admin-Key"); key != "" {
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.AdminKey)) == 1 {
			return true
		}
	}

	// 3. Check Cookie
	if cookie, err := r.Cookie("skoll_admin_key"); err == nil {
		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.cfg.AdminKey)) == 1 {
			return true
		}
	}

	// 4. Check HTTP Basic Auth (username "admin", password = adminKey)
	if _, pass, ok := r.BasicAuth(); ok {
		if subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.AdminKey)) == 1 {
			return true
		}
	}

	return false
}

// handleAdminDashboard renders the role management table and audit feed.
func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = adminLoginTemplate.Execute(w, nil)
		return
	}

	// Set cookie so navigation doesn't require entering key on every click
	if key := r.URL.Query().Get("key"); key != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "skoll_admin_key",
			Value:    key,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	ctx := r.Context()
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		http.Error(w, "Failed to load users: "+err.Error(), http.StatusInternalServerError)
		return
	}

	logs, err := s.db.ListAuditLogs(ctx, 50)
	if err != nil {
		http.Error(w, "Failed to load audit logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate counts
	adminCount := 0
	operatorCount := 0
	for _, u := range users {
		if u.Role == database.RoleAdmin {
			adminCount++
		} else if u.Role == database.RoleOperator {
			operatorCount++
		}
	}

	// Check Fenrir status and fetch stacks
	fenrirStatus := "Online"
	var projects []fenrir.Project
	runningStacksCount := 0
	if s.fenrirClient != nil {
		if _, err := s.fenrirClient.GetHealth(ctx); err != nil {
			fenrirStatus = "Offline / Unreachable"
		} else {
			if projs, err := s.fenrirClient.ListProjects(ctx); err == nil {
				projects = projs
				for i := range projects {
					if strings.ToLower(projects[i].Status) == "running" {
						runningStacksCount++
					}
					// Pre-load compose definition if available so inspect modal is instant
					if projects[i].Compose == "" {
						if comp, err := s.fenrirClient.GetProjectCompose(ctx, projects[i].Name); err == nil {
							projects[i].Compose = comp
						} else {
							projects[i].Compose = fenrir.ReconstructCompose(&projects[i])
						}
					}
				}
			}
		}
	} else {
		fenrirStatus = "Not Configured"
	}

	activeTab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if activeTab == "" {
		activeTab = "stacks"
	}

	data := map[string]any{
		"Users":              users,
		"AuditLogs":          logs,
		"Projects":           projects,
		"TotalUsers":         len(users),
		"AdminCount":         adminCount,
		"OperatorCount":      operatorCount,
		"AuditCount":         len(logs),
		"TotalStacks":        len(projects),
		"RunningStacksCount": runningStacksCount,
		"FenrirStatus":       fenrirStatus,
		"AdminKey":           s.cfg.AdminKey,
		"Notification":       r.URL.Query().Get("notification"),
		"ActiveTab":          activeTab,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminTemplate.Execute(w, data)
}

// handleAdminRoleUpdate handles form submissions from the Admin UI.
func (s *Server) handleAdminRoleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("user_id")
	roleStr := r.FormValue("role")
	adminKey := r.FormValue("key")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetRole := database.Role(roleStr)
	if !targetRole.IsValid() {
		http.Error(w, "Invalid role specified", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	targetUser, err := s.db.GetUserByID(ctx, id)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := s.db.UpdateUserRole(ctx, id, targetRole); err != nil {
		http.Error(w, "Failed to update role: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit record
	_ = s.db.RecordAuditLog(ctx, "admin_ui", "administrator", "role_update", fmt.Sprintf("%s (%s -> %s)", targetUser.Username, targetUser.Role, targetRole), "SUCCESS")

	// Redirect back with notification
	redirectURL := fmt.Sprintf("/admin?notification=Role+for+%s+updated+to+%s", url.QueryEscape(targetUser.Username), url.QueryEscape(string(targetRole)))
	if adminKey != "" {
		redirectURL += "&key=" + url.QueryEscape(adminKey)
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// REST API Handlers

func (s *Server) handleAPIListUsers(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

func (s *Server) handleAPIUpdateRole(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	targetRole := database.Role(req.Role)
	if !targetRole.IsValid() {
		http.Error(w, `{"error":"invalid role"}`, http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateUserRole(r.Context(), id, targetRole); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "role": targetRole})
}

func (s *Server) handleAPIListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	logs, err := s.db.ListAuditLogs(r.Context(), limit)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

func (s *Server) renderError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errorTemplate.Execute(w, map[string]any{
		"ErrorMessage": msg,
	})
}

// handleAdminStacksRedirect redirects /admin/stacks to /admin?tab=stacks.
func (s *Server) handleAdminStacksRedirect(w http.ResponseWriter, r *http.Request) {
	target := "/admin?tab=stacks"
	if key := r.URL.Query().Get("key"); key != "" {
		target += "&key=" + url.QueryEscape(key)
	}
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}

// handleAdminGetCompose retrieves the compose YAML for a stack in JSON format for the admin modal.
func (s *Server) handleAdminGetCompose(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, `{"error":"missing stack name"}`, http.StatusBadRequest)
		return
	}
	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}
	compose, err := s.fenrirClient.GetProjectCompose(r.Context(), name)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    name,
		"compose": compose,
	})
}

// handleAPIListStacks lists all Docker Compose stacks registered with Fenrir.
func (s *Server) handleAPIListStacks(w http.ResponseWriter, r *http.Request) {
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}
	projects, err := s.fenrirClient.ListProjects(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(projects)
}

// handleAPIGetStackCompose returns raw compose file content for a named stack.
func (s *Server) handleAPIGetStackCompose(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"missing stack name"}`, http.StatusBadRequest)
		return
	}
	if _, ok := s.checkAdminRBAC(w, r, "compose_read", name); !ok {
		return
	}
	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}
	compose, err := s.fenrirClient.GetProjectCompose(r.Context(), name)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    name,
		"compose": compose,
	})
}

// checkAdminRBAC verifies that the caller has Administrator privileges.
// If the caller is an Operator, Viewer, Unlinked user, or fails admin authentication,
// it responds with HTTP 403 Forbidden and returns (nil, false).
func (s *Server) checkAdminRBAC(w http.ResponseWriter, r *http.Request, action, target string) (*database.User, bool) {
	ctx := r.Context()

	// 1. Check for explicit user identification (headers, query, form)
	discordUserID := strings.TrimSpace(r.Header.Get("X-Discord-User-ID"))
	if discordUserID == "" {
		discordUserID = strings.TrimSpace(r.Header.Get("X-Discord-ID"))
	}
	if discordUserID == "" {
		discordUserID = strings.TrimSpace(r.Header.Get("X-User-ID"))
	}
	if discordUserID == "" {
		discordUserID = strings.TrimSpace(r.URL.Query().Get("discord_user_id"))
	}
	if discordUserID == "" {
		discordUserID = strings.TrimSpace(r.URL.Query().Get("user_id"))
	}

	// 2. Check for explicit role header or query
	roleHeader := strings.TrimSpace(r.Header.Get("X-Role"))
	if roleHeader == "" {
		roleHeader = strings.TrimSpace(r.Header.Get("X-User-Role"))
	}
	if roleHeader == "" {
		roleHeader = strings.TrimSpace(r.URL.Query().Get("role"))
	}

	if roleHeader != "" {
		parsedRole := database.ParseRole(roleHeader)
		if parsedRole != database.RoleAdmin {
			_ = s.db.RecordAuditLog(ctx, discordUserID, roleHeader, action, target, "UNAUTHORIZED")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":  fmt.Sprintf("forbidden: role '%s' does not have admin permissions", roleHeader),
				"status": http.StatusForbidden,
			})
			return nil, false
		}
	}

	// 3. If a Discord User ID is provided, look up the user in database
	if discordUserID != "" {
		user, err := s.db.GetUserByDiscordID(ctx, discordUserID)
		if err != nil {
			// Also try numeric ID lookup if possible
			if numID, parseErr := strconv.ParseInt(discordUserID, 10, 64); parseErr == nil {
				user, err = s.db.GetUserByID(ctx, numID)
			}
		}

		if err != nil {
			// User not found -> Unlinked user
			_ = s.db.RecordAuditLog(ctx, discordUserID, "unlinked", action, target, "UNAUTHORIZED")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":  "forbidden: unlinked user, admin role required",
				"status": http.StatusForbidden,
			})
			return nil, false
		}

		if user.Role != database.RoleAdmin {
			// Operator, Viewer, or Unlinked
			_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, action, target, "UNAUTHORIZED")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":  fmt.Sprintf("forbidden: user role is %s, admin role required", user.Role),
				"status": http.StatusForbidden,
			})
			return nil, false
		}

		return user, true
	}

	// 4. No Discord User ID provided; check Admin Secret credentials
	if s.verifyAdminAuth(r) {
		return &database.User{
			Username: "administrator",
			Role:     database.RoleAdmin,
		}, true
	}

	// 5. Neither valid user nor admin credentials -> 403 Forbidden
	_ = s.db.RecordAuditLog(ctx, "anonymous", "unlinked", action, target, "UNAUTHORIZED")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  "forbidden: admin privileges required",
		"status": http.StatusForbidden,
	})
	return nil, false
}

// extractYAMLContent parses compose or configuration YAML from raw text, form fields, or JSON envelopes.
// It supports JSON bodies containing compose, yaml, content, or composeContent keys.
func extractYAMLContent(r *http.Request) (string, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))

	// Check form submission if urlencoded or multipart
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		_ = r.ParseForm()
		for _, key := range []string{"compose", "yaml", "content", "composeContent"} {
			if val := strings.TrimSpace(r.FormValue(key)); val != "" {
				return val, nil
			}
		}
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read request body: %w", err)
	}
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return "", errors.New("empty request body")
	}

	trimmed := bytes.TrimSpace(bodyBytes)
	if strings.Contains(contentType, "application/json") || (len(trimmed) > 0 && trimmed[0] == '{') {
		var jsonBody map[string]any
		if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
			for _, key := range []string{"compose", "yaml", "content", "composeContent"} {
				if val, ok := jsonBody[key]; ok {
					if strVal, ok := val.(string); ok && strings.TrimSpace(strVal) != "" {
						return strVal, nil
					}
				}
			}
			return "", errors.New("missing yaml content in JSON body (expected compose, yaml, content, or composeContent)")
		}
	}

	return string(bodyBytes), nil
}

// handleAPIUpdateStackCompose handles PUT /api/v1/projects/{name}/compose and PUT /api/v1/stacks/{name}/compose.
func (s *Server) handleAPIUpdateStackCompose(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing project or stack name"})
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "compose_update", name)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "fenrir daemon client not configured"})
		return
	}

	content, err := extractYAMLContent(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	var redeploy *bool
	if r.URL.Query().Has("redeploy") {
		val := strings.ToLower(r.URL.Query().Get("redeploy")) == "true" || r.URL.Query().Get("redeploy") == "1"
		redeploy = &val
	}

	var dryRun *bool
	if r.URL.Query().Has("dry_run") {
		val := strings.ToLower(r.URL.Query().Get("dry_run")) == "true" || r.URL.Query().Get("dry_run") == "1"
		dryRun = &val
	}

	opts := &fenrir.ComposeUpdateOptions{
		Redeploy: redeploy,
		DryRun:   dryRun,
	}

	if err := s.fenrirClient.UpdateProjectCompose(r.Context(), name, content, opts); err != nil {
		_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "compose_update", name, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "compose_update", name, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"name":     name,
		"message":  "Compose configuration updated successfully",
		"dry_run":  dryRun != nil && *dryRun,
		"redeploy": redeploy == nil || *redeploy,
	})
}

// handleAPIUpdateStackYAML handles PUT /api/v1/projects/{name}/yaml/{filename} and PUT /api/v1/stacks/{name}/yaml/{filename}.
func (s *Server) handleAPIUpdateStackYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	filename := r.PathValue("filename")
	if name == "" || filename == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing project name or filename"})
		return
	}

	target := fmt.Sprintf("%s/%s", name, filename)
	user, ok := s.checkAdminRBAC(w, r, "yaml_update", target)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "fenrir daemon client not configured"})
		return
	}

	content, err := extractYAMLContent(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	var restart *bool
	if r.URL.Query().Has("restart") {
		val := strings.ToLower(r.URL.Query().Get("restart")) == "true" || r.URL.Query().Get("restart") == "1"
		restart = &val
	}

	opts := &fenrir.YAMLUpdateOptions{
		Restart: restart,
	}

	if err := s.fenrirClient.UpdateProjectYAML(r.Context(), name, filename, content, opts); err != nil {
		_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "yaml_update", target, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "yaml_update", target, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"name":     name,
		"filename": filename,
		"message":  fmt.Sprintf("YAML file '%s' updated successfully", filename),
		"restart":  restart != nil && *restart,
	})
}

// handleAPIGetStackYAML handles GET /api/v1/projects/{name}/yaml/{filename} and GET /api/v1/stacks/{name}/yaml/{filename}.
func (s *Server) handleAPIGetStackYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	filename := r.PathValue("filename")
	if name == "" || filename == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing project name or filename"})
		return
	}

	target := fmt.Sprintf("%s/%s", name, filename)
	_, ok := s.checkAdminRBAC(w, r, "yaml_read", target)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "fenrir daemon client not configured"})
		return
	}

	content, err := s.fenrirClient.GetProjectYAML(r.Context(), name, filename)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":     name,
		"filename": filename,
		"content":  content,
	})
}

// handleAdminUpdateCompose handles updates from the Admin UI for project compose files.
func (s *Server) handleAdminUpdateCompose(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(r.PathValue("name"))
	}
	if name == "" {
		http.Error(w, `{"error":"missing stack name"}`, http.StatusBadRequest)
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "compose_update", name)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	content, err := extractYAMLContent(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	redeployStr := r.FormValue("redeploy")
	if redeployStr == "" {
		redeployStr = r.URL.Query().Get("redeploy")
	}
	var redeploy *bool
	if redeployStr != "" {
		val := strings.ToLower(redeployStr) == "true" || redeployStr == "1"
		redeploy = &val
	}

	dryRunStr := r.FormValue("dry_run")
	if dryRunStr == "" {
		dryRunStr = r.URL.Query().Get("dry_run")
	}
	var dryRun *bool
	if dryRunStr != "" {
		val := strings.ToLower(dryRunStr) == "true" || dryRunStr == "1"
		dryRun = &val
	}

	opts := &fenrir.ComposeUpdateOptions{
		Redeploy: redeploy,
		DryRun:   dryRun,
	}

	if err := s.fenrirClient.UpdateProjectCompose(r.Context(), name, content, opts); err != nil {
		_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "compose_update", name, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "compose_update", name, "SUCCESS")

	// If standard HTML form submit
	if r.Header.Get("Accept") != "" && strings.Contains(r.Header.Get("Accept"), "text/html") && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		redirectURL := fmt.Sprintf("/admin?tab=stacks&notification=Compose+updated+successfully+for+%s", url.QueryEscape(name))
		if key := r.FormValue("key"); key != "" {
			redirectURL += "&key=" + url.QueryEscape(key)
		}
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"name":     name,
		"message":  "Compose configuration updated successfully",
		"dry_run":  dryRun != nil && *dryRun,
		"redeploy": redeploy == nil || *redeploy,
	})
}

// handleAdminGetYAML loads an auxiliary YAML file for the Admin UI editor.
func (s *Server) handleAdminGetYAML(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if name == "" || filename == "" {
		http.Error(w, `{"error":"missing stack name or filename"}`, http.StatusBadRequest)
		return
	}

	target := fmt.Sprintf("%s/%s", name, filename)
	_, ok := s.checkAdminRBAC(w, r, "yaml_read", target)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	content, err := s.fenrirClient.GetProjectYAML(r.Context(), name, filename)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":     name,
		"filename": filename,
		"content":  content,
	})
}

// handleAdminUpdateYAML saves an auxiliary YAML file from the Admin UI editor.
func (s *Server) handleAdminUpdateYAML(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(r.URL.Query().Get("name"))
	}
	filename := strings.TrimSpace(r.FormValue("filename"))
	if filename == "" {
		filename = strings.TrimSpace(r.URL.Query().Get("filename"))
	}
	if name == "" || filename == "" {
		http.Error(w, `{"error":"missing stack name or filename"}`, http.StatusBadRequest)
		return
	}

	target := fmt.Sprintf("%s/%s", name, filename)
	user, ok := s.checkAdminRBAC(w, r, "yaml_update", target)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	content, err := extractYAMLContent(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	restartStr := r.FormValue("restart")
	if restartStr == "" {
		restartStr = r.URL.Query().Get("restart")
	}
	var restart *bool
	if restartStr != "" {
		val := strings.ToLower(restartStr) == "true" || restartStr == "1"
		restart = &val
	}

	opts := &fenrir.YAMLUpdateOptions{
		Restart: restart,
	}

	if err := s.fenrirClient.UpdateProjectYAML(r.Context(), name, filename, content, opts); err != nil {
		_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "yaml_update", target, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "yaml_update", target, "SUCCESS")

	// If standard HTML form submit
	if r.Header.Get("Accept") != "" && strings.Contains(r.Header.Get("Accept"), "text/html") && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		redirectURL := fmt.Sprintf("/admin?tab=stacks&notification=YAML+file+%s+updated+for+%s", url.QueryEscape(filename), url.QueryEscape(name))
		if key := r.FormValue("key"); key != "" {
			redirectURL += "&key=" + url.QueryEscape(key)
		}
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"name":     name,
		"filename": filename,
		"message":  fmt.Sprintf("YAML file '%s' updated successfully", filename),
		"restart":  restart != nil && *restart,
	})
}


