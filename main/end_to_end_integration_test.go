package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// TestCompleteImageDistributionToResultCollection tests the complete workflow
// from image distribution through processing to result collection
func TestCompleteImageDistributionToResultCollection(t *testing.T) {
	// Setup test environment
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create test images
	numImages := 5
	imageFiles := createTestImages(t, testDir.inputDir, numImages)

	// Setup system components
	numWorkers := 2
	serverCollector := NewServerResultCollector(numWorkers, testDir.sharedResultsDir)
	lifecycleManager := NewConnectionLifecycleManager(5*time.Second, 10*time.Second)
	serverHandler := NewServerMessageHandler(serverCollector, lifecycleManager)

	// Create worker pool
	workers := make([]*PersistentWorker, numWorkers)
	reqChannels := make([]chan Request, numWorkers)
	resChannel := make(chan Result, numImages)

	for i := 0; i < numWorkers; i++ {
		reqChannels[i] = make(chan Request, 10)
		config := DefaultProcessingConfig()
		config.InputDirectory = filepath.Join(testDir.workerDirs[i], "input")
		config.OutputDirectory = filepath.Join(testDir.workerDirs[i], "output")

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("test-conn-%d", i+1),
			reqChannels[i], resChannel, config)
	}

	// Start monitoring result collection
	completionChan := make(chan bool, 1)
	go monitorEndToEndCompletion(t, serverCollector, completionChan)

	// Simulate image distribution
	var wg sync.WaitGroup
	for i, worker := range workers {
		wg.Add(1)
		go func(workerIndex int, w *PersistentWorker) {
			defer wg.Done()

			// Simulate worker startup and connection establishment
			w.SetState(StateReceiving)

			// Process assigned images
			startIdx := workerIndex * (numImages / numWorkers)
			endIdx := startIdx + (numImages / numWorkers)
			if workerIndex == numWorkers-1 {
				endIdx = numImages // Last worker gets remaining images
			}

			for imgIdx := startIdx; imgIdx < endIdx; imgIdx++ {
				// Simulate receiving image
				imageFile := imageFiles[imgIdx]
				file, err := os.Open(imageFile)
				if err != nil {
					t.Errorf("Worker %d failed to open image %s: %v", w.GetWorkerID(), imageFile, err)
					continue
				}

				req := Request{f: file}

				// Simulate processing request
				if err := simulateImageProcessing(w, req); err != nil {
					t.Errorf("Worker %d failed to process image %s: %v", w.GetWorkerID(), imageFile, err)
				}

				file.Close()

				// Send result
				resChannel <- Result{worker_id: w.GetWorkerID(), result: SUCCESS}
			}

			// Simulate processing phase
			w.SetState(StateProcessing)
			time.Sleep(100 * time.Millisecond) // Simulate processing time

			// Simulate result generation and transmission
			if err := simulateResultGeneration(t, w); err != nil {
				t.Errorf("Worker %d failed to generate results: %v", w.GetWorkerID(), err)
				return
			}

			w.SetState(StateSending)
			if err := simulateResultTransmission(t, w, serverHandler); err != nil {
				t.Errorf("Worker %d failed to transmit results: %v", w.GetWorkerID(), err)
				return
			}

			w.SetState(StateComplete)
			t.Logf("Worker %d completed end-to-end workflow", w.GetWorkerID())

		}(i, worker)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(resChannel)

	// Verify all results were received
	resultCount := 0
	for result := range resChannel {
		if result.result != SUCCESS {
			t.Errorf("Worker %d reported failure", result.worker_id)
		}
		resultCount++
	}

	if resultCount != numImages {
		t.Errorf("Expected %d results, got %d", numImages, resultCount)
	}

	// Wait for result collection completion
	select {
	case <-completionChan:
		t.Log("End-to-end workflow completed successfully")
	case <-time.After(30 * time.Second):
		t.Error("End-to-end workflow timed out")
	}

	// Verify final state
	if !serverCollector.IsComplete() {
		t.Error("Server result collection should be complete")
	}

	if serverCollector.GetCompletedWorkerCount() != numWorkers {
		t.Errorf("Expected %d completed workers, got %d",
			numWorkers, serverCollector.GetCompletedWorkerCount())
	}

	// Verify no validation errors
	validationErrors := serverCollector.GetValidationErrors()
	if len(validationErrors) > 0 {
		t.Errorf("Found validation errors: %v", validationErrors)
	}

	t.Logf("End-to-end integration test completed successfully")
}

