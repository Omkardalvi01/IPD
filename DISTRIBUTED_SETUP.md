# IPD Distributed Worker Setup Guide

This guide explains how to run IPD workers on separate machines for true distributed processing.

## 🏗️ Architecture Overview

The IPD system uses WebRTC for peer-to-peer connections between the main server and worker nodes:

```
┌─────────────────┐    WebRTC     ┌─────────────────┐
│   Main Server   │◄─────────────►│   Worker Node 1 │
│   (Coordinator) │               │                 │
└─────────────────┘               └─────────────────┘
         │                                 
         │         WebRTC                  
         ▼                                 
┌─────────────────┐               ┌─────────────────┐
│ Signaling Server│               │   Worker Node 2 │
│   (WebRTC)      │               │                 │
└─────────────────┘               └─────────────────┘
         │                                 
         │         WebRTC                  
         ▼                                 
┌─────────────────┐               ┌─────────────────┐
│   Worker Node 3 │               │   Worker Node N │
│                 │               │                 │
└─────────────────┘               └─────────────────┘
```

## 🚀 Quick Setup for Distributed Workers

### Prerequisites for All Machines
- Go 1.19+
- Python 3.7+
- Network connectivity between all nodes
- WebRTC signaling server (required for distributed setup)

## 📋 Step-by-Step Distributed Setup

### Step 1: Set Up Signaling Server

First, you need a WebRTC signaling server. Here's a simple Node.js signaling server:

#### Install Node.js Signaling Server
```bash
# On the signaling server machine
mkdir ipd-signaling-server
cd ipd-signaling-server

# Create package.json
cat > package.json << EOF
{
  "name": "ipd-signaling-server",
  "version": "1.0.0",
  "dependencies": {
    "ws": "^8.0.0",
    "express": "^4.18.0"
  }
}
EOF

# Install dependencies
npm install
```

#### Create Signaling Server
```javascript
// server.js
const WebSocket = require('ws');
const express = require('express');
const http = require('http');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

const clients = new Map();

wss.on('connection', (ws) => {
    console.log('New client connected');
    
    ws.on('message', (message) => {
        try {
            const data = JSON.parse(message);
            console.log('Received:', data.type);
            
            // Handle different message types
            switch (data.type) {
                case 'register':
                    clients.set(data.id, ws);
                    console.log(`Client ${data.id} registered`);
                    break;
                    
                case 'offer':
                case 'answer':
                case 'ice-candidate':
                    // Forward to target client
                    const targetWs = clients.get(data.target);
                    if (targetWs && targetWs.readyState === WebSocket.OPEN) {
                        targetWs.send(message);
                    }
                    break;
            }
        } catch (error) {
            console.error('Error processing message:', error);
        }
    });
    
    ws.on('close', () => {
        // Remove client from map
        for (const [id, client] of clients.entries()) {
            if (client === ws) {
                clients.delete(id);
                console.log(`Client ${id} disconnected`);
                break;
            }
        }
    });
});

const PORT = process.env.PORT || 8000;
server.listen(PORT, () => {
    console.log(`IPD Signaling Server running on port ${PORT}`);
});
```

#### Start Signaling Server
```bash
node server.js
```

### Step 2: Set Up Main Server (Coordinator)

On the main server machine:

#### Create Main Server Configuration
```bash
# On main server machine
cd IPD
cp config-example.json config-main.json
```

Edit `config-main.json`:
```json
{
  "system": {
    "max_workers": 8,
    "heartbeat_interval": "30s",
    "processing_timeout": "20m",
    "connection_timeout": "60s",
    "retry_attempts": 5,
    "signaling_server_url": "ws://SIGNALING_SERVER_IP:8000",
    "log_level": "INFO",
    "enable_metrics": true,
    "metrics_port": 9090
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
```

#### Build and Run Main Server
```bash
cd main
go build -o ipd-main-server

# Set configuration
export IPD_CONFIG_PATH=../config-main.json
export IPD_ROLE=coordinator

# Run main server (this will wait for workers to connect)
./ipd-main-server ../input_images
```

### Step 3: Set Up Worker Nodes

On each worker machine:

