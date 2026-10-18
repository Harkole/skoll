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

func TestRegisterApplicationCommandsFullPayloadIntegrity(t *testing.T) {
	var createdCommands []discordgo.ApplicationCommand
	var requestPaths []string

	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("failed to create discordgo session: %v", err)
	}

	session.Client = &http.Client{
		Transport: &interceptingTransport{
			handler: func(w http.ResponseWriter, r *http.Request) {
				requestPaths = append(requestPaths, r.URL.Path)
				var cmd discordgo.ApplicationCommand
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &cmd)
				cmd.ID = "cmd_" + cmd.Name
				createdCommands = append(createdCommands, cmd)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(cmd)
			},
		},
	}

	bot := &Bot{
		Session: session,
		cfg:     &config.Config{DiscordClientID: "test_app_123"},
	}

	// 1. Test Global Registration (guildID == "")
	registered, err := bot.RegisterApplicationCommands("test_app_123", "")
	if err != nil {
		t.Fatalf("RegisterApplicationCommands failed: %v", err)
	}
	if len(registered) != 4 {
		t.Fatalf("expected 4 root commands registered, got %d", len(registered))
	}

	for _, p := range requestPaths {
		if !strings.Contains(p, "/applications/test_app_123/commands") || strings.Contains(p, "guilds") {
			t.Errorf("expected global command path, got %s", p)
		}
	}

	// Verify /stack payload structure sent to Discord
	var stackCmd *discordgo.ApplicationCommand
	for _, c := range createdCommands {
		if c.Name == "stack" {
			stackCmd = &c
			break
		}
	}
	if stackCmd == nil {
		t.Fatalf("missing /stack in registered commands")
	}

	subMap := make(map[string]*discordgo.ApplicationCommandOption)
	for _, opt := range stackCmd.Options {
		subMap[opt.Name] = opt
	}

	for _, sub := range []string{"deploy", "list", "delete"} {
		if _, ok := subMap[sub]; !ok {
			t.Errorf("missing /stack %s subcommand in Discord registration payload", sub)
		}
	}

	// Verify /stack delete option integrity
	delOpt := subMap["delete"]
	if delOpt.Type != discordgo.ApplicationCommandOptionSubCommand {
		t.Errorf("expected /stack delete to be SubCommand, got %v", delOpt.Type)
	}
	if len(delOpt.Options) == 0 || delOpt.Options[0].Name != "name" || !delOpt.Options[0].Required {
		t.Errorf("expected /stack delete to require 'name' option: %+v", delOpt.Options)
	}

	// 2. Test Guild-Scoped Registration (guildID == "guild_456")
	requestPaths = nil
	createdCommands = nil
	registeredGuild, err := bot.RegisterApplicationCommands("test_app_123", "guild_456")
	if err != nil {
		t.Fatalf("guild RegisterApplicationCommands failed: %v", err)
	}
	if len(registeredGuild) != 4 {
		t.Fatalf("expected 4 root commands for guild, got %d", len(registeredGuild))
	}
	for _, p := range requestPaths {
		if !strings.Contains(p, "/applications/test_app_123/guilds/guild_456/commands") {
			t.Errorf("expected guild command path with guild_456, got %s", p)
		}
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
				"status":        "healthy",
				"version":       "2.5.0",
				"uptimeSeconds": 3665.0, // 1h 1m 5s
				"docker": map[string]any{
					"connected": true,
				},
				"database": map[string]any{
					"connected": true,
				},
			})
		case "/api/v1/host":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"host": map[string]any{
						"uptimeSeconds": 3665,
					},
					"cpu": map[string]any{
						"totalPercent": 18.4,
					},
					"memory": map[string]any{
						"usedBytes":   4294967296, // 4 GB
						"totalBytes":  8589934592, // 8 GB
						"usedPercent": 52.5,
					},
					"load": map[string]any{
						"load1":  0.75,
						"load5":  0.65,
						"load15": 0.55,
					},
					"disks": []map[string]any{
						{
							"mountPoint":  "/",
							"usedPercent": 33.0,
						},
					},
				},
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

