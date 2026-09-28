package handlers

import (
	"fmt"
	"log"
	"regexp"
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

// repoNamePattern is GitHub's alphabet for repository names. The name becomes
// an API path segment, so "../other/repo" would otherwise leave the org.
var repoNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

func validRepoName(name string) bool {
	return repoNamePattern.MatchString(name) && name != "." && name != ".."
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

	if !validRepoName(repo) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "That is not a repository name. Try a short name such as `android`, `apple` or `firmware`.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Defer response as API call might take time
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Validate repository exists
	repository, err := GithubClient.GetRepository(GithubOwner, repo)
	if err == nil {
		// A transferred repository redirects to its new owner.
		if got := repository.GetOwner().GetLogin(); got != "" && !strings.EqualFold(got, GithubOwner) {
			err = fmt.Errorf("repository now belongs to %s", got)
		}
	}
	if err != nil {
		log.Printf("Error getting repository %s/%s: %v", GithubOwner, repo, err)
		errorMsg := fmt.Sprintf("Repository `%s/%s` not found in the organization.", GithubOwner, repo)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:         &errorMsg,
			AllowedMentions: noMentions,
		})
		return
	}

	githubURL := repository.GetHTMLURL()

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &githubURL,
		AllowedMentions: noMentions,
	})
}
