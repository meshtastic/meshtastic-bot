package config

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

// The rules below are Discord's, from developers/components/reference: Label
// (type 18) wraps one input with a label of at most 45 characters and a
// description of at most 100; a Text Input inside a Label carries no label of
// its own; "disabled" in a modal is an error; min_values must be at least 1
// unless required is false; File Upload takes 0-10 files.

func testFields() []FieldConfig {
	return []FieldConfig{
		{CustomID: "hardware", Label: "Hardware", Description: "What hardware are you encountering this issue on?",
			Kind: FieldSelect, Options: []string{"T-Beam", "T-Deck", "Heltec V3"}, Multiple: true, Required: true},
		{CustomID: "importance", Label: "Importance", Kind: FieldSelect, Options: []string{"Nice to have", "Critical"}},
		{CustomID: "steps", Label: "Steps to Reproduce", Style: "paragraph", Required: true, MinLength: 1, MaxLength: 4000},
		UploadField(),
	}
}

func dialogJSON(t *testing.T, components []discordgo.MessageComponent) []map[string]any {
	t.Helper()
	raw, err := json.Marshal(discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{CustomID: "modal_continue_bug_1_2", Title: "Bug Report", Components: components},
	})
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Data struct {
			Components []map[string]any `json:"components"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Data.Components
}

func num(v any) int { f, _ := v.(float64); return int(f) }

func TestDialogComponentsFollowDiscordsModalRules(t *testing.T) {
	top := dialogJSON(t, DialogComponents(testFields(), true, false))
	if len(top) > DialogComponentLimit {
		t.Fatalf("%d top-level components, Discord allows %d", len(top), DialogComponentLimit)
	}

	kinds := map[int]int{}
	for _, c := range top {
		switch num(c["type"]) {
		case 10:
			if c["content"] == "" || c["content"] == nil {
				t.Error("text display without content")
			}
			kinds[10]++
		case 18:
			if l, _ := c["label"].(string); l == "" || utf8.RuneCountInString(l) > 45 {
				t.Errorf("label %q is empty or over 45 characters", l)
			}
			if d, _ := c["description"].(string); utf8.RuneCountInString(d) > 100 {
				t.Errorf("description over 100 characters: %q", d)
			}
			child, ok := c["component"].(map[string]any)
			if !ok {
				t.Fatalf("label without a component: %v", c)
			}
			if _, has := child["disabled"]; has {
				t.Errorf("a modal component carries disabled, which Discord rejects: %v", child)
			}
			if min, has := child["min_values"]; has && num(min) < 1 {
				if req, ok := child["required"].(bool); !ok || req {
					t.Errorf("min_values below 1 needs required false: %v", child)
				}
			}
			switch num(child["type"]) {
			case 3:
				if _, has := child["required"]; !has {
					t.Errorf("select must say whether it is required: %v", child)
				}
				for _, o := range child["options"].([]any) {
					if len(o.(map[string]any)) != 2 {
						t.Errorf("select option carries more than label and value: %v", o)
					}
				}
			case 4:
				if _, has := child["label"]; has {
					t.Errorf("a text input inside a Label must not carry its own label: %v", child)
				}
				if _, has := child["required"]; !has {
					t.Errorf("text input must say whether it is required; Discord defaults it to true: %v", child)
				}
			case 19:
				if num(child["max_values"]) > 10 || child["required"] != false || num(child["min_values"]) != 0 {
					t.Errorf("upload must be optional and take at most 10 files: %v", child)
				}
			default:
				t.Errorf("unexpected component inside a label: %v", child)
			}
			kinds[num(child["type"])]++
		default:
			t.Errorf("unexpected top-level component %v", c)
		}
	}
	if kinds[3] != 2 || kinds[4] != 1 || kinds[19] != 1 || kinds[10] != 1 {
		t.Errorf("component mix %v, want 2 selects, 1 text input, 1 upload, 1 notice", kinds)
	}
}

func TestDialogSelectsMatchTheirField(t *testing.T) {
	top := dialogJSON(t, DialogComponents(testFields()[:2], false, false))
	multi := top[0]["component"].(map[string]any)
	if num(multi["min_values"]) != 1 || num(multi["max_values"]) != 3 || multi["required"] != true {
		t.Errorf("required multiple select: %v", multi)
	}
	single := top[1]["component"].(map[string]any)
	if num(single["min_values"]) != 0 || num(single["max_values"]) != 1 || single["required"] != false {
		t.Errorf("optional single select: %v", single)
	}
}

func TestLegacyDialogIsTextInputsOnly(t *testing.T) {
	top := dialogJSON(t, DialogComponents(testFields(), true, true))
	// The upload cannot be a text input, so it is left out.
	if len(top) != 4 {
		t.Fatalf("legacy dialog has %d components, want 3 fields and the notice", len(top))
	}
	for _, c := range top {
		if num(c["type"]) != 1 {
			t.Fatalf("legacy component is not an action row: %v", c)
		}
		input := c["components"].([]any)[0].(map[string]any)
		if num(input["type"]) != 4 || input["label"] == "" || input["label"] == nil {
			t.Errorf("legacy input must be a labelled text input: %v", input)
		}
		if input["custom_id"] == NoticeFieldID && input["required"] != false {
			t.Errorf("legacy notice must be sent as not required: %v", input)
		}
	}
}

func TestLegacyDialogOfOnlyAnUploadStillHasAComponent(t *testing.T) {
	if got := DialogComponents([]FieldConfig{UploadField()}, false, true); len(got) != 1 {
		t.Errorf("an empty dialog is rejected by Discord; got %d components", len(got))
	}
}