func TestHandleStackDeleteCommand(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_stack_delete.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	adminUser, _ := db.UpsertUser(ctx, "discord_admin", "AdminBob", database.RoleAdmin)
	operatorUser, _ := db.UpsertUser(ctx, "discord_op", "OperatorAlice", database.RoleOperator)

	deleted := false
	mockFenrir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/valheim-server" {
			deleted = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "deleted"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockFenrir.Close()

	fenrirClient := fenrir.NewClient(mockFenrir.URL, "secret", 5*time.Second)
	bot := &Bot{
		cfg:          &config.Config{DiscordBotToken: "test-token", PublicURL: "http://localhost:8081"},
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

	// 1. Operator attempt -> Unauthorized (Admin required)
	opInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "interaction_stack_del_op",
			Token: "token_stack_del_op",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: operatorUser.DiscordID, Username: operatorUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "stack",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "delete",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}

	bot.HandleStackCommand(session, opInteraction)

	var opResp struct {
		Data struct {
			Embeds []*discordgo.MessageEmbed `json:"embeds"`
		} `json:"data"`
	}
	_ = json.Unmarshal(interceptedBody, &opResp)
	if len(opResp.Data.Embeds) == 0 || !strings.Contains(opResp.Data.Embeds[0].Title, "Permission Denied") {
		t.Errorf("expected Permission Denied for operator /stack delete, got %+v", opResp.Data.Embeds)
	}
	if deleted {
		t.Errorf("stack should NOT have been deleted by operator")
	}

	// 2. Admin attempt -> Success
	adminInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "interaction_stack_del_admin",
			Token: "token_stack_del_admin",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: adminUser.DiscordID, Username: adminUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "stack",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "delete",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}

	bot.HandleStackCommand(session, adminInteraction)

	var adminResp struct {
		Embeds []*discordgo.MessageEmbed `json:"embeds"`
	}
	_ = json.Unmarshal(interceptedBody, &adminResp)
	if len(adminResp.Embeds) == 0 || !strings.Contains(adminResp.Embeds[0].Title, "Stack Torn Down") {
		t.Errorf("expected Stack Torn Down for admin /stack delete, got: %s", string(interceptedBody))
	}
	if !deleted {
		t.Errorf("expected stack to be deleted in Fenrir by admin")
	}

	// Check audit log
	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil || len(logs) < 2 {
		t.Fatalf("expected audit logs for stack delete attempts")
	}
	// Most recent log should be SUCCESS by admin
	if logs[0].Action != "stack_delete" || logs[0].Status != "SUCCESS" || logs[0].Username != adminUser.Username {
		t.Errorf("unexpected most recent audit log: %+v", logs[0])
	}
	// Previous log should be UNAUTHORIZED by operator
	if logs[1].Action != "stack_delete" || logs[1].Status != "UNAUTHORIZED" || logs[1].Username != operatorUser.Username {
		t.Errorf("unexpected operator audit log: %+v", logs[1])
	}
}

