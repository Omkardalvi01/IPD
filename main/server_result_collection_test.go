package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

func TestServerResultCollector(t *testing.T) {
	// Create temporary directory for test results
	tempDir := t.TempDir()

	// Initialize collector
	collector := NewServerResultCollector(2, tempDir)

	// Test basic initialization
	if collector.GetExpectedWorkerCount() != 2 {
		t.Errorf("Expected 2 workers, got %d", collector.GetExpectedWorkerCount())
	}

	if collector.GetCompletedWorkerCount() != 0 {
		t.Errorf("Expected 0 completed workers, got %d", collector.GetCompletedWorkerCount())
	}

	if collector.IsComplete() {
		t.Error("Collector should not be complete initially")
	}
}

func TestServerResultCollectorHandleResultMessage(t *testing.T) {
	tempDir := t.TempDir()
	collector := NewServerResultCollector(1, tempDir)

	// Create a test processing result
	processingResult := &messaging.ProcessingResult{
		WorkerID:        1,
		Status:          messaging.StatusSuccess,
		ProcessingTime:  time.Second * 5,
		InputFileCount:  3,
		OutputFileCount: 2,
		OutputFiles: []messaging.ProcessedFile{
			{
				Filename:    "output1.txt",
				Size:        11,                                                                 // Correct size for "test data 1"
				Checksum:    "05e8fdb3598f91bcc3ce41a196e587b4592c8cdfc371c217274bfda2d24b1b4e", // Correct SHA256
				ProcessedAt: time.Now(),
				Data:        []byte("test data 1"),
			},
			{
				Filename:    "output2.txt",
				Size:        29,                                                                 // Correct size for "test data 2 with more content"
				Checksum:    "3db5725dec9aca5f9a99d3e7ab73be61cd45ec5644cd46c7999ca13b4b4933bf", // Correct SHA256
				ProcessedAt: time.Now(),
				Data:        []byte("test data 2 with more content"),
			},
		},
		Metadata: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	// Serialize the result
	resultData, err := json.Marshal(processingResult)
	if err != nil {
		t.Fatalf("Failed to marshal processing result: %v", err)
	}

	// Create result message
	resultMsg := &messaging.Message{
		Type:      messaging.MsgTypeResult,
		WorkerID:  1,
		Data:      resultData,
		Timestamp: time.Now(),
	}

	// Handle the result message
	err = collector.HandleResultMessage(1, resultMsg)
	if err != nil {
		t.Fatalf("Failed to handle result message: %v", err)
	}

	// Verify files were stored
	workerDir := filepath.Join(tempDir, "worker_1")

	// Check metadata file
	metadataFile := filepath.Join(workerDir, "result_metadata.json")
	if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
		t.Error("Metadata file was not created")
	}

	// Check output files
	output1File := filepath.Join(workerDir, "output1.txt")
	if _, err := os.Stat(output1File); os.IsNotExist(err) {
		t.Error("Output file 1 was not created")
	}

	output2File := filepath.Join(workerDir, "output2.txt")
	if _, err := os.Stat(output2File); os.IsNotExist(err) {
		t.Error("Output file 2 was not created")
	}

	// Verify file contents
	data1, err := os.ReadFile(output1File)
	if err != nil {
		t.Fatalf("Failed to read output file 1: %v", err)
	}
	if string(data1) != "test data 1" {
		t.Errorf("Output file 1 content mismatch: expected 'test data 1', got '%s'", string(data1))
	}

	// Check completion timestamp
	timestampFile := filepath.Join(workerDir, "completion_time.txt")
	if _, err := os.Stat(timestampFile); os.IsNotExist(err) {
		t.Error("Completion timestamp file was not created")
	}
}

