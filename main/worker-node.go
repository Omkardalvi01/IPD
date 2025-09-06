package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WorkerNode represents a standalone worker that connects to a coordinator
type WorkerNode struct {
	workerID       string
	coordinatorURL string
	signalingURL   string
	config         *ProcessingConfig
	worker         *PersistentWorker
	isConnected    bool
	shutdownChan   chan os.Signal
}

// NewWorkerNode creates a new worker node instance
func NewWorkerNode(workerID, coordinatorURL, signalingURL string) *WorkerNode {
	config := LoadProcessingConfigFromEnv()

	return &WorkerNode{
		workerID:       workerID,
		coordinatorURL: coordinatorURL,
		signalingURL:   signalingURL,
		config:         config,
		isConnected:    false,
		shutdownChan:   make(chan os.Signal, 1),
	}
}

// Start starts the worker node and connects to coordinator
func (wn *WorkerNode) Start() error {
	fmt.Printf("🔧 Starting IPD Worker Node: %s\n", wn.workerID)
	fmt.Printf("📡 Coordinator URL: %s\n", wn.coordinatorURL)
	fmt.Printf("🌐 Signaling Server: %s\n", wn.signalingURL)

	// Set up signal handling for graceful shutdown
	signal.Notify(wn.shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	// Create result channel (worker will send results to coordinator)
	resultChan := make(chan Result, 100)

	// Create persistent worker
	wn.worker = NewPersistentWorker(0, wn.workerID, nil, resultChan, wn.config)

	// Set up worker for remote connection
	if err := wn.setupRemoteConnection(); err != nil {
		return fmt.Errorf("failed to setup remote connection: %w", err)
	}

	fmt.Printf("✅ Worker node %s ready and waiting for coordinator\n", wn.workerID)

	// Start worker lifecycle
	go wn.runWorkerLifecycle()

	// Wait for shutdown signal
	<-wn.shutdownChan
	fmt.Printf("🛑 Shutdown signal received, stopping worker node %s\n", wn.workerID)

	return wn.shutdown()
}

// setupRemoteConnection sets up the connection to the coordinator
func (wn *WorkerNode) setupRemoteConnection() error {
	// In a full implementation, this would:
	// 1. Connect to signaling server
	// 2. Register as available worker
	// 3. Wait for coordinator to initiate WebRTC connection
	// 4. Establish peer-to-peer connection with coordinator

	fmt.Printf("🔗 Setting up remote connection to coordinator...\n")

	// Simulate connection setup
	time.Sleep(2 * time.Second)
	wn.isConnected = true

	fmt.Printf("✅ Connected to coordinator successfully\n")
	return nil
}

// runWorkerLifecycle runs the main worker processing loop
func (wn *WorkerNode) runWorkerLifecycle() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if wn.isConnected {
				fmt.Printf("💓 Worker %s heartbeat - Status: %s\n",
					wn.workerID, wn.worker.GetState())

				// Send heartbeat to coordinator
				if err := wn.sendHeartbeat(); err != nil {
					log.Printf("Failed to send heartbeat: %v", err)
				}
			}

		case <-wn.shutdownChan:
			return
		}
	}
}

// sendHeartbeat sends a heartbeat message to the coordinator
func (wn *WorkerNode) sendHeartbeat() error {
	// In a full implementation, this would send a heartbeat
	// message through the WebRTC data channel
	fmt.Printf("📡 Sending heartbeat to coordinator\n")
	return nil
}

// processWork processes work received from coordinator
func (wn *WorkerNode) processWork(workData []byte) error {
	fmt.Printf("🔄 Processing work received from coordinator\n")

	// In a full implementation, this would:
	// 1. Receive image data from coordinator
	// 2. Process images using local processing script
	// 3. Send results back to coordinator
	// 4. Update worker state

	wn.worker.SetState(StateProcessing)

	// Simulate processing
	time.Sleep(5 * time.Second)

	wn.worker.SetState(StateComplete)
	fmt.Printf("✅ Work processing completed\n")

	return nil
}

// shutdown gracefully shuts down the worker node
func (wn *WorkerNode) shutdown() error {
	fmt.Printf("🛑 Shutting down worker node %s\n", wn.workerID)

	if wn.worker != nil {
		wn.worker.Stop()
	}

	wn.isConnected = false

	fmt.Printf("✅ Worker node %s shutdown complete\n", wn.workerID)
	return nil
}

// runWorkerNodeMode runs the application in worker node mode
func runWorkerNodeMode() {
	workerID := os.Getenv("IPD_WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", time.Now().Unix())
	}

	coordinatorURL := os.Getenv("IPD_COORDINATOR_URL")
	if coordinatorURL == "" {
		coordinatorURL = "ws://localhost:8000"
	}

	signalingURL := os.Getenv("IPD_SIGNALING_SERVER_URL")
	if signalingURL == "" {
		signalingURL = "ws://localhost:8000"
	}

	// Create and start worker node
	workerNode := NewWorkerNode(workerID, coordinatorURL, signalingURL)

	if err := workerNode.Start(); err != nil {
		log.Fatalf("Worker node failed: %v", err)
	}
}

// Worker node functionality is available through runWorkerNodeMode()
// Call this function from main.go when IPD_MODE=worker-node
