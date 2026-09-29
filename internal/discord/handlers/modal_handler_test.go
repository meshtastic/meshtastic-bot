package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meshtastic/meshtastic-bot/internal/config"
	internalgithub "github.com/meshtastic/meshtastic-bot/internal/github"

	"github.com/bwmarrin/discordgo"
)

// submitRecorder captures what a submission sends to Discord, in order.
type submitRecorder struct {
	mu        sync.Mutex
	responses []discordgo.InteractionResponse
	edits     []editRecord
}

// editRecord keeps components raw: MessageComponent is an interface and does
// not decode back into Go.
type editRecord struct {
	Content    *string         `json:"content"`
	Components json.RawMessage `json:"components"`
}

func (r *submitRecorder) session(t *testing.T) *discordgo.Session {
	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if strings.Contains(req.URL.Path, "/callback") {
					var resp discordgo.InteractionResponse
					if err := json.NewDecoder(req.Body).Decode(&resp); err != nil {
						t.Errorf("Failed to decode response: %v", err)
					}
					r.responses = append(r.responses, resp)
				} else if req.Method == "PATCH" {
					var edit editRecord
					if err := json.NewDecoder(req.Body).Decode(&edit); err != nil {
						t.Errorf("Failed to decode edit: %v", err)
					}
					r.edits = append(r.edits, edit)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
			},
		},
	}
	return s
}

func (r *submitRecorder) lastEdit(t *testing.T) editRecord {
	t.Helper()
	if len(r.edits) == 0 {
		t.Fatal("no reply edit was sent")
	}
	return r.edits[len(r.edits)-1]
}

func submitInteraction() *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:   discordgo.InteractionModalSubmit,
		Member: &discordgo.Member{User: &discordgo.User{ID: "42", Username: "reporter"}},
	}}
}

func retryInteraction(stateKey string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:   discordgo.InteractionMessageComponent,
		Data:   discordgo.MessageComponentInteractionData{CustomID: retryPrefix + stateKey},
		Member: &discordgo.Member{User: &discordgo.User{ID: "42", Username: "reporter"}},
	}}
}

func filedState(key string) *ModalState {
	state := &ModalState{
		Title:           "Crash on boot",
		SubmittedValues: map[string]string{"What happened?": "it crashed"},
		Command:         "bug",
		Owner:           "meshtastic",
		Repo:            "web",
	}
	putModalState(key, state)
	return state
}

func withGithubClient(t *testing.T, c internalgithub.Client) {
	t.Helper()
	original := GithubClient
	GithubClient = c
	t.Cleanup(func() { GithubClient = original })
}

func TestCreateIssueFromStateAcknowledgesBeforeFiling(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)

	rec := &submitRecorder{}
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			if len(rec.responses) != 1 {
				t.Errorf("CreateIssue ran before the submission was acknowledged")
			}
			return &internalgithub.IssueResponse{Number: 7, HTMLURL: "https://github.com/meshtastic/web/issues/7"}, nil
		},
	})

	createIssueFromState(rec.session(t), submitInteraction(), state, key)

	if len(rec.responses) != 1 || rec.responses[0].Type != discordgo.InteractionResponseDeferredChannelMessageWithSource ||
		rec.responses[0].Data == nil || rec.responses[0].Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Fatalf("want one deferred ephemeral acknowledgement, got %+v", rec.responses)
	}
	if edit := rec.lastEdit(t); edit.Content == nil || !strings.Contains(*edit.Content, "issues/7") {
		t.Errorf("reply does not link the issue: %+v", edit)
	}
	if _, ok := lookupModalState(key); ok {
		t.Error("state kept after the issue was filed")
	}
}

func TestFailedFilingKeepsAnswersAndRetryFilesThem(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)

	calls := 0
	rec := &submitRecorder{}
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			calls++
			if !strings.Contains(body, "it crashed") {
				t.Errorf("the retried report lost its answers: %q", body)
			}
			if calls == 1 {
				return nil, errors.New("github API returned 502")
			}
			return &internalgithub.IssueResponse{Number: 8, HTMLURL: "https://github.com/meshtastic/web/issues/8"}, nil
		},
	})

	createIssueFromState(rec.session(t), submitInteraction(), state, key)

	edit := rec.lastEdit(t)
	if !strings.Contains(string(edit.Components), retryPrefix+key) {
		t.Fatalf("failure reply has no Retry button: %+v", edit)
	}
	if _, ok := lookupModalState(key); !ok {
		t.Fatal("answers dropped after a failed filing")
	}

	handleButtonClick(rec.session(t), retryInteraction(key))

	if calls != 2 {
		t.Fatalf("Retry called CreateIssue %d times in total, want 2", calls)
	}
	last := rec.responses[len(rec.responses)-1]
	if last.Type != discordgo.InteractionResponseDeferredMessageUpdate {
		t.Errorf("Retry must defer a message update, got type %d", last.Type)
	}
	edit = rec.lastEdit(t)
	if edit.Content == nil || !strings.Contains(*edit.Content, "issues/8") {
		t.Errorf("retry reply does not link the issue: %+v", edit)
	}
	if string(edit.Components) != "[]" {
		t.Errorf("the Retry button must be cleared on success: %s", edit.Components)
	}
}

func TestRetryAfterExpiryFilesNothing(t *testing.T) {
	resetModalStates()
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			t.Error("CreateIssue called for an expired submission")
			return nil, errors.New("unexpected")
		},
	})
	rec := &submitRecorder{}
	handleButtonClick(rec.session(t), retryInteraction("bug_c_42"))
	if len(rec.responses) != 1 || rec.responses[0].Type != discordgo.InteractionResponseUpdateMessage {
		t.Errorf("want one message update saying the session expired, got %+v", rec.responses)
	}
}

