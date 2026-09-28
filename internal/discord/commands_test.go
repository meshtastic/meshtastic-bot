package discord

import "testing"

func TestIssueCommandsCapTheTitle(t *testing.T) {
	for _, cmd := range getCommands() {
		if cmd.Name != "bug" && cmd.Name != "feature" {
			continue
		}
		for _, opt := range cmd.Options {
			if opt.Name == "title" && opt.MaxLength != issueTitleMax {
				t.Errorf("/%s title max_length = %d, want %d", cmd.Name, opt.MaxLength, issueTitleMax)
			}
		}
	}
}
