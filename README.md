# Meshtastic Discord Bot

A Discord bot for the Meshtastic community that streamlines bug reporting and feature requests by creating GitHub issues directly from Discord through interactive modals. Designed to be a friendly customer service helper for users seeking assistance. Built with Go for performance and reliability.

## Features

- **Interactive Bug Reports**: Submit bug reports directly to GitHub through Discord modals
- **Feature Requests**: Create feature requests with rich formatting
- **FAQ System**: Searchable FAQ with autocomplete for quick answers
- **Health Check Endpoint**: Built-in HTTP server for monitoring and container health checks

## Commands

- `/tapsign`: Display a short help message in the channel
- `/faq <topic>`: Search and display frequently asked questions (with autocomplete)
- `/bug <title>`: Submit a bug report (opens an interactive modal)
- `/feature <title>`: Request a new feature (opens an interactive modal)

## Discord Bot Setup

Before running the bot, you need to create a Discord application and configure it properly.

### 1. Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Click "New Application"
3. Give your application a name and click "Create"

### 2. Create a Bot User

1. In your application's settings, navigate to the "Bot" tab
2. Click "Add Bot"
3. Customize the bot's username and icon

### 3. Get the Bot Token

1. On the "Bot" tab, click "Reset Token" to reveal the bot's token
2. **Important:** Treat this token like a password. Never share it or commit it to version control
3. Copy this token for your `.env.dev` file

### 4. Enable Privileged Gateway Intents

On the "Bot" tab, scroll down to "Privileged Gateway Intents" and enable:
- **Message Content Intent** (required for reading messages)
- **Server Members Intent** (if needed for user info)

### 5. Set Bot Permissions and Scopes

1. Navigate to "OAuth2" → "URL Generator"
2. In "Scopes", select:
   - `bot`
   - `applications.commands`
3. In "Bot Permissions", select:
   - `Send Messages`
   - `Use Slash Commands`
   - `Embed Links`
   - `Read Message History`

### 6. Invite the Bot to Your Server

1. Copy the generated URL from the URL Generator
2. Paste it into your browser
3. Select your server (requires "Manage Server" permission)
4. Authorize the bot

### 7. Get Your Server ID

1. Enable Developer Mode in Discord: User Settings → Advanced (has an icon with three dots) → Toggle Developer Mode
2. Right-click your server icon → "Copy Server ID"
3. Save this for your `.env.dev` file

## Local Development Setup

### Prerequisites

- Go 1.25 or higher
- Discord bot token (see Discord Bot Setup above)
- A fine grained Github personal access token with the `issues` scope and read&write permission.

### Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/meshtastic/meshtastic-bot.git
   cd meshtastic-bot
   ```

2. **Create environment file:**
   ```bash
   cp .env.example .env.dev
   ```

3. **Edit `.env` with your credentials:**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   GITHUB_TOKEN=your_github_personal_access_token
   DISCORD_SERVER_ID=your_server_id
   CONFIG_PATH=config.yaml
   FAQ_PATH=faq.yaml
   HEALTHCHECK_PORT=8080
   ```

4. **Install dependencies:**
   ```bash
   go mod download
   ```

5. **Run the bot:**
   ```bash
   go run .
   ```

## Configuration Files

### config.yaml

Defines command-to-GitHub-repository mappings:

```yaml
config:
  - command: bug
    template_url: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml
    channel_id:
      - '871553714782081024'
    title: Bug Report
  - command: feature
    template_url: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/feature.yml
    channel_id:
      - '871553714782081024'
    title: Feature Request
```

### faq.yaml

Defines FAQ items and software modules:

```yaml
faq:
  - name: Getting Started
    url: https://meshtastic.org/docs/getting-started
  - name: Supported Devices
    url: https://meshtastic.org/docs/hardware

software_modules:
  - name: Arduino
    url: https://meshtastic.org/docs/software/arduino
  - name: Python SDK
    url: https://meshtastic.org/docs/software/python
```

## Running with Docker

### Local Development

Use the provided `run.sh` script to run with different environment files:

```bash
# Development mode
./run.sh .env.dev

# Production mode
./run.sh .env.prod

# With custom docker-compose flags
./run.sh .env.dev --build --force-recreate
```

### Manual Docker Commands

```bash
# Build the image
docker build -t meshtastic-bot .

# Run with specific env file
docker run -d --env-file .env.prod -p 8080:8080 meshtastic-bot
```

## Deployment

### Deploying to Fly.io

This project includes automated deployment scripts for Fly.io.

#### First-Time Setup

1. **Install flyctl:**
   ```bash
   # macOS
   brew install flyctl

   # Linux/WSL
   curl -L https://fly.io/install.sh | sh
   ```

2. **Login to Fly.io:**
   ```bash
   fly auth login
   ```

3. **Deploy with secrets:**
   ```bash
   ./deploy.sh --sync-secrets
   ```

#### Subsequent Deployments

```bash
# Regular deployment
./deploy.sh

# Skip confirmation prompt
./deploy.sh -y

# Update secrets only
cat .env.prod | grep -v '^#' | grep -v '^$' | fly secrets import
```

#### Manual Deployment

```bash
# View current status
fly status

# Deploy manually
fly deploy

# View logs
fly logs

# Open the app
fly open
```

### CI/CD Pipeline

The project uses GitHub Actions for continuous integration and deployment.

**Workflow:** `.github/workflows/ci.yml`

**On Pull Requests & Pushes:**
- Linting with golangci-lint
- Unit tests with race detection
- Build verification
- Docker image build

