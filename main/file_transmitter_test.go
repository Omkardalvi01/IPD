package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
	"github.com/pion/webrtc/v3"
)

// MockDataChannel simulates a WebRTC data channel for testing
type MockDataChannel struct {
	sentMessages [][]byte
	isOpen       bool
}

func NewMockDataChannel() *MockDataChannel {
	return &MockDataChannel{
		sentMessages: make([][]byte, 0),
		isOpen:       true,
	}
}

func (mdc *MockDataChannel) Send(data []byte) error {
	if !mdc.isOpen {
		return fmt.Errorf("data channel is closed")
	}
	mdc.sentMessages = append(mdc.sentMessages, data)
	return nil
}

func (mdc *MockDataChannel) ReadyState() webrtc.DataChannelState {
	if mdc.isOpen {
		return webrtc.DataChannelStateOpen
	}
	return webrtc.DataChannelStateClosed
}

func (mdc *MockDataChannel) Close() {
	mdc.isOpen = false
}

func (mdc *MockDataChannel) GetSentMessages() [][]byte {
	return mdc.sentMessages
}

func (mdc *MockDataChannel) GetMessageCount() int {
	return len(mdc.sentMessages)
}

func TestNewFileTransmitter(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	// Create temporary directory for result collector
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	if ft.workerID != workerID {
		t.Errorf("Expected worker ID %d, got %d", workerID, ft.workerID)
	}

	if ft.dataChannel != mockDC {
		t.Error("Expected data channel to be set correctly")
	}

	if ft.resultCollector != rc {
		t.Error("Expected result collector to be set correctly")
	}

	if ft.IsTransmitting() {
		t.Error("Expected transmitter not to be transmitting initially")
	}
}

func TestSetTransmissionOptions(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	options := &TransmissionOptions{
		AckTimeout:    60 * time.Second,
		RetryAttempts: 5,
		ChunkDelay:    200 * time.Millisecond,
	}

	ft.SetTransmissionOptions(options)

	if ft.ackTimeout != options.AckTimeout {
		t.Errorf("Expected ack timeout %v, got %v", options.AckTimeout, ft.ackTimeout)
	}

	if ft.retryAttempts != options.RetryAttempts {
		t.Errorf("Expected retry attempts %d, got %d", options.RetryAttempts, ft.retryAttempts)
	}
}

func TestTransmitAllFilesEmpty(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	// Test with no files
	err = ft.TransmitAllFiles(nil)
	if err != nil {
		t.Errorf("Expected no error for empty file list, got: %v", err)
	}

	// Should have sent no messages since no files to transmit
	if mockDC.GetMessageCount() != 0 {
		t.Errorf("Expected 0 messages for empty transmission, got %d", mockDC.GetMessageCount())
	}
}

func TestTransmissionStatistics(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	stats := ft.GetTransmissionStatistics()

	if stats["is_transmitting"] != false {
		t.Error("Expected is_transmitting to be false initially")
	}

	if stats["retry_attempts"] != 3 {
		t.Errorf("Expected retry_attempts to be 3, got %v", stats["retry_attempts"])
	}

	if stats["pending_acks"] != 0 {
		t.Errorf("Expected pending_acks to be 0, got %v", stats["pending_acks"])
	}

	// Should include collector statistics
	if _, exists := stats["collector_total_files"]; !exists {
		t.Error("Expected collector statistics to be included")
	}
}

func TestHandleAcknowledgment(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	// Create a mock acknowledgment message
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, workerID, "", nil)
	ackMsg.Metadata["ack_id"] = "test_ack_123"
	ackMsg.Metadata["ack_status"] = "success"

	// Handle acknowledgment (should not error even if no pending ack)
	err := ft.HandleAcknowledgment(ackMsg)
	if err != nil {
		t.Errorf("HandleAcknowledgment failed: %v", err)
	}

	// Test with missing ack_id
	badAckMsg := messaging.CreateMessage(messaging.MsgTypeAck, workerID, "", nil)
	err = ft.HandleAcknowledgment(badAckMsg)
	if err == nil {
		t.Error("Expected error for acknowledgment message missing ack_id")
	}
}

func TestCleanup(t *testing.T) {
	workerID := 1
	mockDC := NewMockDataChannel()

	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(workerID, tempDir)
	ft := NewFileTransmitter(workerID, mockDC, rc)

	// Add a mock pending acknowledgment
	ackChan := make(chan bool, 1)
	ft.ackMutex.Lock()
	ft.ackWaitMap["test_ack"] = ackChan
	ft.ackMutex.Unlock()

	// Cleanup should clear pending acknowledgments
	ft.Cleanup()

	// Check that acknowledgment was signaled as failed
	select {
	case success := <-ackChan:
		if success {
			t.Error("Expected cleanup to signal acknowledgment failure")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected acknowledgment channel to receive signal")
	}

	// Check that ackWaitMap is cleared
	ft.ackMutex.Lock()
	if len(ft.ackWaitMap) != 0 {
		t.Errorf("Expected ackWaitMap to be cleared, got %d entries", len(ft.ackWaitMap))
	}
	ft.ackMutex.Unlock()
}

func TestDefaultTransmissionOptions(t *testing.T) {
	options := DefaultTransmissionOptions()

	if options.AckTimeout != 30*time.Second {
		t.Errorf("Expected default ack timeout 30s, got %v", options.AckTimeout)
	}

	if options.RetryAttempts != 3 {
		t.Errorf("Expected default retry attempts 3, got %d", options.RetryAttempts)
	}

	if options.ChunkDelay != 100*time.Millisecond {
		t.Errorf("Expected default chunk delay 100ms, got %v", options.ChunkDelay)
	}
}
