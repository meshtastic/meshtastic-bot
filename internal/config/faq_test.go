package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFAQData_GetAllFAQItems(t *testing.T) {
	tests := []struct {
		name     string
		faqData  *FAQData
		wantLen  int
		wantItem string
	}{
		{
			name: "combines both FAQ and software modules",
			faqData: &FAQData{
				FAQ: []FAQItem{
					{Name: "Getting Started", URL: "https://example.com/start"},
					{Name: "Installation", URL: "https://example.com/install"},
				},
				SoftwareModules: []FAQItem{
					{Name: "Module A", URL: "https://example.com/module-a"},
				},
			},
			wantLen:  3,
			wantItem: "Getting Started",
		},
		{
			name: "empty FAQ",
			faqData: &FAQData{
				FAQ:             []FAQItem{},
				SoftwareModules: []FAQItem{},
			},
			wantLen: 0,
		},
		{
			name: "only FAQ items",
			faqData: &FAQData{
				FAQ: []FAQItem{
					{Name: "Item 1", URL: "url1"},
					{Name: "Item 2", URL: "url2"},
				},
				SoftwareModules: []FAQItem{},
			},
			wantLen: 2,
		},
		{
			name: "only software modules",
			faqData: &FAQData{
				FAQ: []FAQItem{},
				SoftwareModules: []FAQItem{
					{Name: "Module 1", URL: "url1"},
				},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.faqData.GetAllFAQItems()

			if len(result) != tt.wantLen {
				t.Errorf("GetAllFAQItems() returned %d items, want %d", len(result), tt.wantLen)
			}

			if tt.wantItem != "" {
				found := false
				for _, item := range result {
					if item.Name == tt.wantItem {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("GetAllFAQItems() missing expected item %q", tt.wantItem)
				}
			}
		})
	}
}

func TestFAQData_FindFAQItem(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Getting Started", URL: "https://example.com/start"},
			{Name: "Installation", URL: "https://example.com/install"},
		},
		SoftwareModules: []FAQItem{
			{Name: "Arduino", URL: "https://example.com/arduino"},
			{Name: "Python SDK", URL: "https://example.com/python"},
		},
	}

	tests := []struct {
		name      string
		searchFor string
		wantFound bool
		wantURL   string
	}{
		{
			name:      "find in FAQ section",
			searchFor: "Getting Started",
			wantFound: true,
			wantURL:   "https://example.com/start",
		},
		{
			name:      "find in software modules section",
			searchFor: "Arduino",
			wantFound: true,
			wantURL:   "https://example.com/arduino",
		},
		{
			name:      "not found",
			searchFor: "Nonexistent",
			wantFound: false,
		},
		{
			name:      "case-insensitive match",
			searchFor: "getting started",
			wantFound: true,
			wantURL:   "https://example.com/start",
		},
		{
			name:      "surrounding whitespace ignored",
			searchFor: "  python sdk ",
			wantFound: true,
			wantURL:   "https://example.com/python",
		},
		{
			name:      "empty search",
			searchFor: "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := faqData.FindFAQItem(tt.searchFor)

			if found != tt.wantFound {
				t.Errorf("FindFAQItem(%q) found = %v, want %v", tt.searchFor, found, tt.wantFound)
			}

			if tt.wantFound && result.URL != tt.wantURL {
				t.Errorf("FindFAQItem(%q).URL = %q, want %q", tt.searchFor, result.URL, tt.wantURL)
			}
		})
	}
}

