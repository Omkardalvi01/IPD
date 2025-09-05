package messaging

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
)

// MockDataChannel implements a mock WebRTC data channel for testing
type MockDataChannel struct {
	sentMessages [][]byte
	readyState   webrtc.DataChannelState
	mutex        sync.RWMutex
}

func NewMockDataChannel() *MockDataChannel {
	return &MockDataChannel{
		sentMessages: make([][]byte, 0),
		readyState:   webrtc.DataChannelStateOpen,
	}
}

func (mdc *MockDataChannel) Send(data []byte) error {
	mdc.mutex.Lock()
	defer mdc.mutex.Unlock()
	mdc.sentMessages = append(mdc.sentMessages, data)
	return nil
}

func (mdc *MockDataChannel) ReadyState() webrtc.DataChannelState {
	mdc.mutex.RLock()
	defer mdc.mutex.RUnlock()
	return mdc.readyState
}

func (mdc *MockDataChannel) SetReadyState(state webrtc.DataChannelState) {
	mdc.mutex.Lock()
	defer mdc.mutex.Unlock()
	mdc.readyState = state
}

func (mdc *MockDataChannel) GetSentMessages() [][]byte {
	mdc.mutex.RLock()
	defer mdc.mutex.RUnlock()
	messages := make([][]byte, len(mdc.sentMessages))
	copy(messages, mdc.sentMessages)
	return messages
}

func (mdc *MockDataChannel) ClearSentMessages() {
	mdc.mutex.Lock()
	defer mdc.mutex.Unlock()
	mdc.sentMessages = mdc.sentMessages[:0]
}

func TestNewHeartbeatManager(t *testing.T) {
	workerID := 1
	interval := 10 * time.Second
	timeout := 30 * time.Second
	callbackCalled := false

	callback := func() {
		callbackCalled = true
	}

	hm := NewHeartbeatManager(workerID, interval, timeout, callback)

	assert.NotNil(t, hm)
	assert.Equal(t, workerID, hm.workerID)
	assert.Equal(t, interval, hm.interval)
	assert.Equal(t, timeout, hm.timeout)
	assert.NotNil(t, hm.failureCallback)
	assert.False(t, hm.isRunning)
	assert.NotNil(t, hm.stopChan)

	// Use the callback to avoid unused variable warning
	_ = callbackCalled
}

func TestHeartbeatManager_StartWithNilDataChannel(t *testing.T) {
	hm := NewHeartbeatManager(1, 100*time.Millisecond, 300*time.Millisecond, nil)

	// Test start with nil data channel
	err := hm.Start(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "data channel cannot be nil")
}

func TestHeartbeatManager_Stop(t *testing.T) {
	hm := NewHeartbeatManager(1, 100*time.Millisecond, 300*time.Millisecond, nil)

	// Test stopping non-running manager (should not panic)
	hm.Stop()
	assert.False(t, hm.IsRunning())
}

func TestHeartbeatManager_UpdateLastReceived(t *testing.T) {
	hm := NewHeartbeatManager(1, 10*time.Second, 30*time.Second, nil)

	initialTime := hm.GetLastReceived()
	time.Sleep(10 * time.Millisecond) // Ensure time difference

	hm.UpdateLastReceived()
	updatedTime := hm.GetLastReceived()

	assert.True(t, updatedTime.After(initialTime))
}

func TestHeartbeatManager_IsHealthy(t *testing.T) {
	hm := NewHeartbeatManager(1, 100*time.Millisecond, 200*time.Millisecond, nil)

	// Not running should be unhealthy
	assert.False(t, hm.IsHealthy())

	// Manually set running state for testing
	hm.mutex.Lock()
	hm.isRunning = true
	hm.mutex.Unlock()

	// Just started should be healthy
	assert.True(t, hm.IsHealthy())

	// Simulate timeout by setting last received to past
	hm.mutex.Lock()
	hm.lastReceived = time.Now().Add(-300 * time.Millisecond) // Beyond timeout
	hm.mutex.Unlock()

	assert.False(t, hm.IsHealthy())
}

func TestHeartbeatManager_SendHeartbeatWithMock(t *testing.T) {
	hm := NewHeartbeatManager(1, 10*time.Second, 30*time.Second, nil)
	mockDC := NewMockDataChannel()

	// Test successful heartbeat send using sendHeartbeatToInterface
	err := hm.sendHeartbeatToInterface(mockDC)
	assert.NoError(t, err)

	messages := mockDC.GetSentMessages()
	assert.Len(t, messages, 1)

	// Verify the sent message is a valid heartbeat
	msg, err := DeserializeMessage(messages[0])
	assert.NoError(t, err)
	assert.Equal(t, MsgTypeHeartbeat, msg.Type)
	assert.Equal(t, 1, msg.WorkerID)

	// Test send with closed data channel
	mockDC.SetReadyState(webrtc.DataChannelStateClosed)
	err = hm.sendHeartbeatToInterface(mockDC)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "data channel is not open")
}

