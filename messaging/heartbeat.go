package messaging

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// HeartbeatManager manages heartbeat functionality for persistent connections
type HeartbeatManager struct {
	interval        time.Duration
	timeout         time.Duration
	lastReceived    time.Time
	lastSent        time.Time
	stopChan        chan struct{}
	failureCallback func()
	mutex           sync.RWMutex
	isRunning       bool
	workerID        int
}

// NewHeartbeatManager creates a new HeartbeatManager instance
func NewHeartbeatManager(workerID int, interval, timeout time.Duration, failureCallback func()) *HeartbeatManager {
	return &HeartbeatManager{
		interval:        interval,
		timeout:         timeout,
		lastReceived:    time.Now(),
		lastSent:        time.Time{},
		stopChan:        make(chan struct{}),
		failureCallback: failureCallback,
		isRunning:       false,
		workerID:        workerID,
	}
}

// Start begins the heartbeat monitoring process
func (hm *HeartbeatManager) Start(dc *webrtc.DataChannel) error {
	if dc == nil {
		return fmt.Errorf("data channel cannot be nil")
	}

	hm.mutex.Lock()
	if hm.isRunning {
		hm.mutex.Unlock()
		return fmt.Errorf("heartbeat manager is already running")
	}
	hm.isRunning = true
	hm.mutex.Unlock()

	// Start heartbeat sender goroutine
	go hm.sendHeartbeats(dc)

	// Start heartbeat monitor goroutine
	go hm.monitorHeartbeats()

	log.Printf("HeartbeatManager started for worker %d with interval %v and timeout %v",
		hm.workerID, hm.interval, hm.timeout)

	return nil
}

// Stop stops the heartbeat monitoring process
func (hm *HeartbeatManager) Stop() {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if !hm.isRunning {
		return
	}

	hm.isRunning = false
	close(hm.stopChan)
	log.Printf("HeartbeatManager stopped for worker %d", hm.workerID)
}

// UpdateLastReceived updates the timestamp of the last received heartbeat
func (hm *HeartbeatManager) UpdateLastReceived() {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	hm.lastReceived = time.Now()
}

// IsHealthy checks if the connection is healthy based on heartbeat timing
func (hm *HeartbeatManager) IsHealthy() bool {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	if !hm.isRunning {
		return false
	}

	timeSinceLastReceived := time.Since(hm.lastReceived)
	return timeSinceLastReceived <= hm.timeout
}

// GetLastReceived returns the timestamp of the last received heartbeat
func (hm *HeartbeatManager) GetLastReceived() time.Time {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	return hm.lastReceived
}

// GetLastSent returns the timestamp of the last sent heartbeat
func (hm *HeartbeatManager) GetLastSent() time.Time {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	return hm.lastSent
}

// IsRunning returns whether the heartbeat manager is currently running
func (hm *HeartbeatManager) IsRunning() bool {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	return hm.isRunning
}

// sendHeartbeats sends periodic heartbeat messages
func (hm *HeartbeatManager) sendHeartbeats(dc *webrtc.DataChannel) {
	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.stopChan:
			return
		case <-ticker.C:
			if err := hm.sendHeartbeat(dc); err != nil {
				log.Printf("Failed to send heartbeat for worker %d: %v", hm.workerID, err)
				// Continue trying to send heartbeats even if one fails
			}
		}
	}
}

// DataChannelInterface defines the interface we need for heartbeat functionality
type DataChannelInterface interface {
	Send(data []byte) error
	ReadyState() webrtc.DataChannelState
}

// sendHeartbeat sends a single heartbeat message
func (hm *HeartbeatManager) sendHeartbeat(dc *webrtc.DataChannel) error {
	return hm.sendHeartbeatToInterface(dc)
}

// sendHeartbeatToInterface sends a heartbeat message to any DataChannelInterface
func (hm *HeartbeatManager) sendHeartbeatToInterface(dc DataChannelInterface) error {
	heartbeatMsg := CreateMessage(MsgTypeHeartbeat, hm.workerID, "", nil)
	AddMetadata(heartbeatMsg, "heartbeat_id", fmt.Sprintf("%d_%d", hm.workerID, time.Now().UnixNano()))

	data, err := SerializeMessage(heartbeatMsg)
	if err != nil {
		return fmt.Errorf("failed to serialize heartbeat message: %w", err)
	}

	if dc.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel is not open, state: %s", dc.ReadyState())
	}

	err = dc.Send(data)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	hm.mutex.Lock()
	hm.lastSent = time.Now()
	hm.mutex.Unlock()

	return nil
}

// monitorHeartbeats monitors for heartbeat timeouts
func (hm *HeartbeatManager) monitorHeartbeats() {
	ticker := time.NewTicker(hm.interval / 2) // Check more frequently than sending
	defer ticker.Stop()

	for {
		select {
		case <-hm.stopChan:
			return
		case <-ticker.C:
			if !hm.IsHealthy() {
				log.Printf("Heartbeat timeout detected for worker %d. Last received: %v",
					hm.workerID, hm.GetLastReceived())

				if hm.failureCallback != nil {
					go hm.failureCallback() // Run callback in separate goroutine to avoid blocking
				}
				return // Stop monitoring after failure
			}
		}
	}
}

// HandleHeartbeatMessage processes incoming heartbeat messages
func (hm *HeartbeatManager) HandleHeartbeatMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	if msg.Type != MsgTypeHeartbeat {
		return fmt.Errorf("expected heartbeat message, got %s", msg.Type)
	}

	hm.UpdateLastReceived()

	// Log heartbeat reception for debugging
	if heartbeatID, exists := GetMetadata(msg, "heartbeat_id"); exists {
		log.Printf("Received heartbeat from worker %d: %v", msg.WorkerID, heartbeatID)
	}

	return nil
}

// CreateHeartbeatResponse creates a heartbeat response message
func CreateHeartbeatResponse(workerID int, originalMsg *Message) *Message {
	response := CreateMessage(MsgTypeHeartbeat, workerID, "", nil)

	// Copy heartbeat ID from original message if present
	if heartbeatID, exists := GetMetadata(originalMsg, "heartbeat_id"); exists {
		AddMetadata(response, "response_to", heartbeatID)
	}

	AddMetadata(response, "response", true)
	return response
}

// HeartbeatConfig holds configuration for heartbeat functionality
type HeartbeatConfig struct {
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Enabled  bool          `json:"enabled"`
}

// DefaultHeartbeatConfig returns default heartbeat configuration
func DefaultHeartbeatConfig() *HeartbeatConfig {
	return &HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  90 * time.Second, // 3x interval
		Enabled:  true,
	}
}

// Validate checks if the heartbeat configuration is valid
func (hc *HeartbeatConfig) Validate() error {
	if hc.Interval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}

	if hc.Timeout <= 0 {
		return fmt.Errorf("heartbeat timeout must be positive")
	}

	if hc.Timeout <= hc.Interval {
		return fmt.Errorf("heartbeat timeout (%v) must be greater than interval (%v)",
			hc.Timeout, hc.Interval)
	}

	return nil
}
