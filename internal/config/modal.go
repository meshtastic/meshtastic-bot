package config

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	ExcludeFields  []string      `yaml:"exclude_fields,omitempty"`

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
	// DefaultRepo is "owner/repo" for /changelog and a bare /repo. Without it
	// they take the first template's repository, so reordering the list moves them.
	DefaultRepo string        `yaml:"default_repo"`
	Modals      []ModalConfig `yaml:"config"`
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

// GetOwnerAndRepo returns default_repo, or else the owner and repo of the first
// configured template URL. Returns empty strings if neither is configured.
func GetOwnerAndRepo() (string, string) {
	if loadedModals == nil {
		return "", ""
	}

	if owner, repo, ok := strings.Cut(strings.TrimSpace(loadedModals.DefaultRepo), "/"); ok && owner != "" && repo != "" {
		return owner, repo
	}

	// Find the first modal with a template URL
	for _, modal := range loadedModals.Modals {
		if modal.TemplateURL != nil {
			return modal.TemplateURL.Owner(), modal.TemplateURL.Repo()
		}
	}

	return "", ""
}

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

// templateCacheTTL is how long a fetched template is served before a refresh.
// /bug and /feature must answer within Discord's 3 s window, so an expired
// entry is still served while the refresh runs in the background.
const templateCacheTTL = 10 * time.Minute

// ErrNotConfigured means the command has no form in the channel it was run in.
var ErrNotConfigured = errors.New("no form configured")

// ErrTemplateLoading means a template has never been fetched and did not
// arrive within templateWait; the fetch carries on in the background.
var ErrTemplateLoading = errors.New("the issue template is still loading")

var (
	templateHTTP = &http.Client{Timeout: 10 * time.Second}
	// templateWait bounds how long a command waits for a template it has
	// never had, leaving time to answer within Discord's 3 s.
	templateWait = 2 * time.Second
	// templateRetry is how soon a failed first fetch is tried again.
	templateRetry   = time.Minute
	templateCacheMu sync.Mutex
	templateCache   = map[string]*cachedTemplate{}
)

type cachedTemplate struct {
	template   *GitHubIssueTemplate
	fetched    time.Time
	refreshing bool
	// loading is closed when an in-flight first fetch finishes.
	loading chan struct{}
}

// FetchGitHubTemplate returns a template from the cache. A template never
// fetched is loaded in the background, waiting at most templateWait for it.
// The result is shared; callers must not modify it.
func FetchGitHubTemplate(templateURL *TemplateURL) (*GitHubIssueTemplate, error) {
	url := templateURL.RawURL()

	templateCacheMu.Lock()
	entry, ok := templateCache[url]
	if !ok {
		entry = &cachedTemplate{}
		templateCache[url] = entry
	}
	if entry.template != nil {
		if time.Since(entry.fetched) >= templateCacheTTL && !entry.refreshing {
			entry.refreshing = true
			go refreshTemplate(url)
		}
		template := entry.template
		templateCacheMu.Unlock()
		return template, nil
	}
	loading := startLoadLocked(url, entry)
	templateCacheMu.Unlock()

	select {
	case <-loading:
	case <-time.After(templateWait):
		return nil, ErrTemplateLoading
	}
	templateCacheMu.Lock()
	defer templateCacheMu.Unlock()
	if entry.template == nil {
		return nil, ErrTemplateLoading
	}
	return entry.template, nil
}

// startLoadLocked starts a first fetch unless one is running; the caller holds
// templateCacheMu.
func startLoadLocked(url string, entry *cachedTemplate) chan struct{} {
	if entry.loading == nil {
		entry.loading = make(chan struct{})
		go loadTemplate(url, entry)
	}
	return entry.loading
}

// loadTemplate fetches a template the cache has never held, and on failure
// tries again after templateRetry until one succeeds.
func loadTemplate(url string, entry *cachedTemplate) {
	template, err := fetchTemplate(url)
	templateCacheMu.Lock()
	defer templateCacheMu.Unlock()
	close(entry.loading)
	entry.loading = nil
	if err != nil {
		log.Printf("Could not load a template, retrying in %s: %v", templateRetry, err)
		time.AfterFunc(templateRetry, func() {
			templateCacheMu.Lock()
			defer templateCacheMu.Unlock()
			if entry.template == nil {
				startLoadLocked(url, entry)
			}
		})
		return
	}
	entry.template = template
	entry.fetched = time.Now()
}

func refreshTemplate(url string) {
	template, err := fetchTemplate(url)
	templateCacheMu.Lock()
	defer templateCacheMu.Unlock()
	entry := templateCache[url]
	entry.refreshing = false
	if err != nil {
		// Try again in a minute rather than on every command during an outage.
		entry.fetched = time.Now().Add(time.Minute - templateCacheTTL)
		log.Printf("Keeping the cached template after a failed refresh: %v", err)
		return
	}
	entry.template = template
	entry.fetched = time.Now()
}

// PrefetchTemplates starts loading every template so the first /bug or
// /feature finds it cached. It waits at most templateWait for each.
func PrefetchTemplates() {
	if loadedModals == nil {
		return
	}
	for _, modal := range loadedModals.Modals {
		if modal.TemplateURL == nil {
			continue
		}
		if _, err := FetchGitHubTemplate(modal.TemplateURL); err != nil {
			log.Printf("Could not prefetch the %s template: %v", modal.Command, err)
		}
	}
}

