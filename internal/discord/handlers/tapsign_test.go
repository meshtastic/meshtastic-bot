package handlers

import (
	"strings"
	"testing"
)

// A command that exists but goes undocumented is the failure this guards:
// help that omits a command implies the command does not exist.
func TestTapsignHelpCoversEveryCommand(t *testing.T) {
	text := tapsignHelp()

	for name := range commandHandlers {
		// /tapsign is the help itself; it need not list its own name.
		if name == "tapsign" {
			continue
		}
		if !strings.Contains(text, "`/"+name+"`") {
			t.Errorf("/%s is registered in commandHandlers but absent from /tapsign", name)
		}
	}
}

// Discord rejects a message body over 2000 characters, which would make the
// command fail outright rather than merely look untidy.
func TestTapsignHelpWithinDiscordLimit(t *testing.T) {
	if n := len(tapsignHelp()); n > 2000 {
		t.Errorf("help text is %d characters, over Discord's limit of 2000", n)
	}
}

// The parts a newcomer cannot discover alone, and which cost them most if
// missed.
func TestTapsignHelpExplainsTheNonObvious(t *testing.T) {
	text := strings.ToLower(tapsignHelp())

	for _, want := range []string{
		"public",     // the issue is public
		"username",   // and attributed to the reporter
		"screenshot", // cannot be sent from Discord
		"base and a head",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("help text does not mention %q", want)
		}
	}
}
