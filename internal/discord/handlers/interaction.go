package handlers

import (
	"strings"
	"sync"
	"time"

	config "github.com/meshtastic/meshtastic-bot/internal/config"
	github "github.com/meshtastic/meshtastic-bot/internal/github"

	"github.com/bwmarrin/discordgo"
)

var (
	GithubClient github.Client
	GithubOwner  string
	GithubRepo   string
)

func InitializeGithub(token, owner, repo string) {
	GithubClient = github.NewClient(token)
	GithubOwner = owner
	GithubRepo = repo
}

// ModalState tracks the state of multi-part modals
type ModalState struct {
	Title           string
	CreatedAt       time.Time
	AllFields       []config.FieldConfig
	SubmittedValues map[string]string
	Labels          []string
	Command         string
	ChannelID       string
	Owner           string
	Repo            string
}

// modalStates is keyed by command, channel and user.
//
// discordgo dispatches each interaction in its own goroutine, so these entries
// are written and read concurrently; an unguarded map panics the process with
// "concurrent map read and map write". Go through putModalState,
// lookupModalState and dropModalState rather than touching the map directly.
// modalStateTTL bounds how long an unfinished submission is held.
//
// A multi-part submission has to keep what the reporter typed so far between
// modals, but nothing drops that if they simply walk away. Without a bound
// those values sit in memory until the process restarts.
const modalStateTTL = 30 * time.Minute

var (
	modalStatesMu sync.Mutex
	modalStates   = make(map[string]*ModalState)
)

// commandFromStateKey returns the command a state key belongs to.
//
// Keys are "<command>_<channelID>_<userID>". Logging the command on its own
// keeps Discord channel and user IDs out of the container log, which the
// default json-file driver writes to the host's disk.
func commandFromStateKey(key string) string {
	command, _, found := strings.Cut(key, "_")
	if !found || command == "" {
		return "unknown"
	}
	return command
}

func putModalState(key string, state *ModalState) {
	modalStatesMu.Lock()
	defer modalStatesMu.Unlock()
	state.CreatedAt = time.Now()
	purgeExpiredLocked()
	modalStates[key] = state
}

func lookupModalState(key string) (*ModalState, bool) {
	modalStatesMu.Lock()
	defer modalStatesMu.Unlock()
	state, ok := modalStates[key]
	if !ok {
		return nil, false
	}
	if time.Since(state.CreatedAt) > modalStateTTL {
		delete(modalStates, key)
		return nil, false
	}
	return state, true
}

// purgeExpiredLocked drops abandoned submissions. The caller holds the mutex.
func purgeExpiredLocked() {
	for key, state := range modalStates {
		if time.Since(state.CreatedAt) > modalStateTTL {
			delete(modalStates, key)
		}
	}
}

func dropModalState(key string) {
	modalStatesMu.Lock()
	defer modalStatesMu.Unlock()
	delete(modalStates, key)
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"tapsign":   handleTapsign,
	"feature":   handleFeature,
	"faq":       handleFaq,
	"bug":       handleBug,
	"changelog": handleChangelog,
	"repo":      handleRepo,
}

// HandleInteraction routes interactions to appropriate handlers
func HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if handler, exists := commandHandlers[i.ApplicationCommandData().Name]; exists {
			handler(s, i)
		}
	case discordgo.InteractionApplicationCommandAutocomplete:
		handleAutocomplete(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	case discordgo.InteractionMessageComponent:
		handleButtonClick(s, i)
	}
}

// tapsignHelp explains how to use each command, not only what it is called.
//
// The command names alone leave the useful parts undiscoverable: that /faq and
// /changelog offer suggestions, that /changelog wants two versions rather than
// one, that /repo accepts short names, and that /bug and /feature answer in two
// channels only and file something public.
func tapsignHelp() string {
	var b strings.Builder

	b.WriteString("**How to get help or make a suggestion**\n\n")

	b.WriteString("`/faq` — look up a frequently asked question.\n")
	b.WriteString("Start typing a topic and pick one from the suggestions.\n\n")

	b.WriteString("`/changelog` — see what changed between two releases.\n")
	b.WriteString("Takes a base and a head version, both with suggestions, ")
	b.WriteString("as in `/changelog base:v2.7.1 head:v2.7.2`.\n\n")

	b.WriteString("`/repo` — get the GitHub link for a repository.\n")
	b.WriteString("Short names work, such as `apple`, `ios`, `android`, `docs` or `cli`. ")
	b.WriteString("Leave the name out for the default repository.\n\n")

	b.WriteString("`/bug` — report a bug.\n")
	b.WriteString("`/feature` — request a new feature.\n")
	b.WriteString("Both open a form. Give a short title, fill the form in, and the bot files ")
	b.WriteString("a GitHub issue and replies with the link. They answer in the Android app ")
	b.WriteString("and web client channels only.\n")
	b.WriteString("The issue is public and records your Discord username and user ID. ")
	b.WriteString("Screenshots cannot be sent from Discord, so add them in a comment on the ")
	b.WriteString("issue once it exists.\n")

	return b.String()
}

func handleTapsign(s *discordgo.Session, i *discordgo.InteractionCreate) {
	helpText := tapsignHelp()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: helpText,
		},
	})
}

// handleAutocomplete handles autocomplete interactions for commands
func handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	switch data.Name {
	case "faq":
		handleFaqAutocomplete(s, i)
	case "changelog":
		handleChangelogAutocomplete(s, i)
	}
}
