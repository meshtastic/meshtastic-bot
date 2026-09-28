package config

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type templateTransport struct {
	calls atomic.Int32
	fail  atomic.Bool
	name  atomic.Value
}

func (tt *templateTransport) RoundTrip(*http.Request) (*http.Response, error) {
	tt.calls.Add(1)
	if tt.fail.Load() {
		return nil, errors.New("offline")
	}
	body := "name: " + tt.name.Load().(string) + "\nbody: []\n"
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
}

func withTemplateTransport(t *testing.T) *templateTransport {
	t.Helper()
	tt := &templateTransport{}
	tt.name.Store("Bug Report")
	originalHTTP := templateHTTP
	templateHTTP = &http.Client{Transport: tt}
	templateCacheMu.Lock()
	templateCache = map[string]*cachedTemplate{}
	templateCacheMu.Unlock()
	t.Cleanup(func() { templateHTTP = originalHTTP })
	return tt
}

func testTemplateURL(t *testing.T) *TemplateURL {
	t.Helper()
	u, err := ParseTemplateURL("https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func expire(url string) {
	templateCacheMu.Lock()
	templateCache[url].fetched = time.Now().Add(-2 * templateCacheTTL)
	templateCacheMu.Unlock()
}

func waitForRefresh(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		templateCacheMu.Lock()
		done := !templateCache[url].refreshing
		templateCacheMu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("background refresh never finished")
}

func TestFetchGitHubTemplateFetchesOnce(t *testing.T) {
	tt := withTemplateTransport(t)
	u := testTemplateURL(t)
	for n := 0; n < 3; n++ {
		if tpl, err := FetchGitHubTemplate(u); err != nil || tpl.Name != "Bug Report" {
			t.Fatalf("fetch %d: %v %+v", n, err, tpl)
		}
	}
	if got := tt.calls.Load(); got != 1 {
		t.Errorf("fetched %d times, want 1", got)
	}
}

func TestFetchGitHubTemplateServesStaleWhileRefreshing(t *testing.T) {
	tt := withTemplateTransport(t)
	u := testTemplateURL(t)
	if _, err := FetchGitHubTemplate(u); err != nil {
		t.Fatal(err)
	}
	expire(u.RawURL())
	tt.name.Store("Bug Report v2")

	tpl, err := FetchGitHubTemplate(u)
	if err != nil || tpl.Name != "Bug Report" {
		t.Fatalf("an expired entry must be served at once, got %v %+v", err, tpl)
	}
	waitForRefresh(t, u.RawURL())
	if tpl, _ := FetchGitHubTemplate(u); tpl.Name != "Bug Report v2" {
		t.Errorf("refresh not applied, got %q", tpl.Name)
	}
}

func TestFetchGitHubTemplateKeepsCopyWhenRefreshFails(t *testing.T) {
	tt := withTemplateTransport(t)
	u := testTemplateURL(t)
	if _, err := FetchGitHubTemplate(u); err != nil {
		t.Fatal(err)
	}
	expire(u.RawURL())
	tt.fail.Store(true)

	if _, err := FetchGitHubTemplate(u); err != nil {
		t.Fatalf("a failed refresh must not fail the command: %v", err)
	}
	waitForRefresh(t, u.RawURL())
	if tpl, err := FetchGitHubTemplate(u); err != nil || tpl.Name != "Bug Report" {
		t.Errorf("cached copy lost after a failed refresh: %v %+v", err, tpl)
	}
}

func TestFetchGitHubTemplateAnswersInTimeWhileLoading(t *testing.T) {
	release := make(chan struct{})
	originalHTTP, originalWait := templateHTTP, templateWait
	templateHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("name: Bug Report\nbody: []\n")), Header: make(http.Header)}, nil
	})}
	templateWait = 50 * time.Millisecond
	templateCacheMu.Lock()
	templateCache = map[string]*cachedTemplate{}
	templateCacheMu.Unlock()
	t.Cleanup(func() { templateHTTP, templateWait = originalHTTP, originalWait })
	u := testTemplateURL(t)

	start := time.Now()
	if _, err := FetchGitHubTemplate(u); !errors.Is(err, ErrTemplateLoading) {
		t.Fatalf("a slow first fetch must return ErrTemplateLoading, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("waited %s for a template it has never had", time.Since(start))
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tpl, err := FetchGitHubTemplate(u); err == nil && tpl.Name == "Bug Report" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the background load never landed in the cache")
}

func TestFailedFirstLoadIsRetried(t *testing.T) {
	tt := withTemplateTransport(t)
	originalRetry, originalWait := templateRetry, templateWait
	templateRetry, templateWait = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { templateRetry, templateWait = originalRetry, originalWait })
	tt.fail.Store(true)
	u := testTemplateURL(t)

	if _, err := FetchGitHubTemplate(u); !errors.Is(err, ErrTemplateLoading) {
		t.Fatalf("a failed first fetch must return ErrTemplateLoading, got %v", err)
	}
	tt.fail.Store(false)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		templateCacheMu.Lock()
		ready := templateCache[u.RawURL()].template != nil
		templateCacheMu.Unlock()
		if ready {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a failed first load was not retried in the background")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
