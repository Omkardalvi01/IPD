package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
	"github.com/Omkardalvi01/IPD/networking"
	"github.com/pion/webrtc/v3"
)

type result_state int

const (
	SUCCESS result_state = 0
	FAILURE result_state = -1
	END     string       = "EOF"
)

// WorkerState represents the current state of a persistent worker
type WorkerState int

const (
	StateIdle WorkerState = iota
	StateReceiving
	StateProcessing
	StateSending
	StateComplete
	StateError
)

// String returns the string representation of WorkerState
func (ws WorkerState) String() string {
	switch ws {
	case StateIdle:
		return "IDLE"
	case StateReceiving:
		return "RECEIVING"
	case StateProcessing:
		return "PROCESSING"
	case StateSending:
		return "SENDING"
	case StateComplete:
		return "COMPLETE"
	case StateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ProcessingConfig holds configuration for worker processing
type ProcessingConfig struct {
	PythonScriptPath  string
	InputDirectory    string
	OutputDirectory   string
	ProcessingTimeout time.Duration
	HeartbeatInterval time.Duration
}

// DefaultProcessingConfig returns default processing configuration
func DefaultProcessingConfig() *ProcessingConfig {
	return &ProcessingConfig{
		PythonScriptPath:  "python3",
		InputDirectory:    "./input",
		OutputDirectory:   "./output",
		ProcessingTimeout: 5 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
	}
}

// LoadProcessingConfigFromEnv loads ProcessingConfig from environment variables
func LoadProcessingConfigFromEnv() *ProcessingConfig {
	config := DefaultProcessingConfig()

	// Load from environment variables
	if val := os.Getenv("IPD_PYTHON_SCRIPT_PATH"); val != "" {
		config.PythonScriptPath = val
	}
	if val := os.Getenv("IPD_INPUT_DIRECTORY"); val != "" {
		config.InputDirectory = val
	}
	if val := os.Getenv("IPD_OUTPUT_DIRECTORY"); val != "" {
		config.OutputDirectory = val
	}
	if val := os.Getenv("IPD_PROCESSING_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.ProcessingTimeout = duration
		}
	}
	if val := os.Getenv("IPD_HEARTBEAT_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.HeartbeatInterval = duration
		}
	}

	return config
}

// ValidateProcessingConfig validates the processing configuration parameters
func ValidateProcessingConfig(config *ProcessingConfig) error {
	if config.ProcessingTimeout <= 0 {
		return fmt.Errorf("processing timeout must be positive")
	}
	if config.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if config.InputDirectory == "" {
		return fmt.Errorf("input directory cannot be empty")
	}
	if config.OutputDirectory == "" {
		return fmt.Errorf("output directory cannot be empty")
	}
	if config.PythonScriptPath == "" {
		return fmt.Errorf("python script path cannot be empty")
	}

	return nil
}

type Result struct {
	worker_id int
	result    result_state
}

type Request struct {
	f *os.File
}

// MessageQueueItem represents an item in the message queue
type MessageQueueItem struct {
	Message   *messaging.Message
	Timestamp time.Time
}

// MessageHandler defines the interface for handling different message types
type MessageHandler interface {
	HandleMessage(worker *PersistentWorker, msg *messaging.Message) error
}

// PersistentWorker represents an enhanced worker with persistent connection capabilities
type PersistentWorker struct {
	worker_id           int
	conn_id             string
	req_chan            <-chan Request
	res_chan            chan<- Result
	peer_conn           *webrtc.PeerConnection
	data_channel        *webrtc.DataChannel
	config              *ProcessingConfig
	heartbeat           *messaging.HeartbeatManager
	state               WorkerState
	stateMutex          sync.RWMutex
	stopChan            chan struct{}
	isRunning           bool
	runningMutex        sync.RWMutex
	messageQueue        chan MessageQueueItem
	messageHandlers     map[messaging.MessageType]MessageHandler
	handlerMutex        sync.RWMutex
	ackWaitMap          map[string]chan bool
	ackMutex            sync.RWMutex
	scriptExecutor      *ScriptExecutor
	resultCollector     *ResultCollector
	fileTransmitter     *FileTransmitter
	errorRecovery       *ErrorRecoverySystem
	logger              *EnhancedLogger
	integrationCallback func(*PersistentWorker)
}

// Legacy Worker struct for backward compatibility
type Worker struct {
	req_chan  <-chan Request
	res_chan  chan<- Result
	worker_id int
	conn_id   string
}

// NewPersistentWorker creates a new PersistentWorker instance
func NewPersistentWorker(workerID int, connID string, reqChan <-chan Request, resChan chan<- Result, config *ProcessingConfig) *PersistentWorker {
	if config == nil {
		config = DefaultProcessingConfig()
	}

	pw := &PersistentWorker{
		worker_id:       workerID,
		conn_id:         connID,
		req_chan:        reqChan,
		res_chan:        resChan,
		config:          config,
		state:           StateIdle,
		stopChan:        make(chan struct{}),
		isRunning:       false,
		messageQueue:    make(chan MessageQueueItem, 100), // Buffer for 100 messages
		messageHandlers: make(map[messaging.MessageType]MessageHandler),
		ackWaitMap:      make(map[string]chan bool),
	}

	// Initialize enhanced logger
	loggerConfig := DefaultLoggerConfig()
	loggerConfig.LogDirectory = fmt.Sprintf("./logs/worker_%d", workerID)
	logger, err := NewEnhancedLogger(loggerConfig)
	if err != nil {
		log.Printf("Failed to initialize enhanced logger for worker %d: %v", workerID, err)
		// Continue with standard logging
	} else {
		pw.logger = logger
	}

	// Initialize error recovery system
	errorRecovery, err := NewErrorRecoverySystem(1, loggerConfig) // Single worker system
	if err != nil {
		log.Printf("Failed to initialize error recovery system for worker %d: %v", workerID, err)
	} else {
		pw.errorRecovery = errorRecovery
		pw.errorRecovery.Start()
	}

	// Initialize script executor
	pw.scriptExecutor = NewScriptExecutor(
		config.InputDirectory,
		config.OutputDirectory,
		config.ProcessingTimeout,
	)

	// Initialize result collector
	pw.resultCollector = NewResultCollector(workerID, config.OutputDirectory)

	// Register default message handlers
	pw.registerMessageHandlers()

	return pw
}

