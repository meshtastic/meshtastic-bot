package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"
	internalgithub "github.com/meshtastic/meshtastic-bot/internal/github"

	"github.com/bwmarrin/discordgo"
)

const (
	// retryPrefix marks the button that re-files a report after GitHub failed.
	retryPrefix = "retry_"
	// continuePrefix marks the button that opens a report's next dialog.
	continuePrefix = "continue_"
	// continueModalPrefix is every report dialog's custom ID, followed by the
	// state key.
	continueModalPrefix = "modal_continue_"
	// fileAnywayPrefix and cancelPrefix answer the possible-duplicates offer.
	fileAnywayPrefix = "fileanyway_"
	cancelPrefix     = "cancelreport_"
)

func createIssueFromState(s *discordgo.Session, i *discordgo.InteractionCreate, state *ModalState, stateKey string) {
	// Acknowledge before calling GitHub: CreateIssue can outlast Discord's 3 s
	// window, and a reporter shown "interaction failed" files the report again.
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		// The token is dead, so an issue filed now could never be reported
		// back, and the reporter would file it again.
		log.Printf("Not filing the %s submission; Discord refused the acknowledgement: %v", state.Command, err)
		return
	}
	if similar := similarIssues(state); len(similar) > 0 {
		offerSimilar(s, i, similar, stateKey)
		return
	}
	fileIssue(s, i, state, stateKey)
}

// similarShown is how many possible duplicates a reporter is shown.
const similarShown = 3

// similarIssues searches for the report's title; any failure means file it.
func similarIssues(state *ModalState) []internalgithub.SimilarIssue {
	if strings.TrimSpace(state.SearchText) == "" {
		return nil
	}
	similar, err := GithubClient.SimilarIssues(state.Owner, state.Repo, state.SearchText, similarShown)
	if err != nil {
		log.Printf("Skipping the duplicate check for a %s report: %v", state.Command, err)
		return nil
	}
	if len(similar) > similarShown {
		similar = similar[:similarShown]
	}
	return similar
}

func offerSimilar(s *discordgo.Session, i *discordgo.InteractionCreate, similar []internalgithub.SimilarIssue, stateKey string) {
	var b strings.Builder
	b.WriteString("Before this is filed, is it one of these?\n")
	escape := strings.NewReplacer("[", "\\[", "]", "\\]")
	for _, issue := range similar {
		b.WriteString(fmt.Sprintf("- [#%d %s](<%s>) (%s)\n", issue.Number, escape.Replace(issue.Title), issue.URL, issue.State))
	}
	b.WriteString("\nIf it is, add to that issue instead. Otherwise file yours; your answers are kept for 30 minutes.")
	renewModalState(stateKey)
	editReply(s, i, b.String(), []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "File anyway", Style: discordgo.PrimaryButton, CustomID: fileAnywayPrefix + stateKey},
			discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: cancelPrefix + stateKey},
		}},
	})
}

// fileIssue creates the issue and edits the deferred reply with the outcome.
// On failure the answers are kept and the reply offers a Retry button.
func fileIssue(s *discordgo.Session, i *discordgo.InteractionCreate, state *ModalState, stateKey string) {
	if !state.startFiling() {
		editReply(s, i, "This report is already being filed.", nil)
		return
	}

	marker, since, retried := state.submission()
	var issue *internalgithub.IssueResponse
	if retried {
		// A failed call may still have created the issue; filing again would
		// duplicate it.
		found, err := GithubClient.FindSubmission(state.Owner, state.Repo, marker, since)
		if err != nil {
			log.Printf("Could not check for an earlier %s filing: %v", state.Command, err)
			offerRetry(s, i, state, stateKey, "❌ Could not check whether the earlier attempt went through, so nothing was filed. Your answers are kept for 30 minutes, so press Retry to try again.")
			return
		}
		issue = found
	}

	if issue == nil {
		values := state.answers()
		body := issueBody(state.AllFields, values, state.files(), i.Member.User.Username, i.Member.User.ID, marker)
		var err error
		issue, err = GithubClient.CreateIssue(state.Owner, state.Repo, state.Title, body, state.Labels)
		if err != nil {
			log.Printf("Failed to create GitHub issue: %v", err)
			offerRetry(s, i, state, stateKey, "❌ The issue could not be created. Your answers are kept for 30 minutes, so press Retry to try again.")
			return
		}
	}

	confirmationMessage := fmt.Sprintf("✅ Issue #%d created successfully!\n%s", issue.Number, issue.HTMLURL) +
		"\n\n**Note:** Text log files attached in the form are in the issue. Screenshots and screen " +
		"recordings cannot be sent from Discord, so open the issue linked above and add them in a comment." +
		"\n\nThis issue is public and records your Discord username and user ID. See the " +
		"[privacy policy](<https://github.com/meshtastic/meshtastic-bot/blob/main/PRIVACY.md>)."

	editReply(s, i, confirmationMessage, []discordgo.MessageComponent{})
	dropModalState(stateKey)
}

