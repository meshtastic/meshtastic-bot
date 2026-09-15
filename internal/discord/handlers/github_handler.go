package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// repoAliases maps what people type to the repository they mean.
//
// GitHub resolves repository names case-insensitively, so an entry is only
// needed where the name people use differs from the repository's own. web,
// firmware and design already work as typed.
var repoAliases = map[string]string{
	"apple":         "Meshtastic-Apple",
	"ios":           "Meshtastic-Apple",
	"iphone":        "Meshtastic-Apple",
	"ipad":          "Meshtastic-Apple",
	"mac":           "Meshtastic-Apple",
	"macos":         "Meshtastic-Apple",
	"android":       "Meshtastic-Android",
	"docs":          "meshtastic",
	"documentation": "meshtastic",
	"cli":           "python",
}

// resolveRepoAlias returns the repository a name refers to, or the name itself
// when it matches no alias, so anything that worked before still works.
func resolveRepoAlias(name string) string {
	trimmed := strings.TrimSpace(name)
	if repo, ok := repoAliases[strings.ToLower(trimmed)]; ok {
		return repo
	}
	return trimmed
}

func handleRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options

	var repo string
	if len(options) > 0 && options[0].Name == "name" {
		repo = resolveRepoAlias(options[0].StringValue())
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