func TestLoadFAQ(t *testing.T) {
	// Create a temporary FAQ file for testing
	tmpDir := t.TempDir()

	validYAML := `faq:
  - name: Test Item
    url: https://example.com/test
software_modules:
  - name: Test Module
    url: https://example.com/module
`

	invalidYAML := `this is not valid yaml: {{{`

	tests := []struct {
		name        string
		fileContent string
		setupFile   bool
		wantErr     bool
		wantFAQLen  int
		wantModLen  int
	}{
		{
			name:        "valid FAQ file",
			fileContent: validYAML,
			setupFile:   true,
			wantErr:     false,
			wantFAQLen:  1,
			wantModLen:  1,
		},
		{
			name:        "invalid YAML",
			fileContent: invalidYAML,
			setupFile:   true,
			wantErr:     true,
		},
		{
			name:      "file does not exist",
			setupFile: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tt.name+".yaml")

			if tt.setupFile {
				if err := os.WriteFile(testFile, []byte(tt.fileContent), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			result, err := LoadFAQ(testFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadFAQ() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadFAQ() unexpected error: %v", err)
				return
			}

			if len(result.FAQ) != tt.wantFAQLen {
				t.Errorf("LoadFAQ() FAQ length = %d, want %d", len(result.FAQ), tt.wantFAQLen)
			}

			if len(result.SoftwareModules) != tt.wantModLen {
				t.Errorf("LoadFAQ() SoftwareModules length = %d, want %d", len(result.SoftwareModules), tt.wantModLen)
			}

			// Verify global faqData was set
			if GetFAQData() == nil {
				t.Error("LoadFAQ() did not set global faqData")
			}
		})
	}
}

func TestFAQData_FindFAQItem_Aliases(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Default Pairing", URL: "https://example.com/pairing", Aliases: []string{"pairing pin", "bluetooth pin"}},
		},
		SoftwareModules: []FAQItem{
			{Name: "Rangetest", URL: "https://example.com/range", Aliases: []string{"range test"}},
		},
	}

	tests := []struct {
		name      string
		searchFor string
		wantURL   string
		wantFound bool
	}{
		{name: "alias in FAQ section", searchFor: "bluetooth pin", wantURL: "https://example.com/pairing", wantFound: true},
		{name: "alias in modules section", searchFor: "Range Test", wantURL: "https://example.com/range", wantFound: true},
		{name: "unknown alias", searchFor: "pin code", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, found := faqData.FindFAQItem(tt.searchFor)
			if found != tt.wantFound {
				t.Fatalf("FindFAQItem(%q) found = %v, want %v", tt.searchFor, found, tt.wantFound)
			}
			if tt.wantFound && item.URL != tt.wantURL {
				t.Errorf("FindFAQItem(%q).URL = %q, want %q", tt.searchFor, item.URL, tt.wantURL)
			}
		})
	}
}

func TestFAQData_Search(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Default Pairing", URL: "u1", Aliases: []string{"pairing pin", "bluetooth pin"}},
			{Name: "App Units & Locale", URL: "u2", Aliases: []string{"fahrenheit", "celsius"}},
			{Name: "Device Display Units", URL: "u3", Aliases: []string{"fahrenheit on screen"}},
		},
		SoftwareModules: []FAQItem{
			{Name: "Rangetest", URL: "u4", Aliases: []string{"range test", "coverage test"}},
			{Name: "Telemetry", URL: "u5"},
		},
	}

	tests := []struct {
		name     string
		query    string
		limit    int
		wantTop  string
		wantLen  int
		wantNone bool
	}{
		{name: "spacing difference", query: "range test", limit: 5, wantTop: "Rangetest"},
		{name: "alias match", query: "pairing pin", limit: 5, wantTop: "Default Pairing"},
		{name: "alias shared between topics", query: "fahrenheit", limit: 5, wantTop: "App Units & Locale", wantLen: 2},
		{name: "typo in name", query: "telemetery", limit: 5, wantTop: "Telemetry"},
		{name: "no match", query: "quantum tunnelling", limit: 5, wantNone: true},
		{name: "limit respected", query: "test", limit: 1, wantLen: 1},
		{name: "zero limit", query: "range test", limit: 0, wantNone: true},
		{name: "empty query lists file order", query: "", limit: 3, wantTop: "Default Pairing", wantLen: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := faqData.Search(tt.query, tt.limit)

			if tt.wantNone {
				if len(matches) != 0 {
					t.Fatalf("Search(%q) returned %d matches, want none", tt.query, len(matches))
				}
				return
			}

			if len(matches) == 0 {
				t.Fatalf("Search(%q) returned no matches", tt.query)
			}
			if tt.wantLen != 0 && len(matches) != tt.wantLen {
				t.Errorf("Search(%q) returned %d matches, want %d", tt.query, len(matches), tt.wantLen)
			}
			if tt.wantTop != "" && matches[0].Item.Name != tt.wantTop {
				t.Errorf("Search(%q) top match = %q, want %q", tt.query, matches[0].Item.Name, tt.wantTop)
			}

			for idx := 1; idx < len(matches); idx++ {
				if matches[idx-1].Score < matches[idx].Score {
					t.Errorf("Search(%q) is not ordered by score: %v before %v",
						tt.query, matches[idx-1].Score, matches[idx].Score)
				}
			}
		})
	}
}