// GetState returns the current state of the worker
func (pw *PersistentWorker) GetState() WorkerState {
	pw.stateMutex.RLock()
	defer pw.stateMutex.RUnlock()
	return pw.state
}

// SetState sets the worker state
func (pw *PersistentWorker) SetState(state WorkerState) {
	pw.stateMutex.Lock()
	defer pw.stateMutex.Unlock()

	oldState := pw.state
	pw.state = state

	log.Printf("Worker %d state changed: %s -> %s", pw.worker_id, oldState, state)
}

// IsRunning returns whether the worker is currently running
func (pw *PersistentWorker) IsRunning() bool {
	pw.runningMutex.RLock()
	defer pw.runningMutex.RUnlock()
	return pw.isRunning
}

// setRunning sets the running state
func (pw *PersistentWorker) setRunning(running bool) {
	pw.runningMutex.Lock()
	defer pw.runningMutex.Unlock()
	pw.isRunning = running
}

// GetWorkerID returns the worker ID
func (pw *PersistentWorker) GetWorkerID() int {
	return pw.worker_id
}

// GetConnectionID returns the connection ID
func (pw *PersistentWorker) GetConnectionID() string {
	return pw.conn_id
}

// SetIntegrationCallback sets the callback function for server integration
func (pw *PersistentWorker) SetIntegrationCallback(callback func(*PersistentWorker)) {
	pw.integrationCallback = callback
}

// HeartbeatHandler handles heartbeat messages
type HeartbeatHandler struct{}

func (h *HeartbeatHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received heartbeat message", worker.worker_id)

	if worker.heartbeat != nil {
		return worker.heartbeat.HandleHeartbeatMessage(msg)
	}

	return fmt.Errorf("heartbeat manager not initialized")
}

// AckHandler handles acknowledgment messages
type AckHandler struct{}

func (h *AckHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received acknowledgment message", worker.worker_id)

	// Handle file transmitter acknowledgments if available
	if worker.fileTransmitter != nil {
		if err := worker.fileTransmitter.HandleAcknowledgment(msg); err != nil {
			log.Printf("Worker %d file transmitter ack handling failed: %v", worker.worker_id, err)
		}
	}

	// Extract acknowledgment ID from metadata for legacy support
	if ackID, exists := msg.Metadata["ack_id"]; exists {
		if ackIDStr, ok := ackID.(string); ok {
			worker.ackMutex.Lock()
			if ackChan, exists := worker.ackWaitMap[ackIDStr]; exists {
				select {
				case ackChan <- true:
				default:
				}
				delete(worker.ackWaitMap, ackIDStr)
			}
			worker.ackMutex.Unlock()
		}
	}

	return nil
}

// ImageHandler handles image-related messages
type ImageHandler struct{}

func (h *ImageHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received image message: %s", worker.worker_id, msg.Filename)

	// Handle image data reception
	// This would be used when the server sends images to workers
	// For now, we just log and acknowledge

	// Send acknowledgment
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, worker.worker_id, "", nil)
	ackMsg.Metadata["ack_for"] = msg.Type
	ackMsg.Metadata["original_filename"] = msg.Filename

	return worker.sendMessage(ackMsg)
}

// ResultHandler handles result messages
type ResultHandler struct{}

func (h *ResultHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received result message", worker.worker_id)

	// Handle result processing instructions from server
	// This could include commands to start processing, send results, etc.

	// Send acknowledgment
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, worker.worker_id, "", nil)
	ackMsg.Metadata["ack_for"] = msg.Type

	return worker.sendMessage(ackMsg)
}

// MessageErrorHandler handles error messages
type MessageErrorHandler struct{}

func (h *MessageErrorHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received error message: %s", worker.worker_id, string(msg.Data))

	// Handle error messages from server
	// This might trigger error recovery procedures

	worker.SetState(StateError)

	return nil
}

// ConnectionCloseHandler handles connection close messages
type ConnectionCloseHandler struct{}

func (h *ConnectionCloseHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received connection close message", worker.worker_id)

	// Extract close information from metadata
	if closeInfo, exists := msg.Metadata["close_info"]; exists {
		log.Printf("Worker %d close reason: %v", worker.worker_id, closeInfo)
	}

	// Send acknowledgment
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, worker.worker_id, "", nil)
	ackMsg.Metadata["ack_for"] = msg.Type
	ackMsg.Metadata["close_acknowledged"] = true

	if err := worker.sendMessage(ackMsg); err != nil {
		log.Printf("Worker %d failed to send close acknowledgment: %v", worker.worker_id, err)
	}

	// Initiate graceful shutdown
	go worker.initiateGracefulShutdown("Server requested connection close")

	return nil
}

// ShutdownHandler handles shutdown messages
type ShutdownHandler struct{}

