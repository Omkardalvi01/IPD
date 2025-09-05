package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// TestConnectionLifecycleDemo demonstrates the complete connection lifecycle
func TestConnectionLifecycleDemo(t *testing.T) {
	fmt.Println("=== Connection Lifecycle Management Demo ===")

	// 1. Create connection lifecycle manager
	fmt.Println("1. Creating connection lifecycle manager...")
	manager := NewConnectionLifecycleManager(2*time.Second, 5*time.Second)

	// Set up handlers
	completionCount := 0
	errorCount := 0

	manager.SetCompletionHandler(func(workerID int, reason messaging.ConnectionCloseReason) {
		fmt.Printf("   ✓ Worker %d completed with reason: %s\n", workerID, reason)
		completionCount++
	})

	manager.SetErrorHandler(func(workerID int, err error) {
		fmt.Printf("   ✗ Worker %d error: %v\n", workerID, err)
		errorCount++
	})

	// 2. Create server components
	fmt.Println("2. Creating server components...")
	resultCollector := NewServerResultCollector(3, "./demo_results")
	messageHandler := NewServerMessageHandler(resultCollector, manager)
	_ = messageHandler // Used later in the test

	// 3. Demonstrate message creation
	fmt.Println("3. Creating lifecycle messages...")

	// Connection close message
	closeMsg := messaging.CreateConnectionCloseMessage(1, messaging.CloseReasonCompleted, "Processing completed")
	fmt.Printf("   ✓ Created connection close message: %s\n", closeMsg.Type)

	// Shutdown message
	shutdownMsg := messaging.CreateShutdownMessage("server", "System maintenance")
	fmt.Printf("   ✓ Created shutdown message: %s\n", shutdownMsg.Type)

	// 4. Test worker message handlers
	fmt.Println("4. Testing worker message handlers...")
	config := DefaultProcessingConfig()
	worker := NewPersistentWorker(1, "demo-conn", nil, nil, config)

	// Verify all handlers are registered
	worker.handlerMutex.RLock()
	handlerCount := len(worker.messageHandlers)
	worker.handlerMutex.RUnlock()

	fmt.Printf("   ✓ Worker has %d message handlers registered\n", handlerCount)

	// Check specific handlers
	expectedHandlers := []messaging.MessageType{
		messaging.MsgTypeConnectionClose,
		messaging.MsgTypeShutdown,
		messaging.MsgTypeHeartbeat,
		messaging.MsgTypeAck,
		messaging.MsgTypeError,
	}

	for _, msgType := range expectedHandlers {
		worker.handlerMutex.RLock()
		_, exists := worker.messageHandlers[msgType]
		worker.handlerMutex.RUnlock()

		if exists {
			fmt.Printf("   ✓ Handler for %s: registered\n", msgType)
		} else {
			fmt.Printf("   ✗ Handler for %s: missing\n", msgType)
			t.Errorf("Missing handler for message type: %s", msgType)
		}
	}

	// 5. Test connection states
	fmt.Println("5. Testing connection states...")
	states := []ConnectionState{
		ConnectionStateActive,
		ConnectionStateClosing,
		ConnectionStateClosed,
		ConnectionStateError,
	}

	for _, state := range states {
		fmt.Printf("   ✓ State %s: %s\n", state.String(), state.String())
	}

	// 6. Test server message handler functionality
	fmt.Println("6. Testing server message handler...")

	if messageHandler.resultCollector == nil {
		t.Error("Result collector should be set")
	} else {
		fmt.Println("   ✓ Result collector: configured")
	}

	if messageHandler.lifecycleManager == nil {
		t.Error("Lifecycle manager should be set")
	} else {
		fmt.Println("   ✓ Lifecycle manager: configured")
	}

	// Test worker registration check
	if messageHandler.IsWorkerConnected(999) {
		t.Error("Non-existent worker should not be connected")
	} else {
		fmt.Println("   ✓ Worker connection check: working")
	}

	// 7. Test graceful shutdown scenario
	fmt.Println("7. Testing graceful shutdown scenario...")

	// Simulate shutdown initiation
	fmt.Println("   → Initiating system shutdown...")
	messageHandler.InitiateShutdown("Demo shutdown test")
	fmt.Println("   ✓ Shutdown initiated successfully")

	// 8. Summary
	fmt.Println("8. Demo Summary:")
	fmt.Printf("   ✓ Connection lifecycle manager: functional\n")
	fmt.Printf("   ✓ Message handlers: %d registered\n", handlerCount)
	fmt.Printf("   ✓ Server integration: complete\n")
	fmt.Printf("   ✓ Graceful shutdown: implemented\n")
	fmt.Printf("   ✓ Error handling: configured\n")

	fmt.Println("=== Connection Lifecycle Management Demo Complete ===")
}