func TestFAQData_SearchNoDuplicateTopics(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Rangetest", URL: "u1", Aliases: []string{"range test", "rangetest", "test range"}},
		},
	}

	matches := faqData.Search("range test", 10)
	if len(matches) != 1 {
		t.Fatalf("Search returned %d matches for one topic, want 1", len(matches))
	}
}

func TestFAQData_SearchNameBeatsAlias(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Serial", URL: "u1"},
			{Name: "MQTT", URL: "u2", Aliases: []string{"serial"}},
		},
	}

	matches := faqData.Search("serial", 5)
	if len(matches) == 0 || matches[0].Item.Name != "Serial" {
		t.Fatalf("Search(\"serial\") top match = %+v, want the topic named Serial", matches)
	}
}

// TestShippedFAQ checks the file the bot loads, not a fixture.
func TestShippedFAQ(t *testing.T) {
	data, err := LoadFAQ(filepath.Join("..", "..", "faq.yaml"))
	if err != nil {
		t.Fatalf("LoadFAQ(faq.yaml) failed: %v", err)
	}

	queries := map[string]string{
		"range test":              "Rangetest",
		"pairing pin":             "Default Pairing",
		"store and forward":       "Store & Forward",
		"how far":                 "Rangetest",
		"bricked":                 "Enter DFU Mode",
		"what is the pairing pin": "Default Pairing",
		"how far can it go":       "Rangetest",
		"serail":                  "Serial",
	}
	for query, want := range queries {
		matches := data.Search(query, 4)
		if len(matches) == 0 {
			t.Errorf("Search(%q) found nothing, want %q", query, want)
			continue
		}
		if matches[0].Item.Name != want {
			t.Errorf("Search(%q) top match = %q, want %q", query, matches[0].Item.Name, want)
		}
	}

	// "fahrenheit" fits both units topics, so it must offer both.
	unitTopics := map[string]bool{"App Units & Locale": true, "Device Display Units": true}
	matches := data.Search("fahrenheit", 4)
	if len(matches) < 2 {
		t.Errorf("Search(\"fahrenheit\") returned %d matches, want both units topics", len(matches))
	}
	for _, match := range matches[:min(2, len(matches))] {
		if !unitTopics[match.Item.Name] {
			t.Errorf("Search(\"fahrenheit\") offered %q, want only the units topics first", match.Item.Name)
		}
	}

	seen := make(map[string]string)
	for _, item := range data.GetAllFAQItems() {
		if item.Name == "" || item.URL == "" {
			t.Errorf("entry %+v is missing a name or URL", item)
		}
		if item.Answer == "" {
			t.Errorf("entry %q has no answer", item.Name)
			continue
		}
		if lines := strings.Split(item.Answer, "\n"); len(lines) > 2 {
			t.Errorf("entry %q has a %d-line answer, want at most 2", item.Name, len(lines))
		}

		for _, key := range append([]string{item.Name}, item.Aliases...) {
			key = strings.ToLower(strings.TrimSpace(key))
			if owner, dup := seen[key]; dup {
				t.Errorf("%q is used by both %q and %q", key, owner, item.Name)
			}
			seen[key] = item.Name
		}
	}
}