#### Create Worker Application
```bash
# On each worker machine
cd IPD
cp config-example.json config-worker.json
```

Edit `config-worker.json`:
```json
{
  "system": {
    "signaling_server_url": "ws://SIGNALING_SERVER_IP:8000",
    "log_level": "INFO",
    "input_base_directory": "./worker_input",
    "output_base_directory": "./worker_output"
  },
  "network": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ]
  }
}
```

#### Create Worker-Only Application
```bash
# Create a worker-only version
cd main
go build -o ipd-worker

# Create worker startup script
cat > start-worker.sh << 'EOF'
#!/bin/bash

WORKER_ID=${1:-$(hostname)}
SIGNALING_SERVER=${2:-"ws://localhost:8000"}

echo "Starting IPD Worker Node"
echo "Worker ID: $WORKER_ID"
echo "Signaling Server: $SIGNALING_SERVER"

export IPD_CONFIG_PATH=../config-worker.json
export IPD_ROLE=worker
export IPD_WORKER_ID=$WORKER_ID
export IPD_SIGNALING_SERVER_URL=$SIGNALING_SERVER

# Create worker directories
mkdir -p ../worker_input ../worker_output ../logs

# Start worker
./ipd-worker
EOF

chmod +x start-worker.sh
```

#### Start Worker Nodes
```bash
# On worker machine 1
./start-worker.sh worker-1 ws://SIGNALING_SERVER_IP:8000

# On worker machine 2
./start-worker.sh worker-2 ws://SIGNALING_SERVER_IP:8000

# On worker machine N
./start-worker.sh worker-N ws://SIGNALING_SERVER_IP:8000
```

## 🔧 Current System Modification Required

The current IPD system needs modifications to support true distributed workers. Here's what needs to be implemented:

### Required Code Changes

#### 1. Create Worker-Only Mode
```go
// Add to main.go
func runAsWorker() {
    workerID := os.Getenv("IPD_WORKER_ID")
    signalingURL := os.Getenv("IPD_SIGNALING_SERVER_URL")
    
    fmt.Printf("🔧 Starting as Worker Node: %s\n", workerID)
    fmt.Printf("📡 Connecting to signaling server: %s\n", signalingURL)
    
    // Initialize single worker that connects to main server
    config := LoadProcessingConfigFromEnv()
    worker := NewPersistentWorker(0, workerID, nil, nil, config)
    
    // Connect to signaling server and wait for coordinator
    if err := worker.ConnectToCoordinator(signalingURL); err != nil {
        log.Fatalf("Failed to connect to coordinator: %v", err)
    }
    
    // Keep worker running
    select {}
}

func main() {
    role := os.Getenv("IPD_ROLE")
    
    switch role {
    case "worker":
        runAsWorker()
    case "coordinator":
        runAsCoordinator()
    default:
        // Current single-machine mode
        runSingleMachine()
    }
}
```

#### 2. Add Network Discovery
```go
// Add to worker.go
func (pw *PersistentWorker) ConnectToCoordinator(signalingURL string) error {
    // Connect to signaling server
    conn, err := websocket.Dial(signalingURL, "", "http://localhost/")
    if err != nil {
        return fmt.Errorf("failed to connect to signaling server: %w", err)
    }
    
    // Register as worker
    registerMsg := map[string]interface{}{
        "type": "register",
        "id":   pw.worker_id,
        "role": "worker",
    }
    
    if err := websocket.JSON.Send(conn, registerMsg); err != nil {
        return fmt.Errorf("failed to register with signaling server: %w", err)
    }
    
    // Wait for coordinator connection
    return pw.handleSignalingMessages(conn)
}
```

## 🚀 Simplified Distributed Setup (Current Workaround)

Since the current system needs modifications, here's how to simulate distributed processing:

### Method 1: SSH-Based Distribution

