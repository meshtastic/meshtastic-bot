package config

import (
	"encoding/json"

	"github.com/bwmarrin/discordgo"
)

// Discord's limits for a dialog. A payload over any of them fails the whole
// dialog with HTTP 400.
const (
	DialogComponentLimit  = 5
	labelLimit            = 45
	labelDescriptionLimit = 100
	selectOptionLimit     = 25
	selectOptionTextLimit = 100
)

// FieldKind is the dialog component a field becomes.
type FieldKind string

const (
	FieldText   FieldKind = ""
	FieldSelect FieldKind = "select"
	FieldUpload FieldKind = "upload"
)

// UploadFieldID identifies the log upload added after a template's fields.
const UploadFieldID = "__log_files"

// UploadField lets a reporter attach text logs. Images cannot be re-hosted on
// GitHub from here, so they still go in a comment on the issue.
func UploadField() FieldConfig {
	return FieldConfig{
		CustomID:    UploadFieldID,
		Label:       "Log files (optional)",
		Description: "Text files only, posted verbatim in the public issue. Add screenshots in a comment on the issue.",
		Kind:        FieldUpload,
	}
}

// DialogComponents builds one dialog from fields, adding the public-issue
// notice when asked. legacy builds the shape every Discord client accepts, a
// text input per action row, which cannot hold uploads; they are left out.
func DialogComponents(fields []FieldConfig, notice, legacy bool) []discordgo.MessageComponent {
	components := make([]discordgo.MessageComponent, 0, len(fields)+1)
	for _, field := range fields {
		if legacy {
			if field.Kind == FieldUpload {
				continue
			}
			input := textInput(field)
			input.Label = field.Label
			components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{input}})
			continue
		}
		components = append(components, discordgo.Label{
			Label:       field.Label,
			Description: field.Description,
			Component:   fieldComponent(field),
		})
	}

	// A legacy dialog of uploads only would be empty, which Discord rejects.
	if notice || len(components) == 0 {
		if legacy {
			components = append(components, NoticeComponent())
		} else {
			components = append(components, NoticeText())
		}
	}
	return components
}

// modalSelect is a string select for a dialog. discordgo's SelectMenu always
// sends "disabled", which Discord documents as an error inside a modal.
type modalSelect struct {
	CustomID  string
	Options   []string
	MinValues int
	MaxValues int
	Required  bool
}

func (modalSelect) Type() discordgo.ComponentType { return discordgo.SelectMenuComponent }

func (m modalSelect) MarshalJSON() ([]byte, error) {
	type option struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}
	options := make([]option, len(m.Options))
	for i, o := range m.Options {
		options[i] = option{Label: o, Value: o}
	}
	return json.Marshal(struct {
		Type      discordgo.ComponentType `json:"type"`
		CustomID  string                  `json:"custom_id"`
		Options   []option                `json:"options"`
		MinValues int                     `json:"min_values"`
		MaxValues int                     `json:"max_values"`
		Required  bool                    `json:"required"`
	}{m.Type(), m.CustomID, options, m.MinValues, m.MaxValues, m.Required})
}

func fieldComponent(field FieldConfig) discordgo.MessageComponent {
	switch field.Kind {
	case FieldSelect:
		// Discord requires min_values of at least 1 unless required is false.
		menu := modalSelect{CustomID: field.CustomID, Options: field.Options, MaxValues: 1, Required: field.Required}
		if field.Required {
			menu.MinValues = 1
		}
		if field.Multiple {
			menu.MaxValues = len(field.Options)
		}
		return menu
	case FieldUpload:
		min := 0
		return discordgo.FileUpload{CustomID: field.CustomID, MinValues: &min, MaxValues: 5, Required: boolPtr(false)}
	default:
		return textInput(field)
	}
}

func textInput(field FieldConfig) discordgo.TextInput {
	style := discordgo.TextInputShort
	if field.Style == "paragraph" {
		style = discordgo.TextInputParagraph
	}
	input := discordgo.TextInput{
		CustomID:    field.CustomID,
		Style:       style,
		Placeholder: field.Placeholder,
		Required:    boolPtr(field.Required),
	}
	if field.MinLength > 0 {
		input.MinLength = field.MinLength
	}
	if field.MaxLength > 0 {
		input.MaxLength = field.MaxLength
	}
	return input
}