func offerRetry(s *discordgo.Session, i *discordgo.InteractionCreate, state *ModalState, stateKey, message string) {
	state.stopFiling()
	renewModalState(stateKey)
	editReply(s, i, message, []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Retry", Style: discordgo.PrimaryButton, CustomID: retryPrefix + stateKey},
		}},
	})
}

// editReply replaces a deferred reply; empty components clear a Retry button.
func editReply(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	edit := &discordgo.WebhookEdit{Content: &content}
	if components != nil {
		edit.Components = &components
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
		log.Printf("Error editing the submission reply: %v", err)
	}
}

func respondExpired(s *discordgo.Session, i *discordgo.InteractionCreate, stateKey string) {
	log.Printf("Modal state not found for a %s submission", commandFromStateKey(stateKey))
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "This submission expired before it could be filed, so no issue was created. Please run the command again.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	// A dialog opened before a restart lost its state with the process, and one
	// from an older build carries "modal_<command>_<channelID>".
	stateKey, ok := strings.CutPrefix(data.CustomID, continueModalPrefix)
	if !ok {
		respondExpired(s, i, strings.TrimPrefix(data.CustomID, "modal_"))
		return
	}
	state, exists := lookupModalState(stateKey)
	if !exists {
		respondExpired(s, i, stateKey)
		return
	}

	collectSubmittedValues(state, data.Components, data.Resolved.Attachments)

	next := state.finishDialog()
	if next >= len(state.AllFields) {
		createIssueFromState(s, i, state, stateKey)
		return
	}

	totalParts := (len(state.AllFields) + config.DialogComponentLimit - 1) / config.DialogComponentLimit
	currentPart := (next + config.DialogComponentLimit - 1) / config.DialogComponentLimit
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: continuePrompt(currentPart, totalParts),
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "Continue", Style: discordgo.PrimaryButton, CustomID: continuePrefix + stateKey},
				}},
			},
		},
	})
	if err != nil {
		log.Printf("Error responding with continue button: %v", err)
	}
}

func handleButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if stateKey, ok := strings.CutPrefix(customID, cancelPrefix); ok {
		dropModalState(stateKey)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{Content: "No issue was filed.", Components: []discordgo.MessageComponent{}},
		})
		return
	}

	stateKey, ok := strings.CutPrefix(customID, retryPrefix)
	if !ok {
		stateKey, ok = strings.CutPrefix(customID, fileAnywayPrefix)
	}
	if ok {
		state, exists := lookupModalState(stateKey)
		if !exists {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Content:    "❌ Session expired, so no issue was created. Please run the command again.",
					Components: []discordgo.MessageComponent{},
				},
			})
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		}); err != nil {
			log.Printf("Not retrying the %s submission; Discord refused the acknowledgement: %v", state.Command, err)
			return
		}
		fileIssue(s, i, state, stateKey)
		return
	}

	if stateKey, ok := strings.CutPrefix(customID, continuePrefix); ok {
		state, exists := lookupModalState(stateKey)
		if !exists {
			respondExpired(s, i, stateKey)
			return
		}
		showDialog(s, i, state, stateKey)
	}
}
