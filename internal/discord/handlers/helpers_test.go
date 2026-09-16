package handlers

import (
	"strings"
	"testing"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

func TestTruncatePlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short text unchanged",
			input:    "Short text",
			expected: "Short text",
		},
		{
			name:     "exactly 100 chars unchanged",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 100),
		},
		{
			name:     "long text truncated with ellipsis",
			input:    strings.Repeat("a", 150),
			expected: strings.Repeat("a", 97) + "...",
		},
		{
			name:     "101 chars truncated",
			input:    strings.Repeat("x", 101),
			expected: strings.Repeat("x", 97) + "...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePlaceholder(tt.input)
			if result != tt.expected {
				t.Errorf("truncatePlaceholder(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > 100 {
				t.Errorf("truncatePlaceholder(%q) returned string longer than 100 chars: %d", tt.input, len(result))
			}
		})
	}
}

func TestBuildIssueBody(t *testing.T) {
	fields := func(labels ...string) []config.FieldConfig {
		out := make([]config.FieldConfig, 0, len(labels))
		for _, l := range labels {
			out = append(out, config.FieldConfig{Label: l})
		}
		return out
	}

	tests := []struct {
		name            string
		allFields       []config.FieldConfig
		submittedValues map[string]string
		username        string
		userID          string
		wantContains    []string
	}{
		{
			name:      "single field",
			allFields: fields("Description"),
			submittedValues: map[string]string{
				"Description": "This is a bug",
			},
			username: "testuser",
			userID:   "123456",
			wantContains: []string{
				"### Description",
				"This is a bug",
				"Submitted via Discord by: testuser (123456)",
			},
		},
		{
			name:      "multiple fields",
			allFields: fields("Title", "Description", "Steps"),
			submittedValues: map[string]string{
				"Title":       "Bug Title",
				"Description": "Bug description",
				"Steps":       "1. Do this\n2. Do that",
			},
			username: "john_doe",
			userID:   "789",
			wantContains: []string{
				"### Title",
				"Bug Title",
				"### Description",
				"Bug description",
				"### Steps",
				"1. Do this\n2. Do that",
				"Submitted via Discord by: john_doe (789)",
			},
		},
		{
			name:            "empty values",
			allFields:       nil,
			submittedValues: map[string]string{},
			username:        "emptyuser",
			userID:          "000",
			wantContains: []string{
				"Submitted via Discord by: emptyuser (000)",
			},
		},
		{
			name:      "value with no matching field is still included",
			allFields: fields("Known"),
			submittedValues: map[string]string{
				"Known":  "in template",
				"Orphan": "not in template",
			},
			username: "someone",
			userID:   "555",
			wantContains: []string{
				"### Known",
				"in template",
				"### Orphan",
				"not in template",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildIssueBody(tt.allFields, tt.submittedValues, tt.username, tt.userID)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildIssueBody() missing expected content:\nwant substring: %q\ngot: %q", want, result)
				}
			}
		})
	}
}

// Map iteration order in Go is randomised, so a body built by ranging over the
// submitted values alone would vary between identical submissions. Build the
// same report repeatedly and require byte-identical output in template order.
func TestBuildIssueBodyOrderIsDeterministic(t *testing.T) {
	allFields := []config.FieldConfig{
		{Label: "Prerequisites"},
		{Label: "Hardware"},
		{Label: "Expected behavior"},
		{Label: "Actual behavior"},
		{Label: "Steps to reproduce"},
		{Label: "Logs"},
	}
	submitted := map[string]string{
		"Prerequisites":      "checked",
		"Hardware":           "RAK4631",
		"Expected behavior":  "it works",
		"Actual behavior":    "it does not",
		"Steps to reproduce": "1. flash\n2. wait",
		"Logs":               "none",
	}

	first := buildIssueBody(allFields, submitted, "user", "1")

	for i := 0; i < 50; i++ {
		if got := buildIssueBody(allFields, submitted, "user", "1"); got != first {
			t.Fatalf("buildIssueBody() output varied between calls:\nfirst: %q\ngot:   %q", first, got)
		}
	}

	// Sections must follow template order, not map order.
	wantOrder := []string{
		"### Prerequisites",
		"### Hardware",
		"### Expected behavior",
		"### Actual behavior",
		"### Steps to reproduce",
		"### Logs",
	}
	pos := -1
	for _, heading := range wantOrder {
		idx := strings.Index(first, heading)
		if idx == -1 {
			t.Fatalf("buildIssueBody() missing heading %q", heading)
		}
		if idx <= pos {
			t.Errorf("buildIssueBody() heading %q out of template order", heading)
		}
		pos = idx
	}
}

