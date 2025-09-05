package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
	"github.com/pion/webrtc/v3"
)

// ConnectionState represents the current state of a connection
type ConnectionState int

const (
	ConnectionStateActive ConnectionState = iota
	ConnectionStateClosing
	ConnectionStateClosed
	ConnectionStateError
)

// String returns the string representation of ConnectionState
func (cs ConnectionState) String() string {
	switch cs {
	case ConnectionStateActive:
		return "ACTIVE"
	case ConnectionStateClosing:
		return "CLOSING"
	case ConnectionStateClosed:
		return "CLOSED"
	case ConnectionStateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ConnectionInfo holds information about a worker connection
type ConnectionInfo struct {
	WorkerID     int
	State        ConnectionState
	DataChannel  *webrtc.DataChannel
	PeerConn     *webrtc.PeerConnection
	LastActivity time.Time
	CloseReason  messaging.ConnectionCloseReason
	CloseMessage string
	AckReceived  bool
	CloseTimeout *time.Timer
	mutex        sync.RWMutex
}

// GetState returns the current connection state
func (ci *ConnectionInfo) GetState() ConnectionState {
	ci.mutex.RLock()
	defer ci.mutex.RUnlock()
	return ci.State
}

// SetState sets the connection state
func (ci *ConnectionInfo) SetState(state ConnectionState) {
	ci.mutex.Lock()
	defer ci.mutex.Unlock()

	oldState := ci.State
	ci.State = state
	ci.LastActivity = time.Now()

	log.Printf("Connection %d state changed: %s -> %s", ci.WorkerID, oldState, state)
}

// ConnectionLifecycleManager manages the lifecycle of worker connections
type ConnectionLifecycleManager struct {
	connections       map[int]*ConnectionInfo
	mutex             sync.RWMutex
	closeTimeout      time.Duration
	gracefulTimeout   time.Duration
	completionHandler func(workerID int, reason messaging.ConnectionCloseReason)
	errorHandler      func(workerID int, err error)
}

// NewConnectionLifecycleManager creates a new connection lifecycle manager
func NewConnectionLifecycleManager(closeTimeout, gracefulTimeout time.Duration) *ConnectionLifecycleManager {
	return &ConnectionLifecycleManager{
		connections:     make(map[int]*ConnectionInfo),
		closeTimeout:    closeTimeout,
		gracefulTimeout: gracefulTimeout,
	}
}

// SetCompletionHandler sets the handler for connection completion events
func (clm *ConnectionLifecycleManager) SetCompletionHandler(handler func(workerID int, reason messaging.ConnectionCloseReason)) {
	clm.completionHandler = handler
}

// SetErrorHandler sets the handler for connection error events
func (clm *ConnectionLifecycleManager) SetErrorHandler(handler func(workerID int, err error)) {
	clm.errorHandler = handler
}

// RegisterConnection registers a new worker connection
func (clm *ConnectionLifecycleManager) RegisterConnection(workerID int, dc *webrtc.DataChannel, pc *webrtc.PeerConnection) {
	clm.mutex.Lock()
	defer clm.mutex.Unlock()

	connInfo := &ConnectionInfo{
		WorkerID:     workerID,
		State:        ConnectionStateActive,
		DataChannel:  dc,
		PeerConn:     pc,
		LastActivity: time.Now(),
		AckReceived:  false,
	}

	clm.connections[workerID] = connInfo

	// Set up connection event handlers
	dc.OnClose(func() {
		clm.handleConnectionClosed(workerID)
	})

	dc.OnError(func(err error) {
		clm.handleConnectionError(workerID, err)
	})

	log.Printf("Connection lifecycle manager registered worker %d", workerID)
}

// InitiateGracefulClose initiates graceful closure of a worker connection
func (clm *ConnectionLifecycleManager) InitiateGracefulClose(workerID int, reason messaging.ConnectionCloseReason, message string) error {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return fmt.Errorf("connection not found for worker %d", workerID)
	}

	if connInfo.GetState() != ConnectionStateActive {
		clm.mutex.Unlock()
		return fmt.Errorf("connection for worker %d is not active (state: %s)", workerID, connInfo.GetState())
	}

	connInfo.SetState(ConnectionStateClosing)
	connInfo.CloseReason = reason
	connInfo.CloseMessage = message
	clm.mutex.Unlock()

	log.Printf("Initiating graceful close for worker %d, reason: %s", workerID, reason)

	// Send connection close message to worker
	closeMsg := messaging.CreateConnectionCloseMessage(workerID, reason, message)
	if err := clm.sendMessage(workerID, closeMsg); err != nil {
		log.Printf("Failed to send close message to worker %d: %v", workerID, err)
		// Force close if we can't send the message
		return clm.ForceClose(workerID, messaging.CloseReasonError, "Failed to send close message")
	}

	// Set timeout for graceful close
	connInfo.CloseTimeout = time.AfterFunc(clm.gracefulTimeout, func() {
		log.Printf("Graceful close timeout for worker %d, forcing close", workerID)
		clm.ForceClose(workerID, messaging.CloseReasonTimeout, "Graceful close timeout")
	})

	return nil
}

// HandleCloseAcknowledgment handles acknowledgment of close message from worker
func (clm *ConnectionLifecycleManager) HandleCloseAcknowledgment(workerID int) error {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return fmt.Errorf("connection not found for worker %d", workerID)
	}

	if connInfo.GetState() != ConnectionStateClosing {
		clm.mutex.Unlock()
		return fmt.Errorf("worker %d is not in closing state (state: %s)", workerID, connInfo.GetState())
	}

	connInfo.AckReceived = true
	clm.mutex.Unlock()

	log.Printf("Received close acknowledgment from worker %d", workerID)

	// Cancel the timeout timer
	if connInfo.CloseTimeout != nil {
		connInfo.CloseTimeout.Stop()
	}

	// Proceed with actual connection closure
	return clm.completeClose(workerID)
}

