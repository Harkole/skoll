package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Harkole/skoll/internal/config"
	"github.com/Harkole/skoll/internal/database"
	"github.com/Harkole/skoll/internal/fenrir"
)

func TestRBACAuthorize(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_discord_rbac.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Unlinked user (not in DB)
	_, err = Authorize(ctx, db, "unknown_discord_id", database.RoleViewer)
	if err != ErrUnlinkedUser {
		t.Errorf("expected ErrUnlinkedUser for unknown discord user, got %v", err)
	}

	// 2. Insert Admin (first user is Admin)
	adminUser, err := db.UpsertUser(ctx, "discord_admin", "AdminUser", database.RoleAdmin)
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	// 3. Insert Operator
	opUser, err := db.UpsertUser(ctx, "discord_op", "OpUser", database.RoleViewer)
	if err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}
	_ = db.UpdateUserRole(ctx, opUser.ID, database.RoleOperator)

	// 4. Insert Viewer
	viewerUser, err := db.UpsertUser(ctx, "discord_viewer", "ViewerUser", database.RoleViewer)
	if err != nil {
		t.Fatalf("failed to create viewer: %v", err)
	}

	// Test Authorize: Viewer requesting Viewer -> PASS
	u, err := Authorize(ctx, db, viewerUser.DiscordID, database.RoleViewer)
	if err != nil || u == nil {
		t.Errorf("expected viewer to pass Viewer check, got %v", err)
	}

	// Test Authorize: Viewer requesting Operator -> FAIL (ErrInsufficientRole)
	_, err = Authorize(ctx, db, viewerUser.DiscordID, database.RoleOperator)
	if err != ErrInsufficientRole {
		t.Errorf("expected ErrInsufficientRole for Viewer requesting Operator, got %v", err)
	}

	// Test Authorize: Viewer requesting Admin -> FAIL
	_, err = Authorize(ctx, db, viewerUser.DiscordID, database.RoleAdmin)
	if err != ErrInsufficientRole {
		t.Errorf("expected ErrInsufficientRole for Viewer requesting Admin, got %v", err)
	}

	// Test Authorize: Operator requesting Operator -> PASS
	u, err = Authorize(ctx, db, opUser.DiscordID, database.RoleOperator)
	if err != nil || u == nil {
		t.Errorf("expected Operator to pass Operator check, got %v", err)
	}

	// Test Authorize: Operator requesting Admin -> FAIL
	_, err = Authorize(ctx, db, opUser.DiscordID, database.RoleAdmin)
	if err != ErrInsufficientRole {
		t.Errorf("expected Operator requesting Admin to fail with ErrInsufficientRole, got %v", err)
	}

	// Test Authorize: Admin requesting all roles -> PASS
	if _, err := Authorize(ctx, db, adminUser.DiscordID, database.RoleViewer); err != nil {
		t.Errorf("admin failed viewer check: %v", err)
	}
	if _, err := Authorize(ctx, db, adminUser.DiscordID, database.RoleOperator); err != nil {
		t.Errorf("admin failed operator check: %v", err)
	}
	if _, err := Authorize(ctx, db, adminUser.DiscordID, database.RoleAdmin); err != nil {
		t.Errorf("admin failed admin check: %v", err)
	}

	// 5. Test explicitly unlinked role in DB
	_ = db.UpdateUserRole(ctx, viewerUser.ID, database.RoleUnlinked)
	_, err = Authorize(ctx, db, viewerUser.DiscordID, database.RoleViewer)
	if err != ErrUnlinkedUser {
		t.Errorf("expected RoleUnlinked to return ErrUnlinkedUser, got %v", err)
	}
}

func TestGenerateStateToken(t *testing.T) {
	t1, err := GenerateStateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(t1) != 64 { // 32 bytes = 64 hex characters
		t.Errorf("expected 64 char token, got %d", len(t1))
	}

	t2, err := GenerateStateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t1 == t2 {
		t.Errorf("generated state tokens must be cryptographically unique")
	}
}

func TestCommandDefinitions(t *testing.T) {
	expected := map[string]bool{
		"link":   true,
		"status": true,
		"server": true,
		"stack":  true,
	}

	found := make(map[string]bool)
	for _, cmd := range Commands {
		found[cmd.Name] = true
		if cmd.Description == "" {
			t.Errorf("command %s has empty description", cmd.Name)
		}
	}

	for exp := range expected {
		if !found[exp] {
			t.Errorf("missing expected command: %s", exp)
		}
	}
}

type interceptingTransport struct {
	handler http.HandlerFunc
}

func (it *interceptingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	it.handler(rec, req)
	return rec.Result(), nil
}

