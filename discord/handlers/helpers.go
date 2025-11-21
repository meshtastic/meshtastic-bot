package handlers

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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
