#!/bin/bash
# Deploy script for bud on Mac Mini
# Usage: ./deploy.sh [--config /path/to/bud.yaml] [--no-restart]

set -e

# Parse arguments
BUD_CONFIG_PATH=""
NO_RESTART=false
for arg in "$@"; do
    case "$arg" in
        --config=*)
            BUD_CONFIG_PATH="${arg#--config=}"
            ;;
        --config)
            # handled in next iteration
            ;;
        --no-restart)
            NO_RESTART=true
            ;;
    esac
done

# Ensure Homebrew paths are available (Apple Silicon + Intel)
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

BUD_DIR="${BUD_DIR:-/Users/thunder/src/bud2}"
LOG_FILE="${BUD_LOG:-/Users/thunder/Library/Logs/bud-deploy.log}"

cd "$BUD_DIR"

echo "$(date): Starting deploy..." >> "$LOG_FILE"

# Pull latest code
echo "Pulling latest code..."
git pull origin main >> "$LOG_FILE" 2>&1

# Build
echo "Building bud..."
go build -tags "fts5" -o bin/bud ./cmd/bud >> "$LOG_FILE" 2>&1

# Codesign with self-signed cert so macOS TCC permissions persist across restarts
codesign --sign "bud-dev" --force --deep bin/bud >> "$LOG_FILE" 2>&1

echo "$(date): Build complete" >> "$LOG_FILE"

# Restart unless --no-restart flag
if [ "$NO_RESTART" = "false" ]; then
    echo "Restarting bud service..."
    if [ -n "$BUD_CONFIG_PATH" ]; then
        export BUD_CONFIG="$BUD_CONFIG_PATH"
    fi
    launchctl kickstart -k gui/$(id -u)/com.bud.daemon 2>/dev/null || \
        launchctl stop com.bud.daemon 2>/dev/null || true
    sleep 1
    launchctl start com.bud.daemon 2>/dev/null || true
    echo "$(date): Service restarted" >> "$LOG_FILE"
fi

DEPLOY_TIME=$(date -Iseconds)
echo "$DEPLOY_TIME" > /tmp/bud-deploy-success
echo "$(date): Deploy complete!" >> "$LOG_FILE"

echo "Deploy complete! (timestamp written to /tmp/bud-deploy-success)"
