#!/bin/bash
set -e

# Check for docker-compose or podman-compose
if command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
elif command -v podman-compose &> /dev/null; then
    COMPOSE_CMD="podman-compose"
else
    echo "Error: Neither docker-compose nor podman-compose is available"
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

# Export ENV_FILE so docker-compose can use it
export ENV_FILE

# Pass remaining arguments to compose command (skip first arg which is env file)
echo "Running $COMPOSE_CMD in detached mode..."
$COMPOSE_CMD up --build -d "${@:2}"
