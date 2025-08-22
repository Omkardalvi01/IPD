# Worker Edge Implementation Design

## Overview

The worker edge is a standalone Go application that acts as a federated learning participant. It establishes persistent WebRTC connections with the master edge, receives image batches, executes Python ML training scripts, and sends trained weights back. The design emphasizes connection persistence, error handling, and seamless integration with existing networking infrastructure.

## Architecture

### High-Level Architecture
```
Master Edge ←→ WebRTC Data Channel ←→ Worker Edge
                                        │
                                        ├─ Image Storage
                                        ├─ Python ML Script
                                        └─ Weights Storage
```

### Component Interaction Flow
1. **Connection Phase**: Worker edge establishes WebRTC connection via signaling server
2. **Data Reception Phase**: Receives and stores image batches from master edge
3. **Training Phase**: Executes Python script on received images
4. **Weight Transfer Phase**: Sends generated weights back to master edge
5. **Coordination Phase**: Waits for next round or termination signal

## Components and Interfaces

### 1. Main Application (`worker-edge/main.go`)
**Responsibilities:**
- Application entry point and lifecycle management
- Configuration loading and validation
- Coordination between components

**Key Functions:**
```go
func main()
func loadConfig() (*Config, error)
func validateEnvironment() error
```

### 2. Connection Manager (`worker-edge/connection.go`)
**Responsibilities:**
- WebRTC connection establishment and maintenance
- Bidirectional data channel management
- Connection recovery and heartbeat

**Key Functions:**
```go
func EstablishConnection(uid string) (*webrtc.PeerConnection, *webrtc.DataChannel, error)
func MaintainConnection(dc *webrtc.DataChannel) error
func SendHeartbeat(dc *webrtc.DataChannel) error
```

### 3. Data Handler (`worker-edge/data_handler.go`)
**Responsibilities:**
- Image batch reception and storage
- File system management for training data
- Data validation and integrity checks

**Key Functions:**
```go
func ReceiveImageBatch(dc *webrtc.DataChannel, batchDir string) error
func ValidateImageBatch(batchDir string) error
func CleanupBatch(batchDir string) error
```

### 4. Training Coordinator (`worker-edge/training.go`)
**Responsibilities:**
- Python script execution and monitoring
- Training progress tracking
- Error handling for training failures

**Key Functions:**
```go
func ExecuteTraining(batchDir, weightsPath, pythonScript string) error
func MonitorTraining(cmd *exec.Cmd) error
func ValidateWeights(weightsPath string) error
```

### 5. Weight Sender (`worker-edge/weight_sender.go`)
**Responsibilities:**
- Reading generated weights files
- Sending weights data via WebRTC
- Transfer completion signaling

**Key Functions:**
```go
func SendWeights(dc *webrtc.DataChannel, weightsPath string) error
func ReadWeightsFile(weightsPath string) ([]byte, error)
func SignalTrainingComplete(dc *webrtc.DataChannel) error
```

## Data Models

### Configuration Structure
```go
type Config struct {
    WorkerID        string `json:"worker_id"`
    PythonScript    string `json:"python_script"`
    BatchDirectory  string `json:"batch_directory"`
    WeightsPath     string `json:"weights_path"`
    SignalingServer string `json:"signaling_server"`
    HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}
```

### Message Protocol
```go
type MessageType string

const (
    ImageData        MessageType = "IMAGE_DATA"
    BatchComplete    MessageType = "BATCH_COMPLETE"
    TrainingStart    MessageType = "TRAINING_START"
    TrainingComplete MessageType = "TRAINING_COMPLETE"
    WeightsData      MessageType = "WEIGHTS_DATA"
    Error           MessageType = "ERROR"
    Heartbeat       MessageType = "HEARTBEAT"
    Terminate       MessageType = "TERMINATE"
)

type Message struct {
    Type    MessageType `json:"type"`
    Data    []byte      `json:"data,omitempty"`
    Payload string      `json:"payload,omitempty"`
}
```

### Training State Management
```go
type TrainingState int

const (
    Idle TrainingState = iota
    ReceivingData
    Training
    SendingWeights
    Error
)

type WorkerState struct {
    CurrentState    TrainingState
    BatchID         string
    TrainingStartTime time.Time
    LastHeartbeat   time.Time
    ErrorMessage    string
}
```

## Error Handling

### Connection Errors
- **WebRTC Connection Failure**: Retry with exponential backoff
- **Data Channel Closure**: Attempt reconnection and resume from last known state
- **Signaling Server Unavailable**: Switch to backup signaling server if configured

### Training Errors
- **Python Script Not Found**: Log error and notify master edge
- **Training Script Failure**: Capture stderr, log locally, and send error message to master
- **Insufficient Resources**: Check disk space and memory before training
- **Weights File Missing**: Validate Python script output and report specific error

### Data Transfer Errors
- **Corrupted Image Data**: Validate checksums and request retransmission
- **Incomplete Batch**: Track received files and request missing data
- **Weights Transfer Failure**: Retry with chunked transfer if needed

## Testing Strategy

### Unit Tests
- **Connection Manager**: Mock WebRTC connections and test connection lifecycle
- **Data Handler**: Test file operations with temporary directories
- **Training Coordinator**: Mock Python script execution and test error scenarios
- **Weight Sender**: Test file reading and data channel communication

### Integration Tests
- **End-to-End Workflow**: Test complete image reception → training → weight sending cycle
- **Connection Recovery**: Test reconnection scenarios with simulated network failures
- **Python Integration**: Test with actual Python scripts and validate weight generation

### Performance Tests
- **Large Batch Handling**: Test with various batch sizes and image formats
- **Memory Usage**: Monitor memory consumption during training phases
- **Connection Stability**: Long-running tests to validate persistent connections

## Security Considerations

### Data Protection
- **Image Data**: Ensure received images are stored securely and cleaned up after training
- **Weights Data**: Validate weights file integrity before transmission
- **Connection Security**: Use DTLS encryption provided by WebRTC

### Access Control
- **Python Script Execution**: Validate script paths and prevent arbitrary code execution
- **File System Access**: Restrict file operations to designated directories
- **Network Access**: Limit connections to authorized signaling servers

## Deployment Considerations

### Dependencies
- **Go Runtime**: Version 1.24.4 or higher
- **Python Environment**: Configurable Python interpreter with required ML libraries
- **Network Configuration**: Proper firewall rules for WebRTC traffic

### Configuration Management
- **Environment Variables**: Support for deployment-specific configuration
- **Config File**: JSON configuration file for worker-specific settings
- **Runtime Parameters**: Command-line arguments for debugging and testing

### Monitoring and Logging
- **Structured Logging**: JSON-formatted logs for easy parsing
- **Metrics Collection**: Training duration, batch sizes, error rates
- **Health Checks**: Periodic status reporting to master edge