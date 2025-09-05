# Messaging Package

This package provides enhanced message protocol and data structures for bidirectional communication between the main server and workers in the IPD system.

## Features

- **Enhanced Message Protocol**: Supports multiple message types for bidirectional communication
- **Structured Data Models**: Comprehensive data structures for processing results and metadata
- **Serialization/Deserialization**: JSON-based message serialization with error handling
- **Validation**: Built-in validation for messages, results, and processed files
- **Utility Functions**: Helper functions for message creation, checksum calculation, and metadata management

## Message Types

- `MsgTypeImage`: Image data transmission
- `MsgTypeImageEnd`: End of image transmission signal
- `MsgTypeResult`: Processing result transmission
- `MsgTypeResultEnd`: End of result transmission signal
- `MsgTypeHeartbeat`: Connection health monitoring
- `MsgTypeAck`: Message acknowledgment
- `MsgTypeError`: Error reporting

## Core Data Structures

### Message
Base structure for all communication between server and workers.

### Result
Enhanced result structure with processing metadata and status information.

### ProcessingResult
Detailed processing results including file counts, processing time, and output files.

### ProcessedFile
Individual processed file with metadata, checksum, and data.

## Usage Examples

### Creating and Serializing a Message
```go
import "github.com/Omkardalvi01/IPD/messaging"

// Create an image message
msg := messaging.CreateMessage(messaging.MsgTypeImage, 1, "image.jpg", imageData)

// Serialize for transmission
data, err := messaging.SerializeMessage(msg)
if err != nil {
    // handle error
}

// Send data over network...
```

### Deserializing and Validating a Message
```go
// Receive data from network...

// Deserialize message
msg, err := messaging.DeserializeMessage(data)
if err != nil {
    // handle error
}

// Validate message
if err := messaging.ValidateMessage(msg); err != nil {
    // handle validation error
}
```

### Creating Processing Results
```go
// Create processing result
result := messaging.CreateProcessingResult(workerID, messaging.StatusSuccess, 5, 3)
result.ProcessingTime = time.Second * 10

// Add processed files
file1 := messaging.CreateProcessedFile("output1.txt", []byte("result 1"))
result.OutputFiles = append(result.OutputFiles, *file1)

// Create and serialize result
finalResult := messaging.CreateResult(workerID, messaging.StatusSuccess, result)
data, err := messaging.SerializeResult(finalResult)
```

## Validation

All data structures include comprehensive validation:
- Message type and required field validation
- File size and checksum verification
- Processing result consistency checks
- Metadata integrity validation

## Testing

Run tests with:
```bash
go test ./messaging -v
```

The package includes comprehensive tests for:
- Message serialization/deserialization
- Result handling
- Validation functions
- Utility functions
- Error cases