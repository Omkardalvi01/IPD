package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// ServerResultCollector handles collection and storage of results from workers
type ServerResultCollector struct {
	expectedWorkers   int
	workerCompletions map[int]bool
	sharedResultsDir  string // Shared directory where all worker results are stored
	mutex             sync.RWMutex
	completionChan    chan int  // Channel to signal worker completion
	allCompleteChan   chan bool // Channel to signal all workers complete
	validationErrors  map[int][]string
	startTime         time.Time
	isComplete        bool
}

// NewServerResultCollector creates a new server result collector
func NewServerResultCollector(expectedWorkers int, sharedResultsDir string) *ServerResultCollector {
	// Ensure shared results directory exists
	if err := os.MkdirAll(sharedResultsDir, 0755); err != nil {
		log.Printf("Warning: Failed to create shared results directory %s: %v", sharedResultsDir, err)
	}

	return &ServerResultCollector{
		expectedWorkers:   expectedWorkers,
		workerCompletions: make(map[int]bool),
		sharedResultsDir:  sharedResultsDir,
		completionChan:    make(chan int, expectedWorkers),
		allCompleteChan:   make(chan bool, 1),
		validationErrors:  make(map[int][]string),
		startTime:         time.Now(),
		isComplete:        false,
	}
}

// HandleResultMessage processes incoming result messages from workers
func (src *ServerResultCollector) HandleResultMessage(workerID int, msg *messaging.Message) error {
	src.mutex.Lock()
	defer src.mutex.Unlock()

	if src.isComplete {
		return fmt.Errorf("collector already completed, ignoring message from worker %d", workerID)
	}

	log.Printf("Server received result message from worker %d, type: %s", workerID, msg.Type)

	switch msg.Type {
	case messaging.MsgTypeResult:
		return src.handleResultData(workerID, msg)
	case messaging.MsgTypeResultEnd:
		return src.handleResultCompletion(workerID, msg)
	case messaging.MsgTypeError:
		return src.handleWorkerError(workerID, msg)
	default:
		return fmt.Errorf("unexpected message type for result handling: %s", msg.Type)
	}
}

// handleResultData processes result data from a worker and stores it in shared folder
func (src *ServerResultCollector) handleResultData(workerID int, msg *messaging.Message) error {
	// Deserialize processing result from message data
	var processingResult messaging.ProcessingResult
	if err := json.Unmarshal(msg.Data, &processingResult); err != nil {
		validationErr := fmt.Sprintf("failed to deserialize processing result: %v", err)
		src.addValidationError(workerID, validationErr)
		return fmt.Errorf("%s", validationErr)
	}

	// Validate the processing result
	if err := src.validateProcessingResult(workerID, &processingResult); err != nil {
		src.addValidationError(workerID, err.Error())
		return err
	}

	// Store result to shared folder
	if err := src.storeResultToSharedFolder(workerID, &processingResult); err != nil {
		log.Printf("Warning: Failed to store result to shared folder for worker %d: %v", workerID, err)
		return err
	}

	log.Printf("Server stored result from worker %d: %d output files, status: %s",
		workerID, processingResult.OutputFileCount, processingResult.Status)

	return nil
}

// handleResultCompletion processes completion signal from a worker
func (src *ServerResultCollector) handleResultCompletion(workerID int, msg *messaging.Message) error {
	if src.workerCompletions[workerID] {
		return fmt.Errorf("worker %d already marked as complete", workerID)
	}

	src.workerCompletions[workerID] = true

	log.Printf("Server marked worker %d as complete (%d/%d workers completed)",
		workerID, len(src.workerCompletions), src.expectedWorkers)

	// Signal completion
	select {
	case src.completionChan <- workerID:
	default:
		// Channel full, but that's okay
	}

	// Check if all workers are complete
	if len(src.workerCompletions) >= src.expectedWorkers {
		src.finalizeCollection()
		select {
		case src.allCompleteChan <- true:
		default:
			// Channel already signaled
		}
	}

	return nil
}

// handleWorkerError processes error messages from workers
func (src *ServerResultCollector) handleWorkerError(workerID int, msg *messaging.Message) error {
	errorMsg := string(msg.Data)
	src.addValidationError(workerID, fmt.Sprintf("Worker error: %s", errorMsg))

	// Mark worker as complete with error
	src.workerCompletions[workerID] = true

	// Store error information to shared folder
	if err := src.storeWorkerError(workerID, errorMsg); err != nil {
		log.Printf("Warning: Failed to store worker error for worker %d: %v", workerID, err)
	}

	log.Printf("Server received error from worker %d: %s", workerID, errorMsg)

	// Check completion
	if len(src.workerCompletions) >= src.expectedWorkers {
		src.finalizeCollection()
		select {
		case src.allCompleteChan <- true:
		default:
		}
	}

	return nil
}

