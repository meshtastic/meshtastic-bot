package handlers

import (
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
	AllFields       []config.FieldConfig
	SubmittedValues map[string]string
	Labels          []string
	Command         string
	ChannelID       string
	Owner           string
	Repo            string
}

var modalStates = make(map[string]*ModalState)

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

func handleTapsign(s *discordgo.Session, i *discordgo.InteractionCreate) {
	helpText := "**How to get help or make a suggestion:**\n" +
		"`/bug`: To report a bug with the app.\n" +
		"`/feature`: To request a new feature. \n" +
		"`/faq`: Frequently Asked Questions.\n" +
		"`/changelog`: View changes between two versions.\n" +
		"`/repo`: Get the GitHub URL for a repository.\n"

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