// ForceClose forcefully closes a worker connection
func (clm *ConnectionLifecycleManager) ForceClose(workerID int, reason messaging.ConnectionCloseReason, message string) error {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return fmt.Errorf("connection not found for worker %d", workerID)
	}

	connInfo.SetState(ConnectionStateClosing)
	connInfo.CloseReason = reason
	connInfo.CloseMessage = message
	clm.mutex.Unlock()

	log.Printf("Force closing connection for worker %d, reason: %s", workerID, reason)

	// Cancel any pending timeout
	if connInfo.CloseTimeout != nil {
		connInfo.CloseTimeout.Stop()
	}

	return clm.completeClose(workerID)
}

// completeClose completes the connection closure process
func (clm *ConnectionLifecycleManager) completeClose(workerID int) error {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return fmt.Errorf("connection not found for worker %d", workerID)
	}

	connInfo.SetState(ConnectionStateClosed)
	clm.mutex.Unlock()

	// Close the actual connections
	if connInfo.DataChannel != nil {
		connInfo.DataChannel.Close()
	}
	if connInfo.PeerConn != nil {
		connInfo.PeerConn.Close()
	}

	// Notify completion handler
	if clm.completionHandler != nil {
		clm.completionHandler(workerID, connInfo.CloseReason)
	}

	log.Printf("Connection closed for worker %d, reason: %s", workerID, connInfo.CloseReason)
	return nil
}

// handleConnectionClosed handles the actual connection closed event
func (clm *ConnectionLifecycleManager) handleConnectionClosed(workerID int) {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return
	}

	if connInfo.GetState() != ConnectionStateClosed {
		connInfo.SetState(ConnectionStateClosed)
	}
	clm.mutex.Unlock()

	log.Printf("Connection actually closed for worker %d", workerID)

	// Clean up resources
	clm.cleanupConnection(workerID)
}

// handleConnectionError handles connection errors
func (clm *ConnectionLifecycleManager) handleConnectionError(workerID int, err error) {
	clm.mutex.Lock()
	connInfo, exists := clm.connections[workerID]
	if !exists {
		clm.mutex.Unlock()
		return
	}

	connInfo.SetState(ConnectionStateError)
	connInfo.CloseReason = messaging.CloseReasonError
	connInfo.CloseMessage = err.Error()
	clm.mutex.Unlock()

	log.Printf("Connection error for worker %d: %v", workerID, err)

	// Notify error handler
	if clm.errorHandler != nil {
		clm.errorHandler(workerID, err)
	}

	// Force cleanup
	clm.cleanupConnection(workerID)
}

