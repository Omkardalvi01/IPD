package messaging

import (
	"encoding/json"
	"fmt"
	"time"
)

// SerializeMessage converts a Message struct to JSON bytes
func SerializeMessage(msg *Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}

	// Set timestamp if not already set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	return data, nil
}

// DeserializeMessage converts JSON bytes to a Message struct
func DeserializeMessage(data []byte) (*Message, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data cannot be empty")
	}

	var msg Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize message: %w", err)
	}

	return &msg, nil
}

// SerializeResult converts a Result struct to JSON bytes
func SerializeResult(result *Result) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("result cannot be nil")
	}

	// Set timestamp if not already set
	if result.Timestamp.IsZero() {
		result.Timestamp = time.Now()
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize result: %w", err)
	}

	return data, nil
}

// DeserializeResult converts JSON bytes to a Result struct
func DeserializeResult(data []byte) (*Result, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data cannot be empty")
	}

	var result Result
	err := json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize result: %w", err)
	}

	return &result, nil
}

// SerializeProcessingResult converts a ProcessingResult struct to JSON bytes
func SerializeProcessingResult(result *ProcessingResult) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("processing result cannot be nil")
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize processing result: %w", err)
	}

	return data, nil
}

// DeserializeProcessingResult converts JSON bytes to a ProcessingResult struct
func DeserializeProcessingResult(data []byte) (*ProcessingResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data cannot be empty")
	}

	var result ProcessingResult
	err := json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize processing result: %w", err)
	}

	return &result, nil
}

// CreateMessage is a helper function to create a new Message with common fields
func CreateMessage(msgType MessageType, workerID int, filename string, data []byte) *Message {
	return &Message{
		Type:      msgType,
		WorkerID:  workerID,
		Filename:  filename,
		Data:      data,
		Metadata:  make(map[string]interface{}),
		Timestamp: time.Now(),
	}
}

// CreateResult is a helper function to create a new Result with common fields
func CreateResult(workerID int, status ProcessingStatus, processingData *ProcessingResult) *Result {
	return &Result{
		WorkerID:       workerID,
		Status:         status,
		ProcessingData: processingData,
		Timestamp:      time.Now(),
	}
}

// CreateProcessingResult is a helper function to create a new ProcessingResult
func CreateProcessingResult(workerID int, status ProcessingStatus, inputCount, outputCount int) *ProcessingResult {
	return &ProcessingResult{
		WorkerID:        workerID,
		Status:          status,
		InputFileCount:  inputCount,
		OutputFileCount: outputCount,
		OutputFiles:     make([]ProcessedFile, 0),
		Metadata:        make(map[string]interface{}),
	}
}