func (h *ShutdownHandler) HandleMessage(worker *PersistentWorker, msg *messaging.Message) error {
	log.Printf("Worker %d received shutdown message", worker.worker_id)

	// Extract shutdown information from metadata
	if shutdownInfo, exists := msg.Metadata["shutdown_info"]; exists {
		log.Printf("Worker %d shutdown info: %v", worker.worker_id, shutdownInfo)
	}

	// Send acknowledgment
	ackMsg := messaging.CreateMessage(messaging.MsgTypeAck, worker.worker_id, "", nil)
	ackMsg.Metadata["ack_for"] = msg.Type
	ackMsg.Metadata["shutdown_acknowledged"] = true

	if err := worker.sendMessage(ackMsg); err != nil {
		log.Printf("Worker %d failed to send shutdown acknowledgment: %v", worker.worker_id, err)
	}

	// Initiate immediate shutdown
	go worker.initiateGracefulShutdown("System shutdown requested")

	return nil
}

// registerMessageHandlers registers all message handlers
func (pw *PersistentWorker) registerMessageHandlers() {
	pw.handlerMutex.Lock()
	defer pw.handlerMutex.Unlock()

	pw.messageHandlers[messaging.MsgTypeHeartbeat] = &HeartbeatHandler{}
	pw.messageHandlers[messaging.MsgTypeAck] = &AckHandler{}
	pw.messageHandlers[messaging.MsgTypeImage] = &ImageHandler{}
	pw.messageHandlers[messaging.MsgTypeResult] = &ResultHandler{}
	pw.messageHandlers[messaging.MsgTypeError] = &MessageErrorHandler{}
	pw.messageHandlers[messaging.MsgTypeConnectionClose] = &ConnectionCloseHandler{}
	pw.messageHandlers[messaging.MsgTypeShutdown] = &ShutdownHandler{}
}

// RegisterMessageHandler allows registering custom message handlers
func (pw *PersistentWorker) RegisterMessageHandler(msgType messaging.MessageType, handler MessageHandler) {
	pw.handlerMutex.Lock()
	defer pw.handlerMutex.Unlock()

	pw.messageHandlers[msgType] = handler
	log.Printf("Worker %d registered handler for message type: %s", pw.worker_id, msgType)
}

// Start begins the persistent worker lifecycle
func (pw *PersistentWorker) Start(wg *sync.WaitGroup) error {
	if pw.IsRunning() {
		return fmt.Errorf("worker %d is already running", pw.worker_id)
	}

	pw.setRunning(true)
	pw.SetState(StateIdle)

	// Establish WebRTC connection
	peer, dc, err := networking.Peerconnection(pw.conn_id)
	if err != nil {
		log.Printf("Error with peer connection in worker %d: %v", pw.worker_id, err)
		pw.SetState(StateError)
		pw.setRunning(false)
		return err
	}

	pw.peer_conn = peer
	pw.data_channel = dc

	// Initialize heartbeat manager
	heartbeatFailureCallback := func() {
		log.Printf("Heartbeat failure detected for worker %d", pw.worker_id)
		pw.handleConnectionFailure()
	}

	pw.heartbeat = messaging.NewHeartbeatManager(
		pw.worker_id,
		pw.config.HeartbeatInterval,
		pw.config.HeartbeatInterval*3, // timeout is 3x interval
		heartbeatFailureCallback,
	)

	wg.Done()

	// Set up data channel handlers
	dc.OnOpen(func() {
		log.Printf("Data channel opened for persistent worker %d", pw.worker_id)

		// Initialize file transmitter now that data channel is available
		pw.fileTransmitter = NewFileTransmitter(pw.worker_id, dc, pw.resultCollector)

		// Start heartbeat manager
		if err := pw.heartbeat.Start(dc); err != nil {
			log.Printf("Failed to start heartbeat manager for worker %d: %v", pw.worker_id, err)
		}

		// Call integration callback if available
		if pw.integrationCallback != nil {
			pw.integrationCallback(pw)
		}

		// Start the main processing loop
		go pw.processRequests()
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		pw.handleIncomingMessage(msg.Data)
	})

	// Start message processing loop
	go pw.processMessageQueue()

	dc.OnClose(func() {
		log.Printf("Data channel closed for persistent worker %d", pw.worker_id)
		pw.cleanup()
	})

	dc.OnError(func(err error) {
		log.Printf("Data channel error for worker %d: %v", pw.worker_id, err)
		pw.SetState(StateError)
		pw.handleConnectionFailure()
	})

	// Wait for stop signal or connection closure
	<-pw.stopChan
	return nil
}

// Stop gracefully stops the persistent worker
func (pw *PersistentWorker) Stop() {
	if !pw.IsRunning() {
		return
	}

	log.Printf("Stopping persistent worker %d", pw.worker_id)
	pw.SetState(StateComplete)
	pw.cleanup()
}

// processRequests handles the main request processing loop
func (pw *PersistentWorker) processRequests() {
	pw.SetState(StateReceiving)

	for r := range pw.req_chan {
		if !pw.IsRunning() {
			r.f.Close()
			break
		}

		if err := pw.processRequest(r); err != nil {
			log.Printf("Error processing request in worker %d: %v", pw.worker_id, err)
			pw.SetState(StateError)
			r.f.Close()
			continue
		}
		r.f.Close()
	}

	// After processing all requests, maintain connection for result transmission
	log.Printf("Worker %d finished receiving images, maintaining connection for processing", pw.worker_id)
	pw.SetState(StateProcessing)

	// In a future task, this is where external script processing would be triggered
	// For now, we just maintain the connection and wait
	pw.maintainConnectionForProcessing()
}

// processRequest processes a single request (image)
func (pw *PersistentWorker) processRequest(r Request) error {
	if pw.data_channel == nil || pw.data_channel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel is not open")
	}

	// Extract filename
	file_name := strings.Split(r.f.Name(), "/")[1]

	// Send filename
	if err := pw.data_channel.SendText(file_name); err != nil {
		return fmt.Errorf("failed to send filename: %w", err)
	}

	// Get and send image data
	img, err := get_img_data(r.f.Name())
	if err != nil {
		return fmt.Errorf("failed to get image data: %w", err)
	}

	if err := pw.data_channel.Send(img); err != nil {
		return fmt.Errorf("failed to send image data: %w", err)
	}

	// Send end marker
	if err := pw.data_channel.SendText(END); err != nil {
		return fmt.Errorf("failed to send end marker: %w", err)
	}

	// Send success result
	pw.res_chan <- Result{worker_id: pw.worker_id, result: SUCCESS}

	return nil
}

