package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

func TestNewScriptExecutor(t *testing.T) {
	inputDir := "./test_input"
	outputDir := "./test_output"
	timeout := 30 * time.Second

	executor := NewScriptExecutor(inputDir, outputDir, timeout)

	if executor.GetInputDirectory() != inputDir {
		t.Errorf("Expected input directory %s, got %s", inputDir, executor.GetInputDirectory())
	}

	if executor.GetOutputDirectory() != outputDir {
		t.Errorf("Expected output directory %s, got %s", outputDir, executor.GetOutputDirectory())
	}

	if executor.GetTimeout() != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, executor.GetTimeout())
	}

	if executor.IsRunning() {
		t.Error("Expected executor to not be running initially")
	}
}

func TestScriptExecutor_Execute_EmptyDirectory(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory (empty)
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	executor := NewScriptExecutor(inputDir, outputDir, 10*time.Second)

	result, err := executor.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success for empty directory, got failure: %s", result.ErrorMsg)
	}

	if len(result.OutputFiles) != 0 {
		t.Errorf("Expected 0 output files, got %d", len(result.OutputFiles))
	}

	if len(result.InputFiles) != 0 {
		t.Errorf("Expected 0 input files, got %d", len(result.InputFiles))
	}
}

func TestScriptExecutor_Execute_WithFiles(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory with test files
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	// Create test files
	testFiles := []string{"test1.txt", "test2.jpg", "test3.png"}
	for _, filename := range testFiles {
		filePath := filepath.Join(inputDir, filename)
		content := "Test content for " + filename
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	result, err := executor.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.ErrorMsg)
	}

	if len(result.InputFiles) != len(testFiles) {
		t.Errorf("Expected %d input files, got %d", len(testFiles), len(result.InputFiles))
	}

	if len(result.OutputFiles) != len(testFiles) {
		t.Errorf("Expected %d output files, got %d", len(testFiles), len(result.OutputFiles))
	}

	// Verify output files exist and have correct naming pattern
	for _, outputFile := range result.OutputFiles {
		if !strings.Contains(filepath.Base(outputFile), "processed_") {
			t.Errorf("Output file %s doesn't have 'processed_' prefix", outputFile)
		}

		// Check if file exists
		if _, err := os.Stat(outputFile); os.IsNotExist(err) {
			t.Errorf("Output file %s doesn't exist", outputFile)
		}
	}

	// Verify processing time is reasonable
	if result.Duration < 100*time.Millisecond {
		t.Errorf("Processing time seems too short: %v", result.Duration)
	}

	if result.Duration > 10*time.Second {
		t.Errorf("Processing time seems too long: %v", result.Duration)
	}
}

func TestScriptExecutor_Execute_Timeout(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory with test file
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	// Create a test file
	filePath := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set very short timeout
	executor := NewScriptExecutor(inputDir, outputDir, 1*time.Millisecond)

	result, err := executor.Execute()

	// Should timeout
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if result.Success {
		t.Error("Expected failure due to timeout, got success")
	}

	if !strings.Contains(result.ErrorMsg, "timeout") && !strings.Contains(result.ErrorMsg, "timed out") {
		t.Errorf("Expected timeout error message, got: %s", result.ErrorMsg)
	}
}

func TestScriptExecutor_Execute_ConcurrentExecution(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory with test file
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	filePath := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Start first execution
	go func() {
		executor.Execute()
	}()

	// Wait a bit to ensure first execution starts
	time.Sleep(100 * time.Millisecond)

	// Try second execution - should fail
	result, err := executor.Execute()
	if err == nil {
		t.Error("Expected error for concurrent execution, got nil")
	}

	if result != nil && result.Success {
		t.Error("Expected failure for concurrent execution, got success")
	}
}

func TestScriptExecutor_WatchOutputDirectory(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Start watching
	fileChan, err := executor.WatchOutputDirectory()
	if err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	// Create a file in output directory after a short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		testFile := filepath.Join(outputDir, "new_file.txt")
		os.WriteFile(testFile, []byte("test content"), 0644)
	}()

	// Wait for file detection
	select {
	case detectedFile := <-fileChan:
		if !strings.Contains(detectedFile, "new_file.txt") {
			t.Errorf("Expected to detect new_file.txt, got %s", detectedFile)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for file detection")
	}

	// Stop watching
	executor.Stop()
}

