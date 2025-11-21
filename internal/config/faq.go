package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type FAQItem struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type FAQData struct {
	FAQ             []FAQItem `yaml:"faq"`
	SoftwareModules []FAQItem `yaml:"software_modules"`
}

var faqData *FAQData

// LoadFAQ loads FAQ data from the specified YAML file
func LoadFAQ(path string) (*FAQData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read FAQ file: %w", err)
	}

	var faq FAQData
	if err := yaml.Unmarshal(data, &faq); err != nil {
		return nil, fmt.Errorf("failed to parse FAQ YAML: %w", err)
	}

	faqData = &faq
	return &faq, nil
}

// GetFAQData returns the loaded FAQ data
func GetFAQData() *FAQData {
	return faqData
}

// GetAllFAQItems returns all FAQ items combined from both categories
func (f *FAQData) GetAllFAQItems() []FAQItem {
	all := make([]FAQItem, 0, len(f.FAQ)+len(f.SoftwareModules))
	all = append(all, f.FAQ...)
	all = append(all, f.SoftwareModules...)
	return all
}

// FindFAQItem searches for an FAQ item by name (case-insensitive)
func (f *FAQData) FindFAQItem(name string) (FAQItem, bool) {
	for _, item := range f.FAQ {
		if item.Name == name {
			return item, true
		}
	}
	for _, item := range f.SoftwareModules {
		if item.Name == name {
			return item, true
		}
	}
	return FAQItem{}, false
}
