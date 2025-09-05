# Bidirectional Message Handling Implementation

## Overview

This document summarizes the implementation of Task 4: "Implement bidirectional message handling in workers" from the persistent worker processing specification.

## Implemented Features

### 1. Enhanced Message Handling Architecture

- **Message Queue**: Added a buffered channel (`messageQueue`) to handle concurrent messages with a capacity of 100 messages
- **Message Handlers**: Implemented a pluggable handler system using the `MessageHandler` interface
- **Message Routing**: Added `routeMessage()` method to dispatch messages to appropriate handlers based on message type

### 2. Message Handler Interface

```go
type MessageHandler interface {
    HandleMessage(worker *PersistentWorker, msg *messaging.Message) error
}
```

### 3. Default Message Handlers

Implemented handlers for all core message types:

- **HeartbeatHandler**: Processes heartbeat messages and delegates to HeartbeatManager
- **AckHandler**: Handles acknowledgment messages and manages pending acknowledgments
- **ImageHandler**: Processes image messages and sends acknowledgments
- **ResultHandler**: Handles result processing instructions from server
- **ErrorHandler**: Processes error messages and updates worker state

### 4. Message Acknowledgment System

- **Acknowledgment Tracking**: Added `ackWaitMap` to track pending acknowledgments
- **Timeout Support**: `sendMessageWithAck()` method with configurable timeout
- **Unique ACK IDs**: Generated using worker ID and timestamp for uniqueness

### 5. Concurrent Message Processing

- **Message Processing Loop**: `processMessageQueue()` runs in a separate goroutine
- **Thread-Safe Operations**: All message handling operations are thread-safe
- **Queue Management**: Proper handling of queue overflow and cleanup

### 6. Enhanced Worker Structure

Added new fields to `PersistentWorker`:
```go
type PersistentWorker struct {
    // ... existing fields ...
    messageQueue    chan MessageQueueItem
    messageHandlers map[messaging.MessageType]MessageHandler
    handlerMutex    sync.RWMutex
    ackWaitMap      map[string]chan bool
    ackMutex        sync.RWMutex
}
```

## Key Methods Implemented

### Message Handling
- `handleIncomingMessage()`: Enhanced to support both structured and legacy messages
- `processMessageQueue()`: Main message processing loop
- `routeMessage()`: Routes messages to appropriate handlers
- `registerMessageHandlers()`: Registers default handlers
- `RegisterMessageHandler()`: Allows custom handler registration

### Message Sending
- `sendMessage()`: Basic message sending
- `sendMessageWithAck()`: Message sending with acknowledgment waiting
- `SendHeartbeat()`: Convenience method for heartbeat messages
- `SendResult()`: Sends processing results with acknowledgment
- `SendError()`: Sends error messages

### Utility Methods
- `GetQueueSize()`: Returns current message queue size
- `IsMessageQueueFull()`: Checks if queue is at capacity
- `handleLegacyMessage()`: Maintains backward compatibility

## Testing

### Unit Tests
- Message handler registration and routing
- Individual handler functionality
- Acknowledgment system
- Queue utilities

### Integration Tests
- Complete bidirectional message flow
- Concurrent message processing
- Queue overflow handling
- Message tracking and verification

## Backward Compatibility

The implementation maintains full backward compatibility:
- Legacy text-based messages are still supported
- Existing worker functionality remains unchanged
- Original message handling is preserved through `handleLegacyMessage()`

## Requirements Satisfied

✅ **1.3**: Bidirectional message passing through persistent connections
✅ **3.3**: Metadata inclusion in message transmission
✅ **4.2**: Proper message acknowledgment system

## Performance Characteristics

- **Queue Capacity**: 100 messages (configurable)
- **Concurrent Processing**: Single-threaded message processing with concurrent queuing
- **Memory Overhead**: Minimal additional memory usage per worker
- **Acknowledgment Timeout**: Configurable (default: 30 seconds for results)

## Future Enhancements

The architecture supports easy extension for:
- Custom message types and handlers
- Priority-based message processing
- Message persistence and replay
- Advanced acknowledgment patterns
- Message compression and encryption

## Usage Example

```go
// Create worker with bidirectional messaging
worker := NewPersistentWorker(1, "conn-id", reqChan, resChan, config)

// Register custom handler
worker.RegisterMessageHandler(messaging.MessageType("CUSTOM"), &CustomHandler{})

// Send message with acknowledgment
result := &messaging.ProcessingResult{...}
err := worker.SendResult(result)
```

This implementation provides a robust foundation for bidirectional communication between workers and the main server, enabling complex processing workflows while maintaining system reliability and performance.