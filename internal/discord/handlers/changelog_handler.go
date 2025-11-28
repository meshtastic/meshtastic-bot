package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	gogithub "github.com/google/go-github/v57/github"
)

var (
	releaseCache      []*gogithub.RepositoryRelease
	releaseCacheMutex sync.RWMutex
	lastCacheUpdate   time.Time
	cacheDuration     = 5 * time.Minute
)

func handleChangelog(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	var base, head string
	if opt, ok := optionMap["base"]; ok {
		base = opt.StringValue()
	}
	if opt, ok := optionMap["head"]; ok {
		head = opt.StringValue()
	}

	if base == "" || head == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please provide both base and head versions.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Defer response as API call might take time
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	comparison, err := GithubClient.CompareCommits(GithubOwner, GithubRepo, base, head)
	if err != nil {
		log.Printf("Error comparing commits: %v", err)
		errMsg := fmt.Sprintf("Failed to compare versions: %s...%s", base, head)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &errMsg,
		})
		return
	}

	// Format the output
	message := formatChangelogMessage(base, head, comparison)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})
}

func formatChangelogMessage(base, head string, comparison *gogithub.CommitsComparison) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Changes from %s to %s\n", base, head))
	sb.WriteString(fmt.Sprintf("Total commits: %d\n\n", comparison.GetTotalCommits()))

	// List commits (limit to last 10 to avoid hitting message length limits)
	commits := comparison.Commits
	if len(commits) > 10 {
		sb.WriteString(fmt.Sprintf("*Showing last 10 of %d commits*\n\n", len(commits)))
		commits = commits[len(commits)-10:]
	}

	for _, commit := range commits {
		message := commit.GetCommit().GetMessage()
		// Take only the first line of the commit message
		if idx := strings.Index(message, "\n"); idx != -1 {
			message = message[:idx]
		}

		author := commit.GetAuthor().GetLogin()
		if author == "" {
			commitAuthor := commit.GetCommit().GetAuthor()
			if commitAuthor != nil {
				author = commitAuthor.GetName()
			} else {
				author = "Unknown"
			}
		}

		sb.WriteString(fmt.Sprintf("- [`%s`](<%s>) %s - *%s*\n",
			commit.GetSHA()[:7],
			commit.GetHTMLURL(),
			message,
			author,
		))
	}

	sb.WriteString(fmt.Sprintf("\n[View Full Comparison](<%s>)", comparison.GetHTMLURL()))
	return sb.String()
}

func handleChangelogAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Update cache if needed
	if err := updateReleaseCache(); err != nil {
		log.Printf("Error updating release cache: %v", err)
	}

	releaseCacheMutex.RLock()
	defer releaseCacheMutex.RUnlock()

	data := i.ApplicationCommandData()
	var currentInput string
	for _, opt := range data.Options {
		if opt.Focused {
			currentInput = strings.ToLower(opt.StringValue())
			break
		}
	}

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, 25)
	for _, release := range releaseCache {
		tagName := release.GetTagName()
		if currentInput == "" || strings.Contains(strings.ToLower(tagName), currentInput) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  tagName,
				Value: tagName,
			})
		}
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

func updateReleaseCache() error {
	releaseCacheMutex.RLock()
	if time.Since(lastCacheUpdate) < cacheDuration && len(releaseCache) > 0 {
		releaseCacheMutex.RUnlock()
		return nil
	}
	releaseCacheMutex.RUnlock()

	releaseCacheMutex.Lock()
	defer releaseCacheMutex.Unlock()

	// Double check after acquiring write lock
	if time.Since(lastCacheUpdate) < cacheDuration && len(releaseCache) > 0 {
		return nil
	}

	// Fetch releases
	releases, err := GithubClient.GetReleases(GithubOwner, GithubRepo, 100)
	if err != nil {
		return err
	}

	releaseCache = releases
	lastCacheUpdate = time.Now()
	return nil
}
