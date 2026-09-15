package handlers

import (
	"strings"
	"testing"
	"time"
)

func resetModalStates() {
	modalStatesMu.Lock()
	defer modalStatesMu.Unlock()
	modalStates = make(map[string]*ModalState)
}

func TestCommandFromStateKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{name: "normal key", key: "bug_871539863307055134_12345", expected: "bug"},
		{name: "feature key", key: "feature_871539863307055134_12345", expected: "feature"},
		{name: "no separator", key: "bug", expected: "unknown"},
		{name: "empty key", key: "", expected: "unknown"},
		{name: "leading separator", key: "_channel_user", expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandFromStateKey(tt.key); got != tt.expected {
				t.Errorf("commandFromStateKey(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

// A state key carries a Discord channel and user ID, so it must never reach a
// log line. commandFromStateKey is what the handlers log instead; check it
// yields nothing but the command.
func TestCommandFromStateKeyDropsIdentifiers(t *testing.T) {
	const channelID = "871539863307055134"
	const userID = "198765432109876543"

	got := commandFromStateKey("bug_" + channelID + "_" + userID)

	if got != "bug" {
		t.Fatalf("commandFromStateKey() = %q, want %q", got, "bug")
	}
	for _, id := range []string{channelID, userID} {
		if strings.Contains(got, id) {
			t.Errorf("commandFromStateKey() returned %q, which carries the identifier %q", got, id)
		}
	}
}

func TestLookupModalStateDropsExpired(t *testing.T) {
	resetModalStates()
	const key = "bug_channel_user"

	putModalState(key, &ModalState{Command: "bug"})
	if _, ok := lookupModalState(key); !ok {
		t.Fatal("a freshly recorded state should be found")
	}

	// Age the entry past the TTL, as an abandoned submission would.
	modalStatesMu.Lock()
	modalStates[key].CreatedAt = time.Now().Add(-modalStateTTL - time.Minute)
	modalStatesMu.Unlock()

	if _, ok := lookupModalState(key); ok {
		t.Error("an expired state should not be returned")
	}

	modalStatesMu.Lock()
	_, present := modalStates[key]
	modalStatesMu.Unlock()
	if present {
		t.Error("an expired state should be removed from the map, not just hidden")
	}
}

func TestPutModalStatePurgesAbandoned(t *testing.T) {
	resetModalStates()

	modalStatesMu.Lock()
	modalStates["feature_old_user"] = &ModalState{
		Command:   "feature",
		CreatedAt: time.Now().Add(-modalStateTTL - time.Hour),
	}
	modalStatesMu.Unlock()

	// Recording any new submission should sweep the abandoned one.
	putModalState("bug_new_user", &ModalState{Command: "bug"})

	modalStatesMu.Lock()
	_, stale := modalStates["feature_old_user"]
	_, fresh := modalStates["bug_new_user"]
	modalStatesMu.Unlock()

	if stale {
		t.Error("an abandoned submission should be purged when a new one is recorded")
	}
	if !fresh {
		t.Error("the newly recorded submission should be present")
	}
}