// A blank query browses, which is why the command handler refuses it.
func TestFAQData_SearchBlankQueryBrowses(t *testing.T) {
	faqData := &FAQData{FAQ: []FAQItem{{Name: "Tips", URL: "u1"}, {Name: "Role", URL: "u2"}}}

	for _, query := range []string{"", " ", "\t"} {
		matches := faqData.Search(query, 4)
		if len(matches) != 2 {
			t.Fatalf("Search(%q) returned %d matches, want the topics in file order", query, len(matches))
		}
		if matches[0].Item.Name != "Tips" || matches[0].Score != 0 {
			t.Errorf("Search(%q) first match = %+v, want Tips scored 0", query, matches[0])
		}
	}
}

// A word only in an answer finds the topic, ranked below name and alias matches.
func TestFAQData_SearchFallsBackToAnswerText(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Translate Android", URL: "u1", Answer: "Translated on Crowdin, not by pull request."},
			{Name: "Crowdin Setup", URL: "u2", Answer: "Unrelated."},
		},
	}

	matches := faqData.Search("crowdin", 4)
	if len(matches) != 2 {
		t.Fatalf("Search(\"crowdin\") returned %d matches, want 2", len(matches))
	}
	if matches[0].Item.Name != "Crowdin Setup" {
		t.Errorf("top match = %q, want the topic named for it", matches[0].Item.Name)
	}
	if matches[1].Item.Name != "Translate Android" || matches[1].Score != 35 {
		t.Errorf("body match = %+v, want Translate Android scored 35", matches[1])
	}
}

// A word in no name, alias or answer still finds nothing.
func TestFAQData_SearchAnswerFallbackIsNotAWildcard(t *testing.T) {
	faqData := &FAQData{FAQ: []FAQItem{{Name: "Serial", URL: "u1", Answer: "Passes data over the serial pins."}}}

	if matches := faqData.Search("paxcounter", 4); len(matches) != 0 {
		t.Errorf("Search(\"paxcounter\") returned %+v, want nothing", matches)
	}
}

func TestFAQData_SearchBlankQueryHonoursLimit(t *testing.T) {
	faqData := &FAQData{
		FAQ:             []FAQItem{{Name: "Tips", URL: "u1"}, {Name: "Role", URL: "u2"}},
		SoftwareModules: []FAQItem{{Name: "Serial", URL: "u3"}},
	}

	matches := faqData.Search("", 2)
	if len(matches) != 2 {
		t.Fatalf("Search(\"\", 2) returned %d matches, want 2", len(matches))
	}
	if matches[0].Item.Name != "Tips" || matches[1].Item.Name != "Role" {
		t.Errorf("browse order = %q, %q, want file order", matches[0].Item.Name, matches[1].Item.Name)
	}
}

func TestFAQData_SearchTiesBreakByName(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Zebra", URL: "u1", Answer: "Mentions crowdin once."},
			{Name: "Alpha", URL: "u2", Answer: "Mentions crowdin too."},
		},
	}

	matches := faqData.Search("crowdin", 4)
	if len(matches) != 2 {
		t.Fatalf("Search returned %d matches, want 2", len(matches))
	}
	if matches[0].Score != matches[1].Score {
		t.Fatalf("scores %v and %v differ, so this does not test the tie", matches[0].Score, matches[1].Score)
	}
	if matches[0].Item.Name != "Alpha" {
		t.Errorf("first match = %q, want the alphabetically first topic", matches[0].Item.Name)
	}
}

func TestFAQData_SearchTruncatesRankedResults(t *testing.T) {
	faqData := &FAQData{
		FAQ: []FAQItem{
			{Name: "Serial", URL: "u1"},
			{Name: "Serial Module", URL: "u2"},
			{Name: "Serial Port", URL: "u3"},
		},
	}

	matches := faqData.Search("serial", 2)
	if len(matches) != 2 {
		t.Fatalf("Search(\"serial\", 2) returned %d matches, want 2", len(matches))
	}
	if matches[0].Item.Name != "Serial" {
		t.Errorf("first match = %q, want the closest name", matches[0].Item.Name)
	}
}
