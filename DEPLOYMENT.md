# IPD Enhanced System - Deployment Guide

This guide covers deploying the IPD Enhanced Image Processing System in various environments.

## 🚀 Quick Start (Any Platform)

### Windows
```cmd
# Run the automated setup script
run.bat
```

### Linux/macOS
```bash
# Make script executable and run
chmod +x run.sh
./run.sh
```

### Manual Setup
```bash
# Build the application
cd main
go build -o ipd-enhanced

# Create test data
mkdir sample_images
echo "test data" > sample_images/test1.txt

# Run with 2 workers
echo "2" | ./ipd-enhanced ../sample_images
```

## 🏗️ Production Deployment

### 1. System Requirements

**Minimum:**
- CPU: 2 cores
- RAM: 4GB
- Storage: 10GB free space
- Network: Stable internet connection

**Recommended:**
- CPU: 4+ cores
- RAM: 8GB+
- Storage: 50GB+ SSD
- Network: Gigabit ethernet

### 2. Environment Setup

#### Create Deployment Directory
```bash
mkdir -p /opt/ipd-enhanced
cd /opt/ipd-enhanced

# Copy application files
cp -r /path/to/IPD/* .

# Set permissions
chmod +x main/ipd-enhanced
chmod +x scripts/process.py
```

#### Configure System Service (Linux)
```bash
# Create systemd service file
sudo tee /etc/systemd/system/ipd-enhanced.service > /dev/null <<EOF
[Unit]
Description=IPD Enhanced Image Processing System
After=network.target

[Service]
Type=simple
User=ipd
Group=ipd
WorkingDirectory=/opt/ipd-enhanced
ExecStart=/opt/ipd-enhanced/main/ipd-enhanced /opt/ipd-enhanced/input
Restart=always
RestartSec=10
Environment=IPD_LOG_LEVEL=INFO
Environment=IPD_MAX_WORKERS=4

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
sudo systemctl enable ipd-enhanced
sudo systemctl start ipd-enhanced
```

### 3. Configuration Management

#### Production Configuration
```json
{
  "system": {
    "max_workers": 8,
    "heartbeat_interval": "15s",
    "processing_timeout": "30m",
    "connection_timeout": "60s",
    "retry_attempts": 5,
    "external_script_path": "/opt/ipd-enhanced/scripts/process.py",
    "input_base_directory": "/data/input",
    "output_base_directory": "/data/output",
    "signaling_server_url": "wss://your-signaling-server.com:8443",
    "log_level": "INFO",
    "enable_metrics": true,
    "metrics_port": 9090
  },
  "network": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [
      "turn:your-turn-server.com:3478"
    ],
    "data_channel_timeout": "60s",
    "max_reconnect_attempts": 10,
    "reconnect_delay": "5s"
  },
  "file_transmission": {
    "chunk_size": 131072,
    "max_concurrent_files": 5,
    "transmission_timeout": "5m",
    "retry_attempts": 5,
    "enable_checksum": true
  }
}
```

#### Environment Variables for Production
```bash
# System Configuration
export IPD_CONFIG_PATH=/opt/ipd-enhanced/config/production.json
export IPD_MAX_WORKERS=8
export IPD_LOG_LEVEL=INFO
export IPD_INPUT_BASE_DIRECTORY=/data/input
export IPD_OUTPUT_BASE_DIRECTORY=/data/output

# Network Configuration
export IPD_SIGNALING_SERVER_URL=wss://your-signaling-server.com:8443
export IPD_TURN_SERVERS=turn:your-turn-server.com:3478

# Security
export IPD_ENABLE_METRICS=true
export IPD_METRICS_PORT=9090
```

## 🐳 Docker Deployment

### Dockerfile
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN cd main && go build -o ipd-enhanced

FROM python:3.11-alpine

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy application
COPY --from=builder /app/main/ipd-enhanced .
COPY --from=builder /app/scripts ./scripts
COPY --from=builder /app/config-example.json ./config.json

# Create directories
RUN mkdir -p /data/input /data/output /app/logs

# Set permissions
RUN chmod +x ipd-enhanced scripts/process.py

# Expose metrics port
EXPOSE 9090

# Set environment variables
ENV IPD_INPUT_BASE_DIRECTORY=/data/input
ENV IPD_OUTPUT_BASE_DIRECTORY=/data/output
ENV IPD_LOG_LEVEL=INFO

CMD ["./ipd-enhanced", "/data/input"]
```

### Docker Compose
```yaml
version: '3.8'