// maintainConnectionForProcessing keeps the connection alive for processing phase
func (pw *PersistentWorker) maintainConnectionForProcessing() {
	// This method maintains the connection after all images are received
	// Now triggers external script processing using ScriptExecutor

	log.Printf("Worker %d maintaining connection for processing phase", pw.worker_id)

	go func() {
		// Trigger external script processing
		if pw.logger != nil {
			pw.logger.InfoWithContext("Starting external script processing", pw.worker_id, "processing", "script_execution", nil)
		} else {
			log.Printf("Worker %d starting external script processing", pw.worker_id)
		}

		execResult, err := pw.scriptExecutor.Execute()
		if err != nil {
			// Create and handle processing error through error recovery system
			procErr := CreateProcessingError(pw.worker_id, "script_execution",
				"Script execution failed", err)

			if pw.errorRecovery != nil {
				if recoveryErr := pw.errorRecovery.HandleError(procErr); recoveryErr != nil {
					if pw.logger != nil {
						pw.logger.ErrorWithContext("Processing error recovery failed", pw.worker_id, "error_recovery", "processing_error",
							recoveryErr, map[string]interface{}{"original_error": err.Error()})
					}
				}
			}

			if pw.logger != nil {
				pw.logger.ErrorWithContext("Script execution failed", pw.worker_id, "processing", "script_execution",
					err, nil)
			} else {
				log.Printf("Worker %d script execution failed: %v", pw.worker_id, err)
			}

			pw.SetState(StateError)

			// Send error message to server
			errorMsg := fmt.Sprintf("Script execution failed: %v", err)
			if sendErr := pw.SendError(errorMsg); sendErr != nil {
				if pw.logger != nil {
					pw.logger.ErrorWithContext("Failed to send error message", pw.worker_id, "messaging", "send_error",
						sendErr, nil)
				} else {
					log.Printf("Worker %d failed to send error message: %v", pw.worker_id, sendErr)
				}
			}
			return
		}

		log.Printf("Worker %d script execution completed successfully", pw.worker_id)

		// Set state to sending results
		pw.SetState(StateSending)

		// Collect processed files from output directory
		if pw.logger != nil {
			pw.logger.InfoWithContext("Scanning output directory for processed files", pw.worker_id, "processing", "result_collection", nil)
		} else {
			log.Printf("Worker %d scanning output directory for processed files", pw.worker_id)
		}

		newFiles, err := pw.resultCollector.ScanOutputDirectory()
		if err != nil {
			// Create and handle validation error
			validErr := CreateValidationError(pw.worker_id, "result_collection",
				"Failed to scan output directory", err)

			if pw.errorRecovery != nil {
				if recoveryErr := pw.errorRecovery.HandleError(validErr); recoveryErr != nil {
					if pw.logger != nil {
						pw.logger.ErrorWithContext("Result collection error recovery failed", pw.worker_id, "error_recovery", "validation_error",
							recoveryErr, map[string]interface{}{"original_error": err.Error()})
					}
				}
			}

			if pw.logger != nil {
				pw.logger.ErrorWithContext("Failed to scan output directory", pw.worker_id, "processing", "result_collection",
					err, nil)
			} else {
				log.Printf("Worker %d failed to scan output directory: %v", pw.worker_id, err)
			}

			pw.SetState(StateError)

			errorMsg := fmt.Sprintf("Failed to scan output directory: %v", err)
			if sendErr := pw.SendError(errorMsg); sendErr != nil {
				if pw.logger != nil {
					pw.logger.ErrorWithContext("Failed to send error message", pw.worker_id, "messaging", "send_error",
						sendErr, nil)
				} else {
					log.Printf("Worker %d failed to send error message: %v", pw.worker_id, sendErr)
				}
			}
			return
		}

		log.Printf("Worker %d found %d processed files", pw.worker_id, len(newFiles))

		// Create processing result with collected file metadata
		processingResult := pw.resultCollector.CreateProcessingResult(
			len(execResult.InputFiles),
			execResult.Duration,
			messaging.StatusSuccess,
		)

		// Send processing result summary to server
		if err := pw.SendResult(processingResult); err != nil {
			// Create and handle transmission error
			transErr := CreateTransmissionError(pw.worker_id, "result_transmission",
				"Failed to send processing result", err)

			if pw.errorRecovery != nil {
				if recoveryErr := pw.errorRecovery.HandleError(transErr); recoveryErr != nil {
					if pw.logger != nil {
						pw.logger.ErrorWithContext("Result transmission error recovery failed", pw.worker_id, "error_recovery", "transmission_error",
							recoveryErr, map[string]interface{}{"original_error": err.Error()})
					}
				}
			}

			if pw.logger != nil {
				pw.logger.ErrorWithContext("Failed to send processing result", pw.worker_id, "transmission", "result_transmission",
					err, nil)
			} else {
				log.Printf("Worker %d failed to send processing result: %v", pw.worker_id, err)
			}

			pw.SetState(StateError)

			errorMsg := fmt.Sprintf("Failed to send processing result: %v", err)
			if sendErr := pw.SendError(errorMsg); sendErr != nil {
				if pw.logger != nil {
					pw.logger.ErrorWithContext("Failed to send error message", pw.worker_id, "messaging", "send_error",
						sendErr, nil)
				} else {
					log.Printf("Worker %d failed to send error message: %v", pw.worker_id, sendErr)
				}
			}
			return
		}

		// Transmit all collected files
		if pw.fileTransmitter != nil && len(newFiles) > 0 {
			if pw.logger != nil {
				pw.logger.InfoWithContext("Starting file transmission", pw.worker_id, "transmission", "file_transmission",
					map[string]interface{}{"file_count": len(newFiles)})
			} else {
				log.Printf("Worker %d starting file transmission", pw.worker_id)
			}

			transmissionOptions := DefaultTransmissionOptions()
			if err := pw.fileTransmitter.TransmitAllFiles(transmissionOptions); err != nil {
				// Create and handle transmission error
				transErr := CreateTransmissionError(pw.worker_id, "file_transmission",
					"File transmission failed", err)

				if pw.errorRecovery != nil {
					if recoveryErr := pw.errorRecovery.HandleError(transErr); recoveryErr != nil {
						if pw.logger != nil {
							pw.logger.ErrorWithContext("File transmission error recovery failed", pw.worker_id, "error_recovery", "transmission_error",
								recoveryErr, map[string]interface{}{"original_error": err.Error()})
						}
					}
				}

				if pw.logger != nil {
					pw.logger.ErrorWithContext("File transmission failed", pw.worker_id, "transmission", "file_transmission",
						err, map[string]interface{}{"file_count": len(newFiles)})
				} else {
					log.Printf("Worker %d file transmission failed: %v", pw.worker_id, err)
				}

				pw.SetState(StateError)

				errorMsg := fmt.Sprintf("File transmission failed: %v", err)
				if sendErr := pw.SendError(errorMsg); sendErr != nil {
					if pw.logger != nil {
						pw.logger.ErrorWithContext("Failed to send error message", pw.worker_id, "messaging", "send_error",
							sendErr, nil)
					} else {
						log.Printf("Worker %d failed to send error message: %v", pw.worker_id, sendErr)
					}
				}
				return
			}

			if pw.logger != nil {
				pw.logger.InfoWithContext("File transmission completed successfully", pw.worker_id, "transmission", "file_transmission",
					map[string]interface{}{"file_count": len(newFiles)})
			} else {
				log.Printf("Worker %d file transmission completed successfully", pw.worker_id)
			}
		} else {
			if pw.logger != nil {
				pw.logger.InfoWithContext("No files to transmit or transmitter not available", pw.worker_id, "transmission", "file_transmission",
					map[string]interface{}{"file_count": len(newFiles), "transmitter_available": pw.fileTransmitter != nil})
			} else {
				log.Printf("Worker %d no files to transmit or transmitter not available", pw.worker_id)
			}
		}

		log.Printf("Worker %d processing and result transmission complete", pw.worker_id)
		pw.SetState(StateComplete)

		// Send final result end message to signal completion
		resultEndMsg := messaging.CreateMessage(messaging.MsgTypeResultEnd, pw.worker_id, "", nil)
		resultEndMsg.Metadata["completion_status"] = "success"
		resultEndMsg.Metadata["total_files_transmitted"] = len(newFiles)

		if err := pw.sendMessageWithAck(resultEndMsg, 30*time.Second); err != nil {
			log.Printf("Worker %d failed to send result end message: %v", pw.worker_id, err)
		}

		// Request connection closure from server
		if err := pw.requestConnectionClose(messaging.CloseReasonCompleted, "Processing completed successfully"); err != nil {
			log.Printf("Worker %d failed to request connection close: %v", pw.worker_id, err)
			// If we can't request closure, initiate graceful shutdown
			pw.initiateGracefulShutdown("Failed to request connection close")
		}
	}()
}

