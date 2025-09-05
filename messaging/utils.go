package messaging

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ValidateMessage checks if a message has all required fields
func ValidateMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	if msg.Type == "" {
		return fmt.Errorf("message type cannot be empty")
	}

	if msg.WorkerID < 0 {
		return fmt.Errorf("worker ID must be non-negative")
	}

	// Validate specific message types
	switch msg.Type {
	case MsgTypeImage:
		if msg.Filename == "" {
			return fmt.Errorf("image message must have filename")
		}
		if len(msg.Data) == 0 {
			return fmt.Errorf("image message must have data")
		}
	case MsgTypeResult:
		if len(msg.Data) == 0 {
			return fmt.Errorf("result message must have data")
		}
	case MsgTypeHeartbeat:
		// Heartbeat messages don't require additional validation
	case MsgTypeAck:
		// Acknowledgment messages don't require additional validation
	case MsgTypeError:
		if msg.Metadata == nil || msg.Metadata["error"] == nil {
			return fmt.Errorf("error message must have error details in metadata")
		}
	}

	return nil
}

// ValidateResult checks if a result has all required fields
func ValidateResult(result *Result) error {
	if result == nil {
		return fmt.Errorf("result cannot be nil")
	}

	if result.WorkerID < 0 {
		return fmt.Errorf("worker ID must be non-negative")
	}

	// Validate processing data if present
	if result.ProcessingData != nil {
		return ValidateProcessingResult(result.ProcessingData)
	}

	return nil
}

// ValidateProcessingResult checks if a processing result has all required fields
func ValidateProcessingResult(result *ProcessingResult) error {
	if result == nil {
		return fmt.Errorf("processing result cannot be nil")
	}

	if result.WorkerID < 0 {
		return fmt.Errorf("worker ID must be non-negative")
	}

	if result.InputFileCount < 0 {
		return fmt.Errorf("input file count must be non-negative")
	}

	if result.OutputFileCount < 0 {
		return fmt.Errorf("output file count must be non-negative")
	}

	if result.OutputFileCount != len(result.OutputFiles) {
		return fmt.Errorf("output file count mismatch: expected %d, got %d",
			result.OutputFileCount, len(result.OutputFiles))
	}

	// Validate each output file
	for i, file := range result.OutputFiles {
		if err := ValidateProcessedFile(&file); err != nil {
			return fmt.Errorf("invalid output file at index %d: %w", i, err)
		}
	}

	return nil
}

// ValidateProcessedFile checks if a processed file has all required fields
func ValidateProcessedFile(file *ProcessedFile) error {
	if file == nil {
		return fmt.Errorf("processed file cannot be nil")
	}

	if file.Filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	if file.Size < 0 {
		return fmt.Errorf("file size must be non-negative")
	}

	if file.Size != int64(len(file.Data)) {
		return fmt.Errorf("file size mismatch: expected %d, got %d",
			file.Size, len(file.Data))
	}

	if file.Checksum == "" {
		return fmt.Errorf("checksum cannot be empty")
	}

	// Verify checksum
	expectedChecksum := CalculateChecksum(file.Data)
	if file.Checksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s",
			expectedChecksum, file.Checksum)
	}

	return nil
}

// CalculateChecksum calculates SHA256 checksum for data
func CalculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// CreateProcessedFile creates a ProcessedFile with calculated checksum
func CreateProcessedFile(filename string, data []byte) *ProcessedFile {
	return &ProcessedFile{
		Filename:    filename,
		Size:        int64(len(data)),
		Checksum:    CalculateChecksum(data),
		ProcessedAt: time.Now(),
		Data:        data,
	}
}

// IsMessageType checks if a message is of a specific type
func IsMessageType(msg *Message, msgType MessageType) bool {
	return msg != nil && msg.Type == msgType
}

// GetMessageAge returns the age of a message based on its timestamp
func GetMessageAge(msg *Message) time.Duration {
	if msg == nil || msg.Timestamp.IsZero() {
		return 0
	}
	return time.Since(msg.Timestamp)
}

// AddMetadata adds a key-value pair to message metadata
func AddMetadata(msg *Message, key string, value interface{}) {
	if msg == nil {
		return
	}
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}
	msg.Metadata[key] = value
}

// GetMetadata retrieves a value from message metadata
func GetMetadata(msg *Message, key string) (interface{}, bool) {
	if msg == nil || msg.Metadata == nil {
		return nil, false
	}
	value, exists := msg.Metadata[key]
	return value, exists
}
