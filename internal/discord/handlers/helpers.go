package handlers

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// noMentions stops a public reply pinging anyone: they echo user input and
// commit messages, either of which can hold @everyone.
var noMentions = &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}

// githubMention matches an @user or @org/team mention; a word character or @
// before it (an email address) is not one.
var githubMention = regexp.MustCompile(`(^|[^\w@])@([A-Za-z0-9])`)

// defuseMentions keeps the text but stops GitHub notifying anyone, since the
// issue is authored by an org member whose mentions reach teams.
func defuseMentions(s string) string {
	return githubMention.ReplaceAllString(s, "$1@\u200b$2")
}

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

// discordDialogTitleLimit is Discord's maximum length for a dialog title.
// Exceeding it fails the whole dialog with HTTP 400, stranding whatever the
// reporter has already typed.
const discordDialogTitleLimit = 45

// dialogTitle returns a header Discord will accept.
//
// The reporter's own title is arbitrary text and belongs to the issue, not to
// the dialog, so prefer the template name and fall back only far enough to
// guarantee the one to forty-five characters Discord requires.
func dialogTitle(state *ModalState) string {
	title := strings.TrimSpace(state.DisplayTitle)
	if title == "" {
		title = strings.TrimSpace(state.Title)
	}

	if runes := []rune(title); len(runes) > discordDialogTitleLimit {
		title = strings.TrimSpace(string(runes[:discordDialogTitleLimit]))
	}

	// Discord rejects an empty title as firmly as an over-long one.
	if title == "" {
		title = "Report"
	}

	return title
}

// collectSubmittedValues stores one dialog's answers against their field
// labels.
//
// Both the first dialog and every continuation land here. They used to collect
// values separately, which is how the notice came to be skipped on one path and
// not the other: it would have been stored as an answered field, adding an
// empty section to the issue body and throwing off which part comes next.
func collectSubmittedValues(state *ModalState, components []discordgo.MessageComponent, attachments map[string]*discordgo.MessageAttachment) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, component := range components {
		switch c := component.(type) {
		case *discordgo.ActionsRow:
			for _, inner := range c.Components {
				state.collectLocked(inner, attachments)
			}
		case *discordgo.Label:
			state.collectLocked(c.Component, attachments)
		}
	}
}

// collectLocked stores one input's answer; the caller holds st.mu.
func (st *ModalState) collectLocked(component discordgo.MessageComponent, attachments map[string]*discordgo.MessageAttachment) {
	var id, value string
	switch c := component.(type) {
	case *discordgo.TextInput:
		id, value = c.CustomID, c.Value
	case *discordgo.SelectMenu:
		id, value = c.CustomID, strings.Join(c.Values, ", ")
	case *discordgo.FileUpload:
		for _, attachmentID := range c.Values {
			if a := attachments[attachmentID]; a != nil {
				st.Files = append(st.Files, AttachedFile{Name: a.Filename, URL: a.URL, Size: a.Size, ContentType: a.ContentType})
			}
		}
		return
	default:
		return
	}

	// The notice is not a template field.
	if id == config.NoticeFieldID {
		return
	}

	// Values are keyed by label, which is what the issue body prints.
	label := id
	for _, field := range st.AllFields {
		if field.CustomID == id {
			label = field.Label
			break
		}
	}
	st.SubmittedValues[label] = value
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

// buildIssueBody constructs the issue body from submitted values, laid out as
// GitHub's own issue form writes one: a heading, a blank line, the answer or
// "_No response_", and code fences for fields the template renders as code.
//
// Sections follow the order of the issue template's fields. Ranging over the
// map alone would emit them in Go's randomised map order, so the same report
// would be laid out differently every time it was filed.
func buildIssueBody(allFields []config.FieldConfig, submittedValues map[string]string, logs []logFile, username, userID string) string {
	var body strings.Builder

	written := make(map[string]bool, len(submittedValues))
	for _, field := range allFields {
		if field.Kind == config.FieldUpload {
			continue
		}
		value, ok := submittedValues[field.Label]
		if !ok || written[field.Label] {
			continue
		}
		written[field.Label] = true
		body.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", field.Label, formatAnswer(value, field.Render)))
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
		body.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", label, formatAnswer(submittedValues[label], "")))
	}

	if len(logs) > 0 {
		body.WriteString(logsHeading)
		for _, l := range logs {
			body.WriteString(formatLog(l))
		}
	}

	body.WriteString(fmt.Sprintf("\n---\nSubmitted via Discord by: %s (%s)", username, userID))

	return body.String()
}

const (
	// githubBodyLimit is GitHub's cap on an issue body, in characters.
	githubBodyLimit = 65536
	logsHeading     = "### Log files\n\n"
)

// issueBody is the issue body with as much of the attached logs as fits
// GitHub's limit once the answers are in.
// The marker, the retry's only way to find this issue, goes last and is
// never cut.
func issueBody(fields []config.FieldConfig, values map[string]string, files []AttachedFile, username, userID, marker string) string {
	trailer := "\n\n" + marker
	limit := githubBodyLimit - utf8.RuneCountInString(trailer)
	body := buildIssueBody(fields, values, nil, username, userID)
	if len(files) > 0 {
		room := limit - utf8.RuneCountInString(body) - utf8.RuneCountInString(logsHeading)
		if logs := fetchLogs(files, room); len(logs) > 0 {
			body = buildIssueBody(fields, values, logs, username, userID)
		}
	}
	// Only answers far beyond any template's field limits reach this.
	if r := []rune(body); len(r) > limit {
		const cut = "\n\n_Cut to fit GitHub's limit._"
		body = string(r[:limit-utf8.RuneCountInString(cut)]) + cut
	}
	return body + trailer
}

// formatAnswer renders one answer. Mentions inside a code fence notify nobody,
// so rendered fields are left byte for byte, which logs need.
func formatAnswer(value, render string) string {
	if strings.TrimSpace(value) == "" {
		return "_No response_"
	}
	if render != "" {
		return codeFence(value, render)
	}
	return defuseMentions(value)
}

// codeFence wraps text in a fence longer than any backtick run inside it.
func codeFence(text, lang string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + lang + "\n" + strings.TrimRight(text, "\n") + "\n" + fence
}

func formatLog(l logFile) string {
	name := strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(l.Name)
	if l.Content == "" {
		return fmt.Sprintf("- `%s`: %s\n\n", strings.ReplaceAll(l.Name, "`", "'"), l.Note)
	}
	note := ""
	if l.Note != "" {
		note = " (" + l.Note + ")"
	}
	return fmt.Sprintf("<details><summary>%s%s</summary>\n\n%s\n\n</details>\n\n", name, note, codeFence(l.Content, ""))
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

// formUnavailable tells a reporter why no form opened.
func formUnavailable(noun string, err error) string {
	if errors.Is(err, config.ErrNotConfigured) {
		return fmt.Sprintf("Sorry, the %s command is not configured for this channel.", noun)
	}
	return fmt.Sprintf("Sorry, the %s form could not be loaded just now. Please try again in a minute.", noun)
}
