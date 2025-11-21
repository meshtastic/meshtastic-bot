#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
ENV_FILE=".env.prod"
FLY_CONFIG="fly.toml"

echo -e "${GREEN}=== Fly.io Deployment Script ===${NC}\n"

# Check if flyctl is installed
if ! command -v fly &> /dev/null; then
    echo -e "${RED}Error: flyctl is not installed${NC}"
    echo "Install it from: https://fly.io/docs/hands-on/install-flyctl/"
    exit 1
fi

# Check if fly.toml exists
if [[ ! -f "$FLY_CONFIG" ]]; then
    echo -e "${RED}Error: $FLY_CONFIG not found${NC}"
    exit 1
fi

# Extract app name from fly.toml
APP_NAME=$(grep '^app = ' "$FLY_CONFIG" | sed "s/app = '\(.*\)'/\1/")

if [[ -z "$APP_NAME" ]]; then
    echo -e "${RED}Error: Could not extract app name from $FLY_CONFIG${NC}"
    exit 1
fi

# Check if app exists, create if it doesn't
echo -e "${YELLOW}Checking if app '$APP_NAME' exists...${NC}"
if ! fly status --config "$FLY_CONFIG" &>/dev/null; then
    echo -e "${YELLOW}App '$APP_NAME' does not exist. Creating...${NC}"
    fly apps create "$APP_NAME"
    echo -e "${GREEN}✓ App created${NC}\n"
else
    echo -e "${GREEN}✓ App exists${NC}\n"
fi

# Parse options
SKIP_CONFIRMATION=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -y|--yes)
            SKIP_CONFIRMATION=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Usage: $0 [-y|--yes]"
            exit 1
            ;;
    esac
done

# Sync secrets from .env.prod
if [[ ! -f "$ENV_FILE" ]]; then
    echo -e "${RED}Error: $ENV_FILE not found${NC}"
    exit 1
fi

echo -e "${YELLOW}Syncing secrets from $ENV_FILE...${NC}"
cat "$ENV_FILE" | grep -v '^#' | grep -v '^$' | fly secrets import
echo -e "${GREEN}✓ Secrets synced${NC}\n"

# Show current app info
echo -e "${YELLOW}Current app configuration:${NC}"
fly status --config "$FLY_CONFIG" 2>/dev/null || echo "App not yet deployed"
echo ""

# Confirmation prompt
if [[ "$SKIP_CONFIRMATION" == false ]]; then
    read -p "Deploy to fly.io? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Deployment cancelled"
        exit 0
    fi
fi

# Deploy
echo -e "\n${YELLOW}Deploying to fly.io...${NC}"
fly deploy --config "$FLY_CONFIG"

# Show final status
echo -e "\n${GREEN}=== Deployment Complete ===${NC}"
fly status --config "$FLY_CONFIG"

echo -e "\n${GREEN}✓ Deployment successful!${NC}"
echo -e "View logs: ${YELLOW}fly logs${NC}"
echo -e "Open app: ${YELLOW}fly open${NC}"