// handleIncomingMessage processes incoming messages from the server
func (pw *PersistentWorker) handleIncomingMessage(data []byte) {
	// Try to deserialize as a structured message first
	msg, err := messaging.DeserializeMessage(data)
	if err != nil {
		// If deserialization fails, treat as legacy text message
		pw.handleLegacyMessage(data)
		return
	}

	// Add message to queue for processing
	queueItem := MessageQueueItem{
		Message:   msg,
		Timestamp: time.Now(),
	}

	select {
	case pw.messageQueue <- queueItem:
		log.Printf("Worker %d queued message type: %s", pw.worker_id, msg.Type)
	default:
		log.Printf("Worker %d message queue full, dropping message type: %s", pw.worker_id, msg.Type)
	}
}

// handleLegacyMessage handles legacy text-based messages for backward compatibility
func (pw *PersistentWorker) handleLegacyMessage(data []byte) {
	message := string(data)
	log.Printf("Worker %d received legacy message: %s", pw.worker_id, message)

	// Handle legacy messages like "EOF", filenames, etc.
	// This maintains compatibility with existing image transmission protocol
}

// processMessageQueue processes messages from the queue
func (pw *PersistentWorker) processMessageQueue() {
	log.Printf("Worker %d started message processing loop", pw.worker_id)

	for {
		select {
		case queueItem := <-pw.messageQueue:
			if err := pw.routeMessage(queueItem.Message); err != nil {
				log.Printf("Worker %d failed to process message type %s: %v",
					pw.worker_id, queueItem.Message.Type, err)
			}
		case <-pw.stopChan:
			log.Printf("Worker %d stopping message processing loop", pw.worker_id)
			return
		}
	}
}

// routeMessage routes messages to appropriate handlers
func (pw *PersistentWorker) routeMessage(msg *messaging.Message) error {
	pw.handlerMutex.RLock()
	handler, exists := pw.messageHandlers[msg.Type]
	pw.handlerMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no handler registered for message type: %s", msg.Type)
	}

	log.Printf("Worker %d routing message type %s to handler", pw.worker_id, msg.Type)
	return handler.HandleMessage(pw, msg)
}

