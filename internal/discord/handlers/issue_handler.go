package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// issueCommand is what differs between /bug and /feature.
type issueCommand struct {
	name string
	noun string
	// label is used when the template names none of its own.
	label string
}

var (
	bugCommand     = issueCommand{name: "bug", noun: "bug report", label: "bug"}
	featureCommand = issueCommand{name: "feature", noun: "feature request", label: "enhancement"}
)

func handleBug(s *discordgo.Session, i *discordgo.InteractionCreate) {
	openIssueForm(s, i, bugCommand)
}

func handleFeature(s *discordgo.Session, i *discordgo.InteractionCreate) {
	openIssueForm(s, i, featureCommand)
}

func openIssueForm(s *discordgo.Session, i *discordgo.InteractionCreate, cmd issueCommand) {
	form, err := config.GetIssueForm(cmd.name, i.ChannelID)
	if err != nil {
		log.Printf("Error getting the %s form: %v", cmd.name, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: formUnavailable(cmd.noun, err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	fields := append(append([]config.FieldConfig{}, form.Fields...), config.UploadField())
	stateKey := fmt.Sprintf("%s_%s_%s", cmd.name, i.ChannelID, i.Member.User.ID)
	state := &ModalState{
		Title:           issueTitle(form.TitlePrefix, commandTitleOption(i), form.Name),
		SearchText:      commandTitleOption(i),
		DisplayTitle:    form.Name,
		AllFields:       fields,
		SubmittedValues: make(map[string]string),
		Labels:          issueLabels(form.Labels, cmd.label),
		Command:         cmd.name,
		ChannelID:       i.ChannelID,
		Owner:           form.Owner,
		Repo:            form.Repo,
	}
	putModalState(stateKey, state)
	showDialog(s, i, state, stateKey)
}

// issueTitle gives the reporter's title the prefix GitHub's form would add,
// within GitHub's 256-character cap.
func issueTitle(prefix, title, fallback string) string {
	if title == "" {
		title = fallback
	}
	if p := strings.TrimSpace(prefix); p != "" && !strings.HasPrefix(strings.ToLower(title), strings.ToLower(p)) {
		title = prefix + title
	}
	if r := []rune(title); len(r) > 256 {
		title = string(r[:256])
	}
	return title
}

// issueLabels is from-discord plus the template's labels, or the command's
// own label when the template has none.
func issueLabels(template []string, fallback string) []string {
	if len(template) == 0 {
		template = []string{fallback}
	}
	labels := []string{"from-discord"}
	seen := map[string]bool{"from-discord": true}
	for _, l := range template {
		if !seen[strings.ToLower(l)] {
			seen[strings.ToLower(l)] = true
			labels = append(labels, l)
		}
	}
	return labels
}

// showDialog opens the next dialog of a report.
//
// It is built with Discord's labelled components. If Discord refuses that
// payload the interaction is still unanswered, so the same fields are sent
// again as plain text inputs rather than leaving the reporter with a form that
// will not open.
func showDialog(s *discordgo.Session, i *discordgo.InteractionCreate, state *ModalState, stateKey string) {
	start, end := state.beginDialog()
	fields := state.AllFields[start:end]
	notice := end == len(state.AllFields) && len(fields) < config.DialogComponentLimit

	data := &discordgo.InteractionResponseData{
		CustomID:   continueModalPrefix + stateKey,
		Title:      dialogTitle(state),
		Components: config.DialogComponents(fields, notice, false),
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: data})
	if err == nil {
		return
	}
	log.Printf("Discord refused the %s dialog, sending plain text inputs instead: %v", state.Command, err)
	data.Components = config.DialogComponents(fields, notice, true)
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: data}); err != nil {
		log.Printf("Error showing the %s dialog: %v", state.Command, err)
	}
}
