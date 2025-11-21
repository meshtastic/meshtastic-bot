package handlers

import (
	"fmt"
	"log"
	"strings"

	github "meshtastic-bot/github"

	"github.com/bwmarrin/discordgo"
)

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	// Determine which command this modal is for based on CustomID
	// Format: "modal_<command>_<channelID>" or "modal_continue_<stateKey>"
	parts := strings.Split(data.CustomID, "_")
	if len(parts) < 2 {
		log.Printf("Invalid modal CustomID format: %s", data.CustomID)
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
	state, isMultiPart := modalStates[stateKey]

	if isMultiPart {
		// This is the first part of a multi-part modal
		// Extract and store the submitted values
		for _, component := range data.Components {
			if actionRow, ok := component.(*discordgo.ActionsRow); ok {
				for _, comp := range actionRow.Components {
					if textInput, ok := comp.(*discordgo.TextInput); ok {
						// Find the field label from the original fields
						fieldLabel := textInput.CustomID
						for _, field := range state.AllFields {
							if field.CustomID == textInput.CustomID {
								fieldLabel = field.Label
								break
							}
						}
						state.SubmittedValues[fieldLabel] = textInput.Value
					}
				}
			}
		}

		currentIndex := len(state.SubmittedValues)

		// Check if there are more fields to show
		if currentIndex < len(state.AllFields) {
			totalParts := (len(state.AllFields) + 4) / 5
			currentPart := (currentIndex + 4) / 5
			message := fmt.Sprintf("Part %d of %d complete. Click 'Continue' to proceed.",
				currentPart, totalParts)

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
		body := buildIssueBody(state.SubmittedValues, i.Member.User.Username, i.Member.User.ID)
		issue, err := GithubClient.CreateIssue(GithubOwner, GithubRepo, state.Title, body, state.Labels)
		if err != nil {
			log.Printf("Failed to create GitHub issue: %v", err)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Failed to create issue. Please try again later.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			delete(modalStates, stateKey)
			return
		}

		confirmationMessage := fmt.Sprintf("✅ Issue #%d created successfully!\n%s", issue.Number, issue.HTMLURL)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: confirmationMessage,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		delete(modalStates, stateKey)
		return
	}

	// Simple modal (5 or fewer fields)
	// Extract field values
	fields := extractModalFields(data.Components)

	// Get user info
	username := i.Member.User.Username
	userID := i.Member.User.ID

	// Determine labels based on command
	labels := []string{"from-discord"}
	switch command {
	case "bug":
		labels = append(labels, "bug")
	case "feature":
		labels = append(labels, "enhancement")
	}

	// Create GitHub issue
	title := fields["bug_title"]
	if title == "" {
		title = fields["feature_title"]
	}

	description := fields["bug_description"]
	if description == "" {
		description = fields["feature_description"]
	}

	body := github.FormatIssueBody(username, userID, description)

	issue, err := GithubClient.CreateIssue(GithubOwner, GithubRepo, title, body, labels)
	if err != nil {
		log.Printf("Failed to create GitHub issue: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to create issue. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Issue #%d created successfully!\n%s", issue.Number, issue.HTMLURL),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// handleModalContinuation processes multi-part modal submissions
func handleModalContinuation(s *discordgo.Session, i *discordgo.InteractionCreate, stateKey string) {
	state, exists := modalStates[stateKey]
	if !exists {
		log.Printf("Modal state not found for key: %s", stateKey)
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
	for _, component := range data.Components {
		if actionRow, ok := component.(*discordgo.ActionsRow); ok {
			for _, comp := range actionRow.Components {
				if textInput, ok := comp.(*discordgo.TextInput); ok {
					customID := textInput.CustomID
					value := textInput.Value

					// Find the field label from the original fields
					fieldLabel := customID // default to customID
					for _, field := range state.AllFields {
						if field.CustomID == customID {
							fieldLabel = field.Label
							break
						}
					}

					state.SubmittedValues[fieldLabel] = value
				}
			}
		}
	}

	currentIndex := len(state.SubmittedValues)

	// Check if there are more fields to show
	if currentIndex < len(state.AllFields) {
		totalParts := (len(state.AllFields) + 4) / 5
		currentPart := (currentIndex + 4) / 5
		message := fmt.Sprintf("Part %d of %d complete. Click 'Continue' to proceed.",
			currentPart, totalParts)

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
	body := buildIssueBody(state.SubmittedValues, i.Member.User.Username, i.Member.User.ID)
	issue, err := GithubClient.CreateIssue(GithubOwner, GithubRepo, state.Title, body, state.Labels)
	if err != nil {
		log.Printf("Failed to create GitHub issue: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to create issue. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		delete(modalStates, stateKey)
		return
	}

	confirmationMessage := fmt.Sprintf(
		"✅ Issue #%d created successfully!\n%s\n\n"+
			"**Note:** You can use Markdown formatting in your descriptions. "+
			"To add images or other attachments, please edit the issue directly on GitHub.",
		issue.Number, issue.HTMLURL,
	)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: confirmationMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	delete(modalStates, stateKey)
}

func handleButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	// Check if this is a continue button
	if strings.HasPrefix(customID, "continue_") {
		stateKey := strings.TrimPrefix(customID, "continue_")
		state, exists := modalStates[stateKey]
		if !exists {
			log.Printf("Modal state not found for key: %s", stateKey)
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
		currentIndex := len(state.SubmittedValues)
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

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID:   fmt.Sprintf("modal_continue_%s", stateKey),
				Title:      state.Title,
				Components: components,
			},
		})
		if err != nil {
			log.Printf("Error showing next modal: %v", err)
		}
	}
}
