package main

import (
	"sync"
	"testing"
	"time"
)

func TestWorkerState(t *testing.T) {
	// Test WorkerState string representation
	states := []struct {
		state    WorkerState
		expected string
	}{
		{StateIdle, "IDLE"},
		{StateReceiving, "RECEIVING"},
		{StateProcessing, "PROCESSING"},
		{StateSending, "SENDING"},
		{StateComplete, "COMPLETE"},
		{StateError, "ERROR"},
	}

	for _, test := range states {
		if test.state.String() != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, test.state.String())
		}
	}
}

func TestNewPersistentWorker(t *testing.T) {
	reqChan := make(chan Request)
	resChan := make(chan Result)
	config := DefaultProcessingConfig()

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, config)

	if worker == nil {
		t.Fatal("Expected worker to be created, got nil")
	}

	if worker.GetWorkerID() != 1 {
		t.Errorf("Expected worker ID 1, got %d", worker.GetWorkerID())
	}

	if worker.GetConnectionID() != "test-conn" {
		t.Errorf("Expected connection ID 'test-conn', got %s", worker.GetConnectionID())
	}

	if worker.GetState() != StateIdle {
		t.Errorf("Expected initial state IDLE, got %s", worker.GetState())
	}

	if worker.IsRunning() {
		t.Error("Expected worker to not be running initially")
	}
}

func TestPersistentWorkerStateManagement(t *testing.T) {
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, nil)

	// Test state transitions
	worker.SetState(StateReceiving)
	if worker.GetState() != StateReceiving {
		t.Errorf("Expected state RECEIVING, got %s", worker.GetState())
	}

	worker.SetState(StateProcessing)
	if worker.GetState() != StateProcessing {
		t.Errorf("Expected state PROCESSING, got %s", worker.GetState())
	}

	worker.SetState(StateComplete)
	if worker.GetState() != StateComplete {
		t.Errorf("Expected state COMPLETE, got %s", worker.GetState())
	}
}

func TestNewPersistentWorkerPool(t *testing.T) {
	resChan := make(chan Result)
	config := DefaultProcessingConfig()

	pool := NewPersistentWorkerPool(resChan, config)

	if pool == nil {
		t.Fatal("Expected pool to be created, got nil")
	}

	if pool.GetNumWorkers() != 0 {
		t.Errorf("Expected 0 workers initially, got %d", pool.GetNumWorkers())
	}

	states := pool.GetWorkerStates()
	if len(states) != 0 {
		t.Errorf("Expected 0 worker states initially, got %d", len(states))
	}
}

func TestPersistentWorkerPoolWorkerManagement(t *testing.T) {
	resChan := make(chan Result)
	config := DefaultProcessingConfig()

	pool := NewPersistentWorkerPool(resChan, config)

	// Test adding workers to the pool manually (simulating what StartPool would do)
	reqChan1 := make(chan Request)
	reqChan2 := make(chan Request)

	worker1 := NewPersistentWorker(0, "conn1", reqChan1, resChan, config)
	worker2 := NewPersistentWorker(1, "conn2", reqChan2, resChan, config)

	// Manually add workers to test the pool functionality
	pool.workers[0] = worker1
	pool.workers[1] = worker2
	pool.numWorkers = 2

	// Test GetWorker
	retrievedWorker, exists := pool.GetWorker(0)
	if !exists {
		t.Error("Expected worker 0 to exist")
	}
	if retrievedWorker.GetWorkerID() != 0 {
		t.Errorf("Expected worker ID 0, got %d", retrievedWorker.GetWorkerID())
	}

	// Test GetAllWorkers
	allWorkers := pool.GetAllWorkers()
	if len(allWorkers) != 2 {
		t.Errorf("Expected 2 workers, got %d", len(allWorkers))
	}

	// Test GetWorkerStates
	states := pool.GetWorkerStates()
	if len(states) != 2 {
		t.Errorf("Expected 2 worker states, got %d", len(states))
	}

	// Test GetNumWorkers
	if pool.GetNumWorkers() != 2 {
		t.Errorf("Expected 2 workers, got %d", pool.GetNumWorkers())
	}

	// Test GetHealthyWorkerCount (workers are not running, so should be 0)
	healthyCount := pool.GetHealthyWorkerCount()
	if healthyCount != 0 {
		t.Errorf("Expected 0 healthy workers, got %d", healthyCount)
	}
}

func TestDefaultProcessingConfig(t *testing.T) {
	config := DefaultProcessingConfig()

	if config == nil {
		t.Fatal("Expected config to be created, got nil")
	}

	if config.PythonScriptPath != "python3" {
		t.Errorf("Expected python3, got %s", config.PythonScriptPath)
	}

	if config.InputDirectory != "./input" {
		t.Errorf("Expected ./input, got %s", config.InputDirectory)
	}

	if config.OutputDirectory != "./output" {
		t.Errorf("Expected ./output, got %s", config.OutputDirectory)
	}

	if config.ProcessingTimeout != 5*time.Minute {
		t.Errorf("Expected 5m, got %v", config.ProcessingTimeout)
	}

	if config.HeartbeatInterval != 30*time.Second {
		t.Errorf("Expected 30s, got %v", config.HeartbeatInterval)
	}
}

func TestPersistentWorkerConcurrentStateAccess(t *testing.T) {
	reqChan := make(chan Request)
	resChan := make(chan Result)

	worker := NewPersistentWorker(1, "test-conn", reqChan, resChan, nil)

	// Test concurrent state access
	var wg sync.WaitGroup
	numGoroutines := 10

	// Start goroutines that read and write state concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate between reading and writing state
			if id%2 == 0 {
				worker.SetState(StateReceiving)
			} else {
				_ = worker.GetState()
			}
		}(i)
	}

	wg.Wait()

	// Verify worker is still in a valid state
	state := worker.GetState()
	if state != StateIdle && state != StateReceiving {
		t.Errorf("Expected valid state, got %s", state)
	}
}