// sendMessage sends a message through the data channel
func (pw *PersistentWorker) sendMessage(msg *messaging.Message) error {
	if pw.data_channel == nil || pw.data_channel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel is not open")
	}

	data, err := messaging.SerializeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	if err := pw.data_channel.Send(data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	log.Printf("Worker %d sent message type: %s", pw.worker_id, msg.Type)
	return nil
}

// sendMessageWithAck sends a message and waits for acknowledgment
func (pw *PersistentWorker) sendMessageWithAck(msg *messaging.Message, timeout time.Duration) error {
	// Generate unique acknowledgment ID
	ackID := fmt.Sprintf("%d_%d", pw.worker_id, time.Now().UnixNano())

	// Add acknowledgment ID to message metadata
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}
	msg.Metadata["ack_id"] = ackID

	// Create acknowledgment channel
	ackChan := make(chan bool, 1)

	pw.ackMutex.Lock()
	pw.ackWaitMap[ackID] = ackChan
	pw.ackMutex.Unlock()

	// Send the message
	if err := pw.sendMessage(msg); err != nil {
		pw.ackMutex.Lock()
		delete(pw.ackWaitMap, ackID)
		pw.ackMutex.Unlock()
		return err
	}

	// Wait for acknowledgment or timeout
	select {
	case <-ackChan:
		log.Printf("Worker %d received acknowledgment for message type: %s", pw.worker_id, msg.Type)
		return nil
	case <-time.After(timeout):
		pw.ackMutex.Lock()
		delete(pw.ackWaitMap, ackID)
		pw.ackMutex.Unlock()
		return fmt.Errorf("acknowledgment timeout for message type: %s", msg.Type)
	}
}

// handleConnectionFailure handles connection failures with proper cleanup
func (pw *PersistentWorker) handleConnectionFailure() {
	if pw.logger != nil {
		pw.logger.ErrorWithContext("Connection failure detected", pw.worker_id, "connection", "failure",
			nil, map[string]interface{}{"current_state": pw.GetState().String()})
	} else {
		log.Printf("Handling connection failure for worker %d", pw.worker_id)
	}

	currentState := pw.GetState()
	pw.SetState(StateError)

	// Create and handle connection error through error recovery system
	if pw.errorRecovery != nil {
		connErr := CreateConnectionError(pw.worker_id, "connection_failure",
			fmt.Sprintf("Connection failure in state: %s", currentState),
			fmt.Errorf("connection lost"))

		if err := pw.errorRecovery.HandleError(connErr); err != nil {
			if pw.logger != nil {
				pw.logger.ErrorWithContext("Error recovery failed", pw.worker_id, "error_recovery", "handle_error",
					err, map[string]interface{}{"original_error": connErr.Error()})
			}
		}
	}

	// Try to notify server of the failure if connection is still available
	if pw.data_channel != nil && pw.data_channel.ReadyState() == webrtc.DataChannelStateOpen {
		errorMsg := fmt.Sprintf("Connection failure in state: %s", currentState)
		if err := pw.SendError(errorMsg); err != nil {
			if pw.logger != nil {
				pw.logger.ErrorWithContext("Failed to send error notification", pw.worker_id, "connection", "error_notification",
					err, nil)
			} else {
				log.Printf("Worker %d failed to send error notification: %v", pw.worker_id, err)
			}
		}

		// Try to request connection close
		if err := pw.requestConnectionClose(messaging.CloseReasonError, "Connection failure detected"); err != nil {
			if pw.logger != nil {
				pw.logger.ErrorWithContext("Failed to request connection close", pw.worker_id, "connection", "close_request",
					err, nil)
			} else {
				log.Printf("Worker %d failed to request connection close: %v", pw.worker_id, err)
			}
		}
	}

	// Perform cleanup after a short delay to allow messages to be sent
	time.AfterFunc(2*time.Second, func() {
		pw.cleanup()
	})
}

// cleanup performs cleanup operations
func (pw *PersistentWorker) cleanup() {
	if !pw.IsRunning() {
		return
	}

	if pw.logger != nil {
		pw.logger.InfoWithContext("Starting worker cleanup", pw.worker_id, "worker", "cleanup", nil)
	}

	pw.setRunning(false)

	// Stop error recovery system
	if pw.errorRecovery != nil {
		pw.errorRecovery.Stop()
	}

	// Stop script executor
	if pw.scriptExecutor != nil {
		pw.scriptExecutor.Stop()
	}

	// Cleanup file transmitter
	if pw.fileTransmitter != nil {
		pw.fileTransmitter.Cleanup()
	}

	// Stop heartbeat manager
	if pw.heartbeat != nil {
		pw.heartbeat.Stop()
	}

	// Close message queue
	close(pw.messageQueue)

	// Clean up pending acknowledgments
	pw.ackMutex.Lock()
	for ackID, ackChan := range pw.ackWaitMap {
		select {
		case ackChan <- false: // Signal failure
		default:
		}
		delete(pw.ackWaitMap, ackID)
	}
	pw.ackMutex.Unlock()

	// Close connections
	if pw.data_channel != nil {
		pw.data_channel.Close()
	}
	if pw.peer_conn != nil {
		pw.peer_conn.Close()
	}

	// Signal stop
	select {
	case pw.stopChan <- struct{}{}:
	default:
	}

	if pw.logger != nil {
		pw.logger.InfoWithContext("Cleanup completed", pw.worker_id, "worker", "cleanup", nil)
		pw.logger.Close()
	} else {
		log.Printf("Cleanup completed for persistent worker %d", pw.worker_id)
	}
}

// SendHeartbeat sends a heartbeat message
func (pw *PersistentWorker) SendHeartbeat() error {
	heartbeatMsg := messaging.CreateMessage(messaging.MsgTypeHeartbeat, pw.worker_id, "", nil)
	heartbeatMsg.Metadata["worker_state"] = pw.GetState().String()

	return pw.sendMessage(heartbeatMsg)
}

