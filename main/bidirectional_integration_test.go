package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// TestBidirectionalMessageFlow demonstrates the complete bidirectional message flow
func TestBidirectionalMessageFlow(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request, 10)
	resChan := make(chan Result, 10)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Store original handlers
	originalHandlers := make(map[messaging.MessageType]MessageHandler)
	worker.handlerMutex.RLock()
	for msgType, handler := range worker.messageHandlers {
		originalHandlers[msgType] = handler
	}
	worker.handlerMutex.RUnlock()

	// Create a custom handler to track processed messages
	messageTracker := &MessageTracker{
		processedMessages: make(map[messaging.MessageType]int),
		originalHandlers:  originalHandlers,
		worker:            worker,
	}

	// Register the tracker for all message types
	worker.RegisterMessageHandler(messaging.MsgTypeHeartbeat, messageTracker)
	worker.RegisterMessageHandler(messaging.MsgTypeAck, messageTracker)
	worker.RegisterMessageHandler(messaging.MsgTypeImage, messageTracker)
	worker.RegisterMessageHandler(messaging.MsgTypeResult, messageTracker)
	worker.RegisterMessageHandler(messaging.MsgTypeError, messageTracker)

	// Start the message processing loop
	go worker.processMessageQueue()

	// Simulate receiving different types of messages
	testMessages := []*messaging.Message{
		messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil),
		messaging.CreateMessage(messaging.MsgTypeImage, 1, "test1.jpg", []byte("image1")),
		messaging.CreateMessage(messaging.MsgTypeImage, 1, "test2.jpg", []byte("image2")),
		messaging.CreateMessage(messaging.MsgTypeResult, 1, "", []byte("result1")),
		messaging.CreateMessage(messaging.MsgTypeAck, 1, "", nil),
		messaging.CreateMessage(messaging.MsgTypeError, 1, "", []byte("test error")),
	}

	// Send messages to the worker
	for _, msg := range testMessages {
		queueItem := MessageQueueItem{
			Message:   msg,
			Timestamp: time.Now(),
		}

		select {
		case worker.messageQueue <- queueItem:
			t.Logf("Queued message type: %s", msg.Type)
		case <-time.After(1 * time.Second):
			t.Errorf("Failed to queue message type: %s", msg.Type)
		}
	}

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that all messages were processed
	expectedCounts := map[messaging.MessageType]int{
		messaging.MsgTypeHeartbeat: 1,
		messaging.MsgTypeImage:     2,
		messaging.MsgTypeResult:    1,
		messaging.MsgTypeAck:       1,
		messaging.MsgTypeError:     1,
	}

	messageTracker.mutex.RLock()
	for msgType, expectedCount := range expectedCounts {
		if actualCount, exists := messageTracker.processedMessages[msgType]; !exists {
			t.Errorf("Expected message type %s to be processed", msgType)
		} else if actualCount != expectedCount {
			t.Errorf("Expected %d messages of type %s, got %d", expectedCount, msgType, actualCount)
		}
	}
	messageTracker.mutex.RUnlock()

	// Verify worker state changed to error (due to error message)
	if worker.GetState() != StateError {
		t.Errorf("Expected worker state to be StateError, got %s", worker.GetState())
	}

	// Stop the worker
	worker.Stop()
}

// TestMessageQueueConcurrency tests concurrent message handling
func TestMessageQueueConcurrency(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request, 10)
	resChan := make(chan Result, 10)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	messageTracker := &MessageTracker{
		processedMessages: make(map[messaging.MessageType]int),
	}

	worker.RegisterMessageHandler(messaging.MsgTypeHeartbeat, messageTracker)

	// Start the message processing loop
	go worker.processMessageQueue()

	// Send messages concurrently from multiple goroutines
	numGoroutines := 5
	messagesPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				msg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
				msg.Metadata["goroutine_id"] = goroutineID
				msg.Metadata["message_id"] = j

				queueItem := MessageQueueItem{
					Message:   msg,
					Timestamp: time.Now(),
				}

				select {
				case worker.messageQueue <- queueItem:
				case <-time.After(1 * time.Second):
					t.Errorf("Failed to queue message from goroutine %d", goroutineID)
				}
			}
		}(i)
	}

	wg.Wait()

	// Wait for all messages to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify all messages were processed
	expectedTotal := numGoroutines * messagesPerGoroutine
	messageTracker.mutex.RLock()
	actualTotal := messageTracker.processedMessages[messaging.MsgTypeHeartbeat]
	messageTracker.mutex.RUnlock()

	if actualTotal != expectedTotal {
		t.Errorf("Expected %d total messages to be processed, got %d", expectedTotal, actualTotal)
	}

	worker.Stop()
}

// TestMessageQueueOverflow tests message queue overflow handling
func TestMessageQueueOverflow(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request, 10)
	resChan := make(chan Result, 10)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Fill the message queue to capacity
	queueCapacity := cap(worker.messageQueue)

	for i := 0; i < queueCapacity; i++ {
		msg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
		queueItem := MessageQueueItem{
			Message:   msg,
			Timestamp: time.Now(),
		}

		select {
		case worker.messageQueue <- queueItem:
		default:
			t.Errorf("Failed to fill queue at message %d", i)
		}
	}

	// Verify queue is full
	if !worker.IsMessageQueueFull() {
		t.Error("Expected message queue to be full")
	}

	if worker.GetQueueSize() != queueCapacity {
		t.Errorf("Expected queue size to be %d, got %d", queueCapacity, worker.GetQueueSize())
	}

	// Try to add one more message (should be dropped)
	msg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
	queueItem := MessageQueueItem{
		Message:   msg,
		Timestamp: time.Now(),
	}

	select {
	case worker.messageQueue <- queueItem:
		t.Error("Expected message to be dropped when queue is full")
	default:
		// This is expected behavior
	}

	worker.Stop()
}

// MessageTracker is a test helper that tracks processed messages
type MessageTracker struct {
	processedMessages map[messaging.MessageType]int
	mutex             sync.RWMutex
	originalHandlers  map[messaging.MessageType]MessageHandler
	worker            *PersistentWorker
}

func (mt *MessageTracker) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	mt.mutex.Lock()
	mt.processedMessages[msg.Type]++
	mt.mutex.Unlock()

	// Log the message processing
	fmt.Printf("MessageTracker: Processed message type %s (total: %d)\n",
		msg.Type, mt.processedMessages[msg.Type])

	// Call the original handler if it exists
	if originalHandler, exists := mt.originalHandlers[msg.Type]; exists {
		return originalHandler.HandleMessage(worker, msg)
	}

	return nil
}
