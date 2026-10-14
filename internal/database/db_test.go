package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_skoll.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. First user should become Admin automatically
	u1, err := db.UpsertUser(ctx, "discord_111", "Alice", RoleViewer)
	if err != nil {
		t.Fatalf("failed to upsert user 1: %v", err)
	}
	if u1.Role != RoleAdmin {
		t.Errorf("expected first user to be Admin, got %s", u1.Role)
	}
	if u1.Username != "Alice" {
		t.Errorf("expected username Alice, got %s", u1.Username)
	}

	// 2. Second user gets default RoleViewer
	u2, err := db.UpsertUser(ctx, "discord_222", "Bob", RoleViewer)
	if err != nil {
		t.Fatalf("failed to upsert user 2: %v", err)
	}
	if u2.Role != RoleViewer {
		t.Errorf("expected second user to be Viewer, got %s", u2.Role)
	}

	// 3. Upsert existing user: username updates, role is preserved
	u1Updated, err := db.UpsertUser(ctx, "discord_111", "AliceUpdated", RoleOperator)
	if err != nil {
		t.Fatalf("failed to update user 1: %v", err)
	}
	if u1Updated.Username != "AliceUpdated" {
		t.Errorf("expected AliceUpdated, got %s", u1Updated.Username)
	}
	if u1Updated.Role != RoleAdmin {
		t.Errorf("expected role to stay Admin, got %s", u1Updated.Role)
	}

	// 4. Update role explicitly
	if err := db.UpdateUserRole(ctx, u2.ID, RoleOperator); err != nil {
		t.Fatalf("failed to update user 2 role: %v", err)
	}
	u2Fetched, err := db.GetUserByID(ctx, u2.ID)
	if err != nil {
		t.Fatalf("failed to get user 2: %v", err)
	}
	if u2Fetched.Role != RoleOperator {
		t.Errorf("expected Operator role, got %s", u2Fetched.Role)
	}

	// 5. Test invalid role rejection
	if err := db.UpdateUserRole(ctx, u2.ID, "SuperUser"); err != ErrInvalidRole {
		t.Errorf("expected ErrInvalidRole, got %v", err)
	}

	// 6. Test list users
	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	// 7. Audit Logging
	if err := db.RecordAuditLog(ctx, "discord_111", "AliceUpdated", "server_restart", "minecraft-prod", "SUCCESS"); err != nil {
		t.Fatalf("failed to record audit log: %v", err)
	}
	if err := db.RecordAuditLog(ctx, "discord_222", "Bob", "stack_deploy", "valheim-server", "UNAUTHORIZED"); err != nil {
		t.Fatalf("failed to record audit log: %v", err)
	}

	logs, err := db.ListAuditLogs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to list audit logs: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 audit logs, got %d", len(logs))
	}
	if logs[0].Action != "stack_deploy" || logs[0].Status != "UNAUTHORIZED" {
		t.Errorf("expected newest log to be stack_deploy, got %s", logs[0].Action)
	}

	// 8. Link State management
	token := "state_token_xyz_123"
	if err := db.CreateLinkState(ctx, token, "discord_333", 10*time.Minute); err != nil {
		t.Fatalf("failed to create link state: %v", err)
	}

	consumed, err := db.ConsumeLinkState(ctx, token)
	if err != nil {
		t.Fatalf("failed to consume link state: %v", err)
	}
	if consumed.DiscordUserID != "discord_333" {
		t.Errorf("expected discord_333, got %s", consumed.DiscordUserID)
	}

	// Replay must fail
	_, err = db.ConsumeLinkState(ctx, token)
	if err != ErrLinkStateNotFound {
		t.Errorf("expected ErrLinkStateNotFound on reuse, got %v", err)
	}
}