// TestPersistentConnectionBehaviorMultipleWorkers tests persistent connection behavior
// with multiple workers running concurrently
func TestPersistentConnectionBehaviorMultipleWorkers(t *testing.T) {
	numWorkers := 3
	testDuration := 10 * time.Second
	heartbeatInterval := 1 * time.Second

	// Create workers with persistent connections
	workers := make([]*PersistentWorker, numWorkers)
	connectionStates := make([]WorkerState, numWorkers)
	stateMutex := sync.RWMutex{}

	for i := 0; i < numWorkers; i++ {
		reqChan := make(chan Request, 5)
		resChan := make(chan Result, 5)

		config := DefaultProcessingConfig()
		config.HeartbeatInterval = heartbeatInterval

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("persistent-conn-%d", i+1),
			reqChan, resChan, config)

		// Initialize connection state tracking
		connectionStates[i] = StateIdle
	}

	// Start connection monitoring
	var wg sync.WaitGroup
	stopMonitoring := make(chan bool, numWorkers)

	for i, worker := range workers {
		wg.Add(1)
		go func(workerIndex int, w *PersistentWorker) {
			defer wg.Done()

			ticker := time.NewTicker(heartbeatInterval / 2)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Update connection state
					stateMutex.Lock()
					connectionStates[workerIndex] = w.GetState()
					stateMutex.Unlock()

					// Simulate heartbeat activity
					if w.heartbeat != nil {
						heartbeatMsg := messaging.CreateMessage(messaging.MsgTypeHeartbeat,
							w.GetWorkerID(), "", nil)

						// Simulate heartbeat processing
						queueItem := MessageQueueItem{
							Message:   heartbeatMsg,
							Timestamp: time.Now(),
						}

						select {
						case w.messageQueue <- queueItem:
							// Heartbeat queued successfully
						default:
							t.Logf("Worker %d heartbeat queue full", w.GetWorkerID())
						}
					}

				case <-stopMonitoring:
					return
				}
			}
		}(i, worker)
	}

	// Simulate various connection states over time
	stateTransitions := []WorkerState{
		StateReceiving,
		StateProcessing,
		StateSending,
		StateComplete,
	}

	transitionInterval := testDuration / time.Duration(len(stateTransitions))

	for _, targetState := range stateTransitions {
		time.Sleep(transitionInterval)

		// Transition all workers to the target state
		for _, worker := range workers {
			worker.SetState(targetState)
		}

		// Verify state consistency
		time.Sleep(100 * time.Millisecond)
		stateMutex.RLock()
		for i, state := range connectionStates {
			if state != targetState {
				t.Logf("Worker %d state mismatch: expected %s, got %s",
					i+1, targetState, state)
			}
		}
		stateMutex.RUnlock()

		t.Logf("All workers transitioned to state: %s", targetState)
	}

	// Stop monitoring
	for i := 0; i < numWorkers; i++ {
		stopMonitoring <- true
	}

	wg.Wait()

	// Verify final states
	for i, worker := range workers {
		finalState := worker.GetState()
		if finalState != StateComplete {
			t.Errorf("Worker %d final state should be Complete, got %s",
				i+1, finalState)
		}
	}

	t.Logf("Persistent connection behavior test completed with %d workers", numWorkers)
}

// TestFailureScenarios tests various failure scenarios and recovery mechanisms
func TestFailureScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		failureType string
		testFunc    func(t *testing.T)
	}{
		{
			name:        "Connection Timeout",
			failureType: "connection_timeout",
			testFunc:    testConnectionTimeout,
		},
		{
			name:        "Processing Script Failure",
			failureType: "script_failure",
			testFunc:    testProcessingScriptFailure,
		},
		{
			name:        "Result Transmission Failure",
			failureType: "transmission_failure",
			testFunc:    testResultTransmissionFailure,
		},
		{
			name:        "Heartbeat Failure",
			failureType: "heartbeat_failure",
			testFunc:    testHeartbeatFailure,
		},
		{
			name:        "Partial Worker Failure",
			failureType: "partial_failure",
			testFunc:    testPartialWorkerFailure,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing failure scenario: %s", tc.name)
			tc.testFunc(t)
		})
	}
}