// SendResult sends a processing result message
func (pw *PersistentWorker) SendResult(result *messaging.ProcessingResult) error {
	resultMsg := messaging.CreateMessage(messaging.MsgTypeResult, pw.worker_id, "", nil)

	// Serialize the processing result and add to message data
	resultData, err := messaging.SerializeProcessingResult(result)
	if err != nil {
		return fmt.Errorf("failed to serialize processing result: %w", err)
	}

	resultMsg.Data = resultData
	resultMsg.Metadata["result_type"] = "processing_complete"

	return pw.sendMessageWithAck(resultMsg, 30*time.Second)
}

// SendError sends an error message
func (pw *PersistentWorker) SendError(errorMsg string) error {
	errMsg := messaging.CreateMessage(messaging.MsgTypeError, pw.worker_id, "", []byte(errorMsg))
	errMsg.Metadata["worker_state"] = pw.GetState().String()

	return pw.sendMessage(errMsg)
}

// GetQueueSize returns the current size of the message queue
func (pw *PersistentWorker) GetQueueSize() int {
	return len(pw.messageQueue)
}

// IsMessageQueueFull returns true if the message queue is full
func (pw *PersistentWorker) IsMessageQueueFull() bool {
	return len(pw.messageQueue) >= cap(pw.messageQueue)
}

func (w Worker) start(wg *sync.WaitGroup) {
	// uid := create_uid()
	// fmt.Printf("uid for worker %d : %s \n",w.worker_id, uid)
	var stop_worker chan struct{}
	peer, dc, err := networking.Peerconnection(w.conn_id)
	if err != nil {
		log.Printf("Error with peer connection in worker %d", w.worker_id)
		return
	}
	defer dc.Close()
	defer peer.Close()

	wg.Done()

	dc.OnOpen(func() {
		fmt.Println("Data channel Open")
		for r := range w.req_chan {

			file_name := strings.Split(r.f.Name(), "/")[1]
			dc.SendText(file_name)

			img, err := get_img_data(r.f.Name())
			if err != nil {
				log.Fatal("Error while get image data", err)
			}

			err = dc.Send(img)
			if err != nil {
				log.Fatal("Error while sending image data", err)
			}

			dc.SendText(END)

			w.res_chan <- Result{worker_id: w.worker_id, result: SUCCESS}
			r.f.Close()

		}
		stop_worker <- struct{}{}

	})
	<-stop_worker
}

// Legacy Workerpool struct for backward compatibility
type Workerpool struct {
	num_workers int
	resultchan  chan<- Result
}

func (wp *Workerpool) start_pool(n int, id string, wg *sync.WaitGroup) []chan Request {
	wp.num_workers = n
	data_chan := make([]chan Request, 0)
	for i := 0; i < n; i++ {
		rq := make(chan Request)
		w := Worker{worker_id: i, req_chan: rq, res_chan: wp.resultchan, conn_id: id}
		wg.Add(1)
		data_chan = append(data_chan, rq)
		go w.start(wg)
	}
	return data_chan
}

// PersistentWorkerPool manages a pool of persistent workers
type PersistentWorkerPool struct {
	workers             map[int]*PersistentWorker
	numWorkers          int
	resultChan          chan<- Result
	config              *ProcessingConfig
	mutex               sync.RWMutex
	integrationCallback func(*PersistentWorker) // Callback for integrating workers with server components
}

// NewPersistentWorkerPool creates a new persistent worker pool
func NewPersistentWorkerPool(resultChan chan<- Result, config *ProcessingConfig) *PersistentWorkerPool {
	if config == nil {
		config = DefaultProcessingConfig()
	}

	return &PersistentWorkerPool{
		workers:    make(map[int]*PersistentWorker),
		resultChan: resultChan,
		config:     config,
	}
}

// SetIntegrationCallback sets the callback function for integrating workers with server components
func (pwp *PersistentWorkerPool) SetIntegrationCallback(callback func(*PersistentWorker)) {
	pwp.mutex.Lock()
	defer pwp.mutex.Unlock()
	pwp.integrationCallback = callback
}

// StartPool starts a pool of persistent workers
func (pwp *PersistentWorkerPool) StartPool(n int, connID string, wg *sync.WaitGroup) ([]chan Request, error) {
	if n <= 0 {
		return nil, fmt.Errorf("number of workers must be positive")
	}

	pwp.mutex.Lock()
	defer pwp.mutex.Unlock()

	pwp.numWorkers = n
	dataChannels := make([]chan Request, n)

	for i := 0; i < n; i++ {
		reqChan := make(chan Request)

		worker := NewPersistentWorker(i, connID, reqChan, pwp.resultChan, pwp.config)

		// Set integration callback if available
		if pwp.integrationCallback != nil {
			worker.SetIntegrationCallback(pwp.integrationCallback)
		}

		pwp.workers[i] = worker
		dataChannels[i] = reqChan

		wg.Add(1)
		go func(w *PersistentWorker) {
			if err := w.Start(wg); err != nil {
				log.Printf("Failed to start persistent worker %d: %v", w.GetWorkerID(), err)
			}
		}(worker)
	}

	log.Printf("Started persistent worker pool with %d workers", n)
	return dataChannels, nil
}

// GetWorker returns a worker by ID
func (pwp *PersistentWorkerPool) GetWorker(workerID int) (*PersistentWorker, bool) {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()

	worker, exists := pwp.workers[workerID]
	return worker, exists
}

// GetAllWorkers returns all workers
func (pwp *PersistentWorkerPool) GetAllWorkers() map[int]*PersistentWorker {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()

	// Return a copy to avoid race conditions
	workers := make(map[int]*PersistentWorker)
	for id, worker := range pwp.workers {
		workers[id] = worker
	}
	return workers
}

