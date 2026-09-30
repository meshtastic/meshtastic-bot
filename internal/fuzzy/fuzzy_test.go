package fuzzy

import "testing"

func TestScoreTiers(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		target string
		want   float64
	}{
		{name: "identical", query: "Serial", target: "Serial", want: 100},
		{name: "spacing ignored", query: "range test", target: "Rangetest", want: 100},
		{name: "ampersand spelled out", query: "store and forward", target: "Store & Forward", want: 100},
		{name: "punctuation ignored", query: "units locale", target: "Units-Locale", want: 100},
		{name: "empty query", query: "", target: "Serial", want: 0},
		{name: "empty target", query: "serial", target: "", want: 0},
		{name: "unrelated", query: "bluetooth", target: "Telemetry", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Score(tt.query, tt.target); got != tt.want {
				t.Errorf("Score(%q, %q) = %v, want %v", tt.query, tt.target, got, tt.want)
			}
		})
	}
}

func TestScoreOrdering(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		better string
		worse  string
	}{
		{name: "exact beats prefix", query: "serial", better: "Serial", worse: "Serial Module"},
		{name: "prefix beats word prefix", query: "tele", better: "Telemetry", worse: "Send Tele"},
		{name: "word prefix beats substring", query: "note", better: "Note Taking", worse: "Keynotes"},
		{name: "substring beats typo", query: "notification", better: "External Notification", worse: "Notifcation Thing"},
		{name: "shorter target wins a tie", query: "ser", better: "Serial", worse: "Serial Interface Module"},
		{name: "word order ignored", query: "pin pairing", better: "Pairing PIN", worse: "Pin Money"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			good, bad := Score(tt.query, tt.better), Score(tt.query, tt.worse)
			if good <= bad {
				t.Errorf("Score(%q, %q) = %v, want more than Score(%q, %q) = %v",
					tt.query, tt.better, good, tt.query, tt.worse, bad)
			}
		})
	}
}

func TestScoreTypos(t *testing.T) {
	above := []struct{ query, target string }{
		{"pairring", "Pairing"},
		{"telemetery", "Telemetry"},
		{"antena", "Antennas"},
		{"pairring pinn", "Pairing PIN"},
	}
	for _, tt := range above {
		if got := Score(tt.query, tt.target); got < Floor {
			t.Errorf("Score(%q, %q) = %v, want at least Floor (%v)", tt.query, tt.target, got, float64(Floor))
		}
	}

	// A short query must not fuzzy-match everything: one edit is most of it.
	below := []struct{ query, target string }{
		{"mqt", "Audio"},
		{"abc", "ADC"},
		{"zzz", "Serial"},
	}
	for _, tt := range below {
		if got := Score(tt.query, tt.target); got >= Floor {
			t.Errorf("Score(%q, %q) = %v, want below Floor (%v)", tt.query, tt.target, got, float64(Floor))
		}
	}
}

func TestScoreTierRangesDoNotOverlap(t *testing.T) {
	// The length penalty must never drag a match into the tier below it.
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := Score("a", "a"+long); got <= 80 {
		t.Errorf("prefix match of a long target = %v, want above the word-prefix tier (80)", got)
	}
	if got := Score("b c", "bbbb cccc"+long); got <= 70 {
		t.Errorf("word-prefix match of a long target = %v, want above the substring tier (70)", got)
	}
}

func TestScoreLongQueryIsBounded(t *testing.T) {
	// A pathological query must not cost more than the compare cap allows.
	huge := make([]byte, 100000)
	for idx := range huge {
		huge[idx] = 'a'
	}
	if got := Score(string(huge), "Serial"); got != 0 {
		t.Errorf("Score(huge, %q) = %v, want 0", "Serial", got)
	}
}