func TestServerResultCollectorCompletion(t *testing.T) {
	tempDir := t.TempDir()
	collector := NewServerResultCollector(2, tempDir)

	// Create completion channels
	completionChan := collector.GetCompletionChannel()
	allCompleteChan := collector.GetAllCompleteChannel()

	// Create completion messages for two workers
	completionMsg1 := &messaging.Message{
		Type:      messaging.MsgTypeResultEnd,
		WorkerID:  1,
		Timestamp: time.Now(),
	}

	completionMsg2 := &messaging.Message{
		Type:      messaging.MsgTypeResultEnd,
		WorkerID:  2,
		Timestamp: time.Now(),
	}

	// Handle first completion
	err := collector.HandleResultMessage(1, completionMsg1)
	if err != nil {
		t.Fatalf("Failed to handle completion message 1: %v", err)
	}

	// Check that one worker completed
	select {
	case workerID := <-completionChan:
		if workerID != 1 {
			t.Errorf("Expected worker 1 completion, got worker %d", workerID)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for worker 1 completion signal")
	}

	if collector.GetCompletedWorkerCount() != 1 {
		t.Errorf("Expected 1 completed worker, got %d", collector.GetCompletedWorkerCount())
	}

	if collector.IsComplete() {
		t.Error("Collector should not be complete with only 1 worker")
	}

	// Handle second completion
	err = collector.HandleResultMessage(2, completionMsg2)
	if err != nil {
		t.Fatalf("Failed to handle completion message 2: %v", err)
	}

	// Check that all workers completed
	select {
	case workerID := <-completionChan:
		if workerID != 2 {
			t.Errorf("Expected worker 2 completion, got worker %d", workerID)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for worker 2 completion signal")
	}

	select {
	case <-allCompleteChan:
		// Expected
	case <-time.After(time.Second):
		t.Error("Timeout waiting for all complete signal")
	}

	if collector.GetCompletedWorkerCount() != 2 {
		t.Errorf("Expected 2 completed workers, got %d", collector.GetCompletedWorkerCount())
	}

	if !collector.IsComplete() {
		t.Error("Collector should be complete with all workers done")
	}

	// Check that summary file was created
	summaryFile := filepath.Join(tempDir, "collection_summary.json")
	if _, err := os.Stat(summaryFile); os.IsNotExist(err) {
		t.Error("Collection summary file was not created")
	}
}

func TestServerResultCollectorValidation(t *testing.T) {
	tempDir := t.TempDir()
	collector := NewServerResultCollector(1, tempDir)

	// Create invalid processing result (negative file count)
	processingResult := &messaging.ProcessingResult{
		WorkerID:        1,
		Status:          messaging.StatusSuccess,
		ProcessingTime:  time.Second * 5,
		InputFileCount:  -1, // Invalid
		OutputFileCount: 1,
		OutputFiles: []messaging.ProcessedFile{
			{
				Filename:    "output.txt",
				Size:        9,                                                                  // Correct size for "test data"
				Checksum:    "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9", // Correct SHA256
				ProcessedAt: time.Now(),
				Data:        []byte("test data"),
			},
		},
	}

	// Serialize the result
	resultData, err := json.Marshal(processingResult)
	if err != nil {
		t.Fatalf("Failed to marshal processing result: %v", err)
	}

	// Create result message
	resultMsg := &messaging.Message{
		Type:      messaging.MsgTypeResult,
		WorkerID:  1,
		Data:      resultData,
		Timestamp: time.Now(),
	}

	// Handle the result message (should fail validation)
	err = collector.HandleResultMessage(1, resultMsg)
	if err == nil {
		t.Error("Expected validation error for invalid processing result")
	}

	// Check that validation errors were recorded
	validationErrors := collector.GetValidationErrors()
	if len(validationErrors) == 0 {
		t.Error("Expected validation errors to be recorded")
	}

	if errors, exists := validationErrors[1]; !exists || len(errors) == 0 {
		t.Error("Expected validation errors for worker 1")
	}
}

func TestServerResultCollectorErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	collector := NewServerResultCollector(1, tempDir)

	// Create error message
	errorMsg := &messaging.Message{
		Type:      messaging.MsgTypeError,
		WorkerID:  1,
		Data:      []byte("Test error message"),
		Timestamp: time.Now(),
	}

	// Handle the error message
	err := collector.HandleResultMessage(1, errorMsg)
	if err != nil {
		t.Fatalf("Failed to handle error message: %v", err)
	}

	// Check that worker was marked as complete
	if collector.GetCompletedWorkerCount() != 1 {
		t.Errorf("Expected 1 completed worker after error, got %d", collector.GetCompletedWorkerCount())
	}

	if !collector.IsComplete() {
		t.Error("Collector should be complete after error from only worker")
	}

	// Check that error file was created
	workerDir := filepath.Join(tempDir, "worker_1")
	errorFile := filepath.Join(workerDir, "error.txt")
	if _, err := os.Stat(errorFile); os.IsNotExist(err) {
		t.Error("Error file was not created")
	}

	// Verify error file content
	errorData, err := os.ReadFile(errorFile)
	if err != nil {
		t.Fatalf("Failed to read error file: %v", err)
	}

	errorContent := string(errorData)
	if !testContains(errorContent, "Test error message") {
		t.Errorf("Error file does not contain expected message: %s", errorContent)
	}
}

// Helper function to check if string contains substring
func testContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsAt(s, substr, 1))))
}

func containsAt(s, substr string, start int) bool {
	if start >= len(s) {
		return false
	}
	if start+len(substr) > len(s) {
		return containsAt(s, substr, start+1)
	}
	if s[start:start+len(substr)] == substr {
		return true
	}
	return containsAt(s, substr, start+1)
}
