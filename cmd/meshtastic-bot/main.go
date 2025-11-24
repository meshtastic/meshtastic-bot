package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meshtastic/meshtastic-bot/internal/config"
	"github.com/meshtastic/meshtastic-bot/internal/discord"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger := log.Default()

	discordBot, err := discord.New(cfg, logger)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := discordBot.Start(ctx); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	healthServer := &http.Server{
		Addr: fmt.Sprintf(":%s", cfg.HealthCheckPort),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if discordBot.IsHealthy() {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service Unavailable"))
			}
		}),
	}

	go func() {
		log.Printf("Health check server starting on port %s", cfg.HealthCheckPort)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Health check server error: %v", err)
		}
	}()

	// Set up graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	log.Println("Bot is running. Press Ctrl+C to exit")

	<-stop
	log.Println("Shutdown signal received...")
	cancel()

	// Shutdown health check server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Health check server shutdown error: %v", err)
	}

	// Stop the bot
	if err := discordBot.Stop(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Bot stopped gracefully")
}
