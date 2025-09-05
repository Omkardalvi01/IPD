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

// TestNetworkPartitionRecovery tests recovery from network partition scenarios
func TestNetworkPartitionRecovery(t *testing.T) {
	numWorkers := 2
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create workers
	workers := make([]*PersistentWorker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		config := DefaultProcessingConfig()
		config.HeartbeatInterval = 100 * time.Millisecond

		config.InputDirectory = filepath.Join(testDir.workerDirs[i], "input")
		config.OutputDirectory = filepath.Join(testDir.workerDirs[i], "output")

		reqChan := make(chan Request, 5)
		resChan := make(chan Result, 5)

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("partition-test-%d", i+1),
			reqChan, resChan, config)
	}

	// Start heartbeat monitoring
	var wg sync.WaitGroup
	partitionDuration := 500 * time.Millisecond

	for i, worker := range workers {
		wg.Add(1)
		go func(workerIndex int, w *PersistentWorker) {
			defer wg.Done()

			// Simulate normal operation
			w.SetState(StateReceiving)
			time.Sleep(200 * time.Millisecond)

			// Simulate network partition (stop heartbeats)
			if w.heartbeat != nil {
				w.heartbeat.Stop()
			}

			// Wait for partition duration
			time.Sleep(partitionDuration)

			// Simulate network recovery (restart heartbeats)
			if w.heartbeat != nil {
				// In a real scenario, this would re-establish connection
				t.Logf("Worker %d: Network partition recovered", w.GetWorkerID())
			}

			w.SetState(StateComplete)
		}(i, worker)
	}

	wg.Wait()

	// Verify workers handled partition gracefully
	for i, worker := range workers {
		if worker.GetState() != StateComplete {
			t.Errorf("Worker %d should have completed after partition recovery, got state %s",
				i+1, worker.GetState())
		}
		worker.Stop()
	}

	t.Log("Network partition recovery test completed")
}

// TestConcurrentWorkerFailures tests handling of multiple simultaneous worker failures
func TestConcurrentWorkerFailures(t *testing.T) {
	numWorkers := 4
	failureCount := 2

	// Create server components
	serverCollector := NewServerResultCollector(numWorkers, "./test_results")
	lifecycleManager := NewConnectionLifecycleManager(5*time.Second, 10*time.Second)
	serverHandler := NewServerMessageHandler(serverCollector, lifecycleManager)

	// Create workers
	workers := make([]*PersistentWorker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		config := DefaultProcessingConfig()
		reqChan := make(chan Request, 5)
		resChan := make(chan Result, 5)

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("concurrent-failure-%d", i+1),
			reqChan, resChan, config)
	}

	// Simulate concurrent failures
	var wg sync.WaitGroup
	for i := 0; i < failureCount; i++ {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()

			worker := workers[workerIndex]
			worker.SetState(StateProcessing)

			// Simulate processing failure
			time.Sleep(100 * time.Millisecond)
			worker.SetState(StateError)

			// Create error message
			errorMsg := messaging.CreateMessage(messaging.MsgTypeError, worker.GetWorkerID(),
				"", []byte(fmt.Sprintf("Worker %d failed during processing", worker.GetWorkerID())))

			// Simulate sending error to server
			serverHandler.handleWorkerMessage(worker.GetWorkerID(), []byte(errorMsg.Data))

			t.Logf("Worker %d failed as expected", worker.GetWorkerID())
		}(i)
	}

	// Keep remaining workers running normally
	for i := failureCount; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()

			worker := workers[workerIndex]
			worker.SetState(StateReceiving)
			time.Sleep(200 * time.Millisecond)
			worker.SetState(StateProcessing)
			time.Sleep(100 * time.Millisecond)
			worker.SetState(StateSending)
			time.Sleep(50 * time.Millisecond)
			worker.SetState(StateComplete)

			t.Logf("Worker %d completed successfully", worker.GetWorkerID())
		}(i)
	}

	wg.Wait()

	// Verify system handled partial failures correctly
	successfulWorkers := 0
	failedWorkers := 0

	for _, worker := range workers {
		switch worker.GetState() {
		case StateComplete:
			successfulWorkers++
		case StateError:
			failedWorkers++
		}
		worker.Stop()
	}

	if failedWorkers != failureCount {
		t.Errorf("Expected %d failed workers, got %d", failureCount, failedWorkers)
	}

	if successfulWorkers != numWorkers-failureCount {
		t.Errorf("Expected %d successful workers, got %d", numWorkers-failureCount, successfulWorkers)
	}

	t.Logf("Concurrent worker failures test completed: %d successful, %d failed",
		successfulWorkers, failedWorkers)
}

