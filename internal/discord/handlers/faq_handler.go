package handlers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// faqAlternates is how many other matches are offered below the answer.
const faqAlternates = 3

// faqExactEnough is the score above which the top match needs no alternatives.
const faqExactEnough = 95

// faqAlternateGap is how far behind the best match an alternative may be.
const faqAlternateGap = 15

// autocompleteLimit is Discord's cap on autocomplete choices.
const autocompleteLimit = 25

func handleFaq(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondEphemeral(s, i, "Please select a FAQ topic from the autocomplete options.")
		return
	}

	// Discord accepts a space for a required option; blank must not browse.
	query := strings.TrimSpace(options[0].StringValue())
	if query == "" {
		respondEphemeral(s, i, "Please select a FAQ topic from the autocomplete options.")
		return
	}

	faqData := config.GetFAQData()
	if faqData == nil {
		respondEphemeral(s, i, "FAQ data is not available. Please contact an administrator.")
		return
	}

	matches := faqData.Search(query, faqAlternates+1)
	if len(matches) == 0 {
		recordFaqMiss()
		respondEphemeral(s, i, fmt.Sprintf("No FAQ topic matches \u201c%s\u201d. Start typing and pick one of the suggestions.", query))
		return
	}

	recordFaqHit(matches[0].Item.Name)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         faqReply(matches),
			AllowedMentions: noMentions,
		},
	})
}

// faqReply answers with the best match, naming close runners-up when it is
// not near-exact.
func faqReply(matches []config.FAQMatch) string {
	best := matches[0]

	var b strings.Builder
	b.WriteString("**" + best.Item.Name + "**\n")
	if answer := strings.TrimSpace(best.Item.Answer); answer != "" {
		b.WriteString(answer + "\n")
	}
	b.WriteString(best.Item.URL)

	if best.Score >= faqExactEnough || len(matches) == 1 {
		return b.String()
	}

	alternates := make([]config.FAQMatch, 0, len(matches)-1)
	for _, match := range matches[1:] {
		if match.Score >= best.Score-faqAlternateGap {
			alternates = append(alternates, match)
		}
	}
	if len(alternates) == 0 {
		return b.String()
	}

	b.WriteString("\n\nYou might also want:")
	for _, match := range alternates {
		// Angle brackets suppress a preview per link.
		b.WriteString(fmt.Sprintf("\n- %s — <%s>", match.Item.Name, match.Item.URL))
	}
	return b.String()
}

// browseOrder ranks the topics for an untouched autocomplete: most looked up
// first, then file order, cut to Discord's limit.
func browseOrder(items []config.FAQItem) []config.FAQItem {
	ordered := append([]config.FAQItem(nil), items...)
	sort.SliceStable(ordered, func(a, b int) bool {
		return faqUsageCount(ordered[a].Name) > faqUsageCount(ordered[b].Name)
	})
	if len(ordered) > autocompleteLimit {
		ordered = ordered[:autocompleteLimit]
	}
	return ordered
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         content,
			Flags:           discordgo.MessageFlagsEphemeral,
			AllowedMentions: noMentions,
		},
	})
}

// handleFaqAutocomplete suggests topics with the same ranking the command uses.
func handleFaqAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	choices := []*discordgo.ApplicationCommandOptionChoice{}

	if faqData := config.GetFAQData(); faqData != nil {
		var userInput string
		if options := i.ApplicationCommandData().Options; len(options) > 0 {
			userInput = options[0].StringValue()
		}

		var items []config.FAQItem
		if strings.TrimSpace(userInput) == "" {
			items = browseOrder(faqData.GetAllFAQItems())
		} else {
			for _, match := range faqData.Search(userInput, autocompleteLimit) {
				items = append(items, match.Item)
			}
		}

		choices = make([]*discordgo.ApplicationCommandOptionChoice, 0, len(items))
		for _, item := range items {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  item.Name,
				Value: item.Name,
			})
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}