#### On Main Server
```bash
# Create distribution script
cat > distribute-work.sh << 'EOF'
#!/bin/bash

WORKERS=("worker1.example.com" "worker2.example.com" "worker3.example.com")
INPUT_DIR="$1"
TOTAL_FILES=$(ls -1 "$INPUT_DIR" | wc -l)
FILES_PER_WORKER=$((TOTAL_FILES / ${#WORKERS[@]}))

echo "Distributing $TOTAL_FILES files across ${#WORKERS[@]} workers"

# Split input files
split_dir="./split_input"
mkdir -p "$split_dir"

# Create worker directories
for i in "${!WORKERS[@]}"; do
    worker_dir="$split_dir/worker_$i"
    mkdir -p "$worker_dir"
done

# Distribute files
file_count=0
worker_index=0
for file in "$INPUT_DIR"/*; do
    cp "$file" "$split_dir/worker_$worker_index/"
    ((file_count++))
    
    if [ $((file_count % FILES_PER_WORKER)) -eq 0 ]; then
        ((worker_index++))
    fi
done

# Send work to each worker
for i in "${!WORKERS[@]}"; do
    worker="${WORKERS[$i]}"
    echo "Sending work to $worker..."
    
    # Copy files to worker
    scp -r "$split_dir/worker_$i" "$worker:/tmp/ipd_work/"
    
    # Start processing on worker
    ssh "$worker" "cd /opt/ipd && echo '1' | ./main/ipd-enhanced /tmp/ipd_work/worker_$i" &
done

# Wait for all workers to complete
wait

# Collect results
mkdir -p "./distributed_results"
for i in "${!WORKERS[@]}"; do
    worker="${WORKERS[$i]}"
    echo "Collecting results from $worker..."
    scp -r "$worker:/opt/ipd/shared_results" "./distributed_results/worker_$i/"
done

echo "Distributed processing complete!"
EOF

chmod +x distribute-work.sh
```

#### Run Distributed Processing
```bash
./distribute-work.sh ./input_images
```

### Method 2: Docker Swarm Distribution

#### Create Docker Service
```yaml
# docker-compose.distributed.yml
version: '3.8'

services:
  ipd-coordinator:
    build: .
    image: ipd-enhanced:latest
    environment:
      - IPD_ROLE=coordinator
      - IPD_MAX_WORKERS=0  # Workers will connect remotely
    volumes:
      - ./input:/data/input
      - ./results:/data/output
    networks:
      - ipd-network
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role == manager

  ipd-worker:
    image: ipd-enhanced:latest
    environment:
      - IPD_ROLE=worker
      - IPD_COORDINATOR_URL=ipd-coordinator:8000
    networks:
      - ipd-network
    deploy:
      replicas: 4
      placement:
        constraints:
          - node.role == worker

networks:
  ipd-network:
    driver: overlay
```

#### Deploy to Swarm
```bash
# Initialize swarm
docker swarm init

# Add worker nodes
docker swarm join-token worker

# Deploy stack
docker stack deploy -c docker-compose.distributed.yml ipd-stack
```

## 📊 Monitoring Distributed Workers

### Check Worker Status
```bash
# On main server
curl http://localhost:9090/workers/status

# Check individual worker
curl http://worker1.example.com:9090/health
```

### Aggregate Logs
```bash
# Collect logs from all workers
for worker in worker1 worker2 worker3; do
    echo "=== Logs from $worker ==="
    ssh "$worker" "tail -n 50 /opt/ipd/logs/system.log"
done
```

## 🔒 Security Considerations for Distributed Setup

### Network Security
```bash
# Firewall rules for worker nodes
sudo ufw allow from MAIN_SERVER_IP to any port 8000
sudo ufw allow from MAIN_SERVER_IP to any port 9090
sudo ufw deny 8000
sudo ufw deny 9090
```

### Authentication
```bash
# Use SSH keys for secure communication
ssh-keygen -t rsa -b 4096
ssh-copy-id user@worker1.example.com
```

## 🎯 Summary

**Current State**: The IPD system is designed for distributed processing but currently runs all workers on the same machine.

**To Run on Separate Machines**: You need:
1. **WebRTC Signaling Server** (Node.js example provided)
2. **Modified main.go** to support coordinator/worker roles
3. **Network configuration** for inter-machine communication
4. **Proper firewall and security setup**

**Workarounds Available**:
- SSH-based work distribution
- Docker Swarm deployment
- Manual file splitting and result aggregation

The system has all the building blocks for distributed processing - it just needs the networking layer to be extended for multi-machine deployment!