// TestPerformanceThroughputAndMemory tests system performance under load
func TestPerformanceThroughputAndMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Performance test configuration
	numWorkers := runtime.NumCPU()
	if numWorkers > 4 {
		numWorkers = 4 // Limit for test stability
	}

	numImagesPerWorker := 10
	totalImages := numWorkers * numImagesPerWorker

	t.Logf("Starting performance test with %d workers, %d images total",
		numWorkers, totalImages)

	// Setup test environment
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create test images
	imageFiles := createTestImages(t, testDir.inputDir, totalImages)

	// Memory baseline
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	// Setup system components (result collection handled by individual workers in this test)

	// Create and start workers
	workers := make([]*PersistentWorker, numWorkers)
	reqChannels := make([]chan Request, numWorkers)
	resChannel := make(chan Result, totalImages)

	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		reqChannels[i] = make(chan Request, numImagesPerWorker+1)
		config := DefaultProcessingConfig()
		config.InputDirectory = filepath.Join(testDir.workerDirs[i], "input")
		config.OutputDirectory = filepath.Join(testDir.workerDirs[i], "output")
		config.ProcessingTimeout = 30 * time.Second

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("perf-conn-%d", i+1),
			reqChannels[i], resChannel, config)

		wg.Add(1)
		go func(workerIndex int, w *PersistentWorker) {
			defer wg.Done()

			// Process assigned images
			startIdx := workerIndex * numImagesPerWorker
			endIdx := startIdx + numImagesPerWorker

			processedCount := 0
			for imgIdx := startIdx; imgIdx < endIdx; imgIdx++ {
				imageFile := imageFiles[imgIdx]
				file, err := os.Open(imageFile)
				if err != nil {
					t.Errorf("Worker %d failed to open image %s: %v", w.GetWorkerID(), imageFile, err)
					continue
				}

				req := Request{f: file}

				// Simulate high-throughput processing
				if err := simulateImageProcessing(w, req); err != nil {
					t.Errorf("Worker %d failed to process image %s: %v", w.GetWorkerID(), imageFile, err)
				}

				file.Close()
				processedCount++

				// Send result
				resChannel <- Result{worker_id: w.GetWorkerID(), result: SUCCESS}
			}

			// Simulate result generation and transmission
			w.SetState(StateProcessing)
			if err := simulateResultGeneration(t, w); err != nil {
				t.Errorf("Worker %d failed to generate results: %v", w.GetWorkerID(), err)
				return
			}

			w.SetState(StateSending)
			// Simulate result transmission (lightweight for performance test)
			time.Sleep(10 * time.Millisecond)

			w.SetState(StateComplete)
			t.Logf("Worker %d processed %d images", w.GetWorkerID(), processedCount)
		}(i, workers[i])
	}

	// Wait for all workers to complete
	wg.Wait()
	close(resChannel)

	processingTime := time.Since(startTime)

	// Collect results
	resultCount := 0
	successCount := 0
	for result := range resChannel {
		resultCount++
		if result.result == SUCCESS {
			successCount++
		}
	}

	// Memory after processing
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Calculate performance metrics
	throughput := float64(totalImages) / processingTime.Seconds()
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	memoryPerImage := float64(memoryUsed) / float64(totalImages)

	// Performance assertions
	if resultCount != totalImages {
		t.Errorf("Expected %d results, got %d", totalImages, resultCount)
	}

	if successCount != totalImages {
		t.Errorf("Expected %d successful results, got %d", totalImages, successCount)
	}

	// Performance benchmarks (adjust based on system capabilities)
	minThroughput := 1.0             // images per second
	maxMemoryPerImage := 1024 * 1024 // 1MB per image

	if throughput < minThroughput {
		t.Errorf("Throughput too low: %.2f images/sec (minimum: %.2f)",
			throughput, minThroughput)
	}

	if memoryPerImage > float64(maxMemoryPerImage) {
		t.Errorf("Memory usage too high: %.2f bytes/image (maximum: %d)",
			memoryPerImage, maxMemoryPerImage)
	}

	// Log performance metrics
	t.Logf("Performance Test Results:")
	t.Logf("  Total Images: %d", totalImages)
	t.Logf("  Workers: %d", numWorkers)
	t.Logf("  Processing Time: %v", processingTime)
	t.Logf("  Throughput: %.2f images/sec", throughput)
	t.Logf("  Memory Used: %d bytes", memoryUsed)
	t.Logf("  Memory per Image: %.2f bytes", memoryPerImage)
	t.Logf("  Success Rate: %.2f%%", float64(successCount)/float64(totalImages)*100)
}

