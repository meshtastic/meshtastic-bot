// Package fuzzy ranks short candidate strings against a typed query.
package fuzzy

import (
	"sort"
	"strings"
)

// Floor is the lowest score worth showing a user.
const Floor = 30

// ContentMatch is the score for a query found only in a candidate's body text.
const ContentMatch = 35

// maxCompared bounds the runes edit distance is computed over.
const maxCompared = 64

// Score rates query against target from 0 (no match) to 100. Only the ordering
// is meaningful: each tier is a kind of match, strongest first.
func Score(query, target string) float64 {
	q, t := compact(query), compact(target)
	if q == "" || t == "" {
		return 0
	}

	if q == t {
		return 100
	}
	if strings.HasPrefix(t, q) {
		return 90 - lengthGap(q, t)
	}

	// Longest words first: a short word must not take the target word a longer
	// one needs.
	qWords, tWords := words(query), words(target)
	sort.SliceStable(qWords, func(a, b int) bool { return len(qWords[a]) > len(qWords[b]) })

	if matchWords(qWords, tWords, prefixMatch) {
		return 80 - lengthGap(q, t)
	}
	if strings.Contains(t, q) {
		return 70 - lengthGap(q, t)
	}
	if matchWords(qWords, tWords, containsMatch) {
		return 60 - lengthGap(q, t)
	}

	// A question wrapped around the topic: "what is the pairing pin".
	if frac, words := coverage(tWords, qWords); frac >= coverageMin || words >= 2 {
		return 50.5 + 4.5*frac
	}

	// Typos, only once an edit or two is not most of the query.
	if len([]rune(q)) >= 4 {
		if dist := levenshtein(truncate([]rune(q)), truncate([]rune(t))); dist <= allowedEdits(q, t) {
			return 50 - 4*float64(dist-1)
		}
	}

	return 0
}

// ContentScore reports ContentMatch when every meaningful word of query appears
// in text. A last resort: the candidate's own name says nothing.
func ContentScore(query, text string) float64 {
	qWords := contentWords(words(query))
	tWords := words(text)
	if len(qWords) == 0 || len(tWords) == 0 {
		return 0
	}

	for _, qw := range qWords {
		found := false
		for _, tw := range tWords {
			if prefixMatch(tw, qw) {
				found = true
				break
			}
		}
		if !found {
			return 0
		}
	}
	return ContentMatch
}

// allowedEdits: one edit per four characters, up to three.
func allowedEdits(q, t string) int {
	shorter := len([]rune(q))
	if n := len([]rune(t)); n < shorter {
		shorter = n
	}
	allowed := 1 + shorter/4
	if allowed > 3 {
		allowed = 3
	}
	return allowed
}

func contentWords(qWords []string) []string {
	out := make([]string, 0, len(qWords))
	for _, w := range qWords {
		if len([]rune(w)) >= 3 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

var stopWords = map[string]bool{
	"and": true, "any": true, "are": true, "but": true, "can": true, "did": true,
	"does": true, "for": true, "from": true, "get": true, "has": true, "how": true,
	"its": true, "not": true, "the": true, "this": true, "use": true, "want": true,
	"was": true, "what": true, "whats": true, "when": true, "where": true,
	"which": true, "why": true, "will": true, "with": true, "you": true,
	"your": true, "should": true, "there": true, "that": true,
}

// stem drops a plural ending, so "ranges" and "range" compare equal.
func stem(w string) string {
	r := []rune(w)
	switch {
	case len(r) > 4 && strings.HasSuffix(w, "es"):
		return string(r[:len(r)-2])
	case len(r) > 3 && strings.HasSuffix(w, "s"):
		return string(r[:len(r)-1])
	}
	return w
}

func prefixMatch(target, query string) bool {
	return strings.HasPrefix(stem(target), stem(query))
}

func containsMatch(target, query string) bool {
	return strings.Contains(stem(target), stem(query))
}

const coverageMin = 0.6

// coverage returns the share of the target's characters matched, and how many of
// its words matched at all.
func coverage(tWords, qWords []string) (float64, int) {
	if len(tWords) == 0 || len(qWords) == 0 {
		return 0, 0
	}

	used := make([]bool, len(qWords))
	matched, total, words := 0, 0, 0
	for _, tw := range tWords {
		total += len([]rune(tw))
		for idx, qw := range qWords {
			if used[idx] || !coversWord(tw, qw) {
				continue
			}
			used[idx] = true
			words++
			if n := len([]rune(qw)); n < len([]rune(tw)) {
				matched += n
			} else {
				matched += len([]rune(tw))
			}
			break
		}
	}
	return float64(matched) / float64(total), words
}

// coversWord accepts the word, a prefix of three runes or more, or one edit out.
func coversWord(target, query string) bool {
	if target == query {
		return true
	}
	if len([]rune(query)) < 3 {
		return false
	}
	if prefixMatch(target, query) || prefixMatch(query, target) {
		return true
	}
	return len([]rune(query)) >= 5 && levenshtein([]rune(query), []rune(target)) == 1
}

// lengthGap prefers the shorter target, capped short of the tier below.
func lengthGap(q, t string) float64 {
	gap := len(t) - len(q)
	if gap > 9 {
		gap = 9
	}
	return float64(gap) * 0.5
}

// normalize lowercases and spells "&" out.
func normalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "&", " and ")
}

func compact(s string) string {
	var b strings.Builder
	for _, r := range normalize(s) {
		if isAlnum(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func words(s string) []string {
	return strings.FieldsFunc(normalize(s), func(r rune) bool {
		return !isAlnum(r)
	})
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// matchWords pairs every query word with a distinct target word under fits.
func matchWords(qWords, tWords []string, fits func(target, query string) bool) bool {
	if len(qWords) == 0 || len(tWords) == 0 {
		return false
	}

	used := make([]bool, len(tWords))
	for _, qw := range qWords {
		paired := false
		for idx, tw := range tWords {
			if used[idx] || !fits(tw, qw) {
				continue
			}
			used[idx] = true
			paired = true
			break
		}
		if !paired {
			return false
		}
	}
	return true
}

func truncate(r []rune) []rune {
	if len(r) > maxCompared {
		return r[:maxCompared]
	}
	return r
}

// levenshtein counts single-rune edits, an adjacent transposition among them.
func levenshtein(a, b []rune) int {
	prevPrev := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if swap := prevPrev[j-2] + 1; swap < curr[j] {
					curr[j] = swap
				}
			}
		}
		prevPrev, prev, curr = prev, curr, prevPrev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
