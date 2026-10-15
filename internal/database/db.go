package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrLinkStateNotFound = errors.New("link state not found or expired")
	ErrInvalidRole       = errors.New("invalid role")
)

// DB wraps the SQLite database connection with helper methods.
type DB struct {
	db *sql.DB
	mu sync.RWMutex
}

// Open initializes SQLite connection, runs schema migrations, and sets pragmas.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// SQLite connection string with WAL mode and busy timeout for concurrent safety
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Limit idle connections to avoid locked file handles in WAL
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db := &DB{db: conn}
	if err := db.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return db, nil
}

// Close closes the underlying database handle.
func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		discord_id TEXT UNIQUE NOT NULL,
		username TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('Admin', 'Operator', 'Viewer', 'Unlinked')),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_users_discord_id ON users(discord_id);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		discord_user_id TEXT NOT NULL,
		username TEXT NOT NULL,
		action TEXT NOT NULL,
		target_project TEXT NOT NULL,
		status TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);

	CREATE TABLE IF NOT EXISTS link_states (
		token TEXT PRIMARY KEY,
		discord_user_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_link_states_expires ON link_states(expires_at);
	`
	_, err := d.db.Exec(schema)
	return err
}

// UpsertUser inserts a newly linked user or updates an existing user's username and updated_at.
// If it's the very first user in the system, they are automatically granted the 'Admin' role.
func (d *DB) UpsertUser(ctx context.Context, discordID, username string, defaultRole Role) (*User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()

	// Check if this user already exists
	var existing User
	var roleStr string
	query := `SELECT id, discord_id, username, role, created_at, updated_at FROM users WHERE discord_id = ?`
	row := d.db.QueryRowContext(ctx, query, discordID)
	err := row.Scan(&existing.ID, &existing.DiscordID, &existing.Username, &roleStr, &existing.CreatedAt, &existing.UpdatedAt)

	if err == nil {
		// Existing user: update username and timestamp, keep existing role
		existing.Username = username
		existing.Role = Role(roleStr)
		existing.UpdatedAt = now

		updateQuery := `UPDATE users SET username = ?, updated_at = ? WHERE id = ?`
		_, err = d.db.ExecContext(ctx, updateQuery, existing.Username, existing.UpdatedAt, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		return &existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query existing user: %w", err)
	}

	// New user: determine role
	var count int
	_ = d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	assignedRole := defaultRole
	if count == 0 {
		// First user becomes Admin by default for convenience
		assignedRole = RoleAdmin
	} else if !assignedRole.IsValid() {
		assignedRole = RoleViewer
	}

	insertQuery := `
	INSERT INTO users (discord_id, username, role, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	`
	res, err := d.db.ExecContext(ctx, insertQuery, discordID, username, string(assignedRole), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:        id,
		DiscordID: discordID,
		Username:  username,
		Role:      assignedRole,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetUserByDiscordID finds a user by their unique Discord snowflake ID.
func (d *DB) GetUserByDiscordID(ctx context.Context, discordID string) (*User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var u User
	var roleStr string
	query := `SELECT id, discord_id, username, role, created_at, updated_at FROM users WHERE discord_id = ?`
	row := d.db.QueryRowContext(ctx, query, discordID)
	err := row.Scan(&u.ID, &u.DiscordID, &u.Username, &roleStr, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	u.Role = Role(roleStr)
	return &u, nil
}

// GetUserByID finds a user by database ID.
func (d *DB) GetUserByID(ctx context.Context, id int64) (*User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var u User
	var roleStr string
	query := `SELECT id, discord_id, username, role, created_at, updated_at FROM users WHERE id = ?`
	row := d.db.QueryRowContext(ctx, query, id)
	err := row.Scan(&u.ID, &u.DiscordID, &u.Username, &roleStr, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	u.Role = Role(roleStr)
	return &u, nil
}

// ListUsers retrieves all registered users ordered by ID ascending.
func (d *DB) ListUsers(ctx context.Context) ([]*User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `SELECT id, discord_id, username, role, created_at, updated_at FROM users ORDER BY id ASC`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var roleStr string
		if err := rows.Scan(&u.ID, &u.DiscordID, &u.Username, &roleStr, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Role = Role(roleStr)
		users = append(users, &u)
	}
	return users, rows.Err()
}

// UpdateUserRole updates an existing user's role and bumps updated_at.
func (d *DB) UpdateUserRole(ctx context.Context, id int64, newRole Role) error {
	if !newRole.IsValid() {
		return ErrInvalidRole
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	query := `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`
	res, err := d.db.ExecContext(ctx, query, string(newRole), now, id)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

// RecordAuditLog records an event in the audit_logs table.
func (d *DB) RecordAuditLog(ctx context.Context, discordUserID, username, action, targetProject, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO audit_logs (timestamp, discord_user_id, username, action, target_project, status)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := d.db.ExecContext(ctx, query, now, discordUserID, username, action, targetProject, status)
	if err != nil {
		return fmt.Errorf("failed to record audit log: %w", err)
	}
	return nil
}

// ListAuditLogs fetches the most recent audit logs up to limit (max 200).
func (d *DB) ListAuditLogs(ctx context.Context, limit int) ([]*AuditLog, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
	SELECT id, timestamp, discord_user_id, username, action, target_project, status
	FROM audit_logs
	ORDER BY timestamp DESC, id DESC
	LIMIT ?
	`
	rows, err := d.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.Timestamp, &a.DiscordUserID, &a.Username, &a.Action, &a.TargetProject, &a.Status); err != nil {
			return nil, err
		}
		logs = append(logs, &a)
	}
	return logs, rows.Err()
}

// CreateLinkState stores an ephemeral link token associated with a Discord user ID.
func (d *DB) CreateLinkState(ctx context.Context, token, discordUserID string, ttl time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	expires := now.Add(ttl)

	query := `INSERT INTO link_states (token, discord_user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`
	_, err := d.db.ExecContext(ctx, query, token, discordUserID, now, expires)
	if err != nil {
		return fmt.Errorf("failed to create link state: %w", err)
	}
	return nil
}

// ConsumeLinkState validates and deletes an ephemeral link token atomically.
func (d *DB) ConsumeLinkState(ctx context.Context, token string) (*LinkState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	var state LinkState
	query := `SELECT token, discord_user_id, created_at, expires_at FROM link_states WHERE token = ? AND expires_at > ?`
	row := d.db.QueryRowContext(ctx, query, token, now)
	if err := row.Scan(&state.Token, &state.DiscordUserID, &state.CreatedAt, &state.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLinkStateNotFound
		}
		return nil, err
	}

	// Delete once consumed to prevent replay attacks
	_, _ = d.db.ExecContext(ctx, `DELETE FROM link_states WHERE token = ?`, token)
	return &state, nil
}

// PurgeExpiredLinkStates cleans up old expired state tokens.
func (d *DB) PurgeExpiredLinkStates(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx, `DELETE FROM link_states WHERE expires_at <= ?`, now)
	return err
}
