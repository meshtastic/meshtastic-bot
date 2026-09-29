package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/meshtastic/meshtastic-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// submitPayload is a modal submit in the shape Discord documents in
// developers/components/reference: labels wrapping a string select, a text
// input and a file upload, a text display, and the upload's resolved
// attachment.
const submitPayload = `{
  "type": 5,
  "data": {
    "custom_id": "modal_continue_bug_1_42",
    "components": [
      {"type": 18, "id": 1, "component": {"type": 3, "id": 2, "custom_id": "hardware", "values": ["T-Beam", "T-Deck"]}},
      {"type": 18, "id": 3, "component": {"type": 4, "id": 4, "custom_id": "steps", "value": "1. flash\n2. wait"}},
      {"type": 18, "id": 5, "component": {"type": 19, "id": 6, "custom_id": "__log_files", "values": ["111111111111111111111"]}},
      {"type": 10, "id": 7}
    ],
    "resolved": {
      "attachments": {
        "111111111111111111111": {
          "content_type": "text/plain; charset=utf-8",
          "ephemeral": true,
          "filename": "device.log",
          "id": "111111111111111111111",
          "size": 17,
          "url": "https://cdn.discordapp.com/ephemeral-attachments/2/1/device.log?ex=68dc7ce1&is=68db2b61&hm=59"
        }
      }
    }
  }
}`

func TestCollectReadsDiscordsDocumentedSubmit(t *testing.T) {
	var ic discordgo.InteractionCreate
	if err := json.Unmarshal([]byte(submitPayload), &ic); err != nil {
		t.Fatalf("discordgo cannot decode Discord's documented submit: %v", err)
	}
	data := ic.ModalSubmitData()

	state := &ModalState{
		AllFields: []config.FieldConfig{
			{CustomID: "hardware", Label: "Hardware", Kind: config.FieldSelect},
			{CustomID: "steps", Label: "Steps to Reproduce"},
			config.UploadField(),
		},
		SubmittedValues: map[string]string{},
	}
	collectSubmittedValues(state, data.Components, data.Resolved.Attachments)

	if got := state.SubmittedValues["Hardware"]; got != "T-Beam, T-Deck" {
		t.Errorf("Hardware = %q, want both selections", got)
	}
	if got := state.SubmittedValues["Steps to Reproduce"]; got != "1. flash\n2. wait" {
		t.Errorf("Steps = %q", got)
	}
	if len(state.SubmittedValues) != 2 {
		t.Errorf("collected %v; the upload and text display are not answers", state.SubmittedValues)
	}
	if len(state.Files) != 1 || state.Files[0].Name != "device.log" || !strings.HasPrefix(state.Files[0].URL, "https://cdn.discordapp.com/") {
		t.Errorf("attachment not collected: %+v", state.Files)
	}
}

func TestShowDialogFallsBackToTextInputsWhenDiscordRefuses(t *testing.T) {
	var sent []string
	s, _ := discordgo.New("")
	s.Client = &http.Client{Transport: &MockRoundTripper{RoundTripFunc: func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		sent = append(sent, string(b))
		status := 204
		if len(sent) == 1 {
			status = 400
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(`{"code": 50035, "message": "Invalid Form Body"}`)), Header: make(http.Header)}, nil
	}}}

	state := &ModalState{
		Command: "bug",
		AllFields: []config.FieldConfig{
			{CustomID: "hardware", Label: "Hardware", Kind: config.FieldSelect, Options: []string{"T-Beam"}, Placeholder: "One of: T-Beam"},
			config.UploadField(),
		},
		SubmittedValues: map[string]string{},
	}
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ID: "1", Token: "t", Type: discordgo.InteractionApplicationCommand}}
	showDialog(s, i, state, "bug_1_42")

	if len(sent) != 2 {
		t.Fatalf("sent %d dialogs, want the labelled one and then the fallback", len(sent))
	}
	if !strings.Contains(sent[0], `"type":18`) {
		t.Errorf("first dialog is not labelled: %s", sent[0])
	}
	if strings.Contains(sent[1], `"type":18`) || strings.Contains(sent[1], `"type":19`) || !strings.Contains(sent[1], `"type":4`) {
		t.Errorf("fallback must be text inputs only: %s", sent[1])
	}
	if next := state.finishDialog(); next != 2 {
		t.Errorf("after the fallback dialog next = %d, want 2: the upload it left out must not be asked again", next)
	}
}

