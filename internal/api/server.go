package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	sessions     *SessionManager
	mux          *http.ServeMux
	httpServer   *http.Server
}

// NewServer constructs a new API and Web server instance.
func NewServer(cfg *config.Config, db *database.DB, fenrirClient *fenrir.Client, oauth OAuthExchanger) *Server {
	if oauth == nil {
		oauth = NewDefaultDiscordOAuth(cfg.DiscordClientID, cfg.DiscordClientSecret, cfg.DiscordRedirectURI, cfg.HTTPTimeout)
	}

	sessionSecret := cfg.SessionSecret
	if sessionSecret == "" {
		sessionSecret = cfg.DiscordClientSecret
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		fenrirClient: fenrirClient,
		oauth:        oauth,
		sessions:     NewSessionManager(sessionSecret),
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

	// Web Login & Discord OAuth2 Endpoints
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("GET /auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("GET /auth/callback", s.handleDiscordCallback)
	s.mux.HandleFunc("GET /auth/discord/callback", s.handleDiscordCallback)
	s.mux.HandleFunc("GET /auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("GET /link", s.handleLinkRedirect)

	// Public Legal & Compliance Pages (Accessible without authentication)
	s.mux.HandleFunc("GET /tos", s.handleTermsOfService)
	s.mux.HandleFunc("GET /terms", s.handleTermsOfService)
	s.mux.HandleFunc("GET /privacy", s.handlePrivacyPolicy)

	// Admin Control Panel (Web Routes)
	s.mux.HandleFunc("GET /admin", s.handleAdminDashboard)
	s.mux.HandleFunc("GET /admin/stacks", s.handleAdminStacksRedirect)
	s.mux.HandleFunc("GET /admin/compose", s.handleAdminGetCompose)
	s.mux.HandleFunc("POST /admin/compose", s.handleAdminUpdateCompose)
	s.mux.HandleFunc("PUT /admin/compose", s.handleAdminUpdateCompose)
	s.mux.HandleFunc("GET /admin/yaml", s.handleAdminGetYAML)
	s.mux.HandleFunc("POST /admin/yaml", s.handleAdminUpdateYAML)
	s.mux.HandleFunc("PUT /admin/yaml", s.handleAdminUpdateYAML)
	s.mux.HandleFunc("POST /admin/role", s.handleAdminRoleUpdate)
	s.mux.HandleFunc("POST /admin/stacks/action", s.handleAdminStackAction)
	s.mux.HandleFunc("GET /admin/archives", s.handleAdminArchivesRedirect)
	s.mux.HandleFunc("POST /admin/archives/action", s.handleAdminArchiveAction)

	// Admin REST API & Stack Operations
	s.mux.HandleFunc("GET /api/v1/users", s.handleAPIListUsers)
	s.mux.HandleFunc("POST /api/v1/users/{id}/role", s.handleAPIUpdateRole)
	s.mux.HandleFunc("GET /api/v1/audit-logs", s.handleAPIListAuditLogs)
	s.mux.HandleFunc("GET /api/v1/stacks", s.handleAPIListStacks)
	s.mux.HandleFunc("POST /api/v1/stacks", s.handleAPIDeployStack)
	s.mux.HandleFunc("POST /api/v1/projects", s.handleAPIDeployStack)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}/compose", s.handleAPIGetStackCompose)
	s.mux.HandleFunc("PUT /api/v1/stacks/{name}/compose", s.handleAPIUpdateStackCompose)
	s.mux.HandleFunc("PUT /api/v1/projects/{name}/compose", s.handleAPIUpdateStackCompose)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}/yaml/{filename}", s.handleAPIGetStackYAML)
	s.mux.HandleFunc("GET /api/v1/projects/{name}/yaml/{filename}", s.handleAPIGetStackYAML)
	s.mux.HandleFunc("PUT /api/v1/stacks/{name}/yaml/{filename}", s.handleAPIUpdateStackYAML)
	s.mux.HandleFunc("PUT /api/v1/projects/{name}/yaml/{filename}", s.handleAPIUpdateStackYAML)
	s.mux.HandleFunc("POST /api/v1/stacks/{name}/restart", s.handleAPIStackRestart)
	s.mux.HandleFunc("POST /api/v1/projects/{name}/restart", s.handleAPIStackRestart)
	s.mux.HandleFunc("POST /api/v1/stacks/{name}/start", s.handleAPIStackStart)
	s.mux.HandleFunc("POST /api/v1/projects/{name}/start", s.handleAPIStackStart)
	s.mux.HandleFunc("POST /api/v1/stacks/{name}/stop", s.handleAPIStackStop)
	s.mux.HandleFunc("POST /api/v1/projects/{name}/stop", s.handleAPIStackStop)
	s.mux.HandleFunc("DELETE /api/v1/stacks/{name}", s.handleAPIDeleteStack)
	s.mux.HandleFunc("DELETE /api/v1/projects/{name}", s.handleAPIDeleteStack)

	// Archived Stacks Management (90-day retention cooldown)
	s.mux.HandleFunc("GET /api/v1/archives", s.handleAPIListArchives)
	s.mux.HandleFunc("POST /api/v1/archives/{id}/restore", s.handleAPIRestoreArchive)
	s.mux.HandleFunc("DELETE /api/v1/archives/{id}", s.handleAPIDeleteArchive)

	// Container Lifecycle Controls (Operator+)
	s.mux.HandleFunc("POST /api/v1/containers/{name}/start", func(w http.ResponseWriter, r *http.Request) { s.handleAPIContainerAction(w, r, "start") })
	s.mux.HandleFunc("POST /api/v1/containers/{name}/stop", func(w http.ResponseWriter, r *http.Request) { s.handleAPIContainerAction(w, r, "stop") })
	s.mux.HandleFunc("POST /api/v1/containers/{name}/restart", func(w http.ResponseWriter, r *http.Request) { s.handleAPIContainerAction(w, r, "restart") })
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"service":   "skoll",
		"timestamp": time.Now().UTC(),
	})
}

