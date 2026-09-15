package handlers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// commandTitleOption returns the value of the "title" slash-command option.
//
// The GitHub issue templates the modals are built from define no title field,
// so /bug and /feature collect the issue title as a command option instead.
func commandTitleOption(i *discordgo.InteractionCreate) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "title" {
			return strings.TrimSpace(opt.StringValue())
		}
	}
	return ""
}

// collectSubmittedValues stores one dialog's answers against their field
// labels.
//
// Both the first dialog and every continuation land here. They used to collect
// values separately, which is how the notice came to be skipped on one path and
// not the other: it would have been stored as an answered field, adding an
// empty section to the issue body and throwing off which part comes next.
func collectSubmittedValues(state *ModalState, components []discordgo.MessageComponent) {
	for _, component := range components {
		actionRow, ok := component.(*discordgo.ActionsRow)
		if !ok {
			continue
		}

		for _, comp := range actionRow.Components {
			textInput, ok := comp.(*discordgo.TextInput)
			if !ok {
				continue
			}

			// The notice is not a template field.
			if textInput.CustomID == config.NoticeFieldID {
				continue
			}

			// Values are keyed by label, which is what the issue body prints.
			label := textInput.CustomID
			for _, field := range state.AllFields {
				if field.CustomID == textInput.CustomID {
					label = field.Label
					break
				}
			}

			state.SubmittedValues[label] = textInput.Value
		}
	}
}

// continuePrompt is the message shown between parts of a multi-part submission.
//
// The warning lands on the step before the final dialog, where pressing
// Continue opens the last one: that is the reporter's last chance to stop
// before an issue is created. The dialog itself cannot carry this text,
// because every component in it is an input field.
func continuePrompt(currentPart, totalParts int) string {
	message := fmt.Sprintf("Part %d of %d complete. Click 'Continue' to proceed.",
		currentPart, totalParts)

	if currentPart+1 >= totalParts {
		message += "\n\n**The next part is the last one.** Submitting it creates a public " +
			"GitHub issue showing your Discord username and user ID."
	}

	return message
}

// buildIssueBody constructs the issue body from submitted values.
//
// Sections follow the order of the issue template's fields. Ranging over the
// map alone would emit them in Go's randomised map order, so the same report
// would be laid out differently every time it was filed.
func buildIssueBody(allFields []config.FieldConfig, submittedValues map[string]string, username, userID string) string {
	var body strings.Builder

	written := make(map[string]bool, len(submittedValues))
	for _, field := range allFields {
		value, ok := submittedValues[field.Label]
		if !ok || written[field.Label] {
			continue
		}
		written[field.Label] = true
		body.WriteString(fmt.Sprintf("### %s\n%s\n\n", field.Label, value))
	}

	// A submitted value that matches no template field still belongs in the
	// issue; emit those in a stable order rather than dropping them.
	leftover := make([]string, 0, len(submittedValues))
	for label := range submittedValues {
		if !written[label] {
			leftover = append(leftover, label)
		}
	}
	sort.Strings(leftover)
	for _, label := range leftover {
		body.WriteString(fmt.Sprintf("### %s\n%s\n\n", label, submittedValues[label]))
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