**On Main Branch Push:**
- Automatic deployment to Fly.io (after all checks pass)

**Required GitHub Secret:**
- `FLY_API_TOKEN` - Get your token with `fly auth token`

## Running Tests

### Basic Test Commands

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package
go test ./config
go test ./discord/handlers

# Run specific test
go test ./config -run TestParseTemplateURL
```

### Advanced Testing

```bash
# Run with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run with race detection
go test ./... -race

# Run tests in parallel
go test ./... -parallel 4
```

### Test Coverage

Current test coverage includes:
- ✅ Configuration parsing and validation
- ✅ FAQ data loading and searching
- ✅ Helper functions for text processing
- ✅ Modal field extraction
- ✅ Template URL parsing

See `TESTING.md` for detailed testing documentation.

## Project Structure

```
meshtastic-bot/
├── config/              # Configuration loading and validation
│   ├── config.go        # Main config and URL parsing
│   ├── env.go           # Environment variable handling
│   ├── faq.go           # FAQ data structures
│   └── modal.go         # Modal configuration
├── discord/             # Discord bot implementation
│   ├── bot.go           # Bot initialization
│   ├── commands.go      # Slash command definitions
│   ├── handlers.go      # Command handlers
│   └── handlers/        # Individual handler implementations
│       ├── bug_handler.go
│       ├── feature_handler.go
│       ├── faq_handler.go
│       └── helpers.go
├── github/              # GitHub API client
│   └── client.go
├── .github/             # CI/CD workflows
│   └── workflows/
│       └── ci.yml       # Main CI/CD pipeline
├── config.yaml          # Command configuration
├── faq.yaml             # FAQ content
├── run.sh               # Local Docker runner script
├── deploy.sh            # Fly.io deployment script
├── Dockerfile           # Multi-stage Docker build
├── docker-compose.yml   # Docker Compose configuration
├── fly.toml             # Fly.io configuration
└── main.go              # Application entry point
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DISCORD_TOKEN` | Yes | - | Discord bot token |
| `DISCORD_SERVER_ID` | Yes | - | Target Discord server ID |
| `GITHUB_TOKEN` | Yes | - | GitHub personal access token |
| `CONFIG_PATH` | Yes | - | Path to config.yaml |
| `FAQ_PATH` | No | `faq.yaml` | Path to FAQ YAML file |
| `HEALTHCHECK_PORT` | No | `8080` | HTTP health check port |
| `ENV` | No | `dev` | Environment (dev/prod) |

## Health Check Endpoint

The bot includes a built-in HTTP server for health monitoring:

```bash
# Check if bot is running
curl http://localhost:8080/health

# Response: {"status":"ok"}
```

This endpoint is used by:
- Docker health checks
- Fly.io health monitoring
- Load balancers
- Monitoring systems

## Contributing

Contributions are welcome! Please follow these guidelines:

### 1. Fork and Clone

```bash
git clone https://github.com/meshtastic/meshtastic-bot.git
cd meshtastic-bot
```

### 2. Create a Feature Branch

```bash
git checkout -b feature/your-feature-name (ci, fix, chore, feat, breaking, refactor)
```

### 3. Code Style

This project follows standard Go conventions. Use the built-in Go tools:

```bash
# Format code
go fmt ./...

# Static analysis
go vet ./...

# Optional: Install golangci-lint for advanced linting (used in CI)
# go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
# golangci-lint run
```

### 4. Write Tests

- Add tests for new functionality
- Ensure all tests pass: `go test ./...`
- Maintain or improve test coverage

### 5. Commit Guidelines

- Write clear, descriptive commit messages
- Follow conventional commits format
- Keep commits focused and atomic

### 6. Submit Pull Request

1. Push your branch to your fork
2. Open a pull request against `main`
3. Describe your changes clearly
4. Link any related issues
5. Wait for CI checks to pass
6. Address review feedback

## Development Tips

### Hot Reload

For faster development, use a tool like [air](https://github.com/cosmtrek/air):

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
```

### Debug Logging

Enable verbose logging for development:

```go
// In your .env.dev
LOG_LEVEL=debug
```

### Testing Slash Commands

After deploying slash commands, they may take up to an hour to propagate globally. For faster testing:
- Use guild-specific commands (instant updates)
- Test in your development server first

## Troubleshooting

### Bot doesn't respond to commands

1. Check bot has required permissions in Discord server
2. Verify `DISCORD_SERVER_ID` is correct
3. Ensure slash commands are registered (check bot logs)
4. Confirm bot has "Use Slash Commands" permission
5. Ensure your bot shows in the member list, and shows as online in your Discord channel

### GitHub issues not creating

1. Verify `GITHUB_TOKEN` has either access to `all public repos` or `selected repos` 
2. Ensure `GITHUB_TOKEN` has pelmissions with read/write access to `issues`
4. Check template URLs in `config.yaml` are valid
4. Ensure bot has access to target repository
5. Check logs for specific error messages

### Docker build fails

1. Ensure Go 1.25+ is specified in Dockerfile
2. Verify all dependencies are in `go.mod`
3. Check for syntax errors: `go build .`

## License

See LICENSE.md in this repo for more details.

## Support

- Documentation: [Meshtastic Docs](https://meshtastic.org/docs)
- Discord: [Meshtastic Discord](https://discord.gg/meshtastic)
- Issues: [GitHub Issues](https://github.com/meshtastic/meshtastic-bot/issues)
