package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// Worker represents the main worker edge instance
type Worker struct {
	config     *Config
	state      *WorkerState
	stateMutex sync.RWMutex

	// Components
	connectionManager *ConnectionManager
	dataHandler       *DataHandler
	trainingCoord     *TrainingCoordinator
	weightSender      *WeightSender

	// Channels for coordination
	shutdownChan chan struct{}
	errorChan    chan error
}

// NewWorker creates a new worker instance
func NewWorker(config *Config) *Worker {
	return &Worker{
		config: config,
		state: &WorkerState{
			CurrentState:  Idle,
			LastHeartbeat: time.Now(),
		},
		shutdownChan: make(chan struct{}),
		errorChan:    make(chan error, 10),
	}
}

// Start begins the worker operation
func (w *Worker) Start() error {
	log.Printf("Starting worker %s", w.config.WorkerID)

	// Initialize components
	if err := w.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	// Start main worker loop
	return w.run()
}

// Shutdown gracefully shuts down the worker
func (w *Worker) Shutdown() {
	log.Println("Shutting down worker...")
	close(w.shutdownChan)

	// Cleanup components
	if w.connectionManager != nil {
		w.connectionManager.Close()
	}
}

// GetState returns the current worker state (thread-safe)
func (w *Worker) GetState() WorkerState {
	w.stateMutex.RLock()
	defer w.stateMutex.RUnlock()
	return *w.state
}

// SetState updates the worker state (thread-safe)
func (w *Worker) SetState(newState TrainingState) {
	w.stateMutex.Lock()
	defer w.stateMutex.Unlock()

	log.Printf("State transition: %s -> %s", w.state.CurrentState, newState)
	w.state.CurrentState = newState

	if newState == Training {
		w.state.TrainingStartTime = time.Now()
	}
}

// SetError sets an error state with message
func (w *Worker) SetError(err error) {
	w.stateMutex.Lock()
	defer w.stateMutex.Unlock()

	w.state.CurrentState = ErrorState
	w.state.ErrorMessage = err.Error()
	log.Printf("Worker error: %v", err)
}

// initializeComponents initializes all worker components
func (w *Worker) initializeComponents() error {
	// Initialize connection manager
	w.connectionManager = NewConnectionManager(w.config)

	// Initialize data handler
	w.dataHandler = NewDataHandler(w.config)

	// Initialize training coordinator
	w.trainingCoord = NewTrainingCoordinator(w.config)

	// Initialize weight sender
	w.weightSender = NewWeightSender(w.config)

	return nil
}

// run executes the main worker loop
func (w *Worker) run() error {
	// Establish connection
	peer, dataChannel, err := w.connectionManager.EstablishConnection(w.config.WorkerID)
	if err != nil {
		return fmt.Errorf("failed to establish connection: %w", err)
	}
	defer peer.Close()
	defer dataChannel.Close()

	log.Println("Connection established, waiting for data...")

	// Start heartbeat
	go w.startHeartbeat(dataChannel)

	// Handle incoming messages
	go w.handleMessages(dataChannel)

	// Wait for shutdown or error
	select {
	case <-w.shutdownChan:
		log.Println("Received shutdown signal")
		return nil
	case err := <-w.errorChan:
		return fmt.Errorf("worker error: %w", err)
	}
}

// startHeartbeat sends periodic heartbeat messages
func (w *Worker) startHeartbeat(dc *webrtc.DataChannel) {
	heartbeatInterval, err := w.config.GetHeartbeatDuration()
	if err != nil {
		log.Printf("Invalid heartbeat interval: %v, using default 30s", err)
		heartbeatInterval = 30 * time.Second
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.sendHeartbeat(dc); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
				w.errorChan <- err
				return
			}
		case <-w.shutdownChan:
			return
		}
	}
}

// sendHeartbeat sends a heartbeat message
func (w *Worker) sendHeartbeat(dc *webrtc.DataChannel) error {
	w.stateMutex.Lock()
	w.state.LastHeartbeat = time.Now()
	w.stateMutex.Unlock()

	message := Message{
		Type:    Heartbeat,
		Payload: w.config.WorkerID,
	}

	return w.sendMessage(dc, message)
}

// handleMessages processes incoming messages from the data channel
func (w *Worker) handleMessages(dc *webrtc.DataChannel) {
	// This will be implemented in later tasks
	log.Println("Message handler started")
}

// sendMessage sends a message through the data channel
func (w *Worker) sendMessage(dc *webrtc.DataChannel, message Message) error {
	// This will be implemented in later tasks
	log.Printf("Sending message: %s", message.Type)
	return nil
}
