package main

import (
	"fmt"
	"testing"
	"time"
)

// testConnectionTimeout tests connection timeout scenarios
func testConnectionTimeout(t *testing.T) {
	shortTimeout := 100 * time.Millisecond
	config := DefaultProcessingConfig()
	config.HeartbeatInterval = shortTimeout
	config.HeartbeatTimeout = shortTimeout * 2

	reqChan := make(chan Request, 5)
	resChan := make(chan Result, 5)

	worker := NewPersistentWorker(1, "timeout-test", reqChan, resChan, config)

	// Start worker and simulate timeout
	worker.SetState(StateProcessing)

	// Wait longer than timeout
	time.Sleep(shortTimeout * 3)

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
	config.HeartbeatTimeout = 100 * time.Millisecond

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
