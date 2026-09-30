package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/meshtastic/meshtastic-bot/internal/config"
)

func TestFaqReply(t *testing.T) {
	exact := config.FAQMatch{
		Item:  config.FAQItem{Name: "Rangetest", URL: "https://example.com/range", Answer: "Line one.\nLine two."},
		Score: 100,
	}
	weak := config.FAQMatch{
		Item:  config.FAQItem{Name: "Telemetry", URL: "https://example.com/telemetry", Answer: "Reports metrics."},
		Score: 45,
	}
	other := config.FAQMatch{
		Item:  config.FAQItem{Name: "MQTT", URL: "https://example.com/mqtt", Answer: "Bridges to a broker."},
		Score: 40,
	}

	t.Run("answers above the link", func(t *testing.T) {
		got := faqReply([]config.FAQMatch{exact})
		want := "**Rangetest**\nLine one.\nLine two.\nhttps://example.com/range"
		if got != want {
			t.Errorf("faqReply() = %q, want %q", got, want)
		}
	})

	t.Run("an exact match offers no alternatives", func(t *testing.T) {
		got := faqReply([]config.FAQMatch{exact, weak})
		if strings.Contains(got, "You might also want") {
			t.Errorf("faqReply() offered alternatives for an exact match: %q", got)
		}
	})

	t.Run("a weak match offers alternatives", func(t *testing.T) {
		got := faqReply([]config.FAQMatch{weak, other, exact})
		if !strings.Contains(got, "You might also want") {
			t.Fatalf("faqReply() offered no alternatives for a weak match: %q", got)
		}
		if !strings.Contains(got, "<https://example.com/mqtt>") {
			t.Errorf("faqReply() did not suppress the preview on an alternative: %q", got)
		}
		if strings.Index(got, "Telemetry") > strings.Index(got, "MQTT") {
			t.Errorf("faqReply() did not put the best match first: %q", got)
		}
	})

	t.Run("a far-behind match is not offered", func(t *testing.T) {
		near := config.FAQMatch{Item: config.FAQItem{Name: "Serial", URL: "u1", Answer: "Wired data."}, Score: 60}
		far := config.FAQMatch{Item: config.FAQItem{Name: "Telemetry", URL: "u2", Answer: "Metrics."}, Score: 35}

		got := faqReply([]config.FAQMatch{near, far})
		if strings.Contains(got, "You might also want") {
			t.Errorf("faqReply() offered a match %v points behind: %q", near.Score-far.Score, got)
		}
	})

	t.Run("an entry with no answer still replies", func(t *testing.T) {
		bare := config.FAQMatch{Item: config.FAQItem{Name: "Serial", URL: "https://example.com/serial"}, Score: 100}
		got := faqReply([]config.FAQMatch{bare})
		if got != "**Serial**\nhttps://example.com/serial" {
			t.Errorf("faqReply() = %q", got)
		}
	})
}

// Discord rejects a body over 2000 characters, which fails the command outright.
func TestFaqReplyWithinDiscordLimit(t *testing.T) {
	data, err := config.LoadFAQ("../../../faq.yaml")
	if err != nil {
		t.Fatalf("LoadFAQ failed: %v", err)
	}

	// A name scores 100 and skips the alternates block, so build that case.
	all := data.GetAllFAQItems()
	sort.Slice(all, func(a, b int) bool {
		return len(all[a].Name)+len(all[a].Answer)+len(all[a].URL) >
			len(all[b].Name)+len(all[b].Answer)+len(all[b].URL)
	})

	worst := make([]config.FAQMatch, 0, faqAlternates+1)
	for _, item := range all[:faqAlternates+1] {
		worst = append(worst, config.FAQMatch{Item: item, Score: 60})
	}
	if n := len(faqReply(worst)); n > 2000 {
		t.Errorf("worst-case reply is %d characters, over Discord's limit of 2000", n)
	}
	if !strings.Contains(faqReply(worst), "You might also want:") {
		t.Error("worst-case reply did not include the alternates block")
	}

	for _, item := range all {
		matches := data.Search(item.Name, faqAlternates+1)
		if len(matches) == 0 {
			t.Fatalf("Search(%q) found nothing", item.Name)
		}
		if n := len(faqReply(matches)); n > 2000 {
			t.Errorf("reply for %q is %d characters, over Discord's limit of 2000", item.Name, n)
		}
	}
}

func TestFAQUsageSummary(t *testing.T) {
	resetFaqUsage(t)

	if got := FAQUsageSummary(); got != "" {
		t.Errorf("FAQUsageSummary() with no use = %q, want empty", got)
	}

	recordFaqHit("Telemetry")
	recordFaqHit("Rangetest")
	recordFaqHit("Rangetest")
	recordFaqMiss()

	got := FAQUsageSummary()
	want := "FAQ usage: Rangetest=2 Telemetry=1 unmatched=1"
	if got != want {
		t.Errorf("FAQUsageSummary() = %q, want %q", got, want)
	}
}

