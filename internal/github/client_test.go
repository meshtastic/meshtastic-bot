package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
)

func TestFindSubmissionMatchesTheMarkerInTheTokensRecentIssues(t *testing.T) {
	var listed url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user":
			w.Write([]byte(`{"login": "MeshtasticAutomation"}`))
		case "/repos/meshtastic/web/issues":
			listed = r.URL.Query()
			w.Write([]byte(`[
				{"number": 12, "created_at": "2026-09-28T20:05:00Z", "html_url": "https://github.com/meshtastic/web/issues/12", "body": "other report"},
				{"number": 11, "created_at": "2026-09-28T20:01:00Z", "html_url": "https://github.com/meshtastic/web/issues/11", "body": "answers\n\n<!-- meshtastic-bot submission abc -->"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gh := github.NewClient(nil)
	base, _ := url.Parse(srv.URL + "/")
	gh.BaseURL = base
	c := &LiveGitHubClient{client: gh, ctx: t.Context(), repoCache: map[string]*CachedRepository{}}

	since := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)
	got, err := c.FindSubmission("meshtastic", "web", "<!-- meshtastic-bot submission abc -->", since)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Number != 11 {
		t.Fatalf("found %+v, want #11", got)
	}
	if listed.Get("creator") != "MeshtasticAutomation" || listed.Get("state") != "all" || listed.Get("since") != "2026-09-28T20:00:00Z" {
		t.Errorf("listed with %v", listed)
	}

	if none, err := c.FindSubmission("meshtastic", "web", "<!-- meshtastic-bot submission zzz -->", since); err != nil || none != nil {
		t.Errorf("an unmatched marker found %+v, %v", none, err)
	}
}

func TestFindSubmissionPagesUntilTheMarkerOrTheFirstAttempt(t *testing.T) {
	since := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)
	issue := func(n int, created time.Time, body string) string {
		return fmt.Sprintf(`{"number": %d, "created_at": %q, "body": %q}`, n, created.Format(time.RFC3339), body)
	}
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/user" {
			w.Write([]byte(`{"login": "MeshtasticAutomation"}`))
			return
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		recent := since.Add(10 * time.Minute)
		switch page {
		case "", "1":
			var items []string
			for n := 200; n > 100; n-- {
				items = append(items, issue(n, recent, "other report"))
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, "http://"+r.Host, r.URL.Path))
			w.Write([]byte("[" + strings.Join(items, ",") + "]"))
		case "2":
			w.Write([]byte("[" + issue(42, recent, "answers\n\n<!-- meshtastic-bot submission abc -->") + "," +
				issue(41, since.Add(-time.Hour), "older") + "]"))
		default:
			t.Errorf("fetched page %q", page)
			w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	gh := github.NewClient(nil)
	base, _ := url.Parse(srv.URL + "/")
	gh.BaseURL = base
	c := &LiveGitHubClient{client: gh, ctx: t.Context(), repoCache: map[string]*CachedRepository{}}

	got, err := c.FindSubmission("meshtastic", "web", "<!-- meshtastic-bot submission abc -->", since)
	if err != nil || got == nil || got.Number != 42 {
		t.Fatalf("got %+v, %v; want #42 from the second page", got, err)
	}

	pages = nil
	none, err := c.FindSubmission("meshtastic", "web", "<!-- meshtastic-bot submission zzz -->", since)
	if err != nil || none != nil {
		t.Errorf("an unmatched marker: %+v, %v", none, err)
	}
	if len(pages) != 2 {
		t.Errorf("fetched pages %v; the walk must stop at an issue older than the first attempt", pages)
	}
}