func TestScoreWrappedQuestions(t *testing.T) {
	tests := []struct {
		query  string
		target string
	}{
		{"what is the pairing pin", "pairing pin"},
		{"how far can it go", "how far"},
		{"i want a range test", "range test"},
		{"how does the mesh actually work", "how does the mesh work"},
		{"what antenna should i buy", "Antennas"},
	}
	for _, tt := range tests {
		if got := Score(tt.query, tt.target); got < Floor {
			t.Errorf("Score(%q, %q) = %v, want at least Floor (%v)", tt.query, tt.target, got, float64(Floor))
		}
	}

	// A stopword must not stand for a short target on its own.
	if got := Score("what is a node", "ADC"); got >= Floor {
		t.Errorf("Score(\"what is a node\", \"ADC\") = %v, want below Floor", got)
	}
}

func TestScoreWordOrderConflicts(t *testing.T) {
	// A short query word must not consume the target word a longer one needs.
	tests := []struct{ query, target string }{
		{"p pairing", "Pairing PIN"},
		{"pr private", "Private Primary"},
		{"ro router", "router role"},
	}
	for _, tt := range tests {
		if got := Score(tt.query, tt.target); got < Floor {
			t.Errorf("Score(%q, %q) = %v, want at least Floor", tt.query, tt.target, got)
		}
	}
}

func TestScoreTransposition(t *testing.T) {
	// Two swapped letters are one mistake, not two, so short names still match.
	tests := []struct{ query, target string }{
		{"serail", "Serial"},
		{"mtqt", "MQTT"},
		{"antennsa", "Antennas"},
	}
	for _, tt := range tests {
		if got := Score(tt.query, tt.target); got < Floor {
			t.Errorf("Score(%q, %q) = %v, want at least Floor", tt.query, tt.target, got)
		}
	}
}

func TestScoreApproximateMatchesRankBelowDirectOnes(t *testing.T) {
	direct := Score("pin pairing", "pairing pin")
	wrapped := Score("what is the pairing pin", "pairing pin")
	typo := Score("pairring", "pairing")

	if direct <= wrapped {
		t.Errorf("direct match %v, want more than a wrapped question %v", direct, wrapped)
	}
	if wrapped < Floor || typo < Floor {
		t.Errorf("wrapped = %v, typo = %v, want both at least Floor (%v)", wrapped, typo, float64(Floor))
	}
	if typo >= direct {
		t.Errorf("typo %v, want less than a direct match %v", typo, direct)
	}
}

func TestContentScore(t *testing.T) {
	answer := "Android strings and in-app docs are translated on Crowdin, not by pull request.\nPick your language there."

	tests := []struct {
		name  string
		query string
		want  float64
	}{
		{name: "a word only the answer has", query: "crowdin", want: ContentMatch},
		{name: "plural stemmed", query: "docs", want: ContentMatch},
		{name: "every word must appear", query: "crowdin weather", want: 0},
		{name: "filler alone is not a match", query: "what is the", want: 0},
		{name: "absent word", query: "paxcounter", want: 0},
		{name: "empty query", query: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContentScore(tt.query, answer); got != tt.want {
				t.Errorf("ContentScore(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}

	// It must never outrank a match on the candidate's own name.
	if ContentMatch >= Score("crowdin", "Crowdin") {
		t.Error("a body match ranks at or above a name match")
	}
}

func TestScoreMultiWordTypos(t *testing.T) {
	tests := []struct{ query, target string }{
		{"pairring pinn", "Pairing PIN"},
		{"telemmetrry sensorss", "telemetry sensors"},
		{"externall notifcaton", "external notification"},
	}
	for _, tt := range tests {
		if got := Score(tt.query, tt.target); got < Floor {
			t.Errorf("Score(%q, %q) = %v, want at least Floor", tt.query, tt.target, got)
		}
	}
}

func TestScoreEmptyTarget(t *testing.T) {
	for _, target := range []string{"", " ", "—"} {
		if got := Score("serial", target); got != 0 {
			t.Errorf("Score(\"serial\", %q) = %v, want 0", target, got)
		}
	}
}

func TestScoreWordsMatchedMidWord(t *testing.T) {
	got := Score("ternal tification", "External Notification")
	if got < Floor {
		t.Errorf("Score = %v, want at least Floor", got)
	}
	if got >= Score("extern notif", "External Notification") {
		t.Errorf("mid-word match %v, want less than a word-prefix match", got)
	}
}