func TestFAQUsageSummaryTiesBreakByName(t *testing.T) {
	resetFaqUsage(t)

	recordFaqHit("Telemetry")
	recordFaqHit("Audio")

	if got := FAQUsageSummary(); got != "FAQ usage: Audio=1 Telemetry=1 unmatched=0" {
		t.Errorf("FAQUsageSummary() = %q, want equal counts ordered by name", got)
	}
}

// The counts must carry topic names and nothing about who asked.
func TestFAQUsageKeepsNoIdentifiers(t *testing.T) {
	resetFaqUsage(t)

	recordFaqHit("Telemetry")
	recordFaqMiss()

	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()
	if len(faqUsage.hits) != 1 {
		t.Errorf("hits hold %d keys, want 1 topic name", len(faqUsage.hits))
	}
	if _, ok := faqUsage.hits["Telemetry"]; !ok {
		t.Errorf("hits are not keyed by topic name: %v", faqUsage.hits)
	}
}

func TestFAQUsageConcurrent(t *testing.T) {
	resetFaqUsage(t)

	done := make(chan struct{})
	for n := 0; n < 50; n++ {
		go func() {
			recordFaqHit("Telemetry")
			recordFaqMiss()
			FAQUsageSummary()
			done <- struct{}{}
		}()
	}
	for n := 0; n < 50; n++ {
		<-done
	}

	if got := FAQUsageSummary(); got != "FAQ usage: Telemetry=50 unmatched=50" {
		t.Errorf("FAQUsageSummary() = %q", got)
	}
}

func resetFaqUsage(t *testing.T) {
	t.Helper()
	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()
	faqUsage.hits = make(map[string]int)
	faqUsage.misses = 0
}

// Browse order must promote a much-asked topic past Discord's 25-item cut.
func TestFaqBrowseOrderFollowsUsage(t *testing.T) {
	resetFaqUsage(t)

	data, err := config.LoadFAQ("../../../faq.yaml")
	if err != nil {
		t.Fatalf("LoadFAQ failed: %v", err)
	}

	all := data.GetAllFAQItems()
	if len(all) <= autocompleteLimit {
		t.Skipf("faq.yaml has %d topics, too few to test the cut", len(all))
	}
	last := all[len(all)-1].Name

	recordFaqHit(last)
	recordFaqHit(last)

	items := browseOrder(all)

	if len(items) != autocompleteLimit {
		t.Errorf("browse list holds %d topics, want Discord's limit of %d", len(items), autocompleteLimit)
	}
	if items[0].Name != last {
		t.Errorf("browse order starts with %q, want the most looked-up topic %q", items[0].Name, last)
	}
	if items[1].Name != all[0].Name {
		t.Errorf("second topic = %q, want file order to decide ties (%q)", items[1].Name, all[0].Name)
	}
}

// captureResponse returns a session that decodes its response into the pointer.
func captureResponse(t *testing.T) (*discordgo.Session, *discordgo.InteractionResponse) {
	t.Helper()

	captured := &discordgo.InteractionResponse{}
	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(req.Body).Decode(captured); err != nil {
					t.Errorf("failed to decode response body: %v", err)
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}
	return s, captured
}

func faqInteraction(kind discordgo.InteractionType, values ...string) *discordgo.InteractionCreate {
	options := make([]*discordgo.ApplicationCommandInteractionDataOption, 0, len(values))
	for _, value := range values {
		options = append(options, &discordgo.ApplicationCommandInteractionDataOption{
			Name:    "topic",
			Type:    discordgo.ApplicationCommandOptionString,
			Value:   value,
			Focused: true,
		})
	}

	return &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: kind,
			Data: discordgo.ApplicationCommandInteractionData{Name: "faq", Options: options},
		},
	}
}

