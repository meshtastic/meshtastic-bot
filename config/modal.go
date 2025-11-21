package config

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
	"gopkg.in/yaml.v3"
)

type GitHubTemplateField struct {
	Type        string           `yaml:"type"`
	ID          string           `yaml:"id"`
	Attributes  FieldAttributes  `yaml:"attributes"`
	Validations FieldValidations `yaml:"validations,omitempty"`
}

type FieldAttributes struct {
	Label       string   `yaml:"label,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Placeholder string   `yaml:"placeholder,omitempty"`
	Value       string   `yaml:"value,omitempty"`
	Options     []Option `yaml:"options,omitempty"`
	Multiple    bool     `yaml:"multiple,omitempty"`
}

type FieldValidations struct {
	Required bool `yaml:"required,omitempty"`
}

// Option represents a choice in a dropdown or checkbox field
// It can be either a simple string or an object with label and required fields
type Option struct {
	Label    string
	Required bool
}

type GitHubIssueTemplate struct {
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Title       string                `yaml:"title,omitempty"`
	Labels      interface{}           `yaml:"labels,omitempty"`
	Body        []GitHubTemplateField `yaml:"body"`
}

// Legacy FieldConfig for backwards compatibility
type FieldConfig struct {
	CustomID    string `yaml:"custom_id"`
	Label       string `yaml:"label"`
	Style       string `yaml:"style"`
	Placeholder string `yaml:"placeholder"`
	Required    bool   `yaml:"required"`
	MinLength   int    `yaml:"min_length"`
	MaxLength   int    `yaml:"max_length"`
}

type ModalConfig struct {
	Command        string        `yaml:"command"`
	TemplateURLRaw string        `yaml:"template_url,omitempty"`
	ChannelIDs     []string      `yaml:"channel_id"`
	Title          string        `yaml:"title"`
	Fields         []FieldConfig `yaml:"fields,omitempty"`

	// Parsed template URL (populated after loading)
	TemplateURL *TemplateURL `yaml:"-"`
}

// ModalState tracks the state of multi-part modals
type ModalState struct {
	Title           string
	AllFields       []FieldConfig
	SubmittedValues map[string]string
	Labels          []string
	Command         string
	ChannelID       string
}

type ModalsConfig struct {
	Modals []ModalConfig `yaml:"config"`
}

// UnmarshalYAML custom unmarshals an Option from either a string or an object
func (o *Option) UnmarshalYAML(value *yaml.Node) error {
	// Try to unmarshal as a string first (for dropdown options)
	var str string
	if err := value.Decode(&str); err == nil {
		o.Label = str
		o.Required = false
		return nil
	}

	// If that fails, try to unmarshal as an object (for checkbox options)
	var obj struct {
		Label    string `yaml:"label"`
		Required bool   `yaml:"required"`
	}
	if err := value.Decode(&obj); err != nil {
		return err
	}

	o.Label = obj.Label
	o.Required = obj.Required
	return nil
}

var loadedModals *ModalsConfig

// LoadModals reads and parses the modal configuration from the specified YAML file
func LoadModals(ConfigPath string) error {
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read modal config file: %w", err)
	}

	var config ModalsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse modal config: %w", err)
	}

	// Parse template URLs for each modal config
	for i := range config.Modals {
		if config.Modals[i].TemplateURLRaw != "" {
			parsedURL, err := ParseTemplateURL(config.Modals[i].TemplateURLRaw)
			if err != nil {
				return fmt.Errorf("failed to parse template URL for command %s: %w",
					config.Modals[i].Command, err)
			}
			config.Modals[i].TemplateURL = parsedURL
		}
	}

	loadedModals = &config
	return nil
}

// FetchGitHubTemplate fetches and parses a GitHub issue template from a TemplateURL
func FetchGitHubTemplate(templateURL *TemplateURL) (*GitHubIssueTemplate, error) {
	resp, err := http.Get(templateURL.RawURL())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch template from %s: status code %d",
			templateURL.RawURL(), resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	var template GitHubIssueTemplate
	if err := yaml.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("failed to parse template YAML: %w", err)
	}

	return &template, nil
}

// GetTemplateFields returns all interactive fields from a GitHub issue template
func GetTemplateFields(template *GitHubIssueTemplate) []GitHubTemplateField {
	fields := make([]GitHubTemplateField, 0)
	for _, field := range template.Body {
		// Skip markdown and checkboxes fields as they're informational only
		if field.Type != "markdown" && field.Type != "checkboxes" {
			fields = append(fields, field)
		}
	}
	return fields
}

// ConvertGitHubFieldToFieldConfig converts a GitHub template field to a FieldConfig
func ConvertGitHubFieldToFieldConfig(field GitHubTemplateField) *FieldConfig {
	// Skip non-interactive fields
	if field.Type == "markdown" || field.Type == "checkboxes" {
		return nil
	}

	style := "short"
	if field.Type == "textarea" {
		style = "paragraph"
	}

	config := &FieldConfig{
		CustomID:    field.ID,
		Label:       field.Attributes.Label,
		Style:       style,
		Placeholder: field.Attributes.Placeholder,
		Required:    field.Validations.Required,
	}

	// Set reasonable defaults for min/max length
	switch field.Type {
	case "input":
		config.MinLength = 1
		config.MaxLength = 100
	case "textarea":
		config.MinLength = 1
		config.MaxLength = 4000
	}

	return config
}

// GetAllFieldsForModal returns all fields for a modal config (used for multi-part modals)
func GetAllFieldsForModal(command, channelID string) ([]FieldConfig, string, error) {
	if loadedModals == nil {
		return nil, "", fmt.Errorf("modals not loaded")
	}

	// Find the matching modal config
	var modalConfig *ModalConfig
	for _, modal := range loadedModals.Modals {
		if modal.Command == command {
			// Check if this modal applies to the given channel
			for _, cid := range modal.ChannelIDs {
				if cid == channelID {
					modalConfig = &modal
					break
				}
			}
			if modalConfig != nil {
				break
			}
		}
	}

	if modalConfig == nil {
		return nil, "", fmt.Errorf("no modal configured for command '%s' in channel '%s'", command, channelID)
	}

	var fields []FieldConfig

	// If template URL is configured, fetch and convert fields
	if modalConfig.TemplateURL != nil {
		template, err := FetchGitHubTemplate(modalConfig.TemplateURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch template: %w", err)
		}

		templateFields := GetTemplateFields(template)
		for _, field := range templateFields {
			if converted := ConvertGitHubFieldToFieldConfig(field); converted != nil {
				fields = append(fields, *converted)
			}
		}
	} else {
		// Use configured fields
		fields = modalConfig.Fields
	}

	return fields, modalConfig.Title, nil
}

// GetModel returns the modal data for a specific command and channel
func GetModel(command, channelID string) (*discordgo.InteractionResponseData, error) {
	if loadedModals == nil {
		return nil, fmt.Errorf("modals not loaded")
	}

	// Find the matching modal config
	var modalConfig *ModalConfig
	for _, modal := range loadedModals.Modals {
		if modal.Command == command {
			for _, cid := range modal.ChannelIDs {
				if cid == channelID {
					modalConfig = &modal
					break
				}
			}
			if modalConfig != nil {
				break
			}
		}
	}

	if modalConfig == nil {
		return nil, fmt.Errorf("no modal configured for command '%s' in channel '%s'", command, channelID)
	}

	var fields []FieldConfig

	// If template URL is configured, fetch and convert fields
	if modalConfig.TemplateURL != nil {
		template, err := FetchGitHubTemplate(modalConfig.TemplateURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch template: %w", err)
		}

		templateFields := GetTemplateFields(template)
		for _, field := range templateFields {
			if converted := ConvertGitHubFieldToFieldConfig(field); converted != nil {
				fields = append(fields, *converted)
			}
		}
	} else {
		// Use configured fields
		fields = modalConfig.Fields
	}

	// Discord modals can only have 5 components max
	// If there are more, we'll need multi-part modals (handled by the caller)
	maxFields := 5
	if len(fields) > maxFields {
		fields = fields[:maxFields]
	}

	// Build Discord modal components from the fields
	components := make([]discordgo.MessageComponent, 0, len(fields))
	for _, field := range fields {
		style := discordgo.TextInputShort
		if field.Style == "paragraph" {
			style = discordgo.TextInputParagraph
		}

		textInput := discordgo.TextInput{
			CustomID:    field.CustomID,
			Label:       field.Label,
			Style:       style,
			Placeholder: field.Placeholder,
			Required:    field.Required,
		}

		if field.MinLength > 0 {
			textInput.MinLength = field.MinLength
		}
		if field.MaxLength > 0 {
			textInput.MaxLength = field.MaxLength
		}

		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{textInput},
		})
	}

	return &discordgo.InteractionResponseData{
		CustomID:   fmt.Sprintf("modal_%s_%s", command, channelID),
		Title:      modalConfig.Title,
		Components: components,
	}, nil
}
