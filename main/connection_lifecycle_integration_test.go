package main

import (
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

func TestConnectionLifecycleIntegration(t *testing.T) {
	// Test basic connection lifecycle manager functionality
	manager := NewConnectionLifecycleManager(1*time.Second, 2*time.Second)

	// Test that manager is created properly
	if manager == nil {
		t.Fatal("Connection lifecycle manager should not be nil")
	}

	// Test getting active connections when none exist
	active := manager.GetActiveConnections()
	if len(active) != 0 {
		t.Errorf("Expected 0 active connections, got %d", len(active))
	}

	// Test getting all connections when none exist
	all := manager.GetAllConnections()
	if len(all) != 0 {
		t.Errorf("Expected 0 total connections, got %d", len(all))
	}

	// Test getting connection state for non-existent connection
	_, exists := manager.GetConnectionState(1)
	if exists {
		t.Error("Connection 1 should not exist")
	}
}

func TestConnectionCloseMessageCreation(t *testing.T) {
	// Test creating connection close messages
	msg := messaging.CreateConnectionCloseMessage(1, messaging.CloseReasonCompleted, "Test completion")

	if msg == nil {
		t.Fatal("Connection close message should not be nil")
	}

	if msg.Type != messaging.MsgTypeConnectionClose {
		t.Errorf("Expected message type %s, got %s", messaging.MsgTypeConnectionClose, msg.Type)
	}

	if msg.WorkerID != 1 {
		t.Errorf("Expected worker ID 1, got %d", msg.WorkerID)
	}

	// Check metadata
	if msg.Metadata == nil {
		t.Fatal("Message metadata should not be nil")
	}

	closeInfo, exists := msg.Metadata["close_info"]
	if !exists {
		t.Fatal("Close info should exist in metadata")
	}

	// Verify close info structure
	if closeInfo == nil {
		t.Fatal("Close info should not be nil")
	}
}

func TestShutdownMessageCreation(t *testing.T) {
	// Test creating shutdown messages
	msg := messaging.CreateShutdownMessage("server", "Test shutdown")

	if msg == nil {
		t.Fatal("Shutdown message should not be nil")
	}

	if msg.Type != messaging.MsgTypeShutdown {
		t.Errorf("Expected message type %s, got %s", messaging.MsgTypeShutdown, msg.Type)
	}

	// Check metadata
	if msg.Metadata == nil {
		t.Fatal("Message metadata should not be nil")
	}

	shutdownInfo, exists := msg.Metadata["shutdown_info"]
	if !exists {
		t.Fatal("Shutdown info should exist in metadata")
	}

	// Verify shutdown info structure
	if shutdownInfo == nil {
		t.Fatal("Shutdown info should not be nil")
	}
}

func TestConnectionStateString(t *testing.T) {
	// Test connection state string representations
	testCases := []struct {
		state    ConnectionState
		expected string
	}{
		{ConnectionStateActive, "ACTIVE"},
		{ConnectionStateClosing, "CLOSING"},
		{ConnectionStateClosed, "CLOSED"},
		{ConnectionStateError, "ERROR"},
	}

	for _, tc := range testCases {
		if tc.state.String() != tc.expected {
			t.Errorf("Expected state %s to have string %s, got %s",
				tc.expected, tc.expected, tc.state.String())
		}
	}
}

func TestServerMessageHandlerCreation(t *testing.T) {
	// Test creating server message handler with lifecycle manager
	resultCollector := NewServerResultCollector(2, "./test_results")
	lifecycleManager := NewConnectionLifecycleManager(5*time.Second, 10*time.Second)

	handler := NewServerMessageHandler(resultCollector, lifecycleManager)

	if handler == nil {
		t.Fatal("Server message handler should not be nil")
	}

	if handler.resultCollector != resultCollector {
		t.Error("Result collector should be set correctly")
	}

	if handler.lifecycleManager != lifecycleManager {
		t.Error("Lifecycle manager should be set correctly")
	}

	// Test getting registered workers when none exist
	workers := handler.GetRegisteredWorkers()
	if len(workers) != 0 {
		t.Errorf("Expected 0 registered workers, got %d", len(workers))
	}

	// Test checking if worker is connected when none exist
	if handler.IsWorkerConnected(1) {
		t.Error("Worker 1 should not be connected")
	}
}

func TestPersistentWorkerMessageHandlers(t *testing.T) {
	// Test that persistent worker registers all required message handlers
	config := DefaultProcessingConfig()
	worker := NewPersistentWorker(1, "test-conn", nil, nil, config)

	if worker == nil {
		t.Fatal("Persistent worker should not be nil")
	}

	// Check that message handlers are registered
	worker.handlerMutex.RLock()
	defer worker.handlerMutex.RUnlock()

	expectedHandlers := []messaging.MessageType{
		messaging.MsgTypeHeartbeat,
		messaging.MsgTypeAck,
		messaging.MsgTypeImage,
		messaging.MsgTypeResult,
		messaging.MsgTypeError,
		messaging.MsgTypeConnectionClose,
		messaging.MsgTypeShutdown,
	}

	for _, msgType := range expectedHandlers {
		if _, exists := worker.messageHandlers[msgType]; !exists {
			t.Errorf("Message handler for type %s should be registered", msgType)
		}
	}
}
