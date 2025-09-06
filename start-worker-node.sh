#!/bin/bash

# IPD Worker Node Startup Script
# Usage: ./start-worker-node.sh [worker-id] [coordinator-url] [signaling-url]

set -e

# Configuration
WORKER_ID=${1:-"worker-$(hostname)-$(date +%s)"}
COORDINATOR_URL=${2:-"ws://localhost:8000"}
SIGNALING_URL=${3:-"ws://localhost:8000"}

echo "=========================================="
echo "IPD Enhanced Worker Node"
echo "=========================================="
echo "Worker ID: $WORKER_ID"
echo "Coordinator: $COORDINATOR_URL"
echo "Signaling Server: $SIGNALING_URL"
echo "=========================================="

# Check prerequisites
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    exit 1
fi

if ! command -v python3 &> /dev/null; then
    echo "❌ Error: Python 3 is not installed"
    exit 1
fi

# Build worker application if needed
if [ ! -f "main/ipd-worker-node" ]; then
    echo "🔨 Building worker node application..."
    cd main
    go build -o ipd-worker-node worker-node.go worker.go config.go result_collector.go file_transmitter.go script_executor.go error_handling.go retry_logic.go graceful_degradation.go enhanced_logging.go error_recovery_integration.go task.go
    cd ..
    echo "✅ Build completed"
fi

# Create worker directories
mkdir -p worker_input worker_output logs/worker

# Set environment variables
export IPD_MODE=worker-node
export IPD_WORKER_ID="$WORKER_ID"
export IPD_COORDINATOR_URL="$COORDINATOR_URL"
export IPD_SIGNALING_SERVER_URL="$SIGNALING_URL"
export IPD_LOG_LEVEL=INFO
export IPD_INPUT_BASE_DIRECTORY="./worker_input"
export IPD_OUTPUT_BASE_DIRECTORY="./worker_output"

# Create worker configuration
cat > worker-config.json << EOF
{
  "system": {
    "heartbeat_interval": "30s",
    "processing_timeout": "10m",
    "connection_timeout": "60s",
    "retry_attempts": 3,
    "external_script_path": "./scripts/process.py",
    "input_base_directory": "./worker_input",
    "output_base_directory": "./worker_output",
    "log_level": "INFO"
  },
  "network": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "data_channel_timeout": "60s",
    "max_reconnect_attempts": 10,
    "reconnect_delay": "5s"
  }
}
EOF

export IPD_CONFIG_PATH="./worker-config.json"

echo "🚀 Starting worker node..."
echo "📝 Logs will be written to: logs/worker/"
echo "📁 Input directory: worker_input/"
echo "📁 Output directory: worker_output/"
echo ""
echo "Press Ctrl+C to stop the worker node"
echo ""

# Start worker node
cd main
./ipd-worker-node

echo ""
echo "🛑 Worker node stopped"