// handleLoginPage renders the clean login page featuring the "Login with Discord" button.
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated with Operator or Admin role, redirect directly to /admin
	if user, err := s.getSessionUser(r); err == nil && user != nil {
		if user.Role.CanExecute(database.RoleOperator) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
	}

	errMsg := r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTemplate.Execute(w, map[string]any{
		"ErrorMessage": errMsg,
	})
}

// handleTermsOfService renders the public Terms of Service page.
func (s *Server) handleTermsOfService(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tosTemplate.Execute(w, nil)
}

// handlePrivacyPolicy renders the public Privacy Policy page.
func (s *Server) handlePrivacyPolicy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = privacyTemplate.Execute(w, nil)
}

// handleAuthLogin redirects users to Discord's OAuth2 authorization URL with the identify scope.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Generate a cryptographically secure 32-byte state token
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		s.renderError(w, http.StatusInternalServerError, "Failed to generate OAuth state token")
		return
	}
	stateToken := hex.EncodeToString(b)

	// Save ephemeral state token in DB (empty DiscordUserID signifies web login)
	if err := s.db.CreateLinkState(ctx, stateToken, "", 10*time.Minute); err != nil {
		s.renderError(w, http.StatusInternalServerError, "Failed to initialize login state")
		return
	}

	// Set temporary state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "skoll_oauth_state",
		Value:    stateToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isRequestSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	discordAuthURL := fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&response_type=code&scope=identify&state=%s&redirect_uri=%s",
		url.QueryEscape(s.cfg.DiscordClientID),
		url.QueryEscape(stateToken),
		url.QueryEscape(s.cfg.DiscordRedirectURI),
	)

	http.Redirect(w, r, discordAuthURL, http.StatusTemporaryRedirect)
}