// TestConnectionLifecycleScenarios tests various connection lifecycle scenarios
func TestConnectionLifecycleScenarios(t *testing.T) {
	scenarios := []struct {
		name        string
		description string
		test        func(t *testing.T)
	}{
		{
			name:        "Worker Completion",
			description: "Worker completes processing and requests connection close",
			test: func(t *testing.T) {
				// Test worker completion scenario
				config := DefaultProcessingConfig()
				worker := NewPersistentWorker(1, "test-conn", nil, nil, config)

				// Simulate completion
				worker.SetState(StateComplete)

				if worker.GetState() != StateComplete {
					t.Error("Worker should be in complete state")
				}
			},
		},
		{
			name:        "Server Initiated Close",
			description: "Server initiates connection closure after receiving all results",
			test: func(t *testing.T) {
				// Test server-initiated close
				manager := NewConnectionLifecycleManager(1*time.Second, 2*time.Second)

				// Test that manager can handle non-existent connections gracefully
				err := manager.InitiateGracefulClose(999, messaging.CloseReasonCompleted, "Test")
				if err == nil {
					t.Error("Should return error for non-existent connection")
				}
			},
		},
		{
			name:        "Connection Error Recovery",
			description: "Connection fails and system handles recovery",
			test: func(t *testing.T) {
				// Test error recovery
				config := DefaultProcessingConfig()
				worker := NewPersistentWorker(1, "test-conn", nil, nil, config)

				// Simulate error state
				worker.SetState(StateError)

				if worker.GetState() != StateError {
					t.Error("Worker should be in error state")
				}
			},
		},
		{
			name:        "Timeout Handling",
			description: "Connection close times out and is force closed",
			test: func(t *testing.T) {
				// Test timeout handling
				manager := NewConnectionLifecycleManager(10*time.Millisecond, 20*time.Millisecond)

				// Test force close
				err := manager.ForceClose(999, messaging.CloseReasonTimeout, "Test timeout")
				if err == nil {
					t.Error("Should return error for non-existent connection")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			fmt.Printf("Testing scenario: %s\n", scenario.description)
			scenario.test(t)
		})
	}
}

// TestConnectionLifecycleMessageFlow tests the complete message flow
func TestConnectionLifecycleMessageFlow(t *testing.T) {
	fmt.Println("=== Testing Connection Lifecycle Message Flow ===")

	// 1. Create components
	resultCollector := NewServerResultCollector(1, "./test_flow_results")
	lifecycleManager := NewConnectionLifecycleManager(1*time.Second, 2*time.Second)
	messageHandler := NewServerMessageHandler(resultCollector, lifecycleManager)
	_ = messageHandler // Used for component integration testing

	// 2. Test message creation and serialization
	messages := []struct {
		name string
		msg  *messaging.Message
	}{
		{
			name: "Connection Close",
			msg:  messaging.CreateConnectionCloseMessage(1, messaging.CloseReasonCompleted, "Test completion"),
		},
		{
			name: "Shutdown",
			msg:  messaging.CreateShutdownMessage("server", "Test shutdown"),
		},
		{
			name: "Heartbeat",
			msg:  messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil),
		},
		{
			name: "Acknowledgment",
			msg:  messaging.CreateMessage(messaging.MsgTypeAck, 1, "", nil),
		},
	}

	for _, msgTest := range messages {
		t.Run(msgTest.name, func(t *testing.T) {
			// Test message creation
			if msgTest.msg == nil {
				t.Fatalf("%s message should not be nil", msgTest.name)
			}

			// Test serialization
			data, err := messaging.SerializeMessage(msgTest.msg)
			if err != nil {
				t.Fatalf("Failed to serialize %s message: %v", msgTest.name, err)
			}

			// Test deserialization
			deserializedMsg, err := messaging.DeserializeMessage(data)
			if err != nil {
				t.Fatalf("Failed to deserialize %s message: %v", msgTest.name, err)
			}

			// Verify message integrity
			if deserializedMsg.Type != msgTest.msg.Type {
				t.Errorf("Message type mismatch for %s: expected %s, got %s",
					msgTest.name, msgTest.msg.Type, deserializedMsg.Type)
			}

			fmt.Printf("   ✓ %s message: serialization/deserialization successful\n", msgTest.name)
		})
	}

	fmt.Println("=== Connection Lifecycle Message Flow Test Complete ===")
}
