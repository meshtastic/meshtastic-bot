package handlers

import (
	"fmt"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

func handleFaq(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the selected FAQ topic from the interaction
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please select a FAQ topic from the autocomplete options.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	topicName := options[0].StringValue()

	// Get FAQ data
	faqData := config.GetFAQData()
	if faqData == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "FAQ data is not available. Please contact an administrator.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Find the FAQ item
	item, found := faqData.FindFAQItem(topicName)
	if !found {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("FAQ topic '%s' not found.", topicName),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Respond with the FAQ link
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s**\n%s", item.Name, item.URL),
		},
	})
}

// handleFaqAutocomplete provides autocomplete suggestions for FAQ topics
func handleFaqAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	faqData := config.GetFAQData()
	if faqData == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{
				Choices: []*discordgo.ApplicationCommandOptionChoice{},
			},
		})
		return
	}

	// Get the current user input
	options := i.ApplicationCommandData().Options
	var userInput string
	if len(options) > 0 {
		userInput = strings.ToLower(options[0].StringValue())
	}

	// Get all FAQ items
	allItems := faqData.GetAllFAQItems()

	// Filter and create choices
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, 25)
	for _, item := range allItems {
		// Filter by user input if provided
		if userInput != "" && !strings.Contains(strings.ToLower(item.Name), userInput) {
			continue
		}

		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  item.Name,
			Value: item.Name,
		})

		// Discord limits autocomplete to 25 choices
		if len(choices) >= 25 {
			break
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}
