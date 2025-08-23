# IPD - Intelligent Processing and Distribution System

## Overview
IPD is a **distributed edge computing system** that combines **WebRTC peer-to-peer communication**, **worker pools**, and **machine learning** to create an intelligent data processing and distribution network. The system has been enhanced to transform edge nodes into distributed YOLO training nodes for object detection.

## Architecture

### System Components

1. **Worker/Sender (`main/`)** - Scans directories and sends files via WebRTC
2. **Edge Node (`edge/`)** - Receives files and performs distributed YOLO training
3. **Signaling Server (`server/`)** - WebSocket server for WebRTC connection setup
4. **Networking (`networking/`)** - WebRTC configuration and communication utilities
5. **ML Training (`ml/`)** - Python-based YOLO training engine

### Data Flow
```
[Worker Pool] → [Signaling Server] → [Edge Node] → [YOLO Training]
     ↓                    ↓              ↓              ↓
  File Files         SDP Exchange    File Storage    ML Training
  Processing         Role Routing    Image Detection  Model Updates
```

## Features

### Core P2P System
- **WebRTC-based file transfer** for direct peer-to-peer communication
- **Configurable worker pools** for scalable file processing
- **Unique connection IDs** for secure peer matching
- **Real-time signaling** via WebSocket server

### Distributed YOLO Training
- **YOLOv8n (nano) model** for optimal speed/accuracy balance
- **Batch-based training** with configurable batch sizes
- **Incremental learning** from new images
- **Pseudo-annotation generation** using current model
- **Local model weight storage** for edge persistence
- **Training metrics logging** and statistics tracking

## Installation

### Prerequisites
- Go 1.24.4 or later
- Python 3.8+ with pip
- Git

### Setup

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd IPD
   ```

2. **Install Python dependencies**
   ```bash
   cd ml
   pip install -r requirements.txt
   ```

3. **Build Go components**
   ```bash
   # Build worker
   cd main && go build -o main.exe .
   
   # Build edge node
   cd ../edge && go build -o edge.exe .
   
   # Build signaling server
   cd ../server && go build -o server.exe .
   ```

## Usage

### 1. Start the Signaling Server
```bash
cd server
./server.exe
# Server runs on port 8000
```

### 2. Start Edge Node(s)
```bash
cd edge
./edge.exe
# Enter directory name for file storage
# Enter unique connection ID
```

### 3. Start Worker
```bash
cd main
./main.exe
# Enter number of workers (recommended < CPU cores)
# Worker will scan ./train directory for files
```

## Configuration

### YOLO Training Parameters
- **Batch Size**: Default 8 images per batch
- **Learning Rate**: Default 0.001
- **Epochs per Batch**: Default 1
- **Image Size**: 640x640 pixels
- **Device**: CPU (optimized for edge devices)

### Supported Image Formats
- JPEG (.jpg, .jpeg)
- PNG (.png)
- GIF (.gif)
- BMP (.bmp)
- WebP (.webp)

## Directory Structure

```
IPD/
├── main/                 # Worker/Sender components
│   ├── main.go          # Main orchestrator
│   ├── worker.go        # Worker pool implementation
│   └── task.go          # File processing utilities
├── edge/                 # Edge node with YOLO training
│   ├── edge.go          # Main edge node logic
│   └── yolo_trainer.go  # Go YOLO trainer coordinator
├── server/               # Signaling server
│   └── server.go        # WebSocket signaling server
├── networking/           # WebRTC utilities
│   ├── config.go        # WebRTC configuration
│   ├── signaling.go     # WebSocket communication
│   └── user_comms.go    # Peer connection management
├── ml/                   # Python ML components
│   ├── yolo_trainer.py  # YOLO training engine
│   ├── requirements.txt  # Python dependencies
│   └── test_yolo.py     # Dependency test script
└── README.md            # This file
```

### Auto-created Directories
The system automatically creates these directories:
- `temp_images/` - Temporary image storage
- `yolo_weights/` - Trained YOLO model weights
- `yolo_logs/` - Batch training logs
- `yolo_training/images/train/` - YOLO training images
- `yolo_training/images/val/` - YOLO validation images
- `yolo_training/labels/train/` - YOLO training annotations
- `yolo_training/labels/val/` - YOLO validation annotations

## Training Process

### 1. Image Reception
- Edge node receives images via WebRTC data channels
- Images are stored in temporary directory
- File type validation (image files only)

### 2. Batch Processing
- Images are added to current training batch
- Pseudo-annotations generated using current model
- Batch triggers training when full (8 images)

### 3. YOLO Training
- 20% of batch moved to validation set
- YOLOv8n training with specified parameters
- Model weights updated and saved locally
- Training metrics logged and tracked

### 4. Model Persistence
- Updated weights saved as timestamped files
- Training logs stored in JSON format
- Statistics tracked across training sessions

## Performance Characteristics

### Edge Training Optimization
- **CPU-focused training** for edge device compatibility
- **Small batch sizes** to manage memory constraints
- **Incremental learning** for continuous improvement
- **Efficient data handling** with minimal disk I/O

### Scalability Features
- **Configurable worker pools** based on system resources
- **Parallel file processing** across multiple workers
- **Independent edge nodes** for distributed training
- **Non-blocking operations** for continuous processing

## Monitoring and Logging

### Training Statistics
- Total images processed
- Batches completed
- Average training loss
- Average validation mAP
- Total training time

### Log Files
- Individual training session logs
- Batch completion records
- Error tracking and debugging
- Performance metrics

## Use Cases

### IoT and Edge Computing
- **Distributed ML training** on edge devices
- **Real-time object detection** model updates
- **Bandwidth-efficient** model distribution
- **Local data processing** for privacy

### Research and Development
- **Incremental learning** experiments
- **Distributed training** research
- **Edge ML** performance studies
- **P2P communication** for ML systems

## Troubleshooting

### Common Issues

1. **Python dependencies not found**
   ```bash
   cd ml && pip install -r requirements.txt
   ```

2. **Go build errors**
   ```bash
   go mod tidy
   go build .
   ```

3. **WebRTC connection failures**
   - Check signaling server is running
   - Verify unique connection IDs match
   - Check firewall settings

4. **YOLO training errors**
   - Ensure sufficient disk space
   - Check Python environment
   - Verify image file formats

### Debug Mode
Enable verbose logging by modifying log levels in the respective Go files.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## License

[Add your license information here]

## Contact

[Add your contact information here]

