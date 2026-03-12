#!/bin/bash
# restart.sh - Restart the dingtalk-bridge service
#
# Usage:
#   ./restart.sh [--build] [--foreground]
#
# Options:
#   --build      Rebuild the binary before starting
#   --foreground Run in foreground (default: background)
#
# Environment:
#   The script sources .env file if present

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

BINARY_NAME="dingtalk-bridge"
MAIN_PATH="./cmd/dingtalk-bridge"
PID_FILE=".bridge.pid"
LOG_FILE="bridge.log"

# Parse arguments
BUILD=false
FOREGROUND=false

for arg in "$@"; do
    case $arg in
        --build)
            BUILD=true
            shift
            ;;
        --foreground)
            FOREGROUND=true
            shift
            ;;
        *)
            echo "Unknown option: $arg"
            echo "Usage: $0 [--build] [--foreground]"
            exit 1
            ;;
    esac
done

# Load environment variables
if [ -f ".env" ]; then
    echo "Loading environment from .env..."
    set -a
    source .env
    set +a
fi

# Function to stop the bridge
stop_bridge() {
    echo "Stopping dingtalk-bridge..."
    
    # Kill by PID file
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Sending SIGTERM to PID $PID..."
            kill -TERM "$PID"
            
            # Wait for graceful shutdown (max 10 seconds)
            for i in {1..10}; do
                if ! kill -0 "$PID" 2>/dev/null; then
                    echo "Process $PID stopped gracefully"
                    break
                fi
                sleep 1
            done
            
            # Force kill if still running
            if kill -0 "$PID" 2>/dev/null; then
                echo "Force killing PID $PID..."
                kill -9 "$PID" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi
    
    # Kill any orphan processes by name
    pkill -f "dingtalk-bridge" 2>/dev/null || true
    pkill -f "go run.*cmd/dingtalk-bridge" 2>/dev/null || true
    
    echo "Bridge stopped."
}

# Function to start the bridge
start_bridge() {
    echo "Starting dingtalk-bridge..."
    
    if [ "$FOREGROUND" = true ]; then
        # Run in foreground
        go run "$MAIN_PATH"
    else
        # Run in background
        if [ "$BUILD" = true ]; then
            echo "Building binary..."
            go build -o "$BINARY_NAME" "$MAIN_PATH"
        fi
        
        # Use compiled binary if exists, otherwise go run
        if [ -f "$BINARY_NAME" ]; then
            echo "Starting compiled binary..."
            nohup ./"$BINARY_NAME" >> "$LOG_FILE" 2>&1 &
        else
            echo "Starting with go run..."
            nohup go run "$MAIN_PATH" >> "$LOG_FILE" 2>&1 &
        fi
        
        echo $! > "$PID_FILE"
        sleep 1
        
        if kill -0 $(cat "$PID_FILE") 2>/dev/null; then
            echo "Bridge started with PID $(cat $PID_FILE)"
            echo "Logs: tail -f $LOG_FILE"
        else
            echo "Failed to start bridge. Check $LOG_FILE for errors."
            exit 1
        fi
    fi
}

# Main
echo "=== dingtalk-bridge restart ==="
echo "Time: $(date)"
echo ""

stop_bridge
echo ""
start_bridge

echo ""
echo "=== Restart complete ==="