package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Harkole/skoll/internal/api"
	"github.com/Harkole/skoll/internal/config"
	"github.com/Harkole/skoll/internal/database"
	"github.com/Harkole/skoll/internal/discord"
	"github.com/Harkole/skoll/internal/fenrir"
)

func main() {
	log.Println("======================================================")
	log.Println("  Sköll — Fenrir Daemon Companion Service & Bot")
	log.Println("======================================================")

	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	// 2. Initialize Embedded SQLite Database
	log.Printf("[Database] Connecting to SQLite database at: %s", cfg.DatabasePath)
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize SQLite database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("[Database] Error closing database: %v", err)
		}
	}()
	log.Println("[Database] Tables migrated and WAL mode enabled successfully.")

	// 3. Initialize Fenrir Client Bridge
	log.Printf("[Fenrir] Initializing bridge to Fenrir Daemon at: %s", cfg.FenrirAPIURL)
	fenrirClient := fenrir.NewClient(cfg.FenrirAPIURL, cfg.FenrirAPIKey, cfg.HTTPTimeout)

	// 4. Initialize HTTP & Admin Web Server
	server := api.NewServer(cfg, db, fenrirClient, nil)
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()
	log.Printf("[HTTP] Admin Dashboard ready at: %s/admin", cfg.PublicURL)

	// 5. Initialize Discord Bot (if token is provided)
	var bot *discord.Bot
	if cfg.DiscordBotToken != "" {
		log.Println("[Discord] Initializing Discord Bot gateway...")
		var botErr error
		bot, botErr = discord.NewBot(cfg, db, fenrirClient)
		if botErr != nil {
			log.Printf("[Discord] Failed to initialize Discord bot: %v", botErr)
		} else {
			if err := bot.Start(); err != nil {
				log.Printf("[Discord] Error starting Discord gateway: %v", err)
			} else {
				log.Println("[Discord] Bot gateway session started and slash commands registered.")
			}
		}
	} else {
		log.Println("[Discord] Notice: DISCORD_BOT_TOKEN is not set. Running in Web/Admin standalone mode.")
	}

	// 6. Graceful Shutdown on SIGINT / SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("[Shutdown] Received signal: %v. Initiating graceful shutdown...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if bot != nil {
		_ = bot.Stop()
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[HTTP] Error during server shutdown: %v", err)
	}

	log.Println("[Shutdown] Sköll companion service exited cleanly.")
}
