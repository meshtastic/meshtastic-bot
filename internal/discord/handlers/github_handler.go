package handlers

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func handleRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options

	var repo string
	if len(options) > 0 && options[0].Name == "name" {
		repo = options[0].StringValue()
	}

	// Use default repo if none specified
	if repo == "" {
		repo = GithubRepo
	}

	// Defer response as API call might take time
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Validate repository exists
	repository, err := GithubClient.GetRepository(GithubOwner, repo)
	if err != nil {
		log.Printf("Error getting repository %s/%s: %v", GithubOwner, repo, err)
		errorMsg := fmt.Sprintf("Repository `%s/%s` not found in the organization.", GithubOwner, repo)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &errorMsg,
		})
		return
	}

	githubURL := repository.GetHTMLURL()

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &githubURL,
	})
}