func TestFileIssueRefusesASecondConcurrentFiling(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)
	if !state.startFiling() {
		t.Fatal("first claim failed")
	}
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			t.Error("a second filing reached GitHub")
			return nil, errors.New("unexpected")
		},
	})
	rec := &submitRecorder{}
	fileIssue(rec.session(t), submitInteraction(), state, key)
	if edit := rec.lastEdit(t); edit.Content == nil || !strings.Contains(*edit.Content, "already being filed") {
		t.Errorf("want the already-filing reply, got %+v", edit)
	}
}

func TestCollectSubmittedValuesIsSafeConcurrently(t *testing.T) {
	state := &ModalState{SubmittedValues: map[string]string{}}
	components := func(id string) []discordgo.MessageComponent {
		return []discordgo.MessageComponent{&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: id, Value: "v"},
		}}}
	}
	var wg sync.WaitGroup
	for n := 0; n < 50; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			collectSubmittedValues(state, components(string(rune('a'+n%26))), nil)
			_ = state.answeredCount()
		}(n)
	}
	wg.Wait()
	if got := state.answeredCount(); got != 26 {
		t.Errorf("answered = %d, want 26", got)
	}
}

func TestFailedFilingRestartsTheHoldClock(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	filedState(key)
	// The reporter took 25 of the 30 minutes filling the form in.
	modalStatesMu.Lock()
	modalStates[key].CreatedAt = time.Now().Add(-25 * time.Minute)
	modalStatesMu.Unlock()

	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			return nil, errors.New("github API returned 502")
		},
	})
	rec := &submitRecorder{}
	createIssueFromState(rec.session(t), submitInteraction(), mustState(t, key), key)

	modalStatesMu.Lock()
	age := time.Since(modalStates[key].CreatedAt)
	modalStatesMu.Unlock()
	if age > time.Minute {
		t.Errorf("after a failure the report expires in %s, not the 30 minutes the reply promises", modalStateTTL-age)
	}
}

func mustState(t *testing.T, key string) *ModalState {
	t.Helper()
	state, ok := lookupModalState(key)
	if !ok {
		t.Fatalf("no state for %s", key)
	}
	return state
}

func TestAFailedAcknowledgementFilesNothing(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			t.Error("filed an issue the reporter could never be told about")
			return nil, errors.New("unexpected")
		},
	})
	s, _ := discordgo.New("")
	s.Client = &http.Client{Transport: &MockRoundTripper{RoundTripFunc: func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewBufferString(`{"message": "Unknown interaction", "code": 10062}`)), Header: make(http.Header)}, nil
	}}}
	createIssueFromState(s, submitInteraction(), state, key)
	if _, ok := lookupModalState(key); !ok {
		t.Error("the answers were dropped")
	}
}

func TestRetryFindsAnIssueTheLostAttemptCreated(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)

	var firstBody string
	creates := 0
	rec := &submitRecorder{}
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			creates++
			firstBody = body
			// GitHub created it, but the response never arrived.
			return nil, errors.New("context deadline exceeded")
		},
		FindSubmissionFunc: func(owner, repo, marker string, since time.Time) (*internalgithub.IssueResponse, error) {
			if !strings.Contains(firstBody, marker) {
				t.Errorf("searched for %q, which the filed body does not carry", marker)
			}
			return &internalgithub.IssueResponse{Number: 11, HTMLURL: "https://github.com/meshtastic/web/issues/11"}, nil
		},
	})

	createIssueFromState(rec.session(t), submitInteraction(), state, key)
	handleButtonClick(rec.session(t), retryInteraction(key))

	if creates != 1 {
		t.Errorf("CreateIssue called %d times; the retry duplicated the report", creates)
	}
	if !strings.Contains(firstBody, "<!-- meshtastic-bot submission ") {
		t.Errorf("the body carries no submission marker:\n%s", firstBody)
	}
	if edit := rec.lastEdit(t); edit.Content == nil || !strings.Contains(*edit.Content, "issues/11") {
		t.Errorf("the retry did not link the issue it found: %v", edit.Content)
	}
}

func TestRetryFilesNothingWhenItCannotCheck(t *testing.T) {
	resetModalStates()
	key := "bug_c_42"
	state := filedState(key)
	creates := 0
	withGithubClient(t, &MockGitHubClient{
		CreateIssueFunc: func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
			creates++
			return nil, errors.New("github API returned 502")
		},
		FindSubmissionFunc: func(owner, repo, marker string, since time.Time) (*internalgithub.IssueResponse, error) {
			return nil, errors.New("github API returned 502")
		},
	})
	rec := &submitRecorder{}
	createIssueFromState(rec.session(t), submitInteraction(), state, key)
	handleButtonClick(rec.session(t), retryInteraction(key))
	if creates != 1 {
		t.Errorf("CreateIssue called %d times after the check failed, want 1", creates)
	}
	if edit := rec.lastEdit(t); !strings.Contains(string(edit.Components), retryPrefix+key) {
		t.Errorf("no Retry offered after the check failed: %s", edit.Components)
	}
}

func TestFormUnavailableSaysWhy(t *testing.T) {
	if got := formUnavailable("bug report", fmt.Errorf("%w: x", config.ErrNotConfigured)); !strings.Contains(got, "not configured") {
		t.Errorf("not-configured message: %q", got)
	}
	if got := formUnavailable("bug report", config.ErrTemplateLoading); !strings.Contains(got, "try again in a minute") {
		t.Errorf("loading message: %q", got)
	}
}
