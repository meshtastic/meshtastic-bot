package handlers

import (
	"strings"
	"testing"

	gogithub "github.com/google/go-github/v57/github"
)

func TestFormatChangelogMessage(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name       string
		base       string
		head       string
		comparison *gogithub.CommitsComparison
		want       []string // Substrings that should be present
		dontWant   []string // Substrings that should NOT be present
	}{
		{
			name: "basic comparison",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(2),
				HTMLURL:      strPtr("https://github.com/org/repo/compare/v1.0.0...v1.1.0"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abcdef123456"),
						HTMLURL: strPtr("https://github.com/org/repo/commit/abcdef1"),
						Commit: &gogithub.Commit{
							Message: strPtr("feat: cool feature"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("John Doe"),
							},
						},
						Author: &gogithub.User{
							Login: strPtr("johndoe"),
						},
					},
					{
						SHA:     strPtr("123456abcdef"),
						HTMLURL: strPtr("https://github.com/org/repo/commit/123456a"),
						Commit: &gogithub.Commit{
							Message: strPtr("fix: nasty bug\n\nSome details"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("Jane Smith"),
							},
						},
						Author: &gogithub.User{
							Login: strPtr("janesmith"),
						},
					},
				},
			},
			want: []string{
				"## Changes from v1.0.0 to v1.1.0",
				"Total commits: 2",
				"[`abcdef1`](<https://github.com/org/repo/commit/abcdef1>)",
				"feat: cool feature",
				"johndoe",
				"[`123456a`](<https://github.com/org/repo/commit/123456a>)",
				"fix: nasty bug",
				"janesmith",
				"[View Full Comparison](<https://github.com/org/repo/compare/v1.0.0...v1.1.0>)",
			},
			dontWant: []string{
				"Some details",
				"Showing last 10",
			},
		},
		{
			name: "many commits truncated",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(15),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: func() []*gogithub.RepositoryCommit {
					commits := make([]*gogithub.RepositoryCommit, 15)
					for i := 0; i < 15; i++ {
						commits[i] = &gogithub.RepositoryCommit{
							SHA:     strPtr("longhashvalue"),
							HTMLURL: strPtr("url"),
							Commit: &gogithub.Commit{
								Message: strPtr("msg"),
								Author:  &gogithub.CommitAuthor{Name: strPtr("author")},
							},
							Author: &gogithub.User{Login: strPtr("user")},
						}
					}
					return commits
				}(),
			},
			want: []string{
				"Total commits: 15",
				"*Showing last 10 of 15 commits*",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChangelogMessage(tt.base, tt.head, tt.comparison)

			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("formatChangelogMessage() missing %q\nGot:\n%s", w, got)
				}
			}

			for _, dw := range tt.dontWant {
				if strings.Contains(got, dw) {
					t.Errorf("formatChangelogMessage() unexpectedly contains %q", dw)
				}
			}
		})
	}
}