func fetchTemplate(url string) (*GitHubIssueTemplate, error) {
	resp, err := templateHTTP.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch template from %s: status code %d",
			url, resp.StatusCode)
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

// isFieldExcluded checks if a field ID is in the exclusion list (case-insensitive)
func isFieldExcluded(fieldID string, excludeList []string) bool {
	fieldIDLower := strings.ToLower(fieldID)
	for _, excludedField := range excludeList {
		if fieldIDLower == strings.ToLower(excludedField) {
			return true
		}
	}
	return false
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

	placeholder := field.Attributes.Placeholder
	if field.Type == "dropdown" && placeholder == "" {
		placeholder = dropdownPlaceholder(field.Attributes)
	}

	config := &FieldConfig{
		CustomID:    field.ID,
		Label:       field.Attributes.Label,
		Style:       style,
		Placeholder: truncateRunes(placeholder, discordPlaceholderLimit),
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

// discordPlaceholderLimit is Discord's cap on a text input placeholder, in characters.
const discordPlaceholderLimit = 100

// dropdownPlaceholder lists a dropdown's options, since the modal renders it as a plain text box.
func dropdownPlaceholder(attrs FieldAttributes) string {
	if len(attrs.Options) == 0 {
		return ""
	}
	text := "One of: "
	if attrs.Multiple {
		text = "One or more of: "
	}
	const more = ", …"
	for i, opt := range attrs.Options {
		next := opt.Label
		if i > 0 {
			next = ", " + next
		}
		room := discordPlaceholderLimit
		if i < len(attrs.Options)-1 {
			room -= utf8.RuneCountInString(more)
		}
		if utf8.RuneCountInString(text+next) > room {
			if i == 0 {
				return text + "…"
			}
			return text + more
		}
		text += next
	}
	return text
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}

// GetAllFieldsForModal returns all fields for a modal config (used for multi-part modals)
// Returns: fields, title, owner, repo, error
func GetAllFieldsForModal(command, channelID string) ([]FieldConfig, string, string, string, error) {
	if loadedModals == nil {
		return nil, "", "", "", fmt.Errorf("modals not loaded")
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
		return nil, "", "", "", fmt.Errorf("%w: command '%s' in channel '%s'", ErrNotConfigured, command, channelID)
	}

	var fields []FieldConfig
	var title string
	var owner string
	var repo string

	// If template URL is configured, fetch and convert fields
	if modalConfig.TemplateURL != nil {
		template, err := FetchGitHubTemplate(modalConfig.TemplateURL)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("failed to fetch template: %w", err)
		}

		// Use template name as title
		title = template.Name
		// Extract owner and repo from template URL
		owner = modalConfig.TemplateURL.Owner()
		repo = modalConfig.TemplateURL.Repo()

		templateFields := GetTemplateFields(template)
		for _, field := range templateFields {
			// Skip excluded fields
			if isFieldExcluded(field.ID, modalConfig.ExcludeFields) {
				continue
			}
			if converted := ConvertGitHubFieldToFieldConfig(field); converted != nil {
				fields = append(fields, *converted)
			}
		}
	} else {
		// Use configured fields
		fields = modalConfig.Fields
		title = modalConfig.Title
		// For legacy configs without template URL, return empty owner/repo
		owner = ""
		repo = ""
	}

	return fields, title, owner, repo, nil
}

// NoticeFieldID marks the notice appended to the final dialog.
//
// A Discord dialog carries nothing but input fields, so a warning has to be a
// field of its own. This one is never a template field: submission skips it, so
// it reaches neither the issue body nor the count of collected values.
const NoticeFieldID = "__public_issue_notice"

// NoticeComponent returns the notice shown at the end of the final dialog,
// where the reporter is about to submit.
func NoticeComponent() discordgo.ActionsRow {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:    NoticeFieldID,
				Label:       "This creates a public GitHub issue",
				Style:       discordgo.TextInputShort,
				Placeholder: "It shows your Discord username and user ID. Leave this blank.",
				Required:    false,
			},
		},
	}
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
		return nil, fmt.Errorf("%w: command '%s' in channel '%s'", ErrNotConfigured, command, channelID)
	}

	var fields []FieldConfig
	var title string

	// If template URL is configured, fetch and convert fields
	if modalConfig.TemplateURL != nil {
		template, err := FetchGitHubTemplate(modalConfig.TemplateURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch template: %w", err)
		}

		// Use template name as title
		title = template.Name

		templateFields := GetTemplateFields(template)
		for _, field := range templateFields {
			// Skip excluded fields
			if isFieldExcluded(field.ID, modalConfig.ExcludeFields) {
				continue
			}
			if converted := ConvertGitHubFieldToFieldConfig(field); converted != nil {
				fields = append(fields, *converted)
			}
		}
	} else {
		// Use configured fields
		fields = modalConfig.Fields
		title = modalConfig.Title
	}

	// Discord modals can only have 5 components max
	// If there are more, we'll need multi-part modals (handled by the caller)
	maxFields := 5
	moreToCome := len(fields) > maxFields
	if moreToCome {
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

	// Warn on the dialog the reporter submits from. When more fields follow,
	// this is not that dialog, and a full one has no room to spare.
	if !moreToCome && len(components) < maxFields {
		components = append(components, NoticeComponent())
	}

	return &discordgo.InteractionResponseData{
		CustomID:   fmt.Sprintf("modal_%s_%s", command, channelID),
		Title:      title,
		Components: components,
	}, nil
}