// GetWorkerStates returns the current state of all workers
func (pwp *PersistentWorkerPool) GetWorkerStates() map[int]WorkerState {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()

	states := make(map[int]WorkerState)
	for id, worker := range pwp.workers {
		states[id] = worker.GetState()
	}
	return states
}

// GetNumWorkers returns the number of workers in the pool
func (pwp *PersistentWorkerPool) GetNumWorkers() int {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()
	return pwp.numWorkers
}

// StopAll stops all workers in the pool
func (pwp *PersistentWorkerPool) StopAll() {
	pwp.mutex.Lock()
	defer pwp.mutex.Unlock()

	log.Printf("Stopping all %d persistent workers", len(pwp.workers))

	for id, worker := range pwp.workers {
		log.Printf("Stopping persistent worker %d", id)
		worker.Stop()
	}

	// Clear the workers map
	pwp.workers = make(map[int]*PersistentWorker)
	pwp.numWorkers = 0

	log.Printf("All persistent workers stopped")
}

// IsAllWorkersComplete checks if all workers have completed processing
func (pwp *PersistentWorkerPool) IsAllWorkersComplete() bool {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()

	for _, worker := range pwp.workers {
		state := worker.GetState()
		if state != StateComplete && state != StateError {
			return false
		}
	}
	return true
}

// GetHealthyWorkerCount returns the number of healthy (running) workers
func (pwp *PersistentWorkerPool) GetHealthyWorkerCount() int {
	pwp.mutex.RLock()
	defer pwp.mutex.RUnlock()

	count := 0
	for _, worker := range pwp.workers {
		if worker.IsRunning() && worker.GetState() != StateError {
			count++
		}
	}
	return count
}

// initiateGracefulShutdown initiates graceful shutdown of the worker
func (pw *PersistentWorker) initiateGracefulShutdown(reason string) {
	log.Printf("Worker %d initiating graceful shutdown: %s", pw.worker_id, reason)

	// Set state to complete if not already in error state
	currentState := pw.GetState()
	if currentState != StateError {
		pw.SetState(StateComplete)
	}

	// Give some time for any pending operations to complete
	time.Sleep(1 * time.Second)

	// Send completion signal if we haven't already
	if currentState == StateProcessing || currentState == StateSending {
		// Send final result end message
		resultEndMsg := messaging.CreateMessage(messaging.MsgTypeResultEnd, pw.worker_id, "", nil)
		resultEndMsg.Metadata["completion_reason"] = reason
		resultEndMsg.Metadata["final_state"] = pw.GetState().String()

		if err := pw.sendMessage(resultEndMsg); err != nil {
			log.Printf("Worker %d failed to send final result end message: %v", pw.worker_id, err)
		}
	}

	// Perform cleanup
	pw.cleanup()
}

// requestConnectionClose requests connection closure from the server
func (pw *PersistentWorker) requestConnectionClose(reason messaging.ConnectionCloseReason, message string) error {
	log.Printf("Worker %d requesting connection close: %s - %s", pw.worker_id, reason, message)

	closeMsg := messaging.CreateConnectionCloseMessage(pw.worker_id, reason, message)
	closeMsg.Metadata["initiated_by"] = "worker"

	return pw.sendMessageWithAck(closeMsg, 30*time.Second)
}

// GetScriptExecutor returns the script executor for this worker
func (pw *PersistentWorker) GetScriptExecutor() *ScriptExecutor {
	return pw.scriptExecutor
}

// GetResultCollector returns the result collector for this worker
func (pw *PersistentWorker) GetResultCollector() *ResultCollector {
	return pw.resultCollector
}

// GetFileTransmitter returns the file transmitter for this worker
func (pw *PersistentWorker) GetFileTransmitter() *FileTransmitter {
	return pw.fileTransmitter
}

// TriggerProcessing manually triggers the processing phase (for testing)
func (pw *PersistentWorker) TriggerProcessing() error {
	if pw.GetState() != StateReceiving && pw.GetState() != StateIdle {
		return fmt.Errorf("worker %d is not in a state to start processing (current state: %s)",
			pw.worker_id, pw.GetState())
	}

	log.Printf("Worker %d manually triggering processing", pw.worker_id)
	pw.maintainConnectionForProcessing()
	return nil
}

// GetProcessingStatus returns detailed processing status information
func (pw *PersistentWorker) GetProcessingStatus() map[string]interface{} {
	status := map[string]interface{}{
		"worker_id":  pw.worker_id,
		"state":      pw.GetState().String(),
		"is_running": pw.IsRunning(),
		"queue_size": pw.GetQueueSize(),
		"queue_full": pw.IsMessageQueueFull(),
	}

	if pw.scriptExecutor != nil {
		inputCount, outputCount, err := pw.scriptExecutor.GetFileCount()
		if err == nil {
			status["input_file_count"] = inputCount
			status["output_file_count"] = outputCount
		}
		status["script_executor_running"] = pw.scriptExecutor.IsRunning()
		status["input_directory"] = pw.scriptExecutor.GetInputDirectory()
		status["output_directory"] = pw.scriptExecutor.GetOutputDirectory()
		status["processing_timeout"] = pw.scriptExecutor.GetTimeout().String()
	}

	if pw.resultCollector != nil {
		collectorStats := pw.resultCollector.GetStatistics()
		for key, value := range collectorStats {
			status["result_collector_"+key] = value
		}
	}

	if pw.fileTransmitter != nil {
		transmitterStats := pw.fileTransmitter.GetTransmissionStatistics()
		for key, value := range transmitterStats {
			status["file_transmitter_"+key] = value
		}
	}

	return status
}