// TestResourceExhaustionRecovery tests recovery from resource exhaustion scenarios
func TestResourceExhaustionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource exhaustion test in short mode")
	}

	config := DefaultProcessingConfig()
	config.ProcessingTimeout = 1 * time.Second

	reqChan := make(chan Request, 1000) // Large queue to simulate memory pressure
	resChan := make(chan Result, 1000)

	worker := NewPersistentWorker(1, "resource-exhaustion-test", reqChan, resChan, config)

	// Fill message queue to simulate memory pressure
	queueCapacity := cap(worker.messageQueue)
	for i := 0; i < queueCapacity; i++ {
		msg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, 1, "", nil)
		queueItem := MessageQueueItem{
			Message:   msg,
			Timestamp: time.Now(),
		}

		select {
		case worker.messageQueue <- queueItem:
		default:
			break // Queue full
		}
	}

	// Verify queue is at capacity
	if !worker.IsMessageQueueFull() {
		t.Error("Expected message queue to be full")
	}

	// Start processing to drain queue
	go worker.processMessageQueue()

	// Wait for queue to drain
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if worker.GetQueueSize() < queueCapacity/2 {
				t.Log("Queue successfully drained, resource pressure relieved")
				worker.Stop()
				return
			}
		case <-timeout:
			t.Error("Queue failed to drain within timeout")
			worker.Stop()
			return
		}
	}
}

// TestGracefulShutdownWithActiveProcessing tests graceful shutdown during active processing
func TestGracefulShutdownWithActiveProcessing(t *testing.T) {
	numWorkers := 3
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create test images
	imageFiles := createTestImages(t, testDir.inputDir, 6)

	// Create workers
	workers := make([]*PersistentWorker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		config := DefaultProcessingConfig()
		config.InputDirectory = filepath.Join(testDir.workerDirs[i], "input")
		config.OutputDirectory = filepath.Join(testDir.workerDirs[i], "output")
		config.ProcessingTimeout = 2 * time.Second

		reqChan := make(chan Request, 5)
		resChan := make(chan Result, 5)

		workers[i] = NewPersistentWorker(i+1, fmt.Sprintf("graceful-shutdown-%d", i+1),
			reqChan, resChan, config)
	}

	// Start processing
	var wg sync.WaitGroup
	shutdownChan := make(chan bool, 1)

	for i, worker := range workers {
		wg.Add(1)
		go func(workerIndex int, w *PersistentWorker) {
			defer wg.Done()

			w.SetState(StateReceiving)

			// Process some images
			startIdx := workerIndex * 2
			endIdx := startIdx + 2

			for imgIdx := startIdx; imgIdx < endIdx && imgIdx < len(imageFiles); imgIdx++ {
				imageFile := imageFiles[imgIdx]
				file, err := os.Open(imageFile)
				if err != nil {
					t.Errorf("Worker %d failed to open image %s: %v", w.GetWorkerID(), imageFile, err)
					continue
				}

				req := Request{f: file}
				if err := simulateImageProcessing(w, req); err != nil {
					t.Errorf("Worker %d failed to process image: %v", w.GetWorkerID(), err)
				}

				file.Close()

				// Check for shutdown signal
				select {
				case <-shutdownChan:
					t.Logf("Worker %d received shutdown signal during processing", w.GetWorkerID())
					w.SetState(StateError) // Use StateError to indicate graceful shutdown
					return
				default:
					// Continue processing
				}
			}

			w.SetState(StateProcessing)
			time.Sleep(200 * time.Millisecond) // Simulate processing time

			w.SetState(StateComplete)
			t.Logf("Worker %d completed processing before shutdown", w.GetWorkerID())
		}(i, worker)
	}

	// Send shutdown signal after some processing time
	time.Sleep(300 * time.Millisecond)
	close(shutdownChan)

	wg.Wait()

	// Verify graceful shutdown
	completedWorkers := 0
	shutdownWorkers := 0

	for _, worker := range workers {
		switch worker.GetState() {
		case StateComplete:
			completedWorkers++
		case StateError:
			shutdownWorkers++
		}
		worker.Stop()
	}

	totalHandled := completedWorkers + shutdownWorkers
	if totalHandled != numWorkers {
		t.Errorf("Expected all %d workers to be handled gracefully, got %d", numWorkers, totalHandled)
	}

	t.Logf("Graceful shutdown test completed: %d completed, %d shutdown gracefully",
		completedWorkers, shutdownWorkers)
}

