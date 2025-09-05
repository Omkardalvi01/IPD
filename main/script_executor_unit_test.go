package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScriptExecutor_BasicFunctionality(t *testing.T) {
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

	// Create script executor
	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Test execution
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

	// Test file count
	inputCount, outputCount, err := executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount failed: %v", err)
	}

	if inputCount != len(testFiles) {
		t.Errorf("Expected %d input files, got %d", len(testFiles), inputCount)
	}

	if outputCount != len(testFiles) {
		t.Errorf("Expected %d output files, got %d", len(testFiles), outputCount)
	}

	// Test processing result creation
	processingResult := executor.CreateProcessingResult(42, result)
	if processingResult.WorkerID != 42 {
		t.Errorf("Expected worker ID 42, got %d", processingResult.WorkerID)
	}

	if processingResult.InputFileCount != len(testFiles) {
		t.Errorf("Expected input file count %d, got %d", len(testFiles), processingResult.InputFileCount)
	}

	if processingResult.OutputFileCount != len(testFiles) {
		t.Errorf("Expected output file count %d, got %d", len(testFiles), processingResult.OutputFileCount)
	}

	// Test cleanup
	if err := executor.CleanupOutputDirectory(); err != nil {
		t.Fatalf("CleanupOutputDirectory failed: %v", err)
	}

	// Verify cleanup worked
	_, outputCountAfterCleanup, err := executor.GetFileCount()
	if err != nil {
		t.Fatalf("GetFileCount after cleanup failed: %v", err)
	}

	if outputCountAfterCleanup != 0 {
		t.Errorf("Expected 0 output files after cleanup, got %d", outputCountAfterCleanup)
	}
}

func TestScriptExecutor_WatchDirectory(t *testing.T) {
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
		if filepath.Base(detectedFile) != "new_file.txt" {
			t.Errorf("Expected to detect new_file.txt, got %s", filepath.Base(detectedFile))
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for file detection")
	}

	// Stop watching
	executor.Stop()
}

func TestPersistentWorker_ScriptExecutorInitialization(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create processing config
	config := &ProcessingConfig{
		PythonScriptPath:  "python3",
		InputDirectory:    inputDir,
		OutputDirectory:   outputDir,
		ProcessingTimeout: 30 * time.Second,
		HeartbeatInterval: 5 * time.Second,
	}

	// Create channels
	reqChan := make(chan Request, 10)
	resChan := make(chan Result, 10)

	// Create persistent worker
	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Verify script executor is initialized
	if worker.GetScriptExecutor() == nil {
		t.Fatal("Script executor not initialized in persistent worker")
	}

	// Verify script executor configuration
	scriptExecutor := worker.GetScriptExecutor()
	if scriptExecutor.GetInputDirectory() != inputDir {
		t.Errorf("Expected input directory %s, got %s", inputDir, scriptExecutor.GetInputDirectory())
	}

	if scriptExecutor.GetOutputDirectory() != outputDir {
		t.Errorf("Expected output directory %s, got %s", outputDir, scriptExecutor.GetOutputDirectory())
	}

	if scriptExecutor.GetTimeout() != config.ProcessingTimeout {
		t.Errorf("Expected timeout %v, got %v", config.ProcessingTimeout, scriptExecutor.GetTimeout())
	}

	// Test processing status
	status := worker.GetProcessingStatus()
	if status["worker_id"] != 1 {
		t.Errorf("Expected worker ID 1, got %v", status["worker_id"])
	}

	if status["input_directory"] != inputDir {
		t.Errorf("Expected input directory %s, got %v", inputDir, status["input_directory"])
	}

	if status["output_directory"] != outputDir {
		t.Errorf("Expected output directory %s, got %v", outputDir, status["output_directory"])
	}

	// Cleanup
	worker.Stop()
}

func TestScriptExecutor_DirectExecution(t *testing.T) {
	// This test verifies that the script executor can be used independently
	// without the persistent worker infrastructure

	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory with test files
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	// Create test files with different extensions
	testFiles := map[string]string{
		"document.txt": "This is a text document",
		"image.jpg":    "fake jpeg data",
		"data.bin":     "binary data content",
		"config.json":  `{"key": "value"}`,
	}

	for filename, content := range testFiles {
		filePath := filepath.Join(inputDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Create and configure script executor
	executor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Execute processing
	result, err := executor.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify results
	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.ErrorMsg)
	}

	// Should process only files with valid extensions (txt, jpg, bin - json is not in the valid list)
	expectedProcessedFiles := 3 // txt, jpg, bin (json is not in validExtensions list)
	if len(result.OutputFiles) != expectedProcessedFiles {
		t.Errorf("Expected %d output files, got %d", expectedProcessedFiles, len(result.OutputFiles))
	}

	// Verify all output files exist and have correct naming pattern
	for _, outputFile := range result.OutputFiles {
		if _, err := os.Stat(outputFile); os.IsNotExist(err) {
			t.Errorf("Output file %s doesn't exist", outputFile)
		}

		baseName := filepath.Base(outputFile)
		if !filepath.HasPrefix(baseName, "processed_") {
			t.Errorf("Output file %s doesn't have 'processed_' prefix", baseName)
		}
	}

	// Test processing result conversion
	processingResult := executor.CreateProcessingResult(99, result)

	if processingResult.WorkerID != 99 {
		t.Errorf("Expected worker ID 99, got %d", processingResult.WorkerID)
	}

	if len(processingResult.OutputFiles) != expectedProcessedFiles {
		t.Errorf("Expected %d processed files, got %d", expectedProcessedFiles, len(processingResult.OutputFiles))
	}

	// Verify metadata
	if processingResult.Metadata["execution_type"] != "mock_processing" {
		t.Errorf("Expected execution_type 'mock_processing', got %v", processingResult.Metadata["execution_type"])
	}

	if processingResult.Metadata["processor"] != "ScriptExecutor" {
		t.Errorf("Expected processor 'ScriptExecutor', got %v", processingResult.Metadata["processor"])
	}
}
