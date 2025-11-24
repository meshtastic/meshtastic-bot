package discord

import (
	"fmt"
	"log"
	"strings"

	config "github.com/meshtastic/meshtastic-bot/internal/config"
	github "github.com/meshtastic/meshtastic-bot/internal/github"

	"github.com/bwmarrin/discordgo"
)

var (
	githubClient *github.Client
)

// ModalState tracks the state of multi-part modals
type ModalState struct {
	Title           string
	AllFields       []config.FieldConfig
	SubmittedValues map[string]string
	Labels          []string
	Command         string
	ChannelID       string
	Owner           string
	Repo            string
}

// Global storage for modal states (in production, consider using a proper session store)
var modalStates = make(map[string]*ModalState)

// commandHandlers maps command names to their handler functions
var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"tapsign": handleTapsign,
	"feature": handleFeature,
	"faq":     handleFaq,
	"bug":     handleBug,
}

// HandleInteraction routes interactions to appropriate handlers
func HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if handler, exists := commandHandlers[i.ApplicationCommandData().Name]; exists {
			handler(s, i)
		}
	case discordgo.InteractionApplicationCommandAutocomplete:
		handleAutocomplete(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	case discordgo.InteractionMessageComponent:
		handleButtonClick(s, i)
	}
}

func handleTapsign(s *discordgo.Session, i *discordgo.InteractionCreate) {
	helpText := "**How to get help or make a suggestion:**\n" +
		"`/help`: For general Meshtastic questions and help with this bot.\n" +
		"`/bug`: To report a bug with the app.\n" +
		"`/feature`: To request a new feature."

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: helpText,
		},
	})
}

func handleFaq(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the selected FAQ topic from the interaction
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please select a FAQ topic from the autocomplete options.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	topicName := options[0].StringValue()

	// Get FAQ data
	faqData := config.GetFAQData()
	if faqData == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "FAQ data is not available. Please contact an administrator.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Find the FAQ item
	item, found := faqData.FindFAQItem(topicName)
	if !found {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("FAQ topic '%s' not found.", topicName),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Respond with the FAQ link
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s**\n%s", item.Name, item.URL),
		},
	})
}

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

	// If more than 5 fields, set up multi-part modal state
	if len(allFields) > 5 {
		stateKey := fmt.Sprintf("%s_%s_%s", "bug", i.ChannelID, i.Member.User.ID)
		modalStates[stateKey] = &ModalState{
			Title:           title,
			AllFields:       allFields,
			SubmittedValues: make(map[string]string),
			Labels:          []string{"from-discord", "bug"},
			Command:         "bug",
			ChannelID:       i.ChannelID,
			Owner:           owner,
			Repo:            repo,
		}
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
		issue, err := githubClient.CreateIssue(state.Owner, state.Repo, state.Title, body, state.Labels)

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

		// Clean up modal state
		delete(modalStates, stateKey)
		return
	}

	// Simple modal (5 or fewer fields) - old behavior
	// Get owner and repo from modal config
	_, _, owner, repo, err := config.GetAllFieldsForModal(command, channelID)
	if err != nil {
		log.Printf("Error getting modal config: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to create issue. Configuration error.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

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

	issue, err := githubClient.CreateIssue(owner, repo, title, body, labels)
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

	// Success response
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
	issue, err := githubClient.CreateIssue(state.Owner, state.Repo, state.Title, body, state.Labels)

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

// handleButtonClick handles button interactions (e.g., Continue button)
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

// buildIssueBody constructs the issue body from submitted values
func buildIssueBody(submittedValues map[string]string, username, userID string) string {
	var body strings.Builder

	for label, value := range submittedValues {
		body.WriteString(fmt.Sprintf("### %s\n%s\n\n", label, value))
	}

	body.WriteString(fmt.Sprintf("\n---\nSubmitted via Discord by: %s (%s)", username, userID))

	return body.String()
}

// truncatePlaceholder truncates placeholder text to 100 chars
func truncatePlaceholder(text string) string {
	if len(text) > 100 {
		return text[:97] + "..."
	}
	return text
}

// extractModalFields extracts field values from modal components
func extractModalFields(components []discordgo.MessageComponent) map[string]string {
	fields := make(map[string]string)

	for _, component := range components {
		if actionRow, ok := component.(*discordgo.ActionsRow); ok {
			for _, comp := range actionRow.Components {
				if textInput, ok := comp.(*discordgo.TextInput); ok {
					fields[textInput.CustomID] = textInput.Value
				}
			}
		}
	}

	return fields
}

// handleAutocomplete handles autocomplete interactions for commands
func handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	switch data.Name {
	case "faq":
		handleFaqAutocomplete(s, i)
	}
}

// handleFaqAutocomplete provides autocomplete suggestions for FAQ topics
func handleFaqAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	faqData := config.GetFAQData()
	if faqData == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{
				Choices: []*discordgo.ApplicationCommandOptionChoice{},
			},
		})
		return
	}

	// Get the current user input
	options := i.ApplicationCommandData().Options
	var userInput string
	if len(options) > 0 {
		userInput = strings.ToLower(options[0].StringValue())
	}

	// Get all FAQ items
	allItems := faqData.GetAllFAQItems()

	// Filter and create choices
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, 25)
	for _, item := range allItems {
		// Filter by user input if provided
		if userInput != "" && !strings.Contains(strings.ToLower(item.Name), userInput) {
			continue
		}

		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  item.Name,
			Value: item.Name,
		})

		// Discord limits autocomplete to 25 choices
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
