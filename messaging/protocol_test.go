package messaging

import (
	"testing"
	"time"
)

func TestMessageSerialization(t *testing.T) {
	// Test basic message serialization
	msg := CreateMessage(MsgTypeImage, 1, "test.jpg", []byte("test data"))

	// Serialize
	data, err := SerializeMessage(msg)
	if err != nil {
		t.Fatalf("Failed to serialize message: %v", err)
	}

	// Deserialize
	deserializedMsg, err := DeserializeMessage(data)
	if err != nil {
		t.Fatalf("Failed to deserialize message: %v", err)
	}

	// Verify fields
	if deserializedMsg.Type != msg.Type {
		t.Errorf("Expected type %s, got %s", msg.Type, deserializedMsg.Type)
	}
	if deserializedMsg.WorkerID != msg.WorkerID {
		t.Errorf("Expected worker ID %d, got %d", msg.WorkerID, deserializedMsg.WorkerID)
	}
	if deserializedMsg.Filename != msg.Filename {
		t.Errorf("Expected filename %s, got %s", msg.Filename, deserializedMsg.Filename)
	}
	if string(deserializedMsg.Data) != string(msg.Data) {
		t.Errorf("Expected data %s, got %s", string(msg.Data), string(deserializedMsg.Data))
	}
}

func TestResultSerialization(t *testing.T) {
	// Create processing result
	processingResult := CreateProcessingResult(1, StatusSuccess, 5, 3)
	processingResult.ProcessingTime = time.Second * 10

	// Add processed files
	file1 := CreateProcessedFile("output1.txt", []byte("result 1"))
	file2 := CreateProcessedFile("output2.txt", []byte("result 2"))
	processingResult.OutputFiles = []ProcessedFile{*file1, *file2}
	processingResult.OutputFileCount = len(processingResult.OutputFiles)

	// Create result
	result := CreateResult(1, StatusSuccess, processingResult)

	// Serialize
	data, err := SerializeResult(result)
	if err != nil {
		t.Fatalf("Failed to serialize result: %v", err)
	}

	// Deserialize
	deserializedResult, err := DeserializeResult(data)
	if err != nil {
		t.Fatalf("Failed to deserialize result: %v", err)
	}

	// Verify fields
	if deserializedResult.WorkerID != result.WorkerID {
		t.Errorf("Expected worker ID %d, got %d", result.WorkerID, deserializedResult.WorkerID)
	}
	if deserializedResult.Status != result.Status {
		t.Errorf("Expected status %v, got %v", result.Status, deserializedResult.Status)
	}
	if deserializedResult.ProcessingData.OutputFileCount != 2 {
		t.Errorf("Expected 2 output files, got %d", deserializedResult.ProcessingData.OutputFileCount)
	}
}

func TestMessageValidation(t *testing.T) {
	// Test valid message
	validMsg := CreateMessage(MsgTypeImage, 1, "test.jpg", []byte("test data"))
	if err := ValidateMessage(validMsg); err != nil {
		t.Errorf("Valid message failed validation: %v", err)
	}

	// Test invalid message - no filename for image
	invalidMsg := CreateMessage(MsgTypeImage, 1, "", []byte("test data"))
	if err := ValidateMessage(invalidMsg); err == nil {
		t.Error("Invalid message passed validation")
	}

	// Test invalid message - no data for image
	invalidMsg2 := CreateMessage(MsgTypeImage, 1, "test.jpg", nil)
	if err := ValidateMessage(invalidMsg2); err == nil {
		t.Error("Invalid message passed validation")
	}
}

func TestProcessedFileValidation(t *testing.T) {
	// Test valid processed file
	validFile := CreateProcessedFile("test.txt", []byte("test content"))
	if err := ValidateProcessedFile(validFile); err != nil {
		t.Errorf("Valid processed file failed validation: %v", err)
	}

	// Test invalid processed file - wrong checksum
	invalidFile := &ProcessedFile{
		Filename:    "test.txt",
		Size:        12,
		Checksum:    "wrong_checksum",
		ProcessedAt: time.Now(),
		Data:        []byte("test content"),
	}
	if err := ValidateProcessedFile(invalidFile); err == nil {
		t.Error("Invalid processed file passed validation")
	}
}

func TestUtilityFunctions(t *testing.T) {
	// Test checksum calculation
	data := []byte("test data")
	checksum := CalculateChecksum(data)
	if checksum == "" {
		t.Error("Checksum should not be empty")
	}

	// Test message type checking
	msg := CreateMessage(MsgTypeHeartbeat, 1, "", nil)
	if !IsMessageType(msg, MsgTypeHeartbeat) {
		t.Error("Message type check failed")
	}

	// Test metadata operations
	AddMetadata(msg, "test_key", "test_value")
	value, exists := GetMetadata(msg, "test_key")
	if !exists || value != "test_value" {
		t.Error("Metadata operations failed")
	}
}

func TestProcessingStatusString(t *testing.T) {
	tests := []struct {
		status   ProcessingStatus
		expected string
	}{
		{StatusSuccess, "SUCCESS"},
		{StatusError, "ERROR"},
		{StatusTimeout, "TIMEOUT"},
		{StatusPartialSuccess, "PARTIAL_SUCCESS"},
	}

	for _, test := range tests {
		if test.status.String() != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, test.status.String())
		}
	}
}