func TestInteractionDeferralAndEdit(t *testing.T) {
	var lastMethod, lastPath string
	var lastBody []byte

	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("failed to create discordgo session: %v", err)
	}

	session.Client = &http.Client{
		Transport: &interceptingTransport{
			handler: func(w http.ResponseWriter, r *http.Request) {
				lastMethod = r.Method
				lastPath = r.URL.Path
				lastBody, _ = io.ReadAll(r.Body)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"id":"msg123","content":"ok"}`))
			},
		},
	}

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "interaction_999",
			Token: "interaction_token_abc",
			AppID: "app_123",
		},
	}

	// 1. Test DeferResponse
	if err := DeferResponse(session, interaction); err != nil {
		t.Fatalf("unexpected error from DeferResponse: %v", err)
	}
	if lastMethod != http.MethodPost {
		t.Errorf("expected POST method, got %s", lastMethod)
	}
	if !strings.Contains(lastPath, "interaction_999") || !strings.Contains(lastPath, "callback") {
		t.Errorf("unexpected callback path: %s", lastPath)
	}

	var respPayload struct {
		Type int `json:"type"`
	}
	_ = json.Unmarshal(lastBody, &respPayload)
	// Type 5 is InteractionResponseDeferredChannelMessageWithSource
	if respPayload.Type != int(discordgo.InteractionResponseDeferredChannelMessageWithSource) {
		t.Errorf("expected response type %d, got %d", discordgo.InteractionResponseDeferredChannelMessageWithSource, respPayload.Type)
	}

	// 2. Test DeferEphemeralResponse
	if err := DeferEphemeralResponse(session, interaction); err != nil {
		t.Fatalf("unexpected error from DeferEphemeralResponse: %v", err)
	}
	var ephemeralPayload struct {
		Type int `json:"type"`
		Data struct {
			Flags int `json:"flags"`
		} `json:"data"`
	}
	_ = json.Unmarshal(lastBody, &ephemeralPayload)
	if ephemeralPayload.Data.Flags != int(discordgo.MessageFlagsEphemeral) {
		t.Errorf("expected ephemeral flag %d, got %d", discordgo.MessageFlagsEphemeral, ephemeralPayload.Data.Flags)
	}

	// 3. Test EditResponseEmbed
	testEmbed := &discordgo.MessageEmbed{
		Title:       "Test Success",
		Description: "Operation completed",
	}
	if err := EditResponseEmbed(session, interaction, testEmbed); err != nil {
		t.Fatalf("unexpected error from EditResponseEmbed: %v", err)
	}
	if lastMethod != http.MethodPatch {
		t.Errorf("expected PATCH method for webhook edit, got %s", lastMethod)
	}
	if !strings.Contains(lastPath, "webhooks") || !strings.Contains(lastPath, "@original") {
		t.Errorf("unexpected webhook edit path: %s", lastPath)
	}
	if !strings.Contains(string(lastBody), "Test Success") {
		t.Errorf("expected embed title in request body: %s", string(lastBody))
	}

	// 4. Test EditResponseText
	if err := EditResponseText(session, interaction, "Updated text response"); err != nil {
		t.Fatalf("unexpected error from EditResponseText: %v", err)
	}
	if !strings.Contains(string(lastBody), "Updated text response") {
		t.Errorf("expected updated text in request body: %s", string(lastBody))
	}
}

func TestHandleStatusCommandSeparation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_status_cmd.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	viewerUser, err := db.UpsertUser(ctx, "discord_123", "ViewerBob", database.RoleViewer)
	if err != nil {
		t.Fatalf("failed to upsert user: %v", err)
	}

	// Mock Fenrir Server with health and host endpoints
	mockFenrir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":         "healthy",
				"version":        "2.5.0",
				"uptime_seconds": 3665, // 1h 1m 5s
			})
		case "/api/v1/host":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"cpu_percent":       18.4,
				"memory_percent":    52.5,
				"memory_used_bytes": 4294967296, // 4 GB
				"memory_total":      8589934592, // 8 GB
				"load_average":      []float64{0.75, 0.65, 0.55},
				"disk_percent":      33.0,
			})
		case "/api/v1/projects":
			// If /status mistakenly calls projects, return a dummy project to detect leak
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"name": "leaked-project", "status": "running"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockFenrir.Close()

	fenrirClient := fenrir.NewClient(mockFenrir.URL, "secret", 5*time.Second)
	cfg := &config.Config{
		DiscordBotToken: "test-token",
		PublicURL:       "http://localhost:8081",
	}

	bot := &Bot{
		cfg:          cfg,
		db:           db,
		fenrirClient: fenrirClient,
	}

	var interceptedBody []byte
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	session.Client = &http.Client{
		Transport: &interceptingTransport{
			handler: func(w http.ResponseWriter, r *http.Request) {
				interceptedBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		},
	}

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "interaction_status",
			Token: "token_status",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{
					ID:       viewerUser.DiscordID,
					Username: viewerUser.Username,
				},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "status",
			},
		},
	}

	bot.HandleStatusCommand(session, interaction)

	var resp struct {
		Type int `json:"type"`
		Data struct {
			Embeds []*discordgo.MessageEmbed `json:"embeds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(interceptedBody, &resp); err != nil {
		t.Fatalf("failed to unmarshal intercepted response: %v", err)
	}

	if len(resp.Data.Embeds) == 0 {
		t.Fatalf("expected at least one embed in response")
	}

	embed := resp.Data.Embeds[0]
	if !strings.Contains(embed.Title, "Fenrir Daemon Status") {
		t.Errorf("expected title to mention Fenrir Daemon Status, got %s", embed.Title)
	}
	if !strings.Contains(embed.Description, "Healthy") {
		t.Errorf("expected description to show Healthy daemon health, got %s", embed.Description)
	}

	// Verify fields: version, uptime, cpu, ram, load averages, disk
	fieldMap := make(map[string]string)
	for _, f := range embed.Fields {
		fieldMap[f.Name] = f.Value
	}

	if fieldMap["Version"] != "2.5.0" {
		t.Errorf("expected Version 2.5.0, got %s", fieldMap["Version"])
	}
	if fieldMap["Uptime"] != "1h 1m 5s" {
		t.Errorf("expected Uptime 1h 1m 5s, got %s", fieldMap["Uptime"])
	}
	if fieldMap["CPU Usage"] != "18.4%" {
		t.Errorf("expected CPU Usage 18.4%%, got %s", fieldMap["CPU Usage"])
	}
	if !strings.Contains(fieldMap["RAM Usage"], "52.5%") || !strings.Contains(fieldMap["RAM Usage"], "4.00 / 8.00 GB") {
		t.Errorf("unexpected RAM Usage field: %s", fieldMap["RAM Usage"])
	}
	if fieldMap["Load Averages (1m, 5m, 15m)"] != "0.75, 0.65, 0.55" {
		t.Errorf("unexpected Load Averages: %s", fieldMap["Load Averages (1m, 5m, 15m)"])
	}
	if fieldMap["Disk Usage"] != "33.0%" {
		t.Errorf("unexpected Disk Usage: %s", fieldMap["Disk Usage"])
	}

	// Ensure NO stack itemization is in embed (clean separation)
	for k, v := range fieldMap {
		if strings.Contains(k, "Stack") || strings.Contains(v, "leaked-project") {
			t.Errorf("status response must not itemize stacks: field %s = %s", k, v)
		}
	}
	if strings.Contains(embed.Description, "leaked-project") {
		t.Errorf("status description contains stack information: %s", embed.Description)
	}

	// Check audit log
	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected audit log to be created")
	}
	if logs[0].Action != "status_query" || logs[0].TargetProject != "host" || logs[0].Status != "SUCCESS" {
		t.Errorf("unexpected audit log entry: %+v", logs[0])
	}
}

