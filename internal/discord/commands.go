package discord

import (
	"github.com/bwmarrin/discordgo"
)

// returns all slash commands to register
func getCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "tapsign",
			Description: "Display a short help message in the channel",
		},
		{
			Name:        "faq",
			Description: "Frequently Asked Questions",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "topic",
					Description:  "Select a FAQ topic",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "bug",
			Description: "Submit a bug report",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "title",
					Description: "A short, descriptive title for the bug report",
					Required:    true,
				},
			},
		},
		{
			Name:        "feature",
			Description: "Request a new feature",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "title",
					Description: "A short, descriptive title for the feature request",
					Required:    true,
				},
			},
		},
		{
			Name:        "changelog",
			Description: "View changes between two versions",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "base",
					Description:  "The base version (e.g. v2.6.0)",
					Required:     true,
					Autocomplete: true,
				},
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "head",
					Description:  "The head version (e.g. v2.6.4)",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
	}
}
