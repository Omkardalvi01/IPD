package messaging

import (
	"time"
)

// MessageType defines the different types of messages in the protocol
type MessageType string

const (
	MsgTypeImage           MessageType = "IMAGE"
	MsgTypeImageEnd        MessageType = "IMAGE_END"
	MsgTypeResult          MessageType = "RESULT"
	MsgTypeResultEnd       MessageType = "RESULT_END"
	MsgTypeHeartbeat       MessageType = "HEARTBEAT"
	MsgTypeAck             MessageType = "ACK"
	MsgTypeError           MessageType = "ERROR"
	MsgTypeConnectionClose MessageType = "CONNECTION_CLOSE"
	MsgTypeShutdown        MessageType = "SHUTDOWN"
)

// Message represents the base message structure for bidirectional communication
type Message struct {
	Type      MessageType            `json:"type"`
	WorkerID  int                    `json:"worker_id"`
	Filename  string                 `json:"filename,omitempty"`
	Data      []byte                 `json:"data,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ProcessingStatus represents the status of processing operations
type ProcessingStatus int

const (
	StatusSuccess ProcessingStatus = iota
	StatusError
	StatusTimeout
	StatusPartialSuccess
)

// String returns the string representation of ProcessingStatus
func (ps ProcessingStatus) String() string {
	switch ps {
	case StatusSuccess:
		return "SUCCESS"
	case StatusError:
		return "ERROR"
	case StatusTimeout:
		return "TIMEOUT"
	case StatusPartialSuccess:
		return "PARTIAL_SUCCESS"
	default:
		return "UNKNOWN"
	}
}

// ProcessedFile represents a file that has been processed by a worker
type ProcessedFile struct {
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	ProcessedAt time.Time `json:"processed_at"`
	Data        []byte    `json:"data"`
}

// ProcessingResult represents the enhanced result structure with metadata
type ProcessingResult struct {
	WorkerID        int                    `json:"worker_id"`
	Status          ProcessingStatus       `json:"status"`
	ProcessingTime  time.Duration          `json:"processing_time"`
	InputFileCount  int                    `json:"input_file_count"`
	OutputFileCount int                    `json:"output_file_count"`
	OutputFiles     []ProcessedFile        `json:"output_files"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// Enhanced Result structure that extends the original Result
type Result struct {
	WorkerID       int               `json:"worker_id"`
	Status         ProcessingStatus  `json:"status"`
	ProcessingData *ProcessingResult `json:"processing_data,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
}

// ConnectionCloseReason represents the reason for connection closure
type ConnectionCloseReason string

const (
	CloseReasonCompleted     ConnectionCloseReason = "COMPLETED"
	CloseReasonError         ConnectionCloseReason = "ERROR"
	CloseReasonTimeout       ConnectionCloseReason = "TIMEOUT"
	CloseReasonServerRequest ConnectionCloseReason = "SERVER_REQUEST"
	CloseReasonWorkerRequest ConnectionCloseReason = "WORKER_REQUEST"
)

// ConnectionCloseInfo contains information about connection closure
type ConnectionCloseInfo struct {
	WorkerID  int                   `json:"worker_id"`
	Reason    ConnectionCloseReason `json:"reason"`
	Message   string                `json:"message,omitempty"`
	Timestamp time.Time             `json:"timestamp"`
}

// ShutdownInfo contains information about system shutdown
type ShutdownInfo struct {
	InitiatedBy string    `json:"initiated_by"` // "server" or "worker"
	Reason      string    `json:"reason"`
	Timestamp   time.Time `json:"timestamp"`
}

// CreateConnectionCloseMessage creates a connection close message
func CreateConnectionCloseMessage(workerID int, reason ConnectionCloseReason, message string) *Message {
	closeInfo := ConnectionCloseInfo{
		WorkerID:  workerID,
		Reason:    reason,
		Message:   message,
		Timestamp: time.Now(),
	}

	msg := CreateMessage(MsgTypeConnectionClose, workerID, "", nil)
	msg.Metadata["close_info"] = closeInfo
	return msg
}

// CreateShutdownMessage creates a shutdown message
func CreateShutdownMessage(initiatedBy, reason string) *Message {
	shutdownInfo := ShutdownInfo{
		InitiatedBy: initiatedBy,
		Reason:      reason,
		Timestamp:   time.Now(),
	}

	msg := CreateMessage(MsgTypeShutdown, 0, "", nil)
	msg.Metadata["shutdown_info"] = shutdownInfo
	return msg
}
