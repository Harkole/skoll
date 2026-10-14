package discord

import (
	"context"
	"path/filepath"
	"testing"

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
