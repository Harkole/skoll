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

	"github.com/bwmarrin/discordgo"

	"github.com/Harkole/skoll/internal/database"
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

