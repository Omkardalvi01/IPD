package main

import (
	"time"
)

// MessageType represents different types of messages in the protocol
type MessageType string

const (
	ImageData        MessageType = "IMAGE_DATA"
	BatchComplete    MessageType = "BATCH_COMPLETE"
	TrainingStart    MessageType = "TRAINING_START"
	TrainingComplete MessageType = "TRAINING_COMPLETE"
	WeightsData      MessageType = "WEIGHTS_DATA"
	Error            MessageType = "ERROR"
	Heartbeat        MessageType = "HEARTBEAT"
	Terminate        MessageType = "TERMINATE"
)

// Message represents a protocol message
type Message struct {
	Type    MessageType `json:"type"`
	Data    []byte      `json:"data,omitempty"`
	Payload string      `json:"payload,omitempty"`
}

// TrainingState represents the current state of the worker
type TrainingState int

const (
	Idle TrainingState = iota
	ReceivingData
	Training
	SendingWeights
	ErrorState
)

// String returns string representation of TrainingState
func (ts TrainingState) String() string {
	switch ts {
	case Idle:
		return "Idle"
	case ReceivingData:
		return "ReceivingData"
	case Training:
		return "Training"
	case SendingWeights:
		return "SendingWeights"
	case ErrorState:
		return "Error"
	default:
		return "Unknown"
	}
}

// WorkerState tracks the current state of the worker
type WorkerState struct {
	CurrentState      TrainingState
	BatchID           string
	TrainingStartTime time.Time
	LastHeartbeat     time.Time
	ErrorMessage      string
}

// Config holds the worker configuration
type Config struct {
	WorkerID          string `json:"worker_id"`
	PythonScript      string `json:"python_script"`
	BatchDirectory    string `json:"batch_directory"`
	WeightsPath       string `json:"weights_path"`
	SignalingServer   string `json:"signaling_server"`
	HeartbeatInterval string `json:"heartbeat_interval"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		PythonScript:      "train.py",
		BatchDirectory:    "./batches",
		WeightsPath:       "./weights",
		SignalingServer:   "ws://localhost:8000/",
		HeartbeatInterval: "30s",
	}
}

// GetHeartbeatDuration parses the heartbeat interval string into a time.Duration
func (c *Config) GetHeartbeatDuration() (time.Duration, error) {
	return time.ParseDuration(c.HeartbeatInterval)
}
