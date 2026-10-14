package discord

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/Harkole/skoll/internal/database"
)

var (
	ErrUnlinkedUser     = errors.New("unlinked user")
	ErrInsufficientRole = errors.New("insufficient privileges")
)

// Authorize checks if the Discord user is linked and possesses the required role.
func Authorize(ctx context.Context, db *database.DB, discordUserID string, requiredRole database.Role) (*database.User, error) {
	user, err := db.GetUserByDiscordID(ctx, discordUserID)
	if err != nil {
		if errors.Is(err, database.ErrUserNotFound) {
			return nil, ErrUnlinkedUser
		}
		return nil, fmt.Errorf("database lookup failed: %w", err)
	}

	if user.Role == database.RoleUnlinked {
		return user, ErrUnlinkedUser
	}

	if !user.Role.CanExecute(requiredRole) {
		return user, ErrInsufficientRole
	}

	return user, nil
}

// GenerateStateToken creates a cryptographically secure random token for OAuth linking.
func GenerateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ReplyEphemeral sends a private, ephemeral text response to the interaction.
func ReplyEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, message string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// ReplyEphemeralEmbed sends a private, ephemeral rich embed response.
func ReplyEphemeralEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

// ReplyEmbed sends a public or standard rich embed response.
func ReplyEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// GetInteractionUser extracts the sender's User struct from Guild Member or Direct Message.
func GetInteractionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}