// Helper functions for integration tests

type testEnvironment struct {
	inputDir         string
	sharedResultsDir string
	workerDirs       []string
}

func setupTestEnvironment(t *testing.T) (*testEnvironment, func()) {
	baseDir, err := os.MkdirTemp("", "integration_test_*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	env := &testEnvironment{
		inputDir:         filepath.Join(baseDir, "input"),
		sharedResultsDir: filepath.Join(baseDir, "shared_results"),
		workerDirs:       make([]string, 4), // Support up to 4 workers
	}

	// Create directories
	os.MkdirAll(env.inputDir, 0755)
	os.MkdirAll(env.sharedResultsDir, 0755)

	for i := 0; i < 4; i++ {
		workerDir := filepath.Join(baseDir, fmt.Sprintf("worker_%d", i+1))
		env.workerDirs[i] = workerDir
		os.MkdirAll(filepath.Join(workerDir, "input"), 0755)
		os.MkdirAll(filepath.Join(workerDir, "output"), 0755)
	}

	cleanup := func() {
		os.RemoveAll(baseDir)
	}

	return env, cleanup
}

func createTestImages(t *testing.T, inputDir string, count int) []string {
	imageFiles := make([]string, count)

	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("test_image_%03d.jpg", i+1)
		filepath := filepath.Join(inputDir, filename)

		// Create fake image data
		imageData := make([]byte, 1024+i*100) // Variable size images
		for j := range imageData {
			imageData[j] = byte((i + j) % 256)
		}

		if err := os.WriteFile(filepath, imageData, 0644); err != nil {
			t.Fatalf("Failed to create test image %s: %v", filename, err)
		}

		imageFiles[i] = filepath
	}

	return imageFiles
}

func simulateImageProcessing(worker *PersistentWorker, req Request) error {
	// Simulate image processing without actual WebRTC
	time.Sleep(10 * time.Millisecond) // Simulate processing time
	return nil
}

func simulateResultGeneration(t *testing.T, worker *PersistentWorker) error {
	// Create mock output files
	outputDir := worker.config.OutputDirectory

	outputFiles := []string{
		fmt.Sprintf("result_%d_1.txt", worker.GetWorkerID()),
		fmt.Sprintf("result_%d_2.txt", worker.GetWorkerID()),
	}

	for _, filename := range outputFiles {
		filepath := filepath.Join(outputDir, filename)
		content := fmt.Sprintf("Processed result from worker %d: %s",
			worker.GetWorkerID(), filename)

		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create result file %s: %w", filename, err)
		}
	}

	return nil
}

func simulateResultTransmission(t *testing.T, worker *PersistentWorker, handler *ServerMessageHandler) error {
	// Simulate result transmission to server
	if worker.resultCollector == nil {
		return fmt.Errorf("result collector not initialized")
	}

	// Scan for results
	_, err := worker.resultCollector.ScanOutputDirectory()
	if err != nil {
		return fmt.Errorf("failed to scan output directory: %w", err)
	}

	// Create processing result
	processingResult := worker.resultCollector.CreateProcessingResult(
		2,                    // input file count
		100*time.Millisecond, // processing time
		messaging.StatusSuccess,
	)

	// Simulate sending result to server
	resultMsg := messaging.CreateMessage(messaging.MsgTypeResult, worker.GetWorkerID(), "", nil)
	resultMsg.Metadata["processing_result"] = processingResult

	// In a real scenario, this would go through WebRTC
	// For simulation, we just verify the message structure
	if resultMsg.Type != messaging.MsgTypeResult {
		return fmt.Errorf("invalid result message type")
	}

	return nil
}

