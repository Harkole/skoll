package discord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Harkole/skoll/internal/config"
	"github.com/Harkole/skoll/internal/database"
	"github.com/Harkole/skoll/internal/fenrir"
)

// Bot represents the Discord bot service and RBAC command executor.
type Bot struct {
	Session      *discordgo.Session
	cfg          *config.Config
	db           *database.DB
	fenrirClient *fenrir.Client
	registered   []*discordgo.ApplicationCommand
}

// NewBot initializes a new Discord bot session with specified intents.
func NewBot(cfg *config.Config, db *database.DB, fenrirClient *fenrir.Client) (*Bot, error) {
	if cfg.DiscordBotToken == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is not configured")
	}

	token := cfg.DiscordBotToken
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	session, err := discordgo.New(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	// Gateway Intents: Guilds, GuildMessages
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	bot := &Bot{
		Session:      session,
		cfg:          cfg,
		db:           db,
		fenrirClient: fenrirClient,
	}

	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteractionCreate)

	return bot, nil
}

// Start opens the Discord websocket connection and registers application slash commands.
func (b *Bot) Start() error {
	log.Println("[Discord] Opening connection to Discord Gateway...")
	if err := b.Session.Open(); err != nil {
		return fmt.Errorf("failed to open discord gateway session: %w", err)
	}

	appID := b.cfg.DiscordClientID
	if appID == "" && b.Session.State != nil && b.Session.State.User != nil {
		appID = b.Session.State.User.ID
	}

	log.Printf("[Discord] Registering %d application slash commands for AppID %s...", len(Commands), appID)
	for _, cmd := range Commands {
		created, err := b.Session.ApplicationCommandCreate(appID, "", cmd)
		if err != nil {
			log.Printf("[Discord] Warning: failed to register slash command /%s: %v", cmd.Name, err)
		} else {
			b.registered = append(b.registered, created)
		}
	}

	return nil
}

// Stop closes the Discord gateway session cleanly.
func (b *Bot) Stop() error {
	log.Println("[Discord] Closing Discord session...")
	return b.Session.Close()
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("[Discord] Bot logged in as %s#%s (ID: %s)", r.User.Username, r.User.Discriminator, r.User.ID)
	_ = s.UpdateGameStatus(0, "over Fenrir Stacks | /status")
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	cmdData := i.ApplicationCommandData()
	switch cmdData.Name {
	case "link":
		b.HandleLinkCommand(s, i)
	case "status":
		b.HandleStatusCommand(s, i)
	case "server":
		b.HandleServerCommand(s, i)
	case "stack":
		b.HandleStackCommand(s, i)
	default:
		_ = ReplyEphemeral(s, i, "Unknown command.")
	}
}