func TestHandleFaq(t *testing.T) {
	if _, err := config.LoadFAQ("../../../faq.yaml"); err != nil {
		t.Fatalf("LoadFAQ failed: %v", err)
	}

	tests := []struct {
		name          string
		values        []string
		wantContains  []string
		wantAbsent    []string
		wantEphemeral bool
		wantHit       string
		wantMiss      int
	}{
		{
			name:          "no option given",
			values:        nil,
			wantContains:  []string{"autocomplete"},
			wantEphemeral: true,
		},
		{
			name:          "blank option",
			values:        []string{"   "},
			wantContains:  []string{"autocomplete"},
			wantAbsent:    []string{"Tips"},
			wantEphemeral: true,
		},
		{
			name:          "no match",
			values:        []string{"quantum tunnelling"},
			wantContains:  []string{"No FAQ topic matches", "quantum tunnelling"},
			wantEphemeral: true,
			wantMiss:      1,
		},
		{
			name:         "exact match answers alone",
			values:       []string{"Rangetest"},
			wantContains: []string{"**Rangetest**", "Both ends need the module", "module/range-test/"},
			wantAbsent:   []string{"You might also want"},
			wantHit:      "Rangetest",
		},
		{
			name:         "typed wording still answers",
			values:       []string{"range test"},
			wantContains: []string{"**Rangetest**"},
			wantHit:      "Rangetest",
		},
		{
			name:         "ambiguous query offers the alternative",
			values:       []string{"fahrenheit"},
			wantContains: []string{"**Device Display Units**", "You might also want", "<https://meshtastic.org/docs/software/android/user/units-and-locale/>"},
			wantHit:      "Device Display Units",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFaqUsage(t)
			s, captured := captureResponse(t)

			handleFaq(s, faqInteraction(discordgo.InteractionApplicationCommand, tt.values...))

			if captured.Type != discordgo.InteractionResponseChannelMessageWithSource {
				t.Errorf("response type = %v, want a channel message", captured.Type)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(captured.Data.Content, want) {
					t.Errorf("response %q does not contain %q", captured.Data.Content, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(captured.Data.Content, absent) {
					t.Errorf("response %q should not contain %q", captured.Data.Content, absent)
				}
			}

			ephemeral := captured.Data.Flags&discordgo.MessageFlagsEphemeral != 0
			if ephemeral != tt.wantEphemeral {
				t.Errorf("ephemeral = %v, want %v", ephemeral, tt.wantEphemeral)
			}
			if !ephemeral && captured.Data.AllowedMentions == nil {
				t.Error("a public answer must suppress mentions")
			}

			if tt.wantHit != "" && faqUsageCount(tt.wantHit) != 1 {
				t.Errorf("%q counted %d times, want 1", tt.wantHit, faqUsageCount(tt.wantHit))
			}
			if tt.wantHit == "" {
				for _, item := range config.GetFAQData().GetAllFAQItems() {
					if faqUsageCount(item.Name) != 0 {
						t.Errorf("%q was counted for a query that answered nothing", item.Name)
					}
				}
			}

			faqUsage.mu.Lock()
			misses := faqUsage.misses
			faqUsage.mu.Unlock()
			if misses != tt.wantMiss {
				t.Errorf("misses = %d, want %d", misses, tt.wantMiss)
			}
		})
	}
}

func TestHandleFaqAutocomplete(t *testing.T) {
	if _, err := config.LoadFAQ("../../../faq.yaml"); err != nil {
		t.Fatalf("LoadFAQ failed: %v", err)
	}
	all := config.GetFAQData().GetAllFAQItems()

	t.Run("typed input is ranked", func(t *testing.T) {
		resetFaqUsage(t)
		s, captured := captureResponse(t)

		handleFaqAutocomplete(s, faqInteraction(discordgo.InteractionApplicationCommandAutocomplete, "range test"))

		if captured.Type != discordgo.InteractionApplicationCommandAutocompleteResult {
			t.Fatalf("response type = %v, want an autocomplete result", captured.Type)
		}
		if len(captured.Data.Choices) == 0 || captured.Data.Choices[0].Name != "Rangetest" {
			t.Errorf("choices = %+v, want Rangetest first", captured.Data.Choices)
		}
	})

	t.Run("blank input browses by usage", func(t *testing.T) {
		resetFaqUsage(t)
		recordFaqHit(all[len(all)-1].Name)
		s, captured := captureResponse(t)

		handleFaqAutocomplete(s, faqInteraction(discordgo.InteractionApplicationCommandAutocomplete, ""))

		if len(captured.Data.Choices) != autocompleteLimit {
			t.Fatalf("choices = %d, want Discord's limit of %d", len(captured.Data.Choices), autocompleteLimit)
		}
		if captured.Data.Choices[0].Name != all[len(all)-1].Name {
			t.Errorf("first choice = %q, want the most looked-up topic %q", captured.Data.Choices[0].Name, all[len(all)-1].Name)
		}
	})

	t.Run("no match yields no choices", func(t *testing.T) {
		s, captured := captureResponse(t)

		handleFaqAutocomplete(s, faqInteraction(discordgo.InteractionApplicationCommandAutocomplete, "quantum tunnelling"))

		if len(captured.Data.Choices) != 0 {
			t.Errorf("choices = %+v, want none", captured.Data.Choices)
		}
	})
}
