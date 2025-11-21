package config

import (
	"os"
	"path/filepath"
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
			name:      "exact match required",
			searchFor: "getting started",
			wantFound: false,
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
