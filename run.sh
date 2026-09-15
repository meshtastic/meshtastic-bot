#!/bin/bash
set -e

# Check for docker or podman
if command -v docker &> /dev/null; then
    DOCKER_CMD="docker"
elif command -v podman &> /dev/null; then
    DOCKER_CMD="podman"
else
    echo "Error: Neither docker nor podman is available"
    echo "Please install one of them to continue"
    exit 1
fi

# Default to .env.dev if no env file specified
ENV_FILE=${1:-.env.dev}

# Check if env file exists
if [[ ! -f "$ENV_FILE" ]]; then
    echo "Error: Environment file '$ENV_FILE' not found"
    exit 1
fi

echo "Loading environment from: $ENV_FILE"

# Container and image names
CONTAINER_NAME="meshtastic-bot"
IMAGE_NAME="meshtastic-bot:latest"

# Stop and remove existing container if it exists
if $DOCKER_CMD ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping and removing existing container..."
    $DOCKER_CMD stop $CONTAINER_NAME 2>/dev/null || true
    $DOCKER_CMD rm $CONTAINER_NAME 2>/dev/null || true
fi

# Build the image
echo "Building Docker image..."
if [[ "$DOCKER_CMD" == "docker" ]]; then
    DOCKER_BUILDKIT=1 $DOCKER_CMD build -t $IMAGE_NAME .
else
    $DOCKER_CMD build -t $IMAGE_NAME .
fi

# Get healthcheck port from env file or use default
HEALTHCHECK_PORT=$(grep -E '^HEALTHCHECK_PORT=' "$ENV_FILE" 2>/dev/null | cut -d '=' -f2 || echo "8080")

# Run the container
echo "Running container in detached mode..."
$DOCKER_CMD run -d \
    --name $CONTAINER_NAME \
    --env-file "$ENV_FILE" \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    -e CONFIG_PATH=/app/config.yaml \
    -e FAQ_PATH=/app/faq.yaml \
    -p "${HEALTHCHECK_PORT}:8080" \
    --restart unless-stopped \
    $IMAGE_NAME

echo "Container started successfully!"
echo "Health check available at: http://localhost:${HEALTHCHECK_PORT}/health"