// HandleLinkCommand generates a secure one-time link token and returns the OAuth URL.
func (b *Bot) HandleLinkCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := GetInteractionUser(i)
	if user == nil {
		_ = ReplyEphemeral(s, i, "Unable to resolve Discord user context.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if already linked
	existing, err := b.db.GetUserByDiscordID(ctx, user.ID)
	if err == nil && existing != nil && existing.Role != database.RoleUnlinked {
		embed := &discordgo.MessageEmbed{
			Title:       "Account Already Linked",
			Description: fmt.Sprintf("Your Discord account is already linked to Sköll!\n\n**Username:** %s\n**Role:** `%s`", existing.Username, existing.Role),
			Color:       0x38bdf8, // Cyan
		}
		_ = ReplyEphemeralEmbed(s, i, embed)
		return
	}

	// Generate secure token
	token, err := GenerateStateToken()
	if err != nil {
		_ = ReplyEphemeral(s, i, "Failed to generate security token.")
		return
	}

	// Persist link state token for 15 minutes
	if err := b.db.CreateLinkState(ctx, token, user.ID, 15*time.Minute); err != nil {
		_ = ReplyEphemeral(s, i, "Database error creating link session.")
		return
	}

	linkURL := fmt.Sprintf("%s/link?state=%s", b.cfg.PublicURL, token)

	embed := &discordgo.MessageEmbed{
		Title: "🔗 Link Your Discord Account",
		Description: fmt.Sprintf("Click the link below to verify your Discord identity and bind your account to Sköll:\n\n"+
			"[**👉 Authenticate with Sköll**](%s)\n\n"+
			"*This secure link expires in 15 minutes.*", linkURL),
		Color: 0x6366f1, // Indigo
	}

	_ = ReplyEphemeralEmbed(s, i, embed)
}

// HandleStatusCommand queries Fenrir for host metrics and stacks (Viewer+ required).
func (b *Bot) HandleStatusCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	u := GetInteractionUser(i)
	if u == nil {
		_ = ReplyEphemeral(s, i, "Unable to resolve Discord user.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// RBAC: Viewer role or higher required
	dbUser, err := Authorize(ctx, b.db, u.ID, database.RoleViewer)
	if err != nil {
		b.respondRBACError(s, i, u, "status", "all", database.RoleViewer, err)
		return
	}

	if b.fenrirClient == nil {
		_ = ReplyEphemeral(s, i, "Fenrir Daemon client is not configured on this Sköll instance.")
		return
	}

	// Query Fenrir Health (GET /api/v1/health) & Host Telemetry (GET /api/v1/host)
	health, err := b.fenrirClient.GetHealth(ctx)
	statusText := "Online"
	color := 0x22c55e // Green
	if err != nil {
		statusText = "Offline (" + err.Error() + ")"
		color = 0xef4444 // Red
	} else if health != nil && health.Status != "" {
		statusText = titleCase(health.Status)
	}

	host, _ := b.fenrirClient.GetHost(ctx)

	// Format embed focusing exclusively on daemon health, version, uptime, and host hardware metrics
	embed := &discordgo.MessageEmbed{
		Title:       "🐺 Fenrir Daemon Status",
		Description: fmt.Sprintf("**Daemon Health:** `%s`", statusText),
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      []*discordgo.MessageEmbedField{},
	}

	if health != nil && health.Version != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Version",
			Value:  health.Version,
			Inline: true,
		})
	}

	// Uptime calculation from host telemetry or health
	uptimeStr := ""
	if host != nil && host.UptimeFormatted != "" {
		uptimeStr = host.UptimeFormatted
	} else if host != nil && host.UptimeSeconds > 0 {
		uptimeStr = formatUptime(host.UptimeSeconds)
	} else if health != nil && health.Uptime != "" {
		uptimeStr = health.Uptime
	} else if health != nil && health.UptimeSeconds > 0 {
		uptimeStr = formatUptime(health.UptimeSeconds)
	}

	if uptimeStr != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Uptime",
			Value:  uptimeStr,
			Inline: true,
		})
	}

	if host != nil {
		// CPU Usage
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "CPU Usage",
			Value:  fmt.Sprintf("%.1f%%", host.CPUPercent),
			Inline: true,
		})

		// RAM Usage
		ramVal := fmt.Sprintf("%.1f%%", host.MemoryPercent)
		if host.MemoryUsedBytes > 0 && host.MemoryTotal > 0 {
			usedGB := float64(host.MemoryUsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(host.MemoryTotal) / (1024 * 1024 * 1024)
			ramVal = fmt.Sprintf("%.1f%% (%.2f / %.2f GB)", host.MemoryPercent, usedGB, totalGB)
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "RAM Usage",
			Value:  ramVal,
			Inline: true,
		})

		// Load Averages
		loadStr := ""
		if len(host.LoadAverage) >= 3 {
			loadStr = fmt.Sprintf("%.2f, %.2f, %.2f", host.LoadAverage[0], host.LoadAverage[1], host.LoadAverage[2])
		} else if host.Load1 > 0 || host.Load5 > 0 || host.Load15 > 0 {
			loadStr = fmt.Sprintf("%.2f, %.2f, %.2f", host.Load1, host.Load5, host.Load15)
		}
		if loadStr != "" {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Load Averages (1m, 5m, 15m)",
				Value:  loadStr,
				Inline: true,
			})
		}

		// Disk Usage
		if host.DiskPercent > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Disk Usage",
				Value:  fmt.Sprintf("%.1f%%", host.DiskPercent),
				Inline: true,
			})
		}
	}

	_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "status_query", "host", "SUCCESS")
	_ = ReplyEmbed(s, i, embed)
}