// handleLinkRedirect initiates Discord OAuth authorization for the Discord bot /link flow.
func (s *Server) handleLinkRedirect(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		s.renderError(w, http.StatusBadRequest, "Missing or invalid link state token. Please run /link in Discord.")
		return
	}

	discordAuthURL := fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&response_type=code&scope=identify&state=%s&redirect_uri=%s",
		url.QueryEscape(s.cfg.DiscordClientID),
		url.QueryEscape(state),
		url.QueryEscape(s.cfg.DiscordRedirectURI),
	)

	http.Redirect(w, r, discordAuthURL, http.StatusTemporaryRedirect)
}

// handleDiscordCallback handles the authorization code exchange, fetches the Discord profile,
// establishes an encrypted secure HTTP-only session cookie, and directs the user to /admin.
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

	// 1. Consume state token atomically (validates expiry and prevents replay attacks)
	linkState, err := s.db.ConsumeLinkState(ctx, stateToken)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "Invalid or expired login session. Please try logging in again.")
		return
	}

	// 2. Exchange authorization code for OAuth2 Access Token
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

	// If linkState was targeted to a specific user (/link command), verify identity matches
	if linkState.DiscordUserID != "" && linkState.DiscordUserID != discordUser.ID {
		_ = s.db.RecordAuditLog(ctx, discordUser.ID, discordUser.DisplayName(), "oauth_link_attempt", "security", "UNAUTHORIZED")
		s.renderError(w, http.StatusForbidden, "Discord user mismatch: this link token was generated for a different Discord user.")
		return
	}

	// 4. Upsert user in database. First user bootstrap automatically grants Admin role.
	user, err := s.db.UpsertUser(ctx, discordUser.ID, discordUser.DisplayName(), database.RoleViewer)
	if err != nil {
		log.Printf("[OAuth] Upsert failed: %v", err)
		s.renderError(w, http.StatusInternalServerError, "Failed to persist user identity in database.")
		return
	}

	// 5. Establish encrypted, secure HTTP-only session cookie
	if err := s.sessions.SetSessionCookie(w, user, s.isRequestSecure(r)); err != nil {
		log.Printf("[Session] Failed to set session cookie: %v", err)
		s.renderError(w, http.StatusInternalServerError, "Failed to establish secure session.")
		return
	}

	// Clear temporary state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "skoll_oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isRequestSecure(r),
		MaxAge:   -1,
	})

	// 6. Record unified audit log entry
	actionName := "login"
	if linkState.DiscordUserID != "" {
		actionName = "account_linked"
	}
	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, actionName, "system", "SUCCESS")

	// 7. Route user:
	if linkState.DiscordUserID != "" {
		// Bot linking confirmation view
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = linkSuccessTemplate.Execute(w, map[string]any{
			"Username": user.Username,
			"Role":     user.Role,
		})
		return
	}

	// Web-initiated login -> redirect to Admin control panel
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleAuthLogout terminates the session and redirects to the login page.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if user, err := s.getSessionUser(r); err == nil && user != nil {
		_ = s.db.RecordAuditLog(r.Context(), user.DiscordID, user.Username, "logout", "system", "SUCCESS")
	}
	s.sessions.ClearSessionCookie(w, s.isRequestSecure(r))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// getSessionUser extracts and verifies the active user identity against the database.
func (s *Server) getSessionUser(r *http.Request) (*database.User, error) {
	session, err := s.sessions.GetSession(r)
	if err != nil {
		return nil, err
	}
	// Always look up current role and status directly in the database
	user, err := s.db.GetUserByDiscordID(r.Context(), session.DiscordUserID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Server) isRequestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(s.cfg.PublicURL), "https://") {
		return true
	}
	return false
}

