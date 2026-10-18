package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configurations for Sköll.
type Config struct {
	// Server settings
	Port        string // e.g. "8081" or ":8081"
	PublicURL   string // Base URL for OAuth redirects, e.g. "http://localhost:8081"
	AdminKey    string // Secret token for fallback Admin UI access
	DatabasePath string // Path to SQLite database file

	// Discord Bot & OAuth2 credentials
	DiscordBotToken     string
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
	DiscordGuildID      string // Target Guild ID for instant slash command sync without global 1hr cache delay
	SessionSecret       string // Secret key for AES-256 session encryption

	// Fenrir Daemon connection
	FenrirAPIURL string
	FenrirAPIKey string

	// Timeouts & tuning
	HTTPTimeout time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
// It also parses a local .env file if present in the current working directory.
func Load() (*Config, error) {
	loadDotEnv(".env")

	port := getEnv("SKOLL_PORT", "8081")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	dbPath := getEnv("SKOLL_DB_PATH", "skoll.db")
	publicURL := strings.TrimRight(getEnv("SKOLL_PUBLIC_URL", "http://localhost"+port), "/")
	adminKey := getEnv("SKOLL_ADMIN_KEY", "skoll-admin-secret")

	botToken := getEnv("DISCORD_BOT_TOKEN", "")
	clientID := getEnv("DISCORD_CLIENT_ID", "")
	clientSecret := getEnv("DISCORD_CLIENT_SECRET", "")
	redirectURI := getEnv("DISCORD_REDIRECT_URI", publicURL+"/auth/callback")

	sessionSecret := getEnv("SKOLL_SESSION_SECRET", "")
	if sessionSecret == "" {
		sessionSecret = clientSecret + ":" + adminKey
	}

	fenrirURL := strings.TrimRight(getEnv("FENRIR_API_URL", "http://localhost:8080"), "/")
	fenrirKey := getEnv("FENRIR_API_KEY", "")

	guildID := getEnv("DISCORD_GUILD_ID", getEnv("SKOLL_DISCORD_GUILD_ID", ""))

	timeoutSec := getEnvAsInt("SKOLL_HTTP_TIMEOUT_SECONDS", 15)

	cfg := &Config{
		Port:                port,
		PublicURL:           publicURL,
		AdminKey:            adminKey,
		DatabasePath:        dbPath,
		DiscordBotToken:     botToken,
		DiscordClientID:     clientID,
		DiscordClientSecret: clientSecret,
		DiscordRedirectURI:  redirectURI,
		DiscordGuildID:      guildID,
		SessionSecret:       sessionSecret,
		FenrirAPIURL:        fenrirURL,
		FenrirAPIKey:        fenrirKey,
		HTTPTimeout:         time.Duration(timeoutSec) * time.Second,
	}

	return cfg, nil
}

// Validate validates that critical parameters are provided when bot/OAuth is required.
func (c *Config) Validate(strict bool) error {
	if c.Port == "" {
		return fmt.Errorf("SKOLL_PORT cannot be empty")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("SKOLL_DB_PATH cannot be empty")
	}

	if strict {
		if c.DiscordBotToken == "" {
			return fmt.Errorf("DISCORD_BOT_TOKEN is required in strict mode")
		}
		if c.DiscordClientID == "" {
			return fmt.Errorf("DISCORD_CLIENT_ID is required in strict mode")
		}
		if c.DiscordClientSecret == "" {
			return fmt.Errorf("DISCORD_CLIENT_SECRET is required in strict mode")
		}
		if c.FenrirAPIURL == "" {
			return fmt.Errorf("FENRIR_API_URL is required in strict mode")
		}
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if valStr, ok := os.LookupEnv(key); ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			return val
		}
	}
	return defaultVal
}

// loadDotEnv parses key=value pairs from a file and sets them in the environment if not already set.
func loadDotEnv(filepath string) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

