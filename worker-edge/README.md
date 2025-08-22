# Worker Edge Node

This is a worker edge node implementation for federated learning that receives image batches from a master edge, executes Python ML training scripts, and sends trained weights back.

## Features

- **WebRTC Communication**: Persistent bidirectional connections with master edge
- **Image Batch Processing**: Receives and stores image batches locally
- **Python ML Integration**: Executes configurable Python training scripts
- **Weight Transfer**: Sends trained model weights back to master edge
- **Error Handling**: Comprehensive error handling and recovery
- **Heartbeat Monitoring**: Maintains connection health with periodic heartbeats

## Project Structure

```
worker-edge/
├── main.go              # Application entry point
├── types.go             # Core data structures and types
├── config.go            # Configuration management
├── worker.go            # Main worker logic and coordination
├── connection.go        # WebRTC connection management
├── data_handler.go      # Image batch reception and storage
├── training.go          # Python script execution and monitoring
├── weight_sender.go     # Weight transfer back to master
├── config.json          # Default configuration file
├── train.py             # Example Python training script
└── README.md            # This file
```

## Usage

### Basic Usage

```bash
# Run with default configuration
go run . -worker-id worker-001

# Run with custom configuration
go run . -worker-id worker-001 -config custom-config.json
```

### Configuration

The worker edge uses a JSON configuration file with the following structure:

```json
{
  "worker_id": "",
  "python_script": "./train.py",
  "batch_directory": "./batches",
  "weights_path": "./weights/model_weights",
  "signaling_server": "ws://localhost:8000/",
  "heartbeat_interval": "30s"
}
```

### Python Training Script

The Python training script should accept the following arguments:
- `--batch-dir`: Directory containing the image batch
- `--weights-output`: Path where weights should be saved

Example:
```python
python train.py --batch-dir ./batches/batch-001 --weights-output ./weights/model_weights
```

## Dependencies

- Go 1.24.4 or higher
- Python 3.x with required ML libraries
- WebRTC signaling server running on configured port

## Workflow

1. **Connection**: Establishes WebRTC connection with master edge
2. **Reception**: Receives image batches from master edge
3. **Training**: Executes Python ML script on received data
4. **Transfer**: Sends trained weights back to master edge
5. **Coordination**: Waits for next training round or termination

## Error Handling

The worker edge handles various error scenarios:
- Connection failures with automatic retry
- Training script failures with detailed logging
- Missing or corrupted weights files
- Network interruptions with connection recovery

## Logging

All operations are logged with timestamps and severity levels for debugging and monitoring purposes.