func TestHandleStackListCommand(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_stack_list.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	viewerUser, err := db.UpsertUser(ctx, "discord_456", "StackViewer", database.RoleViewer)
	if err != nil {
		t.Fatalf("failed to upsert user: %v", err)
	}

	mockFenrir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/projects" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":   "valheim-server",
						"status": "running",
						"services": []map[string]any{
							{"name": "valheim", "state": "running"},
							{"name": "backup", "state": "running"},
						},
					},
					{
						"name":   "palworld-server",
						"status": "stopped",
						"services": []map[string]any{
							{"name": "palworld", "state": "exited"},
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockFenrir.Close()

	fenrirClient := fenrir.NewClient(mockFenrir.URL, "secret", 5*time.Second)
	cfg := &config.Config{
		DiscordBotToken: "test-token",
		PublicURL:       "http://localhost:8081",
	}

	bot := &Bot{
		cfg:          cfg,
		db:           db,
		fenrirClient: fenrirClient,
	}

	var interceptedBody []byte
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	session.Client = &http.Client{
		Transport: &interceptingTransport{
			handler: func(w http.ResponseWriter, r *http.Request) {
				interceptedBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		},
	}

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "interaction_stack_list",
			Token: "token_stack_list",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{
					ID:       viewerUser.DiscordID,
					Username: viewerUser.Username,
				},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "stack",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "list",
					},
				},
			},
		},
	}

	bot.HandleStackCommand(session, interaction)

	var resp struct {
		Type int `json:"type"`
		Data struct {
			Embeds []*discordgo.MessageEmbed `json:"embeds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(interceptedBody, &resp); err != nil {
		t.Fatalf("failed to unmarshal intercepted response: %v", err)
	}

	if len(resp.Data.Embeds) == 0 {
		t.Fatalf("expected at least one embed in response")
	}

	embed := resp.Data.Embeds[0]
	if !strings.Contains(embed.Title, "Registered Docker Compose Stacks (2)") {
		t.Errorf("expected title to show 2 registered stacks, got %s", embed.Title)
	}

	if !strings.Contains(embed.Description, "valheim-server") || !strings.Contains(embed.Description, "2/2 services running") {
		t.Errorf("expected valheim-server info in description, got %s", embed.Description)
	}
	if !strings.Contains(embed.Description, "palworld-server") || !strings.Contains(embed.Description, "0/1 services running") {
		t.Errorf("expected palworld-server info in description, got %s", embed.Description)
	}

	// Check audit log
	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected audit log to be created")
	}
	if logs[0].Action != "stack_list" || logs[0].TargetProject != "all" || logs[0].Status != "SUCCESS" {
		t.Errorf("unexpected audit log entry: %+v", logs[0])
	}
}