// handleAdminDashboard renders the role management table and audit feed for Operator+ users.
func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := s.getSessionUser(r)
	if err != nil || user == nil {
		// Unauthenticated -> redirect to clean login page with "Login with Discord" button
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Verify that the logged-in user possesses the required Admin/Operator role
	if !user.Role.CanExecute(database.RoleOperator) {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "admin_access", "dashboard", "UNAUTHORIZED")
		s.renderForbidden(w, user, "Operator or Admin role required to access the Sköll Control Panel.")
		return
	}

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
		switch u.Role {
		case database.RoleAdmin:
			adminCount++
		case database.RoleOperator:
			operatorCount++
		}
	}

	// Check Fenrir status and fetch stacks & archives
	fenrirStatus := "Online"
	var projects []fenrir.Project
	var archives []fenrir.Archive
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
			if archs, err := s.fenrirClient.ListArchives(ctx); err == nil {
				archives = archs
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
		"CurrentUser":        user,
		"Users":              users,
		"AuditLogs":          logs,
		"Projects":           projects,
		"Archives":           archives,
		"TotalArchives":      len(archives),
		"TotalUsers":         len(users),
		"AdminCount":         adminCount,
		"OperatorCount":      operatorCount,
		"AuditCount":         len(logs),
		"TotalStacks":        len(projects),
		"RunningStacksCount": runningStacksCount,
		"FenrirStatus":       fenrirStatus,
		"Notification":       r.URL.Query().Get("notification"),
		"ActiveTab":          activeTab,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminTemplate.Execute(w, data)
}