func TestFetchLogsInlinesTextAndExplainsTheRest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device.log":
			w.Write([]byte("INFO boot\n```\nERROR radio"))
		case "/binary.log":
			w.Write([]byte{0xff, 0xfe, 0x00, 0x80})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	logs := fetchLogs([]AttachedFile{
		{Name: "device.log", URL: srv.URL + "/device.log", ContentType: "text/plain"},
		{Name: "bug.png", URL: srv.URL + "/bug.png", ContentType: "image/png"},
		{Name: "binary.log", URL: srv.URL + "/binary.log"},
		{Name: "gone.txt", URL: srv.URL + "/gone.txt"},
	}, githubBodyLimit)
	if len(logs) != 4 {
		t.Fatalf("got %d entries, want one per file", len(logs))
	}
	if logs[0].Content != "INFO boot\n```\nERROR radio" || logs[0].Note != "" {
		t.Errorf("text log: %+v", logs[0])
	}
	for _, l := range logs[1:] {
		if l.Content != "" || l.Note == "" {
			t.Errorf("%s must be left out with a note: %+v", l.Name, l)
		}
	}

	body := buildIssueBody(nil, nil, logs, "reporter", "42")
	if !strings.Contains(body, "````\nINFO boot\n```\nERROR radio\n````") {
		t.Errorf("a log with a fence inside needs a longer fence:\n%s", body)
	}
	if !strings.Contains(body, "- `bug.png`: not included") {
		t.Errorf("the image is not listed as left out:\n%s", body)
	}
}

func TestBuildIssueBodyMatchesGitHubsFormLayout(t *testing.T) {
	fields := []config.FieldConfig{
		{CustomID: "steps", Label: "Steps"},
		{CustomID: "browser", Label: "Browser"},
		{CustomID: "logs", Label: "Relevant console output", Render: "shell"},
		config.UploadField(),
	}
	body := buildIssueBody(fields, map[string]string{
		"Steps":                   "click @jamesarich",
		"Browser":                 "  ",
		"Relevant console output": "@everyone here",
	}, nil, "reporter", "42")

	for _, want := range []string{
		"### Steps\n\nclick @\u200bjamesarich\n\n",
		"### Browser\n\n_No response_\n\n",
		"### Relevant console output\n\n```shell\n@everyone here\n```\n\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Log files (optional)") {
		t.Errorf("the upload field is not an answer:\n%s", body)
	}
}

func TestIssueTitleAndLabelsFollowTheTemplate(t *testing.T) {
	if got := issueTitle("[Bug]: ", "Crash on boot", "Bug Report"); got != "[Bug]: Crash on boot" {
		t.Errorf("title = %q", got)
	}
	if got := issueTitle("[Bug]: ", "[bug]: Crash", "Bug Report"); got != "[bug]: Crash" {
		t.Errorf("an already prefixed title got a second prefix: %q", got)
	}
	if got := []rune(issueTitle("[Feature Request]: ", strings.Repeat("x", 256), "")); len(got) != 256 {
		t.Errorf("title is %d characters, GitHub caps it at 256", len(got))
	}
	if got := issueLabels([]string{"bug", "from-discord"}, "bug"); strings.Join(got, ",") != "from-discord,bug" {
		t.Errorf("labels = %v", got)
	}
	if got := issueLabels(nil, "enhancement"); strings.Join(got, ",") != "from-discord,enhancement" {
		t.Errorf("fallback labels = %v", got)
	}
}

func TestIssueBodyStaysWithinGitHubsLimit(t *testing.T) {
	// Five full-length answers plus five large logs, each with a backtick run
	// that forces a long fence: the worst the web and Android bug forms allow.
	big := strings.Repeat("é", 4000)
	logText := strings.Repeat("`", 900) + "\n" + strings.Repeat("ERROR radio é\n", 20000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(logText)) }))
	defer srv.Close()

	var fields []config.FieldConfig
	values := map[string]string{}
	for n := 0; n < 5; n++ {
		label := fmt.Sprintf("Answer %d", n)
		fields = append(fields, config.FieldConfig{CustomID: label, Label: label, Style: "paragraph"})
		values[label] = big
	}
	var files []AttachedFile
	for n := 0; n < 5; n++ {
		files = append(files, AttachedFile{Name: fmt.Sprintf("log%d.txt", n), URL: srv.URL + "/log", ContentType: "text/plain"})
	}

	const marker = "<!-- meshtastic-bot submission 0123456789abcdef -->"
	body := issueBody(fields, values, files, "reporter", "42", marker)
	if n := utf8.RuneCountInString(body); n > githubBodyLimit {
		t.Fatalf("body is %d characters, over GitHub's %d", n, githubBodyLimit)
	}
	if !strings.Contains(body, "(truncated)") || !strings.Contains(body, "Submitted via Discord by: reporter (42)") {
		t.Errorf("want a truncated log and the footer intact")
	}
	if !strings.HasSuffix(body, "\n\n"+marker) {
		t.Errorf("the submission marker must end the body, uncut")
	}
	// Every fence that opens a log also closes it.
	fence := strings.Repeat("`", 901)
	if opens := strings.Count(body, fence+"\n"); opens == 0 || strings.Count(body, "\n"+fence+"\n") < opens {
		t.Errorf("a log fence was cut open")
	}
}

func TestFitLogCountsTheRenderedEntry(t *testing.T) {
	l := logFile{Name: "device.log", Content: strings.Repeat("`", 50) + strings.Repeat("x", 5000)}
	for _, room := range []int{0, 40, 200, 1000, 6000} {
		got := fitLog(l, room)
		if size := renderedSize(got); size > room && got.Content != "" {
			t.Errorf("room %d: rendered entry is %d characters", room, size)
		}
	}
}
