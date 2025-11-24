package discord

import (
	"context"
	"fmt"
	"log"

	"github.com/meshtastic/meshtastic-bot/internal/config"
	"github.com/meshtastic/meshtastic-bot/internal/discord/handlers"

	"github.com/bwmarrin/discordgo"
)

type DiscordBot struct {
	session  *discordgo.Session
	config   *config.Config
	logger   *log.Logger
	commands []*discordgo.ApplicationCommand
}

func New(cfg *config.Config, logger *log.Logger) (*DiscordBot, error) {
	if logger == nil {
		logger = log.Default()
	}

	if err := config.LoadModals(cfg.ConfigPath); err != nil {
		return nil, fmt.Errorf("failed to load modals: %w", err)
	}

	if _, err := config.LoadFAQ(cfg.FAQPath); err != nil {
		return nil, fmt.Errorf("failed to load FAQ: %w", err)
	}

	owner, repo := config.GetOwnerAndRepo()
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("failed to extract owner/repo from config template URLs")
	}
	handlers.InitializeGithub(cfg.GithubToken, owner, repo)
	logger.Printf("Initialized GitHub client for %s/%s", owner, repo)

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create DiscordBot session: %w", err)
	}

	bot := &DiscordBot{
		session:  session,
		config:   cfg,
		logger:   logger,
		commands: getCommands(),
	}

	bot.session.AddHandler(handlers.HandleInteraction)
	bot.session.AddHandler(bot.handleReady)

	return bot, nil
}

func (b *DiscordBot) Start(ctx context.Context) error {
	b.logger.Println("Opening DiscordBot session...")
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}

	b.logger.Println("Registering slash commands...")
	if err := b.registerCommands(); err != nil {
		b.session.Close()
		return fmt.Errorf("failed to register commands: %w", err)
	}

	b.logger.Println("DiscordBot is now running")
	return nil
}

func (b *DiscordBot) Stop(ctx context.Context) error {
	b.logger.Println("Shutting down bot...")

	if b.config.RemoveCommands {
		b.logger.Println("Removing registered commands...")
		if err := b.removeCommands(); err != nil {
			b.logger.Printf("Error removing commands: %v", err)
		}
	}

	if err := b.session.Close(); err != nil {
		return fmt.Errorf("error closing session: %w", err)
	}

	b.logger.Println("DiscordBot stopped successfully")
	return nil
}

func (b *DiscordBot) registerCommands() error {
	registeredCommands := make([]*discordgo.ApplicationCommand, 0, len(b.commands))

	for _, cmd := range b.commands {
		registered, err := b.session.ApplicationCommandCreate(
			b.session.State.User.ID,
			b.config.ServerID,
			cmd,
		)
		if err != nil {
			return fmt.Errorf("failed to create command '%s': %w", cmd.Name, err)
		}
		registeredCommands = append(registeredCommands, registered)
		b.logger.Printf("Registered command: %s", cmd.Name)
	}

	b.commands = registeredCommands
	return nil
}

// removeCommands removes all registered commands
func (b *DiscordBot) removeCommands() error {
	for _, cmd := range b.commands {
		err := b.session.ApplicationCommandDelete(
			b.session.State.User.ID,
			b.config.ServerID,
			cmd.ID,
		)
		if err != nil {
			b.logger.Printf("Failed to delete command '%s': %v", cmd.Name, err)
			continue
		}
		b.logger.Printf("Deleted command: %s", cmd.Name)
	}
	return nil
}

// handleReady is called when the bot successfully connects
func (b *DiscordBot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	b.logger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
}

// IsHealthy returns true if the DiscordBot session is open and connected
func (b *DiscordBot) IsHealthy() bool {
	return b.session != nil && b.session.DataReady
}
