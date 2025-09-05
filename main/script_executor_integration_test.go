package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistentWorker_ScriptExecutorIntegration(t *testing.T) {
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

	// Test manual processing trigger
	if err := worker.TriggerProcessing(); err != nil {
		t.Fatalf("Failed to trigger processing: %v", err)
	}

	// Wait for processing to complete (or reach error state due to no data channel)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for processing to complete")
		case <-ticker.C:
			state := worker.GetState()
			if state == StateComplete {
				t.Logf("Processing completed successfully")
				goto processingComplete
			} else if state == StateError {
				// In test environment without WebRTC, we expect error due to no data channel
				// But the script execution itself should have worked
				t.Logf("Processing reached error state (expected in test without WebRTC)")
				goto processingComplete
			}
		}
	}

processingComplete:
	// Verify output files were created
	inputCount, outputCount, err := scriptExecutor.GetFileCount()
	if err != nil {
		t.Fatalf("Failed to get file count: %v", err)
	}

	if inputCount != len(testFiles) {
		t.Errorf("Expected %d input files, got %d", len(testFiles), inputCount)
	}

	if outputCount != len(testFiles) {
		t.Errorf("Expected %d output files, got %d", len(testFiles), outputCount)
	}

	// Verify processing status
	status := worker.GetProcessingStatus()
	if status["worker_id"] != 1 {
		t.Errorf("Expected worker ID 1, got %v", status["worker_id"])
	}

	if status["state"] != "COMPLETE" {
		t.Errorf("Expected state COMPLETE, got %v", status["state"])
	}

	if status["input_file_count"] != inputCount {
		t.Errorf("Expected input file count %d, got %v", inputCount, status["input_file_count"])
	}

	if status["output_file_count"] != outputCount {
		t.Errorf("Expected output file count %d, got %v", outputCount, status["output_file_count"])
	}

	// Cleanup
	worker.Stop()
}

func TestPersistentWorker_ScriptExecutorError(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	// Create input directory but make it unreadable to trigger error
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	// Create processing config with very short timeout to trigger timeout error
	config := &ProcessingConfig{
		PythonScriptPath:  "python3",
		InputDirectory:    inputDir,
		OutputDirectory:   outputDir,
		ProcessingTimeout: 1 * time.Millisecond, // Very short timeout
		HeartbeatInterval: 5 * time.Second,
	}

	// Create test file
	filePath := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create channels
	reqChan := make(chan Request, 10)
	resChan := make(chan Result, 10)

	// Create persistent worker
	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	// Test processing trigger with error condition
	if err := worker.TriggerProcessing(); err != nil {
		t.Fatalf("Failed to trigger processing: %v", err)
	}

	// Wait for processing to fail
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for processing to fail")
		case <-ticker.C:
			state := worker.GetState()
			if state == StateError {
				t.Logf("Processing failed as expected")
				goto processingFailed
			} else if state == StateComplete {
				t.Fatal("Expected processing to fail, but it completed successfully")
			}
		}
	}

processingFailed:
	// Verify error state
	status := worker.GetProcessingStatus()
	if status["state"] != "ERROR" {
		t.Errorf("Expected state ERROR, got %v", status["state"])
	}

	// Cleanup
	worker.Stop()
}

func TestPersistentWorker_ScriptExecutorConcurrency(t *testing.T) {
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

	// Start first processing
	if err := worker.TriggerProcessing(); err != nil {
		t.Fatalf("Failed to trigger first processing: %v", err)
	}

	// Wait a bit for processing to start
	time.Sleep(200 * time.Millisecond)

	// Check if worker is in processing state
	currentState := worker.GetState()
	if currentState == StateProcessing || currentState == StateSending {
		// Try to start second processing - should fail
		if err := worker.TriggerProcessing(); err == nil {
			t.Error("Expected error when triggering processing while already processing, got nil")
		} else if !strings.Contains(err.Error(), "not in a state to start processing") {
			t.Errorf("Expected 'not in a state to start processing' error, got: %v", err)
		}
	} else {
		// If processing completed very quickly or failed, that's also valid behavior
		t.Logf("Processing completed or failed quickly, current state: %s", currentState)
	}

	// Wait for first processing to complete
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for processing to complete")
		case <-ticker.C:
			state := worker.GetState()
			if state == StateComplete || state == StateError {
				goto processingDone
			}
		}
	}

processingDone:
	// Cleanup
	worker.Stop()
}

func TestPersistentWorkerPool_ScriptExecutorIntegration(t *testing.T) {
	// Create temporary directories for multiple workers
	tempDir := t.TempDir()
	numWorkers := 3

	// Create channels
	resChan := make(chan Result, 100)

	// Create configs for each worker
	configs := make([]*ProcessingConfig, numWorkers)
	for i := 0; i < numWorkers; i++ {
		inputDir := filepath.Join(tempDir, "worker"+string(rune('0'+i)), "input")
		outputDir := filepath.Join(tempDir, "worker"+string(rune('0'+i)), "output")

		// Create input directory with test files
		if err := os.MkdirAll(inputDir, 0755); err != nil {
			t.Fatalf("Failed to create input directory for worker %d: %v", i, err)
		}

		// Create test file
		filePath := filepath.Join(inputDir, "test.txt")
		content := "Test content for worker " + string(rune('0'+i))
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file for worker %d: %v", i, err)
		}

		configs[i] = &ProcessingConfig{
			PythonScriptPath:  "python3",
			InputDirectory:    inputDir,
			OutputDirectory:   outputDir,
			ProcessingTimeout: 30 * time.Second,
			HeartbeatInterval: 5 * time.Second,
		}
	}

	// Create worker pool with first config as default
	pool := NewPersistentWorkerPool(resChan, configs[0])

	// Start workers with individual configs
	reqChannels := make([]chan Request, numWorkers)

	for i := 0; i < numWorkers; i++ {
		reqChan := make(chan Request, 10)
		reqChannels[i] = reqChan

		// Create worker with specific config
		worker := NewPersistentWorker(i, "test-conn", reqChan, resChan, configs[i])
		pool.workers[i] = worker
	}

	// Trigger processing for all workers
	for i := 0; i < numWorkers; i++ {
		worker, exists := pool.GetWorker(i)
		if !exists {
			t.Fatalf("Worker %d not found in pool", i)
		}

		if err := worker.TriggerProcessing(); err != nil {
			t.Fatalf("Failed to trigger processing for worker %d: %v", i, err)
		}
	}

	// Wait for all workers to complete
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for all workers to complete processing")
		case <-ticker.C:
			if pool.IsAllWorkersComplete() {
				t.Logf("All workers completed processing")
				goto allComplete
			}
		}
	}

allComplete:
	// Verify all workers completed successfully
	states := pool.GetWorkerStates()
	for workerID, state := range states {
		if state != StateComplete {
			t.Errorf("Worker %d did not complete successfully, state: %s", workerID, state)
		}
	}

	// Verify output files were created for each worker
	for i := 0; i < numWorkers; i++ {
		worker, _ := pool.GetWorker(i)
		scriptExecutor := worker.GetScriptExecutor()

		inputCount, outputCount, err := scriptExecutor.GetFileCount()
		if err != nil {
			t.Errorf("Failed to get file count for worker %d: %v", i, err)
			continue
		}

		if inputCount != 1 {
			t.Errorf("Worker %d: expected 1 input file, got %d", i, inputCount)
		}

		if outputCount != 1 {
			t.Errorf("Worker %d: expected 1 output file, got %d", i, outputCount)
		}
	}

	// Cleanup
	pool.StopAll()
}
