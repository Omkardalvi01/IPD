package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// MockMessageHandler for testing
type MockMessageHandler struct {
	handledMessages []*messaging.Message
	shouldError     bool
}

func (m *MockMessageHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	m.handledMessages = append(m.handledMessages, msg)
	if m.shouldError {
		return fmt.Errorf("mock error")
	}
	return nil
}

func TestMessageHandlerRegistration(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Test that default handlers are registered
	worker.handlerMutex.RLock()
	if len(worker.messageHandlers) == 0 {
		t.Error("Expected default message handlers to be registered")
	}

	// Check specific handlers
	expectedTypes := []messaging.MessageType{
		messaging.MsgTypeHeartbeat,
		messaging.MsgTypeAck,
		messaging.MsgTypeImage,
		messaging.MsgTypeResult,
		messaging.MsgTypeError,
	}

	for _, msgType := range expectedTypes {
		if _, exists := worker.messageHandlers[msgType]; !exists {
			t.Errorf("Expected handler for message type %s to be registered", msgType)
		}
	}
	worker.handlerMutex.RUnlock()

	// Test custom handler registration
	mockHandler := &MockMessageHandler{}
	customType := messaging.MessageType("CUSTOM")

	worker.RegisterMessageHandler(customType, mockHandler)

	worker.handlerMutex.RLock()
	if handler, exists := worker.messageHandlers[customType]; !exists {
		t.Error("Expected custom handler to be registered")
	} else if handler != mockHandler {
		t.Error("Expected registered handler to match the provided handler")
	}
	worker.handlerMutex.RUnlock()
}

func TestMessageRouting(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Register a mock handler
	mockHandler := &MockMessageHandler{}
	testType := messaging.MessageType("TEST")
	worker.RegisterMessageHandler(testType, mockHandler)

	// Create a test message
	testMsg := messaging.CreateMessage(testType, 1, "test.txt", []byte("test data"))

	// Route the message
	err := worker.routeMessage(testMsg)
	if err != nil {
		t.Errorf("Expected no error routing message, got: %v", err)
	}

	// Check that the handler received the message
	if len(mockHandler.handledMessages) != 1 {
		t.Errorf("Expected 1 handled message, got %d", len(mockHandler.handledMessages))
	}

	if mockHandler.handledMessages[0] != testMsg {
		t.Error("Expected handled message to match the sent message")
	}
}

func TestMessageRoutingError(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Try to route a message with no registered handler
	unknownType := messaging.MessageType("UNKNOWN")
	testMsg := messaging.CreateMessage(unknownType, 1, "test.txt", []byte("test data"))

	err := worker.routeMessage(testMsg)
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestMessageHandlerError(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Register a mock handler that returns an error
	mockHandler := &MockMessageHandler{shouldError: true}
	testType := messaging.MessageType("ERROR_TEST")
	worker.RegisterMessageHandler(testType, mockHandler)

	// Create a test message
	testMsg := messaging.CreateMessage(testType, 1, "test.txt", []byte("test data"))

	// Route the message
	err := worker.routeMessage(testMsg)
	if err == nil {
		t.Error("Expected error from handler")
	}
}

func TestHeartbeatHandler(t *testing.T) {
	handler := &HeartbeatHandler{}

	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Test with no heartbeat manager (should error)
	heartbeatMsg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
	err := handler.HandleMessage(worker, heartbeatMsg)
	if err == nil {
		t.Error("Expected error when heartbeat manager is not initialized")
	}
}

func TestAckHandler(t *testing.T) {
	handler := &AckHandler{}

	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Set up an acknowledgment waiting channel
	ackID := "test-ack-123"
	ackChan := make(chan bool, 1)
	worker.ackWaitMap[ackID] = ackChan

	// Create acknowledgment message
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, 1, "", nil)
	ackMsg.Metadata = map[string]interface{}{
		"ack_id": ackID,
	}

	// Handle the acknowledgment
	err := handler.HandleMessage(worker, ackMsg)
	if err != nil {
		t.Errorf("Expected no error handling acknowledgment, got: %v", err)
	}

	// Check that the acknowledgment was received
	select {
	case acked := <-ackChan:
		if !acked {
			t.Error("Expected acknowledgment to be true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected acknowledgment to be received")
	}

	// Check that the ack ID was removed from the map
	if _, exists := worker.ackWaitMap[ackID]; exists {
		t.Error("Expected ack ID to be removed from wait map")
	}
}

func TestImageHandler(t *testing.T) {
	handler := &ImageHandler{}

	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Create image message
	imageMsg := messaging.CreateMessage(messaging.MsgTypeImage, 1, "test.jpg", []byte("image data"))

	// Handle the image message (this will fail because no data channel is set up)
	err := handler.HandleMessage(worker, imageMsg)
	if err == nil {
		t.Error("Expected error when data channel is not available")
	}
}

func TestResultHandler(t *testing.T) {
	handler := &ResultHandler{}

	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Create result message
	resultMsg := messaging.CreateMessage(messaging.MsgTypeResult, 1, "", []byte("result data"))

	// Handle the result message (this will fail because no data channel is set up)
	err := handler.HandleMessage(worker, resultMsg)
	if err == nil {
		t.Error("Expected error when data channel is not available")
	}
}

func TestMessageErrorHandler(t *testing.T) {
	handler := &MessageErrorHandler{}

	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Create error message
	errorMsg := messaging.CreateMessage(messaging.MsgTypeError, 1, "", []byte("error occurred"))

	// Handle the error message
	err := handler.HandleMessage(worker, errorMsg)
	if err != nil {
		t.Errorf("Expected no error handling error message, got: %v", err)
	}

	// Check that worker state was set to error
	if worker.GetState() != StateError {
		t.Errorf("Expected worker state to be StateError, got %s", worker.GetState())
	}
}

func TestQueueUtilities(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Test initial queue size
	if worker.GetQueueSize() != 0 {
		t.Errorf("Expected initial queue size to be 0, got %d", worker.GetQueueSize())
	}

	// Test queue not full initially
	if worker.IsMessageQueueFull() {
		t.Error("Expected queue not to be full initially")
	}

	// Add messages to queue to test size
	for i := 0; i < 5; i++ {
		testMsg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
		queueItem := MessageQueueItem{
			Message:   testMsg,
			Timestamp: time.Now(),
		}
		worker.messageQueue <- queueItem
	}

	if worker.GetQueueSize() != 5 {
		t.Errorf("Expected queue size to be 5, got %d", worker.GetQueueSize())
	}
}
