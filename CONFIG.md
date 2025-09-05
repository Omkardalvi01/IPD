# Configuration Management System

This document describes the configuration management system for the IPD (Image Processing Distributed) application.

## Overview

The configuration system supports loading configuration from:
1. JSON configuration files
2. Environment variables
3. Default values for development

Configuration is organized into several categories:
- **System Configuration**: Core system parameters
- **Network Configuration**: WebRTC and networking settings
- **File Transmission Configuration**: File transfer parameters
- **Processing Configuration**: Worker processing settings (defined in worker.go)

## Configuration Structure

### System Configuration (`SystemConfig`)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `max_workers` | int | 4 | Maximum number of workers |
| `heartbeat_interval` | duration | 30s | Heartbeat interval for connections |
| `processing_timeout` | duration | 10m | Maximum processing time |
| `connection_timeout` | duration | 30s | Connection establishment timeout |
| `retry_attempts` | int | 3 | Number of retry attempts for failed operations |
| `external_script_path` | string | "./scripts/process.py" | Path to external processing script |
| `input_base_directory` | string | "./data/input" | Base directory for input files |
| `output_base_directory` | string | "./data/output" | Base directory for output files |
| `signaling_server_url` | string | "ws://localhost:8080" | WebRTC signaling server URL |
| `log_level` | string | "INFO" | Logging level (DEBUG, INFO, WARN, ERROR) |
| `enable_metrics` | bool | false | Enable metrics collection |
| `metrics_port` | int | 9090 | Port for metrics server |

### Network Configuration (`NetworkConfig`)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `stun_servers` | []string | ["stun:stun.l.google.com:19302", "stun:stun1.l.google.com:19302"] | STUN servers for NAT traversal |
| `turn_servers` | []string | [] | TURN servers for relay |
| `data_channel_timeout` | duration | 30s | Data channel establishment timeout |
| `max_reconnect_attempts` | int | 5 | Maximum reconnection attempts |
| `reconnect_delay` | duration | 2s | Delay between reconnection attempts |

### File Transmission Configuration (`FileTransmissionConfig`)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `chunk_size` | int | 65536 | File chunk size in bytes (64KB) |
| `max_concurrent_files` | int | 3 | Maximum concurrent file transmissions |
| `transmission_timeout` | duration | 2m | File transmission timeout |
| `retry_attempts` | int | 3 | Number of retry attempts for file transmission |
| `enable_checksum` | bool | true | Enable file integrity checking |

### Processing Configuration (`ProcessingConfig`)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `python_script_path` | string | "python3" | Path to Python interpreter |
| `input_directory` | string | "./input" | Directory for input files |
| `output_directory` | string | "./output" | Directory for output files |
| `processing_timeout` | duration | 5m | Processing script timeout |
| `heartbeat_interval` | duration | 30s | Worker heartbeat interval |

## Configuration Loading

### 1. File-based Configuration

Create a JSON configuration file (e.g., `config.json`):

```json
{
  "system": {
    "max_workers": 8,
    "log_level": "DEBUG"
  },
  "network": {
    "stun_servers": ["stun:custom.server.com:19302"]
  }
}
```

Load configuration in your application:

```go
config, err := LoadConfig("./config.json")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}
```

### 2. Environment Variables

Set environment variables with the `IPD_` prefix:

```bash
export IPD_MAX_WORKERS=8
export IPD_LOG_LEVEL=DEBUG
export IPD_PROCESSING_TIMEOUT=15m
export IPD_STUN_SERVERS="stun:server1.com:19302,stun:server2.com:19302"
```

Load configuration from environment:

```go
config, err := LoadConfigFromEnv()
if err != nil {
    log.Fatalf("Failed to load config from environment: %v", err)
}
```

### 3. Combined Loading

Load from file first, then override with environment variables:

```go
config, err := LoadConfig("./config.json")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}
```

## Environment Variable Reference