// cleanupConnection cleans up resources for a connection
func (clm *ConnectionLifecycleManager) cleanupConnection(workerID int) {
	clm.mutex.Lock()
	defer clm.mutex.Unlock()

	connInfo, exists := clm.connections[workerID]
	if !exists {
		return
	}

	// Cancel any pending timers
	if connInfo.CloseTimeout != nil {
		connInfo.CloseTimeout.Stop()
	}

	// Remove from connections map
	delete(clm.connections, workerID)

	log.Printf("Cleaned up connection resources for worker %d", workerID)
}

// sendMessage sends a message to a specific worker
func (clm *ConnectionLifecycleManager) sendMessage(workerID int, msg *messaging.Message) error {
	clm.mutex.RLock()
	connInfo, exists := clm.connections[workerID]
	clm.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found for worker %d", workerID)
	}

	if connInfo.DataChannel == nil || connInfo.DataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not available for worker %d", workerID)
	}

	data, err := messaging.SerializeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	if err := connInfo.DataChannel.Send(data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetConnectionState returns the current state of a connection
func (clm *ConnectionLifecycleManager) GetConnectionState(workerID int) (ConnectionState, bool) {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()

	connInfo, exists := clm.connections[workerID]
	if !exists {
		return ConnectionStateClosed, false
	}

	return connInfo.GetState(), true
}

// GetActiveConnections returns a list of active connection IDs
func (clm *ConnectionLifecycleManager) GetActiveConnections() []int {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()

	var active []int
	for workerID, connInfo := range clm.connections {
		if connInfo.GetState() == ConnectionStateActive {
			active = append(active, workerID)
		}
	}

	return active
}

// GetAllConnections returns information about all connections
func (clm *ConnectionLifecycleManager) GetAllConnections() map[int]ConnectionState {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()

	states := make(map[int]ConnectionState)
	for workerID, connInfo := range clm.connections {
		states[workerID] = connInfo.GetState()
	}

	return states
}

// InitiateShutdown initiates shutdown of all connections
func (clm *ConnectionLifecycleManager) InitiateShutdown(reason string) {
	clm.mutex.RLock()
	workerIDs := make([]int, 0, len(clm.connections))
	for workerID := range clm.connections {
		workerIDs = append(workerIDs, workerID)
	}
	clm.mutex.RUnlock()

	log.Printf("Initiating shutdown of %d connections, reason: %s", len(workerIDs), reason)

	// Send shutdown message to all workers
	shutdownMsg := messaging.CreateShutdownMessage("server", reason)
	for _, workerID := range workerIDs {
		if err := clm.sendMessage(workerID, shutdownMsg); err != nil {
			log.Printf("Failed to send shutdown message to worker %d: %v", workerID, err)
		}
	}

	// Give workers time to acknowledge, then force close
	time.AfterFunc(clm.gracefulTimeout, func() {
		for _, workerID := range workerIDs {
			if state, exists := clm.GetConnectionState(workerID); exists && state != ConnectionStateClosed {
				clm.ForceClose(workerID, messaging.CloseReasonServerRequest, "Shutdown timeout")
			}
		}
	})
}

// GetHealthyConnectionCount returns the number of healthy connections
func (clm *ConnectionLifecycleManager) GetHealthyConnectionCount() int {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()

	count := 0
	for _, connInfo := range clm.connections {
		if connInfo.GetState() == ConnectionStateActive {
			count++
		}
	}
	return count
}

// GetTotalConnectionCount returns the total number of connections
func (clm *ConnectionLifecycleManager) GetTotalConnectionCount() int {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()
	return len(clm.connections)
}

// GetUnhealthyConnections returns a list of unhealthy connection worker IDs
func (clm *ConnectionLifecycleManager) GetUnhealthyConnections() []int {
	clm.mutex.RLock()
	defer clm.mutex.RUnlock()

	var unhealthy []int
	for workerID, connInfo := range clm.connections {
		state := connInfo.GetState()
		if state != ConnectionStateActive {
			unhealthy = append(unhealthy, workerID)
		}
	}
	return unhealthy
}
