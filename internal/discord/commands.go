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
	}
}