### System Configuration
- `IPD_MAX_WORKERS` - Maximum number of workers
- `IPD_CONNECTION_TIMEOUT` - Connection timeout (e.g., "30s")
- `IPD_RETRY_ATTEMPTS` - Number of retry attempts
- `IPD_SIGNALING_SERVER_URL` - Signaling server URL
- `IPD_LOG_LEVEL` - Log level (DEBUG, INFO, WARN, ERROR)
- `IPD_ENABLE_METRICS` - Enable metrics (true/false)
- `IPD_METRICS_PORT` - Metrics server port

### Network Configuration
- `IPD_STUN_SERVERS` - Comma-separated list of STUN servers
- `IPD_TURN_SERVERS` - Comma-separated list of TURN servers
- `IPD_DATA_CHANNEL_TIMEOUT` - Data channel timeout
- `IPD_MAX_RECONNECT_ATTEMPTS` - Maximum reconnection attempts
- `IPD_RECONNECT_DELAY` - Reconnection delay

### File Transmission Configuration
- `IPD_CHUNK_SIZE` - File chunk size in bytes
- `IPD_MAX_CONCURRENT_FILES` - Maximum concurrent file transfers
- `IPD_TRANSMISSION_TIMEOUT` - File transmission timeout
- `IPD_FILE_RETRY_ATTEMPTS` - File transmission retry attempts
- `IPD_ENABLE_CHECKSUM` - Enable checksums (true/false)

### Processing Configuration
- `IPD_PYTHON_SCRIPT_PATH` - Python interpreter path
- `IPD_INPUT_DIRECTORY` - Input directory path
- `IPD_OUTPUT_DIRECTORY` - Output directory path
- `IPD_PROCESSING_TIMEOUT` - Processing timeout
- `IPD_HEARTBEAT_INTERVAL` - Heartbeat interval

## Configuration Validation

All configuration parameters are validated when loaded:

```go
if err := ValidateConfig(config); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}
```

Validation includes:
- Range checks for numeric values
- Required field validation
- Format validation for URLs and paths
- Enum validation for log levels

## Usage Examples

### Basic Usage

```go
// Load default configuration
config := DefaultAppConfig()

// Or load from file
config, err := LoadConfig("./config.json")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

// Validate configuration
if err := ValidateConfig(config); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}

// Use configuration
workerPool := NewPersistentWorkerPool(resultChan, processingConfig)
```

### Creating Default Configuration File

```go
// Create a default configuration file
if err := CreateDefaultConfigFile("./config.json"); err != nil {
    log.Fatalf("Failed to create default config: %v", err)
}
```

### Custom Configuration Path

```bash
export IPD_CONFIG_PATH="/etc/ipd/config.json"
```

```go
configPath := GetConfigPath() // Returns custom path from environment
config, err := LoadConfig(configPath)
```

## Development vs Production

### Development
- Use default configuration or simple JSON file
- Enable debug logging: `IPD_LOG_LEVEL=DEBUG`
- Use local directories for input/output

### Production
- Use environment variables for sensitive configuration
- Enable metrics: `IPD_ENABLE_METRICS=true`
- Configure appropriate timeouts and retry attempts
- Use production STUN/TURN servers

## Testing

The configuration system includes comprehensive tests:

```bash
# Run configuration tests
go test -v ./main/config_test.go ./main/config.go

# Test specific functionality
go test -v ./main -run TestDefaultConfigurations
go test -v ./main -run TestConfigValidation
go test -v ./main -run TestEnvironmentVariables
```

## Error Handling

The configuration system provides detailed error messages:

- File not found: Returns default configuration
- Invalid JSON: Returns parsing error
- Validation failures: Returns specific validation errors
- Environment variable parsing: Falls back to defaults for invalid values

## Best Practices

1. **Use environment variables for production**: Avoid hardcoding sensitive values
2. **Validate configuration early**: Call `ValidateConfig()` at startup
3. **Provide defaults**: Ensure the application works with minimal configuration
4. **Document changes**: Update this README when adding new configuration options
5. **Test configuration**: Include configuration in your integration tests