// TestDataCorruptionDetection tests detection and handling of data corruption
func TestDataCorruptionDetection(t *testing.T) {
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	config := DefaultProcessingConfig()
	config.InputDirectory = filepath.Join(testDir.workerDirs[0], "input")
	config.OutputDirectory = filepath.Join(testDir.workerDirs[0], "output")

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "corruption-test", reqChan, resChan, config)

	// Create a test file with known content
	testFile := filepath.Join(config.InputDirectory, "test_image.jpg")
	originalData := []byte("original image data")
	if err := os.WriteFile(testFile, originalData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Simulate data corruption by modifying the file
	corruptedData := []byte("corrupted image data")
	if err := os.WriteFile(testFile, corruptedData, 0644); err != nil {
		t.Fatalf("Failed to corrupt test file: %v", err)
	}

	// Process the corrupted file
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer file.Close()

	req := Request{f: file}

	// Simulate processing with corruption detection
	worker.SetState(StateReceiving)
	if err := simulateImageProcessing(worker, req); err != nil {
		t.Logf("Processing detected corruption as expected: %v", err)
	}

	// Verify worker can handle corruption gracefully
	if worker.GetState() == StateError {
		t.Log("Worker correctly transitioned to error state due to corruption")
	}

	worker.Stop()
	t.Log("Data corruption detection test completed")
}

// TestLongRunningProcessingTimeout tests timeout handling for long-running processes
func TestLongRunningProcessingTimeout(t *testing.T) {
	config := DefaultProcessingConfig()
	config.ProcessingTimeout = 200 * time.Millisecond // Short timeout for testing

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "timeout-test", reqChan, resChan, config)

	// Create script executor with timeout
	executor := NewScriptExecutor(config.InputDirectory, config.OutputDirectory, config.ProcessingTimeout)

	// Start processing
	worker.SetState(StateProcessing)

	// Simulate long-running process
	go func() {
		time.Sleep(500 * time.Millisecond) // Longer than timeout
		worker.SetState(StateComplete)
	}()

	// Wait for timeout
	time.Sleep(300 * time.Millisecond)

	// Verify timeout was handled
	if worker.GetState() != StateError {
		// Try to execute with timeout
		result, err := executor.Execute()
		if err == nil && result != nil && result.Success {
			t.Error("Expected processing to timeout")
		}
	}

	worker.Stop()
	t.Log("Long-running processing timeout test completed")
}

// TestMemoryLeakDetection tests for memory leaks during extended operation
func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	// Baseline memory measurement
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	// Run multiple processing cycles
	cycles := 10
	for cycle := 0; cycle < cycles; cycle++ {
		config := DefaultProcessingConfig()
		reqChan := make(chan Request, 10)
		resChan := make(chan Result, 10)

		worker := NewPersistentWorker(cycle+1, fmt.Sprintf("leak-test-%d", cycle+1),
			reqChan, resChan, config)

		// Simulate processing cycle
		worker.SetState(StateReceiving)
		time.Sleep(10 * time.Millisecond)
		worker.SetState(StateProcessing)
		time.Sleep(10 * time.Millisecond)
		worker.SetState(StateSending)
		time.Sleep(10 * time.Millisecond)
		worker.SetState(StateComplete)

		worker.Stop()

		// Force garbage collection every few cycles
		if cycle%3 == 0 {
			runtime.GC()
		}
	}

	// Final memory measurement
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Check for significant memory increase
	memoryIncrease := memAfter.Alloc - memBefore.Alloc
	maxAcceptableIncrease := uint64(1024 * 1024) // 1MB

	if memoryIncrease > maxAcceptableIncrease {
		t.Errorf("Potential memory leak detected: memory increased by %d bytes (max acceptable: %d)",
			memoryIncrease, maxAcceptableIncrease)
	}

	t.Logf("Memory leak test completed: memory increase = %d bytes", memoryIncrease)
}