services:
  ipd-enhanced:
    build: .
    container_name: ipd-enhanced
    restart: unless-stopped
    volumes:
      - ./data/input:/data/input
      - ./data/output:/data/output
      - ./logs:/app/logs
      - ./config:/app/config
    environment:
      - IPD_CONFIG_PATH=/app/config/production.json
      - IPD_MAX_WORKERS=4
      - IPD_LOG_LEVEL=INFO
    ports:
      - "9090:9090"  # Metrics port
    networks:
      - ipd-network

  signaling-server:
    image: your-signaling-server:latest
    container_name: ipd-signaling
    restart: unless-stopped
    ports:
      - "8000:8000"
    networks:
      - ipd-network

networks:
  ipd-network:
    driver: bridge
```

### Build and Run
```bash
# Build and start services
docker-compose up -d

# View logs
docker-compose logs -f ipd-enhanced

# Scale workers (if supported)
docker-compose up -d --scale ipd-enhanced=3
```

## ☸️ Kubernetes Deployment

### ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ipd-config
data:
  config.json: |
    {
      "system": {
        "max_workers": 4,
        "heartbeat_interval": "30s",
        "processing_timeout": "20m",
        "log_level": "INFO",
        "enable_metrics": true,
        "metrics_port": 9090
      }
    }
```

### Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ipd-enhanced
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ipd-enhanced
  template:
    metadata:
      labels:
        app: ipd-enhanced
    spec:
      containers:
      - name: ipd-enhanced
        image: ipd-enhanced:latest
        ports:
        - containerPort: 9090
        env:
        - name: IPD_CONFIG_PATH
          value: "/config/config.json"
        - name: IPD_MAX_WORKERS
          value: "4"
        volumeMounts:
        - name: config-volume
          mountPath: /config
        - name: data-volume
          mountPath: /data
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "4Gi"
            cpu: "2"
      volumes:
      - name: config-volume
        configMap:
          name: ipd-config
      - name: data-volume
        persistentVolumeClaim:
          claimName: ipd-data-pvc
```

### Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: ipd-enhanced-service
spec:
  selector:
    app: ipd-enhanced
  ports:
  - port: 9090
    targetPort: 9090
  type: LoadBalancer
```

## 📊 Monitoring and Observability

### Prometheus Configuration
```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'ipd-enhanced'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 30s
    metrics_path: /metrics
```

### Grafana Dashboard
```json
{
  "dashboard": {
    "title": "IPD Enhanced Monitoring",
    "panels": [
      {
        "title": "Active Workers",
        "type": "stat",
        "targets": [
          {
            "expr": "ipd_active_workers"
          }
        ]
      },
      {
        "title": "Processing Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(ipd_processed_images_total[5m])"
          }
        ]
      }
    ]
  }
}
```

### Health Check Endpoint
```bash
# Check system health
curl http://localhost:9090/health

# Check metrics
curl http://localhost:9090/metrics
```

## 🔒 Security Configuration

### TLS Configuration
```json
{
  "network": {
    "signaling_server_url": "wss://secure-signaling.example.com:8443",
    "turn_servers": [
      "turns:secure-turn.example.com:5349"
    ],
    "enable_tls": true,
    "cert_file": "/etc/ssl/certs/ipd.crt",
    "key_file": "/etc/ssl/private/ipd.key"
  }
}
```

### Firewall Rules
```bash
# Allow required ports
sudo ufw allow 8000/tcp  # Signaling server
sudo ufw allow 9090/tcp  # Metrics
sudo ufw allow 3478/udp  # STUN/TURN
sudo ufw allow 5349/tcp  # TURNS
```

## 🚨 Troubleshooting Production Issues

### Common Production Issues

1. **High Memory Usage**
   ```bash
   # Monitor memory usage
   docker stats ipd-enhanced
   
   # Reduce workers if needed
   export IPD_MAX_WORKERS=2
   ```

2. **Connection Timeouts**
   ```bash
   # Increase timeouts
   export IPD_CONNECTION_TIMEOUT=120s
   export IPD_PROCESSING_TIMEOUT=60m
   ```

3. **Log Analysis**
   ```bash
   # View system logs
   tail -f /opt/ipd-enhanced/logs/system.log
   
   # Search for errors
   grep -i error /opt/ipd-enhanced/logs/*.log
   ```

### Performance Tuning
```bash
# Optimize for high throughput
export IPD_MAX_WORKERS=16
export IPD_CHUNK_SIZE=262144
export IPD_MAX_CONCURRENT_FILES=10

# Optimize for low latency
export IPD_HEARTBEAT_INTERVAL=10s
export IPD_CONNECTION_TIMEOUT=30s
```

## 📈 Scaling Strategies

### Horizontal Scaling
- Deploy multiple IPD instances
- Use load balancer for distribution
- Shared storage for input/output

### Vertical Scaling
- Increase worker count per instance
- Allocate more CPU and memory
- Use faster storage (NVMe SSD)

### Auto-scaling (Kubernetes)
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ipd-enhanced-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ipd-enhanced
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

This deployment guide provides comprehensive instructions for running the IPD Enhanced System in various environments, from development to production-scale deployments.