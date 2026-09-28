package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"
	internalgithub "github.com/meshtastic/meshtastic-bot/internal/github"

	"github.com/bwmarrin/discordgo"
)

// retryPrefix marks the button that re-files a report after GitHub failed.
const retryPrefix = "retry_"

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
	fileIssue(s, i, state, stateKey)
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
		body := buildIssueBody(state.AllFields, values, i.Member.User.Username, i.Member.User.ID) + "\n\n" + marker
		var err error
		issue, err = GithubClient.CreateIssue(state.Owner, state.Repo, state.Title, body, state.Labels)
		if err != nil {
			log.Printf("Failed to create GitHub issue: %v", err)
			offerRetry(s, i, state, stateKey, "❌ The issue could not be created. Your answers are kept for 30 minutes, so press Retry to try again.")
			return
		}
	}

	// A Discord modal takes text only, so screenshots, recordings and other
	// attachments cannot be collected here at all. Say so on every issue, and
	// point at the link just given, rather than leaving the reporter to work out
	// where their screenshot was meant to go.
	confirmationMessage := fmt.Sprintf("✅ Issue #%d created successfully!\n%s", issue.Number, issue.HTMLURL) +
		"\n\n**Note:** Screenshots, screen recordings and other attachments cannot be sent from Discord. " +
		"Open the issue linked above and add them in a comment. Markdown works there too." +
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

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	// Determine which command this modal is for based on CustomID
	// Format: "modal_<command>_<channelID>" or "modal_continue_<stateKey>"
	parts := strings.Split(data.CustomID, "_")
	if len(parts) < 2 {
		log.Printf("Invalid modal CustomID format (%d segments)", len(parts))
		return
	}

	// Check if this is a continuation modal
	if parts[1] == "continue" && len(parts) >= 3 {
		handleModalContinuation(s, i, strings.Join(parts[2:], "_"))
		return
	}

	command := parts[1]
	channelID := i.ChannelID

	// Check if this is a multi-part modal
	stateKey := fmt.Sprintf("%s_%s_%s", command, channelID, i.Member.User.ID)
	state, hasState := lookupModalState(stateKey)

	if hasState {
		collectSubmittedValues(state, data.Components)

		currentIndex := state.answeredCount()

		// Check if there are more fields to show
		if currentIndex < len(state.AllFields) {
			totalParts := (len(state.AllFields) + 4) / 5
			currentPart := (currentIndex + 4) / 5
			message := continuePrompt(currentPart, totalParts)

			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: message,
					Flags:   discordgo.MessageFlagsEphemeral,
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.Button{
									Label:    "Continue",
									Style:    discordgo.PrimaryButton,
									CustomID: fmt.Sprintf("continue_%s", stateKey),
								},
							},
						},
					},
				},
			})
			if err != nil {
				log.Printf("Error responding with continue button: %v", err)
			}
			return
		}

		// All fields collected - create the GitHub issue
		createIssueFromState(s, i, state, stateKey)
		return
	}

	// No state means the submission was lost: the bot restarted, or the modal
	// sat open across a redeploy. /bug and /feature both record state when they
	// open the modal, so this is never a normal single-modal submission.
	//
	// Filing an issue from whatever this one modal happens to hold would create
	// a partial report and silently drop every field that was never collected,
	// so ask for a fresh submission instead.
	log.Printf("No modal state for a %s submission; asking for resubmission", command)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "This submission expired before it could be filed, so no issue was created. Please run the command again.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// handleModalContinuation processes multi-part modal submissions
func handleModalContinuation(s *discordgo.Session, i *discordgo.InteractionCreate, stateKey string) {
	state, exists := lookupModalState(stateKey)
	if !exists {
		log.Printf("Modal state not found for a %s submission", commandFromStateKey(stateKey))
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Session expired. Please start over.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Extract submitted values
	data := i.ModalSubmitData()
	collectSubmittedValues(state, data.Components)

	currentIndex := state.answeredCount()

	// Check if there are more fields to show
	if currentIndex < len(state.AllFields) {
		totalParts := (len(state.AllFields) + 4) / 5
		currentPart := (currentIndex + 4) / 5
		message := continuePrompt(currentPart, totalParts)

		// Create continue button
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.Button{
								Label:    "Continue",
								Style:    discordgo.PrimaryButton,
								CustomID: fmt.Sprintf("continue_%s", stateKey),
							},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("Error responding with continue button: %v", err)
		}
		return
	}

	// All fields collected - create the GitHub issue
	createIssueFromState(s, i, state, stateKey)
}

func handleButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if strings.HasPrefix(customID, retryPrefix) {
		stateKey := strings.TrimPrefix(customID, retryPrefix)
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

	// Check if this is a continue button
	if strings.HasPrefix(customID, "continue_") {
		stateKey := strings.TrimPrefix(customID, "continue_")
		state, exists := lookupModalState(stateKey)
		if !exists {
			log.Printf("Modal state not found for a %s submission", commandFromStateKey(stateKey))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Session expired. Please start over.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Show the next modal chunk
		currentIndex := state.answeredCount()
		endIndex := currentIndex + 5
		if endIndex > len(state.AllFields) {
			endIndex = len(state.AllFields)
		}
		nextChunk := state.AllFields[currentIndex:endIndex]

		// Build modal components
		components := make([]discordgo.MessageComponent, 0, len(nextChunk))
		for _, field := range nextChunk {
			style := discordgo.TextInputShort
			if field.Style == "paragraph" {
				style = discordgo.TextInputParagraph
			}

			components = append(components, discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    field.CustomID,
						Label:       field.Label,
						Style:       style,
						Placeholder: truncatePlaceholder(field.Placeholder),
						Required:    field.Required,
					},
				},
			})
		}

		// Warn on the dialog the reporter submits from, if it has room.
		if endIndex == len(state.AllFields) && len(components) < 5 {
			components = append(components, config.NoticeComponent())
		}

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID:   fmt.Sprintf("modal_continue_%s", stateKey),
				Title:      dialogTitle(state),
				Components: components,
			},
		})
		if err != nil {
			log.Printf("Error showing next modal: %v", err)
		}
	}
}