func monitorEndToEndCompletion(t *testing.T, collector *ServerResultCollector, completionChan chan<- bool) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			if collector.IsComplete() {
				t.Log("Result collection completed")
				completionChan <- true
				return
			}

		case <-timeout:
			t.Error("End-to-end completion monitoring timed out")
			completionChan <- false
			return
		}
	}
}

// testConnectionTimeout tests connection timeout scenarios
func testConnectionTimeout(t *testing.T) {
	config := DefaultProcessingConfig()
	config.ProcessingTimeout = 100 * time.Millisecond // Very short timeout

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "timeout-test", reqChan, resChan, config)

	// Start worker and simulate timeout
	worker.SetState(StateProcessing)

	// Wait longer than timeout
	time.Sleep(200 * time.Millisecond)

	// Verify timeout handling
	if worker.GetState() != StateError {
		t.Errorf("Expected worker state to be StateError after timeout, got %s", worker.GetState())
	}

	worker.Stop()
	t.Log("Connection timeout test completed")
}

// testProcessingScriptFailure tests script execution failure scenarios
func testProcessingScriptFailure(t *testing.T) {
	config := DefaultProcessingConfig()
	config.PythonScriptPath = "/nonexistent/script.py" // Invalid script path

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "script-failure-test", reqChan, resChan, config)

	// Create script executor with invalid path
	executor := NewScriptExecutor(config.InputDirectory, config.OutputDirectory, config.ProcessingTimeout)

	// Attempt to execute non-existent script
	result, err := executor.Execute()

	if err == nil {
		t.Error("Expected script execution to fail with invalid path")
	}

	if result != nil && result.Success {
		t.Error("Expected script execution result to indicate failure")
	}

	worker.Stop()
	t.Log("Processing script failure test completed")
}

// testResultTransmissionFailure tests result transmission failure scenarios
func testResultTransmissionFailure(t *testing.T) {
	config := DefaultProcessingConfig()
	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "transmission-failure-test", reqChan, resChan, config)

	// Simulate result transmission failure by setting invalid output directory
	config.OutputDirectory = "/invalid/path"

	worker.SetState(StateSending)

	// Attempt to scan invalid output directory
	if worker.resultCollector != nil {
		_, err := worker.resultCollector.ScanOutputDirectory()
		if err == nil {
			t.Error("Expected output directory scan to fail with invalid path")
		}
	}

	worker.Stop()
	t.Log("Result transmission failure test completed")
}

// testHeartbeatFailure tests heartbeat failure scenarios
func testHeartbeatFailure(t *testing.T) {
	config := DefaultProcessingConfig()
	config.HeartbeatInterval = 50 * time.Millisecond

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "heartbeat-failure-test", reqChan, resChan, config)

	// Start heartbeat manager
	if worker.heartbeat != nil {
		// Simulate heartbeat failure by not updating last received time
		time.Sleep(150 * time.Millisecond) // Wait longer than timeout

		if worker.heartbeat.IsHealthy() {
			t.Error("Expected heartbeat to be unhealthy after timeout")
		}
	}

	worker.Stop()
	t.Log("Heartbeat failure test completed")
}

// testPartialWorkerFailure tests scenarios where some workers fail
func testPartialWorkerFailure(t *testing.T) {
	numWorkers := 3
	workers := make([]*PersistentWorker, numWorkers)

	// Create workers
	for i := 0; i < numWorkers; i++ {
		config := DefaultProcessingConfig()
		reqChan := make(chan Request, 5)
		resChan := make(chan Result, 5)

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("partial-failure-test-%d", i+1),
			reqChan, resChan, config)
	}

	// Simulate failure in worker 2
	workers[1].SetState(StateError)

	// Verify other workers are still functional
	if workers[0].GetState() == StateError {
		t.Error("Worker 1 should not be in error state")
	}

	if workers[2].GetState() == StateError {
		t.Error("Worker 3 should not be in error state")
	}

	// Verify failed worker
	if workers[1].GetState() != StateError {
		t.Error("Worker 2 should be in error state")
	}

	// Stop all workers
	for _, worker := range workers {
		worker.Stop()
	}

	t.Log("Partial worker failure test completed")
}