func TestExtractModalFields(t *testing.T) {
	tests := []struct {
		name       string
		components []discordgo.MessageComponent
		expected   map[string]string
	}{
		{
			name: "single text input",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "title",
							Value:    "My Title",
						},
					},
				},
			},
			expected: map[string]string{
				"title": "My Title",
			},
		},
		{
			name: "multiple text inputs",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "title",
							Value:    "Bug Report",
						},
					},
				},
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "description",
							Value:    "The app crashes",
						},
					},
				},
			},
			expected: map[string]string{
				"title":       "Bug Report",
				"description": "The app crashes",
			},
		},
		{
			name:       "empty components",
			components: []discordgo.MessageComponent{},
			expected:   map[string]string{},
		},
		{
			name: "multiple inputs in same row",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "field1",
							Value:    "value1",
						},
						&discordgo.TextInput{
							CustomID: "field2",
							Value:    "value2",
						},
					},
				},
			},
			expected: map[string]string{
				"field1": "value1",
				"field2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModalFields(tt.components)

			if len(result) != len(tt.expected) {
				t.Errorf("extractModalFields() returned %d fields, want %d", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("extractModalFields() missing key %q", key)
				} else if actualValue != expectedValue {
					t.Errorf("extractModalFields()[%q] = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestCommandTitleOption(t *testing.T) {
	interaction := func(opts []*discordgo.ApplicationCommandInteractionDataOption) *discordgo.InteractionCreate {
		return &discordgo.InteractionCreate{
			Interaction: &discordgo.Interaction{
				Type: discordgo.InteractionApplicationCommand,
				Data: discordgo.ApplicationCommandInteractionData{
					Name:    "bug",
					Options: opts,
				},
			},
		}
	}
	opt := func(name, value string) *discordgo.ApplicationCommandInteractionDataOption {
		return &discordgo.ApplicationCommandInteractionDataOption{
			Name:  name,
			Type:  discordgo.ApplicationCommandOptionString,
			Value: value,
		}
	}

	tests := []struct {
		name     string
		options  []*discordgo.ApplicationCommandInteractionDataOption
		expected string
	}{
		{
			name:     "title option returned",
			options:  []*discordgo.ApplicationCommandInteractionDataOption{opt("title", "Map tiles fail to load")},
			expected: "Map tiles fail to load",
		},
		{
			name:     "surrounding whitespace trimmed",
			options:  []*discordgo.ApplicationCommandInteractionDataOption{opt("title", "  Padded title  ")},
			expected: "Padded title",
		},
		{
			name:     "title found among other options",
			options:  []*discordgo.ApplicationCommandInteractionDataOption{opt("other", "ignored"), opt("title", "Real title")},
			expected: "Real title",
		},
		{
			name:     "no options yields empty string",
			options:  nil,
			expected: "",
		},
		{
			name:     "missing title option yields empty string",
			options:  []*discordgo.ApplicationCommandInteractionDataOption{opt("other", "ignored")},
			expected: "",
		},
		{
			name:     "whitespace-only title yields empty string",
			options:  []*discordgo.ApplicationCommandInteractionDataOption{opt("title", "   ")},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandTitleOption(interaction(tt.options)); got != tt.expected {
				t.Errorf("commandTitleOption() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// The warning has to reach the reporter before the final dialog, not after the
// issue exists, so it belongs on the step whose Continue opens the last part.
func TestContinuePrompt(t *testing.T) {
	const warning = "The next part is the last one."

	tests := []struct {
		name        string
		currentPart int
		totalParts  int
		wantWarning bool
	}{
		{name: "first of three does not warn", currentPart: 1, totalParts: 3, wantWarning: false},
		{name: "second of three warns", currentPart: 2, totalParts: 3, wantWarning: true},
		{name: "first of two warns", currentPart: 1, totalParts: 2, wantWarning: true},
		{name: "first of four does not warn", currentPart: 1, totalParts: 4, wantWarning: false},
		{name: "third of four warns", currentPart: 3, totalParts: 4, wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := continuePrompt(tt.currentPart, tt.totalParts)

			if !strings.Contains(got, "Click 'Continue' to proceed.") {
				t.Errorf("continuePrompt(%d, %d) lost the continue instruction: %q",
					tt.currentPart, tt.totalParts, got)
			}
			if hasWarning := strings.Contains(got, warning); hasWarning != tt.wantWarning {
				t.Errorf("continuePrompt(%d, %d) warning = %v, want %v\ngot: %q",
					tt.currentPart, tt.totalParts, hasWarning, tt.wantWarning, got)
			}
			if tt.wantWarning && !strings.Contains(got, "public") {
				t.Errorf("continuePrompt(%d, %d) warns but does not say the issue is public: %q",
					tt.currentPart, tt.totalParts, got)
			}
		})
	}
}

// The notice must never be collected. It is not a template field, so storing it
// would add an empty section to the issue body and count as an answered field,
// changing which part comes next. Both the first dialog and every continuation
// go through this collector, which is what keeps the two paths in agreement.
func TestCollectSubmittedValuesSkipsNotice(t *testing.T) {
	row := func(customID, value string) discordgo.MessageComponent {
		return &discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				// discordgo delivers submitted inputs as pointers.
				&discordgo.TextInput{CustomID: customID, Value: value},
			},
		}
	}

	state := &ModalState{
		AllFields: []config.FieldConfig{
			{CustomID: "steps", Label: "Steps to reproduce"},
			{CustomID: "logs", Label: "Logs"},
		},
		SubmittedValues: make(map[string]string),
	}

	collectSubmittedValues(state, []discordgo.MessageComponent{
		row("steps", "1. flash\n2. wait"),
		row(config.NoticeFieldID, ""),
		row("logs", "none"),
	})

	if _, present := state.SubmittedValues[config.NoticeFieldID]; present {
		t.Error("the notice was collected under its custom ID")
	}
	for label := range state.SubmittedValues {
		if strings.Contains(strings.ToLower(label), "public github issue") {
			t.Errorf("the notice was collected under its label %q", label)
		}
	}

	if got := len(state.SubmittedValues); got != 2 {
		t.Errorf("collected %d values, want 2 — an extra entry shifts which part comes next: %v",
			got, state.SubmittedValues)
	}
	if got := state.SubmittedValues["Steps to reproduce"]; got != "1. flash\n2. wait" {
		t.Errorf("Steps to reproduce = %q, want the submitted value", got)
	}
	if got := state.SubmittedValues["Logs"]; got != "none" {
		t.Errorf("Logs = %q, want %q", got, "none")
	}
}

// Discord rejects a dialog title outside 1-45 characters with HTTP 400, which
// fails the whole dialog and strands what the reporter has already typed. This
// is the regression seen in production on 2026-09-16.
func TestDialogTitle(t *testing.T) {
	long := strings.Repeat("a", 60)

	tests := []struct {
		name     string
		state    *ModalState
		expected string
	}{
		{
			name:     "template name preferred over the reporter's title",
			state:    &ModalState{DisplayTitle: "Bug Report", Title: "Map tiles fail to load on Android"},
			expected: "Bug Report",
		},
		{
			name:     "falls back to the reporter's title when no header is set",
			state:    &ModalState{Title: "Short enough"},
			expected: "Short enough",
		},
		{
			name:     "an over-long fallback is clamped, not rejected",
			state:    &ModalState{Title: long},
			expected: strings.Repeat("a", 45),
		},
		{
			name:     "an over-long header is clamped too",
			state:    &ModalState{DisplayTitle: long},
			expected: strings.Repeat("a", 45),
		},
		{
			name:     "empty state still yields a usable title",
			state:    &ModalState{},
			expected: "Report",
		},
		{
			name:     "whitespace-only is treated as empty",
			state:    &ModalState{DisplayTitle: "   ", Title: "  "},
			expected: "Report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dialogTitle(tt.state)

			if got != tt.expected {
				t.Errorf("dialogTitle() = %q, want %q", got, tt.expected)
			}
			if n := len([]rune(got)); n < 1 || n > discordDialogTitleLimit {
				t.Errorf("dialogTitle() = %q, %d runes, outside Discord's 1-%d",
					got, n, discordDialogTitleLimit)
			}
		})
	}
}

// Whatever the reporter types, the header must stay within Discord's limit.
func TestDialogTitleNeverExceedsLimit(t *testing.T) {
	for _, n := range []int{0, 1, 44, 45, 46, 200, 4000} {
		state := &ModalState{Title: strings.Repeat("x", n)}
		got := dialogTitle(state)
		if r := len([]rune(got)); r < 1 || r > discordDialogTitleLimit {
			t.Errorf("title of %d chars produced a %d-rune header: %q", n, r, got)
		}
	}
}