// validateProcessingResult validates the integrity and content of a processing result
func (src *ServerResultCollector) validateProcessingResult(workerID int, result *messaging.ProcessingResult) error {
	var errors []string

	// Validate worker ID matches
	if result.WorkerID != workerID {
		errors = append(errors, fmt.Sprintf("worker ID mismatch: expected %d, got %d", workerID, result.WorkerID))
	}

	// Validate status
	if result.Status < messaging.StatusSuccess || result.Status > messaging.StatusPartialSuccess {
		errors = append(errors, fmt.Sprintf("invalid processing status: %d", result.Status))
	}

	// Validate file counts
	if result.OutputFileCount < 0 {
		errors = append(errors, "negative output file count")
	}

	if result.InputFileCount < 0 {
		errors = append(errors, "negative input file count")
	}

	if len(result.OutputFiles) != result.OutputFileCount {
		errors = append(errors, fmt.Sprintf("output file count mismatch: declared %d, actual %d",
			result.OutputFileCount, len(result.OutputFiles)))
	}

	// Validate processing time
	if result.ProcessingTime < 0 {
		errors = append(errors, "negative processing time")
	}

	// Validate output files
	for i, file := range result.OutputFiles {
		if file.Filename == "" {
			errors = append(errors, fmt.Sprintf("empty filename for output file %d", i))
		}

		if file.Size < 0 {
			errors = append(errors, fmt.Sprintf("negative file size for %s", file.Filename))
		}

		if file.Checksum == "" {
			errors = append(errors, fmt.Sprintf("missing checksum for %s", file.Filename))
		}

		// Validate checksum if data is present
		if len(file.Data) > 0 {
			expectedChecksum := fmt.Sprintf("%x", sha256.Sum256(file.Data))
			if file.Checksum != expectedChecksum {
				errors = append(errors, fmt.Sprintf("checksum mismatch for %s: expected %s, got %s",
					file.Filename, expectedChecksum, file.Checksum))
			}

			// Validate data size matches declared size
			if int64(len(file.Data)) != file.Size {
				errors = append(errors, fmt.Sprintf("data size mismatch for %s: declared %d, actual %d",
					file.Filename, file.Size, len(file.Data)))
			}
		}
	}

	if len(errors) > 0 {
		for _, err := range errors {
			src.addValidationError(workerID, err)
		}
		return fmt.Errorf("validation failed for worker %d: %d errors", workerID, len(errors))
	}

	return nil
}

// addValidationError adds a validation error for a worker
func (src *ServerResultCollector) addValidationError(workerID int, error string) {
	if src.validationErrors[workerID] == nil {
		src.validationErrors[workerID] = make([]string, 0)
	}
	src.validationErrors[workerID] = append(src.validationErrors[workerID], error)
	log.Printf("Validation error for worker %d: %s", workerID, error)
}

// storeResultToSharedFolder stores a processing result to the shared folder
func (src *ServerResultCollector) storeResultToSharedFolder(workerID int, result *messaging.ProcessingResult) error {
	// Create worker-specific directory in shared folder
	workerDir := filepath.Join(src.sharedResultsDir, fmt.Sprintf("worker_%d", workerID))
	if err := os.MkdirAll(workerDir, 0755); err != nil {
		return fmt.Errorf("failed to create worker directory: %w", err)
	}

	// Store result metadata
	metadataFile := filepath.Join(workerDir, "result_metadata.json")
	metadataData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result metadata: %w", err)
	}

	if err := os.WriteFile(metadataFile, metadataData, 0644); err != nil {
		return fmt.Errorf("failed to write result metadata: %w", err)
	}

	// Store output files in worker directory
	for _, file := range result.OutputFiles {
		if len(file.Data) > 0 {
			filePath := filepath.Join(workerDir, file.Filename)
			if err := os.WriteFile(filePath, file.Data, 0644); err != nil {
				log.Printf("Warning: Failed to write output file %s for worker %d: %v",
					file.Filename, workerID, err)
			} else {
				log.Printf("Stored file %s for worker %d in shared folder", file.Filename, workerID)
			}
		}
	}

	// Store completion timestamp
	timestampFile := filepath.Join(workerDir, "completion_time.txt")
	timestamp := time.Now().Format(time.RFC3339)
	if err := os.WriteFile(timestampFile, []byte(timestamp), 0644); err != nil {
		log.Printf("Warning: Failed to write completion timestamp for worker %d: %v", workerID, err)
	}

	log.Printf("Stored results for worker %d in shared folder: %s", workerID, workerDir)
	return nil
}

