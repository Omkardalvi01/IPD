package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
	"github.com/pion/webrtc/v3"
)

// ServerMessageHandler handles incoming messages from workers on the server side
type ServerMessageHandler struct {
	resultCollector      *ServerResultCollector
	workerChannels       map[int]*webrtc.DataChannel // Map of worker ID to data channel
	lifecycleManager     *ConnectionLifecycleManager
	completionAckTimeout time.Duration
}

// NewServerMessageHandler creates a new server message handler
func NewServerMessageHandler(resultCollector *ServerResultCollector, lifecycleManager *ConnectionLifecycleManager) *ServerMessageHandler {
	return &ServerMessageHandler{
		resultCollector:      resultCollector,
		workerChannels:       make(map[int]*webrtc.DataChannel),
		lifecycleManager:     lifecycleManager,
		completionAckTimeout: 30 * time.Second,
	}
}

// RegisterWorkerChannel registers a data channel for a specific worker
func (smh *ServerMessageHandler) RegisterWorkerChannel(workerID int, channel *webrtc.DataChannel) {
	smh.workerChannels[workerID] = channel
	log.Printf("Server registered data channel for worker %d", workerID)

	// Set up message handler for this channel
	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		smh.handleWorkerMessage(workerID, msg.Data)
	})
}

// handleWorkerMessage processes incoming messages from a specific worker
func (smh *ServerMessageHandler) handleWorkerMessage(workerID int, data []byte) {
	// Deserialize the message
	var msg messaging.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Server failed to deserialize message from worker %d: %v", workerID, err)
		return
	}

	log.Printf("Server received message from worker %d, type: %s", workerID, msg.Type)

	// Handle different message types
	switch msg.Type {
	case messaging.MsgTypeResult, messaging.MsgTypeError:
		// Forward result-related messages to the result collector
		if err := smh.resultCollector.HandleResultMessage(workerID, &msg); err != nil {
			log.Printf("Server error handling result message from worker %d: %v", workerID, err)
		}

		// Send acknowledgment back to worker
		smh.sendAcknowledgment(workerID, &msg)

	case messaging.MsgTypeResultEnd:
		// Handle result end message (worker completion)
		if err := smh.resultCollector.HandleResultMessage(workerID, &msg); err != nil {
			log.Printf("Server error handling result end message from worker %d: %v", workerID, err)
		}

		// Send acknowledgment back to worker
		smh.sendAcknowledgment(workerID, &msg)

		// Initiate graceful connection closure
		smh.handleWorkerCompletion(workerID)

	case messaging.MsgTypeConnectionClose:
		// Handle connection close requests from workers
		smh.handleConnectionCloseRequest(workerID, &msg)

	case messaging.MsgTypeHeartbeat:
		// Handle heartbeat messages
		smh.handleHeartbeat(workerID, &msg)

	case messaging.MsgTypeAck:
		// Handle acknowledgment messages
		smh.handleAcknowledgment(workerID, &msg)

	default:
		log.Printf("Server received unknown message type from worker %d: %s", workerID, msg.Type)
	}
}

// sendAcknowledgment sends an acknowledgment message back to a worker
func (smh *ServerMessageHandler) sendAcknowledgment(workerID int, originalMsg *messaging.Message) {
	channel, exists := smh.workerChannels[workerID]
	if !exists {
		log.Printf("Server cannot send ack to worker %d: no channel registered", workerID)
		return
	}

	// Create acknowledgment message
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, 0, "", nil) // Server ID is 0
	ackMsg.Metadata = map[string]interface{}{
		"ack_for":           originalMsg.Type,
		"original_filename": originalMsg.Filename,
		"worker_id":         workerID,
	}

	// Serialize and send
	data, err := json.Marshal(ackMsg)
	if err != nil {
		log.Printf("Server failed to serialize ack message for worker %d: %v", workerID, err)
		return
	}

	if err := channel.Send(data); err != nil {
		log.Printf("Server failed to send ack to worker %d: %v", workerID, err)
	} else {
		log.Printf("Server sent acknowledgment to worker %d for message type %s", workerID, originalMsg.Type)
	}
}

// handleHeartbeat processes heartbeat messages from workers
func (smh *ServerMessageHandler) handleHeartbeat(workerID int, msg *messaging.Message) {
	log.Printf("Server received heartbeat from worker %d", workerID)

	// Send heartbeat response
	channel, exists := smh.workerChannels[workerID]
	if !exists {
		log.Printf("Server cannot respond to heartbeat from worker %d: no channel registered", workerID)
		return
	}

	// Create heartbeat response
	heartbeatResponse := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 0, "", nil) // Server ID is 0
	heartbeatResponse.Metadata = map[string]interface{}{
		"response_to": workerID,
		"server_time": msg.Timestamp,
	}

	// Serialize and send
	data, err := json.Marshal(heartbeatResponse)
	if err != nil {
		log.Printf("Server failed to serialize heartbeat response for worker %d: %v", workerID, err)
		return
	}

	if err := channel.Send(data); err != nil {
		log.Printf("Server failed to send heartbeat response to worker %d: %v", workerID, err)
	}
}

