package database

import (
	"strings"
	"time"
)

// Role represents user authorization privilege levels.
type Role string

const (
	RoleAdmin    Role = "Admin"
	RoleOperator Role = "Operator"
	RoleViewer   Role = "Viewer"
	RoleUnlinked Role = "Unlinked"
)

// IsValid checks if the role is a valid known role.
func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleViewer, RoleUnlinked:
		return true
	default:
		return false
	}
}

// Level returns an integer value representing privilege hierarchy:
// Admin (3) > Operator (2) > Viewer (1) > Unlinked (0).
func (r Role) Level() int {
	switch r {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// CanExecute returns true if the user's role meets or exceeds the required role.
func (r Role) CanExecute(required Role) bool {
	return r.Level() >= required.Level()
}

// ParseRole converts a string into a Role, returning Viewer if invalid or empty.
func ParseRole(s string) Role {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "admin":
		return RoleAdmin
	case "operator":
		return RoleOperator
	case "viewer":
		return RoleViewer
	case "unlinked":
		return RoleUnlinked
	default:
		return RoleViewer
	}
}

// User represents a linked Discord identity and assigned authorization role.
type User struct {
	ID        int64     `json:"id"`
	DiscordID string    `json:"discord_id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditLog records security-relevant user interactions and orchestrator commands.
type AuditLog struct {
	ID            int64     `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	DiscordUserID string    `json:"discord_user_id"`
	Username      string    `json:"username"`
	Action        string    `json:"action"`
	TargetProject string    `json:"target_project"`
	Status        string    `json:"status"` // e.g. "SUCCESS", "FAILED", "UNAUTHORIZED"
}

// LinkState stores pending Discord OAuth2 linking sessions.
type LinkState struct {
	Token         string    `json:"token"`
	DiscordUserID string    `json:"discord_user_id"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}