func formatUptime(seconds uint64) string {
	if seconds == 0 {
		return "0s"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// HandleServerCommand handles server lifecycle (/server restart, start, stop). Requires Operator+.
func (b *Bot) HandleServerCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	u := GetInteractionUser(i)
	if u == nil {
		_ = ReplyEphemeral(s, i, "Unable to resolve user.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		_ = ReplyEphemeral(s, i, "Missing subcommand.")
		return
	}

	subCmd := options[0]
	action := subCmd.Name // "restart", "start", "stop"

	var stackName string
	for _, opt := range subCmd.Options {
		if opt.Name == "name" {
			stackName = strings.TrimSpace(opt.StringValue())
		}
	}

	if stackName == "" {
		_ = ReplyEphemeral(s, i, "Target stack name is required.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// RBAC: Operator role or higher required
	dbUser, err := Authorize(ctx, b.db, u.ID, database.RoleOperator)
	if err != nil {
		b.respondRBACError(s, i, u, "server_"+action, stackName, database.RoleOperator, err)
		return
	}

	if b.fenrirClient == nil {
		_ = ReplyEphemeral(s, i, "Fenrir Daemon client is not configured.")
		return
	}

	// Defer interaction response immediately to prevent Discord's 3-second timeout
	if err := DeferResponse(s, i); err != nil {
		log.Printf("[Discord] Warning: failed to defer interaction for /server %s: %v", action, err)
	}

	// Execute requested action
	var execErr error
	switch action {
	case "restart":
		execErr = b.fenrirClient.RestartProject(ctx, stackName)
	case "start":
		execErr = b.fenrirClient.StartProject(ctx, stackName)
	case "stop":
		execErr = b.fenrirClient.StopProject(ctx, stackName)
	default:
		_ = EditResponseText(s, i, "Unknown server subcommand.")
		return
	}

	if execErr != nil {
		_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "server_"+action, stackName, "FAILED")
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("❌ Failed to %s stack", action),
			Description: fmt.Sprintf("An error occurred while communicating with Fenrir Daemon for stack `%s`:\n\n```%s```", stackName, execErr.Error()),
			Color:       0xef4444, // Red
		}
		_ = EditResponseEmbed(s, i, embed)
		return
	}

	_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "server_"+action, stackName, "SUCCESS")
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🔄 Stack %sed successfully", titleCase(action)),
		Description: fmt.Sprintf("Successfully signaled Fenrir Daemon to **%s** stack `%s`.\nOperator: <@%s>", action, stackName, u.ID),
		Color:       0x22c55e, // Green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	_ = EditResponseEmbed(s, i, embed)
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// HandleStackCommand handles stack orchestration (/stack deploy, list). Deploy requires Admin.
func (b *Bot) HandleStackCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	u := GetInteractionUser(i)
	if u == nil {
		_ = ReplyEphemeral(s, i, "Unable to resolve user.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		_ = ReplyEphemeral(s, i, "Missing subcommand.")
		return
	}

	subCmd := options[0]
	action := subCmd.Name // "deploy", "list"

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	switch action {
	case "deploy":
		// Deploy requires Admin role
		dbUser, err := Authorize(ctx, b.db, u.ID, database.RoleAdmin)
		if err != nil {
			b.respondRBACError(s, i, u, "stack_deploy", "unknown", database.RoleAdmin, err)
			return
		}

		var stackName, composeYAML string
		for _, opt := range subCmd.Options {
			if opt.Name == "name" {
				stackName = strings.TrimSpace(opt.StringValue())
			} else if opt.Name == "compose" {
				composeYAML = opt.StringValue()
			}
		}

		if stackName == "" || composeYAML == "" {
			_ = ReplyEphemeral(s, i, "Both `name` and `compose` options are required.")
			return
		}

		if b.fenrirClient == nil {
			_ = ReplyEphemeral(s, i, "Fenrir Daemon client is not configured.")
			return
		}

		// Defer interaction response immediately to prevent Discord's 3-second timeout
		if err := DeferResponse(s, i); err != nil {
			log.Printf("[Discord] Warning: failed to defer interaction for /stack deploy: %v", err)
		}

		if err := b.fenrirClient.DeployProject(ctx, stackName, composeYAML); err != nil {
			_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "stack_deploy", stackName, "FAILED")
			embed := &discordgo.MessageEmbed{
				Title:       "❌ Deployment Failed",
				Description: fmt.Sprintf("Fenrir rejected the deployment for stack `%s`:\n\n```%s```", stackName, err.Error()),
				Color:       0xef4444,
			}
			_ = EditResponseEmbed(s, i, embed)
			return
		}

		_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "stack_deploy", stackName, "SUCCESS")
		embed := &discordgo.MessageEmbed{
			Title:       "🚀 Stack Deployed Successfully",
			Description: fmt.Sprintf("Docker Compose stack `%s` has been registered and initiated in Fenrir Daemon.\nDeployed by Administrator <@%s>", stackName, u.ID),
			Color:       0x22c55e,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		_ = EditResponseEmbed(s, i, embed)

	case "list":
		// List requires Viewer role
		dbUser, err := Authorize(ctx, b.db, u.ID, database.RoleViewer)
		if err != nil {
			b.respondRBACError(s, i, u, "stack_list", "all", database.RoleViewer, err)
			return
		}

		if b.fenrirClient == nil {
			_ = ReplyEphemeral(s, i, "Fenrir client not configured.")
			return
		}

		projects, err := b.fenrirClient.ListProjects(ctx)
		if err != nil {
			_ = ReplyEphemeral(s, i, "Failed to fetch projects: "+err.Error())
			return
		}

		var desc strings.Builder
		if len(projects) == 0 {
			desc.WriteString("*No Docker Compose stacks currently registered.*")
		} else {
			for _, p := range projects {
				stateEmoji := "🟢"
				statusLower := strings.ToLower(p.Status)
				if statusLower != "running" && statusLower != "up" {
					if statusLower == "degraded" || statusLower == "restarting" {
						stateEmoji = "🟡"
					} else {
						stateEmoji = "🔴"
					}
				}
				runningServices := 0
				for _, svc := range p.Services {
					if strings.ToLower(svc.State) == "running" {
						runningServices++
					}
				}
				if len(p.Services) > 0 {
					desc.WriteString(fmt.Sprintf("%s **%s** — `%s` (%d/%d services running)\n", stateEmoji, p.Name, p.Status, runningServices, len(p.Services)))
				} else {
					desc.WriteString(fmt.Sprintf("%s **%s** — `%s` (0 services)\n", stateEmoji, p.Name, p.Status))
				}
			}
		}

		_ = b.db.RecordAuditLog(ctx, dbUser.DiscordID, dbUser.Username, "stack_list", "all", "SUCCESS")
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("📋 Registered Docker Compose Stacks (%d)", len(projects)),
			Description: desc.String(),
			Color:       0x38bdf8,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		_ = ReplyEmbed(s, i, embed)
	}
}

func (b *Bot) respondRBACError(s *discordgo.Session, i *discordgo.InteractionCreate, u *discordgo.User, action, target string, required database.Role, err error) {
	ctx := context.Background()
	_ = b.db.RecordAuditLog(ctx, u.ID, u.Username, action, target, "UNAUTHORIZED")

	var embed *discordgo.MessageEmbed
	if err == ErrUnlinkedUser {
		embed = &discordgo.MessageEmbed{
			Title: "🔒 Account Not Linked",
			Description: "You have not linked your Discord identity to Sköll yet.\n\n" +
				"Please use the **/link** command to authenticate your account first.",
			Color: 0xf59e0b, // Amber
		}
	} else {
		// Insufficient role
		userRecord, _ := b.db.GetUserByDiscordID(ctx, u.ID)
		currentRole := database.RoleViewer
		if userRecord != nil {
			currentRole = userRecord.Role
		}

		embed = &discordgo.MessageEmbed{
			Title: "⛔ Permission Denied",
			Description: fmt.Sprintf("You do not have sufficient permissions to execute this command.\n\n"+
				"• **Your Role:** `%s`\n"+
				"• **Required Role:** `%s` or higher\n\n"+
				"Contact a server Administrator to request an elevated role.", currentRole, required),
			Color: 0xef4444, // Red
		}
	}

	_ = ReplyEphemeralEmbed(s, i, embed)
}