// storeWorkerError stores error information for a worker
func (src *ServerResultCollector) storeWorkerError(workerID int, errorMsg string) error {
	// Create worker-specific directory in shared folder
	workerDir := filepath.Join(src.sharedResultsDir, fmt.Sprintf("worker_%d", workerID))
	if err := os.MkdirAll(workerDir, 0755); err != nil {
		return fmt.Errorf("failed to create worker directory: %w", err)
	}

	// Store error information
	errorFile := filepath.Join(workerDir, "error.txt")
	errorData := fmt.Sprintf("Error occurred at: %s\nError message: %s\n",
		time.Now().Format(time.RFC3339), errorMsg)

	if err := os.WriteFile(errorFile, []byte(errorData), 0644); err != nil {
		return fmt.Errorf("failed to write error file: %w", err)
	}

	log.Printf("Stored error information for worker %d in shared folder", workerID)
	return nil
}

// finalizeCollection completes the collection process
func (src *ServerResultCollector) finalizeCollection() {
	if src.isComplete {
		return
	}

	src.isComplete = true
	completionTime := time.Now()
	totalDuration := completionTime.Sub(src.startTime)

	// Create summary file in shared folder
	summaryFile := filepath.Join(src.sharedResultsDir, "collection_summary.json")

	summary := map[string]interface{}{
		"total_workers":      src.expectedWorkers,
		"completed_workers":  len(src.workerCompletions),
		"start_time":         src.startTime.Format(time.RFC3339),
		"completion_time":    completionTime.Format(time.RFC3339),
		"total_duration":     totalDuration.String(),
		"validation_errors":  len(src.validationErrors),
		"worker_directories": src.getWorkerDirectories(),
	}

	summaryData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Printf("Warning: Failed to marshal collection summary: %v", err)
	} else {
		if err := os.WriteFile(summaryFile, summaryData, 0644); err != nil {
			log.Printf("Warning: Failed to write collection summary: %v", err)
		} else {
			log.Printf("Collection summary stored: %s", summaryFile)
		}
	}

	log.Printf("Result collection completed: %d/%d workers completed in %v",
		len(src.workerCompletions), src.expectedWorkers, totalDuration)
}

// getWorkerDirectories returns a list of worker directories in the shared folder
func (src *ServerResultCollector) getWorkerDirectories() []string {
	var directories []string

	entries, err := os.ReadDir(src.sharedResultsDir)
	if err != nil {
		log.Printf("Warning: Failed to read shared results directory: %v", err)
		return directories
	}

	for _, entry := range entries {
		if entry.IsDir() && len(entry.Name()) > 7 && entry.Name()[:7] == "worker_" {
			directories = append(directories, entry.Name())
		}
	}

	return directories
}

// GetCompletionChannel returns the channel that signals worker completions
func (src *ServerResultCollector) GetCompletionChannel() <-chan int {
	return src.completionChan
}

// GetAllCompleteChannel returns the channel that signals when all workers are complete
func (src *ServerResultCollector) GetAllCompleteChannel() <-chan bool {
	return src.allCompleteChan
}

// IsComplete returns whether the collection is complete
func (src *ServerResultCollector) IsComplete() bool {
	src.mutex.RLock()
	defer src.mutex.RUnlock()
	return src.isComplete
}

// GetCompletedWorkerCount returns the number of completed workers
func (src *ServerResultCollector) GetCompletedWorkerCount() int {
	src.mutex.RLock()
	defer src.mutex.RUnlock()
	return len(src.workerCompletions)
}

// GetExpectedWorkerCount returns the expected number of workers
func (src *ServerResultCollector) GetExpectedWorkerCount() int {
	src.mutex.RLock()
	defer src.mutex.RUnlock()
	return src.expectedWorkers
}

// GetValidationErrors returns validation errors for all workers
func (src *ServerResultCollector) GetValidationErrors() map[int][]string {
	src.mutex.RLock()
	defer src.mutex.RUnlock()

	// Return a copy
	errors := make(map[int][]string)
	for workerID, workerErrors := range src.validationErrors {
		errors[workerID] = make([]string, len(workerErrors))
		copy(errors[workerID], workerErrors)
	}

	return errors
}

// GetSharedResultsDirectory returns the path to the shared results directory
func (src *ServerResultCollector) GetSharedResultsDirectory() string {
	return src.sharedResultsDir
}

// Reset resets the collector for a new batch of workers
func (src *ServerResultCollector) Reset(expectedWorkers int) {
	src.mutex.Lock()
	defer src.mutex.Unlock()

	src.expectedWorkers = expectedWorkers
	src.workerCompletions = make(map[int]bool)
	src.validationErrors = make(map[int][]string)
	src.startTime = time.Now()
	src.isComplete = false

	// Drain channels
	select {
	case <-src.completionChan:
	default:
	}

	select {
	case <-src.allCompleteChan:
	default:
	}

	log.Printf("Server result collector reset for %d workers", expectedWorkers)
}