func TestScriptExecutor_CreateProcessingResult(t *testing.T) {
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Create mock execution result
	execResult := &ExecutionResult{
		Success:     true,
		Duration:    2 * time.Second,
		OutputFiles: []string{},
		InputFiles:  []string{"input1.txt", "input2.txt"},
	}

	// Create a test output file
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	testOutputFile := filepath.Join(outputDir, "processed_test.txt")
	testContent := "processed content"
	if err := os.WriteFile(testOutputFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test output file: %v", err)
	}

	execResult.OutputFiles = []string{testOutputFile}

	workerID := 42
	result := executor.CreateProcessingResult(workerID, execResult)

	if result.WorkerID != workerID {
		t.Errorf("Expected worker ID %d, got %d", workerID, result.WorkerID)
	}

	if result.Status != messaging.StatusSuccess {
		t.Errorf("Expected status SUCCESS, got %v", result.Status)
	}

	if result.ProcessingTime != execResult.Duration {
		t.Errorf("Expected processing time %v, got %v", execResult.Duration, result.ProcessingTime)
	}

	if result.InputFileCount != len(execResult.InputFiles) {
		t.Errorf("Expected input file count %d, got %d", len(execResult.InputFiles), result.InputFileCount)
	}

	if result.OutputFileCount != len(execResult.OutputFiles) {
		t.Errorf("Expected output file count %d, got %d", len(execResult.OutputFiles), result.OutputFileCount)
	}

	if len(result.OutputFiles) != 1 {
		t.Errorf("Expected 1 processed file, got %d", len(result.OutputFiles))
	}

	processedFile := result.OutputFiles[0]
	if processedFile.Filename != "processed_test.txt" {
		t.Errorf("Expected filename 'processed_test.txt', got %s", processedFile.Filename)
	}

	if processedFile.Size != int64(len(testContent)) {
		t.Errorf("Expected file size %d, got %d", len(testContent), processedFile.Size)
	}

	if string(processedFile.Data) != testContent {
		t.Errorf("Expected file data '%s', got '%s'", testContent, string(processedFile.Data))
	}

	// Check metadata
	if result.Metadata["execution_type"] != "mock_processing" {
		t.Errorf("Expected execution_type 'mock_processing', got %v", result.Metadata["execution_type"])
	}

	if result.Metadata["processor"] != "ScriptExecutor" {
		t.Errorf("Expected processor 'ScriptExecutor', got %v", result.Metadata["processor"])
	}
}

func TestScriptExecutor_CreateProcessingResult_Failure(t *testing.T) {
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Create mock execution result with failure
	execResult := &ExecutionResult{
		Success:     false,
		Duration:    1 * time.Second,
		OutputFiles: []string{},
		InputFiles:  []string{"input1.txt"},
		ErrorMsg:    "Mock processing failed",
	}

	workerID := 42
	result := executor.CreateProcessingResult(workerID, execResult)

	if result.Status != messaging.StatusError {
		t.Errorf("Expected status ERROR, got %v", result.Status)
	}

	if result.ErrorMessage != execResult.ErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", execResult.ErrorMsg, result.ErrorMessage)
	}
}

func TestScriptExecutor_GetFileCount(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create directories
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Initially should be 0 files
	inputCount, outputCount, err := executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount failed: %v", err)
	}

	if inputCount != 0 {
		t.Errorf("Expected 0 input files, got %d", inputCount)
	}

	if outputCount != 0 {
		t.Errorf("Expected 0 output files, got %d", outputCount)
	}

	// Create some files
	inputFile := filepath.Join(inputDir, "input.txt")
	outputFile := filepath.Join(outputDir, "output.txt")

	if err := os.WriteFile(inputFile, []byte("input content"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	if err := os.WriteFile(outputFile, []byte("output content"), 0644); err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}

	// Check counts again
	inputCount, outputCount, err = executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount failed: %v", err)
	}

	if inputCount != 1 {
		t.Errorf("Expected 1 input file, got %d", inputCount)
	}

	if outputCount != 1 {
		t.Errorf("Expected 1 output file, got %d", outputCount)
	}
}

func TestScriptExecutor_CleanupOutputDirectory(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create both input and output directories
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Create test files in output directory
	testFiles := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, filename := range testFiles {
		filePath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Verify files exist
	_, outputCount, err := executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount failed: %v", err)
	}

	if outputCount != len(testFiles) {
		t.Errorf("Expected %d output files before cleanup, got %d", len(testFiles), outputCount)
	}

	// Cleanup
	if err := executor.CleanupOutputDirectory(); err != nil {
		t.Fatalf("CleanupOutputDirectory failed: %v", err)
	}

	// Verify files are gone
	_, outputCount, err = executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount failed after cleanup: %v", err)
	}

	if outputCount != 0 {
		t.Errorf("Expected 0 output files after cleanup, got %d", outputCount)
	}
}

func TestScriptExecutor_SetTimeout(t *testing.T) {
	executor := NewScriptExecutor("./input", "./output", 30*time.Second)

	newTimeout := 60 * time.Second
	executor.SetTimeout(newTimeout)

	if executor.GetTimeout() != newTimeout {
		t.Errorf("Expected timeout %v, got %v", newTimeout, executor.GetTimeout())
	}
}

func TestScriptExecutor_Stop(t *testing.T) {
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory with test file
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	filePath := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Start execution in goroutine
	resultChan := make(chan *ExecutionResult, 1)
	go func() {
		result, _ := executor.Execute()
		resultChan <- result
	}()

	// Wait a bit then stop
	time.Sleep(100 * time.Millisecond)
	executor.Stop()

	// Should get stopped result
	select {
	case result := <-resultChan:
		if result.Success {
			t.Error("Expected failure due to stop, got success")
		}
		if !strings.Contains(result.ErrorMsg, "stopped") {
			t.Errorf("Expected 'stopped' in error message, got: %s", result.ErrorMsg)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for execution to stop")
	}
}