func TestHeartbeatManager_HandleHeartbeatMessage(t *testing.T) {
	hm := NewHeartbeatManager(1, 10*time.Second, 30*time.Second, nil)

	// Test with nil message
	err := hm.HandleHeartbeatMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message cannot be nil")

	// Test with wrong message type
	wrongMsg := CreateMessage(MsgTypeImage, 2, "test.jpg", []byte("data"))
	err = hm.HandleHeartbeatMessage(wrongMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected heartbeat message")

	// Test with valid heartbeat message
	heartbeatMsg := CreateMessage(MsgTypeHeartbeat, 2, "", nil)
	AddMetadata(heartbeatMsg, "heartbeat_id", "test_123")

	initialTime := hm.GetLastReceived()
	time.Sleep(10 * time.Millisecond)

	err = hm.HandleHeartbeatMessage(heartbeatMsg)
	assert.NoError(t, err)
	assert.True(t, hm.GetLastReceived().After(initialTime))
}

func TestCreateHeartbeatResponse(t *testing.T) {
	originalMsg := CreateMessage(MsgTypeHeartbeat, 2, "", nil)
	AddMetadata(originalMsg, "heartbeat_id", "test_123")

	response := CreateHeartbeatResponse(1, originalMsg)

	assert.NotNil(t, response)
	assert.Equal(t, MsgTypeHeartbeat, response.Type)
	assert.Equal(t, 1, response.WorkerID)

	// Check metadata
	responseFlag, exists := GetMetadata(response, "response")
	assert.True(t, exists)
	assert.Equal(t, true, responseFlag)

	responseTo, exists := GetMetadata(response, "response_to")
	assert.True(t, exists)
	assert.Equal(t, "test_123", responseTo)
}

func TestHeartbeatConfig_DefaultAndValidation(t *testing.T) {
	// Test default config
	config := DefaultHeartbeatConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 30*time.Second, config.Interval)
	assert.Equal(t, 90*time.Second, config.Timeout)
	assert.True(t, config.Enabled)

	// Test valid config
	err := config.Validate()
	assert.NoError(t, err)

	// Test invalid interval
	config.Interval = 0
	err = config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be positive")

	// Test invalid timeout
	config.Interval = 30 * time.Second
	config.Timeout = 0
	err = config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout must be positive")

	// Test timeout <= interval
	config.Timeout = 20 * time.Second
	err = config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.Contains(t, err.Error(), "must be greater than interval")
}

func TestHeartbeatManager_ConcurrentAccess(t *testing.T) {
	hm := NewHeartbeatManager(1, 100*time.Millisecond, 300*time.Millisecond, nil)

	// Test concurrent access to methods
	var wg sync.WaitGroup

	// Concurrent UpdateLastReceived calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hm.UpdateLastReceived()
		}()
	}

	// Concurrent IsHealthy calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hm.IsHealthy()
		}()
	}

	// Concurrent GetLastReceived calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hm.GetLastReceived()
		}()
	}

	wg.Wait()

	// Should not panic or cause race conditions
	assert.True(t, true, "Concurrent access test completed without issues")
}

func TestHeartbeatManager_TimeoutDetection(t *testing.T) {
	callbackCalled := false
	var callbackMutex sync.Mutex

	callback := func() {
		callbackMutex.Lock()
		callbackCalled = true
		callbackMutex.Unlock()
	}

	// Use very short intervals for testing
	hm := NewHeartbeatManager(1, 50*time.Millisecond, 100*time.Millisecond, callback)

	// Manually start monitoring without starting the full heartbeat system
	hm.mutex.Lock()
	hm.isRunning = true
	hm.lastReceived = time.Now().Add(-200 * time.Millisecond) // Set to past timeout
	hm.mutex.Unlock()

	// Start monitoring goroutine
	go hm.monitorHeartbeats()

	// Wait for timeout detection
	time.Sleep(150 * time.Millisecond)

	callbackMutex.Lock()
	wasCallbackCalled := callbackCalled
	callbackMutex.Unlock()

	assert.True(t, wasCallbackCalled, "Failure callback should have been called")

	hm.Stop()
}