// BroadcastMessage sends a message to all registered workers
func (smh *ServerMessageHandler) BroadcastMessage(msg *messaging.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Server failed to serialize broadcast message: %v", err)
		return
	}

	for workerID, channel := range smh.workerChannels {
		if err := channel.Send(data); err != nil {
			log.Printf("Server failed to broadcast message to worker %d: %v", workerID, err)
		} else {
			log.Printf("Server broadcasted message to worker %d", workerID)
		}
	}
}

// SendMessageToWorker sends a message to a specific worker
func (smh *ServerMessageHandler) SendMessageToWorker(workerID int, msg *messaging.Message) error {
	channel, exists := smh.workerChannels[workerID]
	if !exists {
		return fmt.Errorf("no channel registered for worker %d", workerID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	if err := channel.Send(data); err != nil {
		return fmt.Errorf("failed to send message to worker %d: %w", workerID, err)
	}

	log.Printf("Server sent message to worker %d, type: %s", workerID, msg.Type)
	return nil
}

// UnregisterWorkerChannel removes a worker's data channel
func (smh *ServerMessageHandler) UnregisterWorkerChannel(workerID int) {
	delete(smh.workerChannels, workerID)
	log.Printf("Server unregistered data channel for worker %d", workerID)
}

// GetRegisteredWorkers returns a list of registered worker IDs
func (smh *ServerMessageHandler) GetRegisteredWorkers() []int {
	workers := make([]int, 0, len(smh.workerChannels))
	for workerID := range smh.workerChannels {
		workers = append(workers, workerID)
	}
	return workers
}

// IsWorkerConnected checks if a worker is connected
func (smh *ServerMessageHandler) IsWorkerConnected(workerID int) bool {
	_, exists := smh.workerChannels[workerID]
	return exists
}

// handleWorkerCompletion handles worker completion and initiates connection closure
func (smh *ServerMessageHandler) handleWorkerCompletion(workerID int) {
	log.Printf("Server handling completion for worker %d", workerID)

	if smh.lifecycleManager != nil {
		// Initiate graceful close through lifecycle manager
		if err := smh.lifecycleManager.InitiateGracefulClose(workerID, messaging.CloseReasonCompleted, "Worker processing completed"); err != nil {
			log.Printf("Server failed to initiate graceful close for worker %d: %v", workerID, err)
			// Force close if graceful close fails
			smh.lifecycleManager.ForceClose(workerID, messaging.CloseReasonError, "Failed to initiate graceful close")
		}
	} else {
		log.Printf("Server lifecycle manager not available, cannot manage connection closure for worker %d", workerID)
	}
}

// handleConnectionCloseRequest handles connection close requests from workers
func (smh *ServerMessageHandler) handleConnectionCloseRequest(workerID int, msg *messaging.Message) {
	log.Printf("Server received connection close request from worker %d", workerID)

	// Send acknowledgment
	smh.sendAcknowledgment(workerID, msg)

	// Extract close information
	var reason messaging.ConnectionCloseReason = messaging.CloseReasonWorkerRequest
	var message string = "Worker requested connection close"

	if closeInfo, exists := msg.Metadata["close_info"]; exists {
		if closeInfoMap, ok := closeInfo.(map[string]interface{}); ok {
			if reasonStr, ok := closeInfoMap["reason"].(string); ok {
				reason = messaging.ConnectionCloseReason(reasonStr)
			}
			if msgStr, ok := closeInfoMap["message"].(string); ok {
				message = msgStr
			}
		}
	}

	// Handle through lifecycle manager
	if smh.lifecycleManager != nil {
		if err := smh.lifecycleManager.InitiateGracefulClose(workerID, reason, message); err != nil {
			log.Printf("Server failed to handle connection close request from worker %d: %v", workerID, err)
		}
	}
}

// handleAcknowledgment handles acknowledgment messages from workers
func (smh *ServerMessageHandler) handleAcknowledgment(workerID int, msg *messaging.Message) {
	log.Printf("Server received acknowledgment from worker %d", workerID)

	// Check if this is a close acknowledgment
	if ackFor, exists := msg.Metadata["ack_for"]; exists {
		if ackFor == string(messaging.MsgTypeConnectionClose) {
			// Handle close acknowledgment through lifecycle manager
			if smh.lifecycleManager != nil {
				if err := smh.lifecycleManager.HandleCloseAcknowledgment(workerID); err != nil {
					log.Printf("Server failed to handle close acknowledgment from worker %d: %v", workerID, err)
				}
			}
		}
	}
}

// SetLifecycleManager sets the connection lifecycle manager
func (smh *ServerMessageHandler) SetLifecycleManager(manager *ConnectionLifecycleManager) {
	smh.lifecycleManager = manager
}

// InitiateShutdown initiates shutdown of all worker connections
func (smh *ServerMessageHandler) InitiateShutdown(reason string) {
	log.Printf("Server initiating shutdown of all connections: %s", reason)

	if smh.lifecycleManager != nil {
		smh.lifecycleManager.InitiateShutdown(reason)
	} else {
		// Fallback: send shutdown messages directly
		shutdownMsg := messaging.CreateShutdownMessage("server", reason)
		smh.BroadcastMessage(shutdownMsg)
	}
}
