package handlers

import (
	"fmt"
	"log"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

func handleBug(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get all fields to check if we need multi-part modals
	allFields, title, owner, repo, err := config.GetAllFieldsForModal("bug", i.ChannelID)
	if err != nil {
		log.Printf("Error getting modal fields: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Sorry, the bug report command is not configured for this channel.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// The template defines no title field, so take the title from the command
	// option and fall back to the template name if it is somehow absent.
	issueTitle := commandTitleOption(i)
	if issueTitle == "" {
		issueTitle = title
	}

	// Record state for every submission, not only multi-part ones. The state
	// carries the title through to issue creation and collects the submitted
	// values that build the issue body; without it a single-modal template
	// (five fields or fewer) falls through to the legacy path, which looks up
	// field names no GitHub template defines and so sends an empty title.
	stateKey := fmt.Sprintf("%s_%s_%s", "bug", i.ChannelID, i.Member.User.ID)
	modalStates[stateKey] = &ModalState{
		Title:           issueTitle,
		AllFields:       allFields,
		SubmittedValues: make(map[string]string),
		Labels:          []string{"from-discord", "bug"},
		Command:         "bug",
		ChannelID:       i.ChannelID,
		Owner:           owner,
		Repo:            repo,
	}

	modalData, err := config.GetModel("bug", i.ChannelID)
	if err != nil {
		log.Printf("Error getting modal config: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Sorry, the bug report command is not configured for this channel.",
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