// handleAdminRoleUpdate handles form submissions from the Admin UI to change user roles.
func (s *Server) handleAdminRoleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify acting user has Administrator role
	actingUser, ok := s.checkAdminRBAC(w, r, "role_update", "roles")
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("user_id")
	roleStr := r.FormValue("role")

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

	targetUser, err := s.db.GetUserByID(ctx, id)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := s.db.UpdateUserRole(ctx, id, targetRole); err != nil {
		http.Error(w, "Failed to update role: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Unified Audit record with authenticated Discord username and ID from session
	_ = s.db.RecordAuditLog(ctx, actingUser.DiscordID, actingUser.Username, "role_update", fmt.Sprintf("%s (%s -> %s)", targetUser.Username, targetUser.Role, targetRole), "SUCCESS")

	// Redirect back with notification
	redirectURL := fmt.Sprintf("/admin?tab=users&notification=Role+for+%s+updated+to+%s", url.QueryEscape(targetUser.Username), url.QueryEscape(string(targetRole)))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// REST API Handlers

func (s *Server) handleAPIListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkAdminRBAC(w, r, "list_users", "users"); !ok {
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
	actingUser, ok := s.checkAdminRBAC(w, r, "role_update", "roles")
	if !ok {
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

	ctx := r.Context()
	targetUser, err := s.db.GetUserByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	if err := s.db.UpdateUserRole(ctx, id, targetRole); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	_ = s.db.RecordAuditLog(ctx, actingUser.DiscordID, actingUser.Username, "role_update", fmt.Sprintf("%s (%s -> %s)", targetUser.Username, targetUser.Role, targetRole), "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "role": targetRole})
}

func (s *Server) handleAPIListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkOperatorRBAC(w, r, "list_audit_logs", "audit"); !ok {
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

func (s *Server) renderForbidden(w http.ResponseWriter, user *database.User, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = forbiddenTemplate.Execute(w, map[string]any{
		"ErrorMessage": msg,
		"Username":     user.Username,
		"Role":         user.Role,
	})
}

func (s *Server) respondForbidden(w http.ResponseWriter, r *http.Request, msg string) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = forbiddenTemplate.Execute(w, map[string]any{
			"ErrorMessage": msg,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  msg,
		"status": http.StatusForbidden,
	})
}

// handleAdminStacksRedirect redirects /admin/stacks to /admin?tab=stacks.
func (s *Server) handleAdminStacksRedirect(w http.ResponseWriter, r *http.Request) {
	if user, err := s.getSessionUser(r); err != nil || user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?tab=stacks", http.StatusTemporaryRedirect)
}

// handleAdminGetCompose retrieves the compose YAML for a stack in JSON format for the admin modal.
func (s *Server) handleAdminGetCompose(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkOperatorRBAC(w, r, "compose_read", "modal"); !ok {
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
	if _, ok := s.checkOperatorRBAC(w, r, "list_stacks", "stacks"); !ok {
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

// checkAdminRBAC verifies that caller possesses the Administrator role.
func (s *Server) checkAdminRBAC(w http.ResponseWriter, r *http.Request, action, target string) (*database.User, bool) {
	return s.checkRequiredRole(w, r, database.RoleAdmin, action, target)
}

// checkOperatorRBAC verifies that caller possesses at least Operator privileges.
func (s *Server) checkOperatorRBAC(w http.ResponseWriter, r *http.Request, action, target string) (*database.User, bool) {
	return s.checkRequiredRole(w, r, database.RoleOperator, action, target)
}

// checkRequiredRole validates the caller's identity and checks database permissions.
func (s *Server) checkRequiredRole(w http.ResponseWriter, r *http.Request, requiredRole database.Role, action, target string) (*database.User, bool) {
	ctx := r.Context()

	// 1. Check encrypted HTTP-only session cookie first
	if sessionUser, err := s.getSessionUser(r); err == nil && sessionUser != nil {
		if !sessionUser.Role.CanExecute(requiredRole) {
			_ = s.db.RecordAuditLog(ctx, sessionUser.DiscordID, sessionUser.Username, action, target, "UNAUTHORIZED")
			s.respondForbidden(w, r, fmt.Sprintf("forbidden: user role is %s, %s role required", sessionUser.Role, requiredRole))
			return nil, false
		}
		return sessionUser, true
	}

	// 2. Check for explicit user identification (headers, query, form) for REST API callers
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

	// Check for explicit role header or query
	roleHeader := strings.TrimSpace(r.Header.Get("X-Role"))
	if roleHeader == "" {
		roleHeader = strings.TrimSpace(r.Header.Get("X-User-Role"))
	}
	if roleHeader == "" {
		roleHeader = strings.TrimSpace(r.URL.Query().Get("role"))
	}

	if roleHeader != "" {
		parsedRole := database.ParseRole(roleHeader)
		if !parsedRole.CanExecute(requiredRole) {
			_ = s.db.RecordAuditLog(ctx, discordUserID, roleHeader, action, target, "UNAUTHORIZED")
			s.respondForbidden(w, r, fmt.Sprintf("forbidden: role '%s' does not have %s permissions", roleHeader, requiredRole))
			return nil, false
		}
	}

	// Look up user in database if Discord User ID is provided
	if discordUserID != "" {
		user, err := s.db.GetUserByDiscordID(ctx, discordUserID)
		if err != nil {
			if numID, parseErr := strconv.ParseInt(discordUserID, 10, 64); parseErr == nil {
				user, err = s.db.GetUserByID(ctx, numID)
			}
		}

		if err != nil {
			_ = s.db.RecordAuditLog(ctx, discordUserID, "unlinked", action, target, "UNAUTHORIZED")
			s.respondForbidden(w, r, fmt.Sprintf("forbidden: unlinked user, %s role required", requiredRole))
			return nil, false
		}

		if !user.Role.CanExecute(requiredRole) {
			_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, action, target, "UNAUTHORIZED")
			s.respondForbidden(w, r, fmt.Sprintf("forbidden: user role is %s, %s role required", user.Role, requiredRole))
			return nil, false
		}

		return user, true
	}

	// 3. Unauthorized
	_ = s.db.RecordAuditLog(ctx, "anonymous", "unlinked", action, target, "UNAUTHORIZED")
	s.respondForbidden(w, r, fmt.Sprintf("forbidden: %s privileges required", requiredRole))
	return nil, false
}

// extractYAMLContent parses compose or configuration YAML from raw text, form fields, or JSON envelopes.
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
		name = strings.TrimSpace(r.PathValue("name"))
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

// handleAPIStackRestart handles POST /api/v1/stacks/{name}/restart.
func (s *Server) handleAPIStackRestart(w http.ResponseWriter, r *http.Request) {
	s.handleStackAction(w, r, "restart")
}

// handleAPIStackStart handles POST /api/v1/stacks/{name}/start.
func (s *Server) handleAPIStackStart(w http.ResponseWriter, r *http.Request) {
	s.handleStackAction(w, r, "start")
}

// handleAPIStackStop handles POST /api/v1/stacks/{name}/stop.
func (s *Server) handleAPIStackStop(w http.ResponseWriter, r *http.Request) {
	s.handleStackAction(w, r, "stop")
}

// handleAdminStackAction handles form actions from web UI.
func (s *Server) handleAdminStackAction(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	action := strings.TrimSpace(r.FormValue("action"))
	s.executeStackOperation(w, r, name, action, true)
}

func (s *Server) handleStackAction(w http.ResponseWriter, r *http.Request, action string) {
	name := r.PathValue("name")
	s.executeStackOperation(w, r, name, action, false)
}

func (s *Server) executeStackOperation(w http.ResponseWriter, r *http.Request, name, action string, isWebForm bool) {
	if name == "" {
		http.Error(w, `{"error":"missing stack name"}`, http.StatusBadRequest)
		return
	}

	var user *database.User
	var ok bool
	auditAction := "server_" + action
	if action == "delete" || action == "teardown" {
		auditAction = "stack_delete"
		user, ok = s.checkAdminRBAC(w, r, auditAction, name)
	} else {
		user, ok = s.checkOperatorRBAC(w, r, auditAction, name)
	}
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var err error
	if action == "delete" || action == "teardown" {
		err = s.fenrirClient.DeleteProject(ctx, name)
	} else {
		switch action {
		case "restart":
			err = s.fenrirClient.RestartProject(ctx, name)
		case "start":
			err = s.fenrirClient.StartProject(ctx, name)
		case "stop":
			err = s.fenrirClient.StopProject(ctx, name)
		default:
			http.Error(w, `{"error":"invalid stack action"}`, http.StatusBadRequest)
			return
		}
	}

	if err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, name, "FAILED")
		if isWebForm {
			http.Redirect(w, r, fmt.Sprintf("/admin?tab=stacks&notification=Error: %s", url.QueryEscape(err.Error())), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, name, "SUCCESS")

	msg := fmt.Sprintf("Stack %s %sed successfully", name, action)
	if action == "delete" || action == "teardown" {
		msg = fmt.Sprintf("Stack %s torn down and archived successfully", name)
	}

	if isWebForm {
		http.Redirect(w, r, fmt.Sprintf("/admin?tab=stacks&notification=%s", url.QueryEscape(msg)), http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"name":    name,
		"action":  action,
		"message": msg,
	})
}

// handleAPIDeleteStack handles DELETE /api/v1/stacks/{name} and DELETE /api/v1/projects/{name}.
func (s *Server) handleAPIDeleteStack(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		http.Error(w, `{"error":"missing stack name"}`, http.StatusBadRequest)
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "stack_delete", name)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	if err := s.fenrirClient.DeleteProject(ctx, name); err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "stack_delete", name, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "stack_delete", name, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"name":    name,
		"message": fmt.Sprintf("Stack %s torn down and archived successfully", name),
	})
}

// handleAPIDeployStack handles POST /api/v1/stacks and POST /api/v1/projects.
func (s *Server) handleAPIDeployStack(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name    string `json:"name"`
		Compose string `json:"compose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" || strings.TrimSpace(payload.Compose) == "" {
		http.Error(w, `{"error":"both 'name' and 'compose' are required"}`, http.StatusBadRequest)
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "stack_deploy", payload.Name)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	if err := s.fenrirClient.DeployProject(ctx, payload.Name, payload.Compose); err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "stack_deploy", payload.Name, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "stack_deploy", payload.Name, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"name":    payload.Name,
		"message": fmt.Sprintf("Stack %s deployed successfully", payload.Name),
	})
}

// handleAPIContainerAction handles container lifecycle actions (start/stop/restart) enforcing Operator/Admin RBAC.
func (s *Server) handleAPIContainerAction(w http.ResponseWriter, r *http.Request, action string) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		http.Error(w, `{"error":"missing container name"}`, http.StatusBadRequest)
		return
	}

	auditAction := "container_" + action
	user, ok := s.checkOperatorRBAC(w, r, auditAction, name)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var err error
	switch action {
	case "start":
		err = s.fenrirClient.StartContainer(ctx, name)
	case "stop":
		err = s.fenrirClient.StopContainer(ctx, name)
	case "restart":
		err = s.fenrirClient.RestartContainer(ctx, name)
	default:
		http.Error(w, `{"error":"invalid container action"}`, http.StatusBadRequest)
		return
	}

	if err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, name, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, name, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"container": name,
		"action":    action,
		"message":   fmt.Sprintf("Container %s %sed successfully", name, action),
	})
}

// handleAdminArchivesRedirect redirects /admin/archives to /admin?tab=archives.
func (s *Server) handleAdminArchivesRedirect(w http.ResponseWriter, r *http.Request) {
	if user, err := s.getSessionUser(r); err != nil || user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?tab=archives", http.StatusTemporaryRedirect)
}

// handleAPIListArchives lists all archived stacks from Fenrir Daemon.
func (s *Server) handleAPIListArchives(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkOperatorRBAC(w, r, "list_archives", "archives"); !ok {
		return
	}
	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}
	archives, err := s.fenrirClient.ListArchives(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(archives)
}

// handleAPIRestoreArchive handles POST /api/v1/archives/{id}/restore.
func (s *Server) handleAPIRestoreArchive(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, `{"error":"missing archive id"}`, http.StatusBadRequest)
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "archive_restore", id)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	if err := s.fenrirClient.RestoreArchive(ctx, id); err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "archive_restore", id, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "archive_restore", id, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"id":      id,
		"message": fmt.Sprintf("Archive %s restored back into stack directory successfully", id),
	})
}

// handleAPIDeleteArchive handles DELETE /api/v1/archives/{id}.
func (s *Server) handleAPIDeleteArchive(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, `{"error":"missing archive id"}`, http.StatusBadRequest)
		return
	}

	user, ok := s.checkAdminRBAC(w, r, "archive_purge", id)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	if err := s.fenrirClient.DeleteArchive(ctx, id); err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "archive_purge", id, "FAILED")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, "archive_purge", id, "SUCCESS")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"id":      id,
		"message": fmt.Sprintf("Archive %s permanently purged from disk and database", id),
	})
}

// handleAdminArchiveAction handles form and AJAX actions from the web UI for archives.
func (s *Server) handleAdminArchiveAction(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	action := strings.TrimSpace(r.FormValue("action"))
	if id == "" {
		id = strings.TrimSpace(r.PathValue("id"))
	}
	if id == "" {
		http.Error(w, `{"error":"missing archive id"}`, http.StatusBadRequest)
		return
	}

	auditAction := "archive_" + action
	if action == "delete" || action == "purge" {
		auditAction = "archive_purge"
	} else if action == "restore" {
		auditAction = "archive_restore"
	}

	user, ok := s.checkAdminRBAC(w, r, auditAction, id)
	if !ok {
		return
	}

	if s.fenrirClient == nil {
		http.Error(w, `{"error":"fenrir daemon client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var err error
	var msg string

	switch action {
	case "restore":
		err = s.fenrirClient.RestoreArchive(ctx, id)
		msg = fmt.Sprintf("Archive %s restored back into stack directory successfully", id)
	case "purge", "delete":
		err = s.fenrirClient.DeleteArchive(ctx, id)
		msg = fmt.Sprintf("Archive %s permanently purged from disk and database", id)
	default:
		http.Error(w, `{"error":"invalid archive action"}`, http.StatusBadRequest)
		return
	}

	isJSON := strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("X-Requested-With"), "XMLHttpRequest")

	if err != nil {
		_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, id, "FAILED")
		if !isJSON {
			http.Redirect(w, r, fmt.Sprintf("/admin?tab=archives&notification=Error: %s", url.QueryEscape(err.Error())), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = s.db.RecordAuditLog(ctx, user.DiscordID, user.Username, auditAction, id, "SUCCESS")

	if !isJSON {
		http.Redirect(w, r, fmt.Sprintf("/admin?tab=archives&notification=%s", url.QueryEscape(msg)), http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"id":      id,
		"action":  action,
		"message": msg,
	})
}
