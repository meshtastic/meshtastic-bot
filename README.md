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

## Environment Files

This bot uses environment files to manage configuration and secrets for different environments.

**Recommendation:** Create both `.env.dev` and `.env.prod` files with different Discord servers:
- **Development (`.env.dev`)**: Use a personal Discord server for testing bot changes and updates without affecting your production users
- **Production (`.env.prod`)**: Use your main Discord server where real users interact with the bot

### `.env.dev` - Development Secrets
Use this file when developing or testing the bot locally. Development mode:
- Provides verbose logging for debugging
- Registers slash commands instantly to your test server (no global propagation delay)
- Should point to a personal/test Discord server (use a different `DISCORD_SERVER_ID` & `DISCORD_TOKEN`)
- Runs using Docker via the included `run.sh` script

### `.env.prod` - Production Secrets
Use this file when deploying the bot to your production Discord server. Production mode:
- Uses optimized logging
- Registers commands globally (may take up to 1 hour to propagate)
- Points to your main production Discord server
- Runs using Docker via the included `run.sh` script

Both modes use the same Docker setup - the only difference is which environment file you pass to `run.sh`. More details about using the `run.sh` file are covered in the [Deployment](#deployment) section  

## Discord Bot Setup

Before running the bot, you need to create a Discord application and configure it properly.

### 1. Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Click "New Application"
3. Give your application a name and click "Create"

### 2. Create a Bot User

1. In your application's settings, navigate to the "Bot" tab
2. On the username field give this bot a name of "Meshtastic Bot"
3. Upload the Meshtastic icon as the avatar

### 3. Get the Bot Token

1. On the "Bot" tab, click "Reset Token" to reveal the bot's token
2. **Important:** Treat this token like a password. Never share it or commit it to version control
3. Copy this token for your environment file (`.env.dev` or `.env.prod`)

### 4. Enable Privileged Gateway Intents

On the "Bot" tab, scroll down to "Privileged Gateway Intents" and enable:
- **Message Content Intent** (required for reading messages)

### 5. Set Bot Permissions and Scopes

1. Navigate to "OAuth2" → "URL Generator"
2. In "Scopes", select:
   - `bot`
   - `applications.commands`
3. In "Bot Permissions", select:
   - `Send Messages`
   - `Use Slash Commands`
   - `Embed Links`

### 6. Invite the Bot to Your Server

1. Copy the below generated URL from the URL Generator
2. Paste it into your browser
3. Select your server (requires "Manage Server" permission)
4. Authorize the bot

### 7. Get Your Server ID

1. Enable Developer Mode in Discord: User Settings → Advanced (has an icon with three dots) → Toggle Developer Mode
2. Right-click your server icon → "Copy Server ID"
3. Copy this for your environment file (`.env.dev` or `.env.prod`)

## GitHub Personal Access Token Setup

The bot needs a GitHub personal access token to create issues on your behalf.

### 1. Create a Fine-Grained Personal Access Token

1. Go to [GitHub Settings → Developer settings → Personal access tokens → Fine-grained tokens](https://github.com/settings/tokens?type=beta)
2. Click "Generate new token"
3. Give your token a descriptive name (e.g., "Meshtastic Discord Bot Prod")
4. Set an expiration date (recommended: 90 days or custom)
5. Under "Repository access", select either:
   - **Only select repositories** (recommended): Choose the specific repositories where the bot should create issues
   - **All repositories**: If you want the bot to have access to all your repositories

### 2. Configure Permissions

Under "Permissions" → "Repository permissions":
1. Find **Issues** in the list
2. Set the access level to **Read and write**
3. This is the only permission required for the bot to function

### 3. Generate and Save the Token

1. Click "Generate token" at the bottom of the page
2. **Important:** Copy the token immediately - you won't be able to see it again
3. Treat this token like a password. Never share it or commit it to version control
4. Copy this token for your environment file (`.env.dev` or `.env.prod`)

## Local Development

### Prerequisites

- Docker installed
- Discord bot token (see Discord Bot Setup above)
- GitHub personal access token (see GitHub Personal Access Token Setup above)

### Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/meshtastic/meshtastic-bot.git
   cd meshtastic-bot
   ```

2. **Create your development environment file:**
   ```bash
   cp .env.example .env.dev
   ```

3. **Edit `.env.dev` with your development credentials:**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   GITHUB_TOKEN=your_github_personal_access_token
   DISCORD_SERVER_ID=your_server_id
   CONFIG_PATH=config.yaml
   FAQ_PATH=faq.yaml
   HEALTHCHECK_PORT=8081  # Different from production (8080) to avoid port conflicts
   ENV=dev
   ```

4. **Run the bot using Docker:**
   ```bash
   ./run.sh .env.dev
   ```

The bot will start in development mode with verbose logging. Slash commands will register instantly to your test server.

### Running Tests

#### Basic Test Commands

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

#### Advanced Testing

```bash
# Run with coverage
go tool cover -html=coverage.out

# Run with race detection
go test ./... -race

# Run tests in parallel
go test ./... -parallel 4
```

## Configuration Files

### config.yaml

Defines command-to-GitHub-repository mappings:

```yaml
config:
  - command: bug
    template_url: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml
    channel_id:
      - '123456789'
    title: Bug Report
  - command: feature
    template_url: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/feature.yml
    channel_id:
      - '123456789'
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

## Deployment

### Prerequisites

- Docker installed on your server
- Discord bot token and GitHub token (see setup sections above)

### Production Setup

1. **Create production environment file:**
   ```bash
   cp .env.example .env.prod
   ```

2. **Edit `.env.prod` with your production credentials:**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   GITHUB_TOKEN=your_github_personal_access_token
   DISCORD_SERVER_ID=your_server_id
   CONFIG_PATH=config.yaml
   FAQ_PATH=faq.yaml
   HEALTHCHECK_PORT=8080
   ENV=prod
   ```

3. **Deploy the bot:**
   ```bash
   ./run.sh .env.prod
   ```

   > The `run.sh` script uses your local Docker instance to build and run the bot, creating a new container tagged `meshtastic-bot`.

The bot will start in production mode. Slash commands will register globally (may take up to 1 hour to propagate).

### Production Management

```bash
# View logs
docker logs -f meshtastic-bot

# Restart the bot
docker restart meshtastic-bot

# Update and redeploy
git pull
./run.sh .env.prod

# Check health
curl http://localhost:8080/health
```

### CI/CD Pipeline

The project uses GitHub Actions for continuous integration.

**Workflow:** `.github/workflows/ci.yml` and `.github/workflows/pr.yml`

**On Pull Requests & Pushes:**
- Unit tests 
- Build verification
- Docker image build

**On Main Branch Push:**
- All CI checks (after all checks pass)
- Docker image validation

## Project Structure

```
meshtastic-bot/
├── cmd/
│   └── meshtastic-bot/
│       └── main.go          # Application entry point
├── internal/                # Internal packages
│   ├── config/              # Configuration loading and validation
│   │   ├── config.go        # Main config and URL parsing
│   │   ├── env.go           # Environment variable handling
│   │   ├── faq.go           # FAQ data structures
│   │   └── modal.go         # Modal configuration
│   ├── discord/             # Discord bot implementation
│   │   ├── bot.go           # Bot initialization
│   │   ├── commands.go      # Slash command definitions
│   │   ├── handlers.go      # Command handlers
│   │   └── handlers/        # Individual handler implementations
│   ├── github/              # GitHub API client
│   │   └── client.go
│   └── routes/              # HTTP routes and health checks
│       └── routes.go
├── .github/                 # CI/CD workflows
│   └── workflows/
│       ├── ci.yml           # Main CI/CD pipeline
│       └── pr.yml           # Pull request checks
├── config.yaml              # Command configuration
├── faq.yaml                 # FAQ content
├── run.sh                   # Docker runner script
└── Dockerfile               # Multi-stage Docker build
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DISCORD_TOKEN` | Yes | - | Discord bot token |
| `DISCORD_SERVER_ID` | Yes | - | Target Discord server ID |
| `GITHUB_TOKEN` | Yes | - | GitHub personal access token |
| `CONFIG_PATH` | No | `config.yaml` | Path to config.yaml |
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
- Container orchestration monitoring
- Load balancers and reverse proxies

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

### Testing Slash Commands

After deploying slash commands, they may take up to an hour to propagate globally in production mode. For faster testing:
- Use development mode (`.env.dev`) for instant command registration to your test server
- Test in your development server first before deploying to production

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
4. Check template URLs in `config.yaml` are valid, verify the url by pasting it into your web browser. 
4. Ensure bot has read/write access in the github token to target repository
5. Check logs for specific error messages

### Docker build fails

1. Ensure Go 1.25+ is specified in Dockerfile
2. Verify all dependencies are in `go.mod`, try running `go mod tidy` to ensure all dependencies are downloaded.
3. Check for syntax errors: `go build .`

## License

See LICENSE.md in this repo for more details.

## Support

- Documentation: [Meshtastic Docs](https://meshtastic.org/docs)
- Discord: [Meshtastic Discord](https://discord.gg/meshtastic)
- Issues: [GitHub Issues](https://github.com/meshtastic/meshtastic-bot/issues)
