package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/meshtastic/meshtastic-bot/internal/fuzzy"

	"gopkg.in/yaml.v3"
)

type FAQItem struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	// Answer is the short reply shown above the link.
	Answer string `yaml:"answer"`
	// Aliases are other words for this topic, matched like the name.
	Aliases []string `yaml:"aliases"`
}

type FAQData struct {
	FAQ             []FAQItem `yaml:"faq"`
	SoftwareModules []FAQItem `yaml:"software_modules"`
}

// FAQMatch is one search result and how well it matched.
type FAQMatch struct {
	Item  FAQItem
	Score float64
}

// aliasPenalty puts a name match ahead of an equally good alias match.
const aliasPenalty = 2

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

// FindFAQItem looks up an item by exact name or alias (case-insensitive).
// Free text goes through Search instead.
func (f *FAQData) FindFAQItem(name string) (FAQItem, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FAQItem{}, false
	}
	for _, item := range f.GetAllFAQItems() {
		if strings.EqualFold(item.Name, name) {
			return item, true
		}
		for _, alias := range item.Aliases {
			if strings.EqualFold(alias, name) {
				return item, true
			}
		}
	}
	return FAQItem{}, false
}

// Search returns the topics matching query, best first, at most limit of them.
// An empty query browses: the first topics in file order.
func (f *FAQData) Search(query string, limit int) []FAQMatch {
	if limit <= 0 {
		return nil
	}

	all := f.GetAllFAQItems()

	if strings.TrimSpace(query) == "" {
		if len(all) > limit {
			all = all[:limit]
		}
		matches := make([]FAQMatch, 0, len(all))
		for _, item := range all {
			matches = append(matches, FAQMatch{Item: item})
		}
		return matches
	}

	matches := make([]FAQMatch, 0, len(all))
	for _, item := range all {
		if score := scoreItem(query, item); score >= fuzzy.Floor {
			matches = append(matches, FAQMatch{Item: item, Score: score})
		}
	}

	// Equal scores break by name, for a stable list.
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].Score != matches[b].Score {
			return matches[a].Score > matches[b].Score
		}
		return matches[a].Item.Name < matches[b].Item.Name
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// scoreItem returns the best score over a topic's name and aliases, falling
// back to the answer text when neither matches.
func scoreItem(query string, item FAQItem) float64 {
	best := fuzzy.Score(query, item.Name)
	for _, alias := range item.Aliases {
		// The floor applies to the match; the penalty only ranks it.
		score := fuzzy.Score(query, alias)
		if score >= fuzzy.Floor && score-aliasPenalty > best {
			best = score - aliasPenalty
		}
	}
	if best < fuzzy.Floor {
		if score := fuzzy.ContentScore(query, item.Answer); score > best {
			best = score
		}
	}
	return best
}