func TestDiscordRoleBasedPrivilegesAlignment(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_discord_privileges.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	adminUser, _ := db.UpsertUser(ctx, "discord_admin", "AdminBob", database.RoleAdmin)
	operatorUser, _ := db.UpsertUser(ctx, "discord_op", "OperatorAlice", database.RoleOperator)
	viewerUser, _ := db.UpsertUser(ctx, "discord_viewer", "ViewerCharlie", database.RoleViewer)

	restarted := false
	deployed := false
	mockFenrir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/valheim-server/restart" {
			restarted = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "restarted"})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects" {
			deployed = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "accepted"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockFenrir.Close()

	fenrirClient := fenrir.NewClient(mockFenrir.URL, "secret", 5*time.Second)
	bot := &Bot{
		cfg:          &config.Config{DiscordBotToken: "test-token", PublicURL: "http://localhost:8081"},
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

	// 1. /server restart: Viewer attempt -> Permission Denied
	viewerRestartInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "i_srv_viewer",
			Token: "t_srv_viewer",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: viewerUser.DiscordID, Username: viewerUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "server",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "restart",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}
	bot.HandleServerCommand(session, viewerRestartInteraction)
	var viewerResp struct {
		Data struct {
			Embeds []*discordgo.MessageEmbed `json:"embeds"`
		} `json:"data"`
	}
	_ = json.Unmarshal(interceptedBody, &viewerResp)
	if len(viewerResp.Data.Embeds) == 0 || !strings.Contains(viewerResp.Data.Embeds[0].Title, "Permission Denied") {
		t.Errorf("expected Permission Denied for viewer /server restart, got: %s", string(interceptedBody))
	}
	if restarted {
		t.Errorf("stack should not have restarted for viewer")
	}

	// 2. /server restart: Operator attempt -> Success
	opRestartInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "i_srv_op",
			Token: "t_srv_op",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: operatorUser.DiscordID, Username: operatorUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "server",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "restart",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}
	bot.HandleServerCommand(session, opRestartInteraction)
	var opResp struct {
		Embeds []*discordgo.MessageEmbed `json:"embeds"`
	}
	_ = json.Unmarshal(interceptedBody, &opResp)
	if len(opResp.Embeds) == 0 || !strings.Contains(opResp.Embeds[0].Title, "Restarted") {
		t.Errorf("expected Restarted for operator /server restart, got: %s", string(interceptedBody))
	}
	if !restarted {
		t.Errorf("expected stack to be restarted by operator")
	}

	// 3. /stack deploy: Operator attempt -> Permission Denied
	opDeployInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "i_dep_op",
			Token: "t_dep_op",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: operatorUser.DiscordID, Username: operatorUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "stack",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "deploy",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
							{Name: "compose", Value: "services:\n  valheim:\n    image: valheim:latest", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}
	bot.HandleStackCommand(session, opDeployInteraction)
	var opDeployResp struct {
		Data struct {
			Embeds []*discordgo.MessageEmbed `json:"embeds"`
		} `json:"data"`
	}
	_ = json.Unmarshal(interceptedBody, &opDeployResp)
	if len(opDeployResp.Data.Embeds) == 0 || !strings.Contains(opDeployResp.Data.Embeds[0].Title, "Permission Denied") {
		t.Errorf("expected Permission Denied for operator /stack deploy, got: %s", string(interceptedBody))
	}
	if deployed {
		t.Errorf("stack should not have deployed for operator")
	}

	// 4. /stack deploy: Admin attempt -> Success
	adminDeployInteraction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:    "i_dep_admin",
			Token: "t_dep_admin",
			Type:  discordgo.InteractionApplicationCommand,
			Member: &discordgo.Member{
				User: &discordgo.User{ID: adminUser.DiscordID, Username: adminUser.Username},
			},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "stack",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name: "deploy",
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{Name: "name", Value: "valheim-server", Type: discordgo.ApplicationCommandOptionString},
							{Name: "compose", Value: "services:\n  valheim:\n    image: valheim:latest", Type: discordgo.ApplicationCommandOptionString},
						},
					},
				},
			},
		},
	}
	bot.HandleStackCommand(session, adminDeployInteraction)
	var adminDeployResp struct {
		Embeds []*discordgo.MessageEmbed `json:"embeds"`
	}
	_ = json.Unmarshal(interceptedBody, &adminDeployResp)
	if len(adminDeployResp.Embeds) == 0 || !strings.Contains(adminDeployResp.Embeds[0].Title, "Deployed") {
		t.Errorf("expected Deployed for admin /stack deploy, got: %s", string(interceptedBody))
	}
	if !deployed {
		t.Errorf("expected stack to be deployed by admin")
	}
}
