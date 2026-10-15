package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
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
	s.mux.HandleFunc("POST /admin/role", s.handleAdminRoleUpdate)

	// Admin REST API
	s.mux.HandleFunc("GET /api/v1/users", s.handleAPIListUsers)
	s.mux.HandleFunc("POST /api/v1/users/{id}/role", s.handleAPIUpdateRole)
	s.mux.HandleFunc("GET /api/v1/audit-logs", s.handleAPIListAuditLogs)
	s.mux.HandleFunc("GET /api/v1/stacks", s.handleAPIListStacks)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}/compose", s.handleAPIGetStackCompose)
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
	if !s.verifyAdminAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
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

