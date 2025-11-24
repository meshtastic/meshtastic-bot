package handlers

import (
	"fmt"
	"log"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

func handleFeature(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get all fields to check if we need multi-part modals
	allFields, title, owner, repo, err := config.GetAllFieldsForModal("feature", i.ChannelID)
	if err != nil {
		log.Printf("Error getting modal fields: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Sorry, the feature request command is not configured for this channel.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// If more than 5 fields, set up multi-part modal state
	if len(allFields) > 5 {
		stateKey := fmt.Sprintf("%s_%s_%s", "feature", i.ChannelID, i.Member.User.ID)
		modalStates[stateKey] = &ModalState{
			Title:           title,
			AllFields:       allFields,
			SubmittedValues: make(map[string]string),
			Labels:          []string{"from-discord", "enhancement"},
			Command:         "feature",
			ChannelID:       i.ChannelID,
			Owner:           owner,
			Repo:            repo,
		}
	}

	modalData, err := config.GetModel("feature", i.ChannelID)
	if err != nil {
		log.Printf("Error getting modal config: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Sorry, the feature request command is not configured for this channel.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: modalData,
	})
	if err != nil {
		log.Printf("Error responding with modal: %v", err)
	}
}
