package handlers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// faqUsage counts which topics get looked up. Topic names and a miss tally
// only: no user, channel or typed text, and nothing persisted.
var faqUsage = struct {
	mu     sync.Mutex
	hits   map[string]int
	misses int
}{hits: make(map[string]int)}

func recordFaqHit(name string) {
	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()
	faqUsage.hits[name]++
}

func recordFaqMiss() {
	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()
	faqUsage.misses++
}

// faqUsageCount is how often a topic has been looked up since the last restart.
func faqUsageCount(name string) int {
	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()
	return faqUsage.hits[name]
}

// FAQUsageSummary reports the counts so far, most looked up first.
func FAQUsageSummary() string {
	faqUsage.mu.Lock()
	defer faqUsage.mu.Unlock()

	if len(faqUsage.hits) == 0 && faqUsage.misses == 0 {
		return ""
	}

	names := make([]string, 0, len(faqUsage.hits))
	for name := range faqUsage.hits {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b int) bool {
		if faqUsage.hits[names[a]] != faqUsage.hits[names[b]] {
			return faqUsage.hits[names[a]] > faqUsage.hits[names[b]]
		}
		return names[a] < names[b]
	})

	parts := make([]string, 0, len(names)+1)
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, faqUsage.hits[name]))
	}
	parts = append(parts, fmt.Sprintf("unmatched=%d", faqUsage.misses))

	return "FAQ usage: " + strings.Join(parts, " ")
}
