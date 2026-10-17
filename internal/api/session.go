package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Harkole/skoll/internal/database"
)

const (
	// SessionCookieName defines the name of the HTTP-only session cookie.
	SessionCookieName = "skoll_session"

	// DefaultSessionTTL defines session duration (7 days).
	DefaultSessionTTL = 7 * 24 * time.Hour
)

// SessionData represents the decrypted payload stored inside the encrypted session cookie.
type SessionData struct {
	DiscordUserID string        `json:"uid"`
	Username      string        `json:"name"`
	Role          database.Role `json:"role"`
	CreatedAt     int64         `json:"iat"`
	ExpiresAt     int64         `json:"exp"`
}

// SessionManager provides authenticated encryption (AES-256-GCM) and cookie management.
type SessionManager struct {
	key []byte
}

// NewSessionManager initializes a SessionManager with an AES-256 key derived via SHA-256.
func NewSessionManager(secret string) *SessionManager {
	if secret == "" {
		secret = "skoll-default-session-encryption-secret-key"
	}
	hash := sha256.Sum256([]byte(secret))
	return &SessionManager{key: hash[:]}
}

// Encrypt encrypts the session payload using AES-256-GCM and returns a base64-url string.
func (sm *SessionManager) Encrypt(data *SessionData) (string, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to serialize session: %w", err)
	}

	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and 16-byte authentication tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes and verifies the encrypted session cookie, returning the SessionData.
func (sm *SessionManager) Decrypt(cookieValue string) (*SessionData, error) {
	if cookieValue == "" {
		return nil, errors.New("empty session cookie")
	}

	raw, err := base64.URLEncoding.DecodeString(cookieValue)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoding: %w", err)
	}

	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("session decryption verification failed: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(plaintext, &session); err != nil {
		return nil, fmt.Errorf("invalid session json: %w", err)
	}

	if time.Now().Unix() > session.ExpiresAt {
		return nil, errors.New("session expired")
	}

	return &session, nil
}

// SetSessionCookie creates and attaches the encrypted HTTP-only cookie to the response.
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, user *database.User, isSecure bool) error {
	now := time.Now().UTC()
	data := &SessionData{
		DiscordUserID: user.DiscordID,
		Username:      user.Username,
		Role:          user.Role,
		CreatedAt:     now.Unix(),
		ExpiresAt:     now.Add(DefaultSessionTTL).Unix(),
	}

	encrypted, err := sm.Encrypt(data)
	if err != nil {
		return err
	}

	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    encrypted,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(DefaultSessionTTL.Seconds()),
		Expires:  now.Add(DefaultSessionTTL),
	}
	http.SetCookie(w, cookie)
	return nil
}

// ClearSessionCookie deletes the session cookie.
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter, isSecure bool) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, cookie)
}

// GetSession extracts and decrypts the session from the HTTP request cookie.
func (sm *SessionManager) GetSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	return sm.Decrypt(cookie.Value)
}
