package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeConnection    ErrorType = "CONNECTION"
	ErrorTypeProcessing    ErrorType = "PROCESSING"
	ErrorTypeTransmission  ErrorType = "TRANSMISSION"
	ErrorTypeValidation    ErrorType = "VALIDATION"
	ErrorTypeTimeout       ErrorType = "TIMEOUT"
	ErrorTypeConfiguration ErrorType = "CONFIGURATION"
	ErrorTypeSystem        ErrorType = "SYSTEM"
)

// ErrorSeverity represents the severity level of an error
type ErrorSeverity string

const (
	SeverityLow      ErrorSeverity = "LOW"
	SeverityMedium   ErrorSeverity = "MEDIUM"
	SeverityHigh     ErrorSeverity = "HIGH"
	SeverityCritical ErrorSeverity = "CRITICAL"
)

// ProcessingError represents a comprehensive error with context
type ProcessingError struct {
	Type        ErrorType              `json:"type"`
	Severity    ErrorSeverity          `json:"severity"`
	WorkerID    int                    `json:"worker_id"`
	Operation   string                 `json:"operation"`
	Message     string                 `json:"message"`
	Cause       error                  `json:"cause,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Recoverable bool                   `json:"recoverable"`
	RetryCount  int                    `json:"retry_count"`
}

// Error implements the error interface
func (pe *ProcessingError) Error() string {
	if pe.Cause != nil {
		return fmt.Sprintf("[%s:%s] Worker %d - %s: %s (caused by: %v)",
			pe.Type, pe.Severity, pe.WorkerID, pe.Operation, pe.Message, pe.Cause)
	}
	return fmt.Sprintf("[%s:%s] Worker %d - %s: %s",
		pe.Type, pe.Severity, pe.WorkerID, pe.Operation, pe.Message)
}

// Unwrap returns the underlying cause
func (pe *ProcessingError) Unwrap() error {
	return pe.Cause
}

// NewProcessingError creates a new ProcessingError
func NewProcessingError(errorType ErrorType, severity ErrorSeverity, workerID int, operation, message string, cause error) *ProcessingError {
	return &ProcessingError{
		Type:        errorType,
		Severity:    severity,
		WorkerID:    workerID,
		Operation:   operation,
		Message:     message,
		Cause:       cause,
		Timestamp:   time.Now(),
		Context:     make(map[string]interface{}),
		Recoverable: determineRecoverability(errorType, severity),
		RetryCount:  0,
	}
}

// AddContext adds contextual information to the error
func (pe *ProcessingError) AddContext(key string, value interface{}) {
	if pe.Context == nil {
		pe.Context = make(map[string]interface{})
	}
	pe.Context[key] = value
}

// IncrementRetryCount increments the retry count
func (pe *ProcessingError) IncrementRetryCount() {
	pe.RetryCount++
}

// determineRecoverability determines if an error is recoverable based on type and severity
func determineRecoverability(errorType ErrorType, severity ErrorSeverity) bool {
	switch severity {
	case SeverityCritical:
		return false
	case SeverityHigh:
		return errorType == ErrorTypeConnection || errorType == ErrorTypeTransmission
	case SeverityMedium, SeverityLow:
		return true
	default:
		return false
	}
}

// ErrorHandler handles error processing and recovery
type ErrorHandler struct {
	retryConfig     *RetryConfig
	errorLog        []ProcessingError
	errorMutex      sync.RWMutex
	recoveryActions map[ErrorType]RecoveryAction
	errorCallbacks  map[ErrorSeverity][]ErrorCallback
	callbackMutex   sync.RWMutex
}

// ErrorCallback defines a callback function for error handling
type ErrorCallback func(error *ProcessingError)

// RecoveryAction defines an action to take for error recovery
type RecoveryAction func(error *ProcessingError) error

// NewErrorHandler creates a new error handler
func NewErrorHandler(retryConfig *RetryConfig) *ErrorHandler {
	eh := &ErrorHandler{
		retryConfig:     retryConfig,
		errorLog:        make([]ProcessingError, 0),
		recoveryActions: make(map[ErrorType]RecoveryAction),
		errorCallbacks:  make(map[ErrorSeverity][]ErrorCallback),
	}

	// Register default recovery actions
	eh.registerDefaultRecoveryActions()

	return eh
}

// HandleError processes an error and attempts recovery
func (eh *ErrorHandler) HandleError(err *ProcessingError) error {
	eh.logError(err)

	// Execute callbacks for this severity level
	eh.executeCallbacks(err)

	// Attempt recovery if the error is recoverable
	if err.Recoverable {
		if recoveryAction, exists := eh.recoveryActions[err.Type]; exists {
			log.Printf("Attempting recovery for error: %s", err.Error())
			if recoveryErr := recoveryAction(err); recoveryErr != nil {
				log.Printf("Recovery failed for error %s: %v", err.Error(), recoveryErr)
				return recoveryErr
			}
			log.Printf("Recovery successful for error: %s", err.Error())
		}
	}

	return nil
}

// logError adds an error to the error log
func (eh *ErrorHandler) logError(err *ProcessingError) {
	eh.errorMutex.Lock()
	defer eh.errorMutex.Unlock()

	eh.errorLog = append(eh.errorLog, *err)

	// Log to standard logger with appropriate level
	switch err.Severity {
	case SeverityCritical:
		log.Printf("CRITICAL ERROR: %s", err.Error())
	case SeverityHigh:
		log.Printf("HIGH ERROR: %s", err.Error())
	case SeverityMedium:
		log.Printf("MEDIUM ERROR: %s", err.Error())
	case SeverityLow:
		log.Printf("LOW ERROR: %s", err.Error())
	}
}

// executeCallbacks executes registered callbacks for an error severity
func (eh *ErrorHandler) executeCallbacks(err *ProcessingError) {
	eh.callbackMutex.RLock()
	callbacks, exists := eh.errorCallbacks[err.Severity]
	eh.callbackMutex.RUnlock()

	if exists {
		for _, callback := range callbacks {
			go func(cb ErrorCallback) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Error callback panicked: %v", r)
					}
				}()
				cb(err)
			}(callback)
		}
	}
}

// RegisterRecoveryAction registers a recovery action for an error type
func (eh *ErrorHandler) RegisterRecoveryAction(errorType ErrorType, action RecoveryAction) {
	eh.recoveryActions[errorType] = action
}

// RegisterErrorCallback registers a callback for an error severity
func (eh *ErrorHandler) RegisterErrorCallback(severity ErrorSeverity, callback ErrorCallback) {
	eh.callbackMutex.Lock()
	defer eh.callbackMutex.Unlock()

	if eh.errorCallbacks[severity] == nil {
		eh.errorCallbacks[severity] = make([]ErrorCallback, 0)
	}
	eh.errorCallbacks[severity] = append(eh.errorCallbacks[severity], callback)
}

// registerDefaultRecoveryActions registers default recovery actions
func (eh *ErrorHandler) registerDefaultRecoveryActions() {
	// Connection error recovery
	eh.RegisterRecoveryAction(ErrorTypeConnection, func(err *ProcessingError) error {
		log.Printf("Attempting connection recovery for worker %d", err.WorkerID)
		// Connection recovery would be handled by the connection manager
		return nil
	})

	// Processing error recovery
	eh.RegisterRecoveryAction(ErrorTypeProcessing, func(err *ProcessingError) error {
		log.Printf("Attempting processing recovery for worker %d", err.WorkerID)
		// Processing recovery might involve restarting the script executor
		return nil
	})

	// Transmission error recovery
	eh.RegisterRecoveryAction(ErrorTypeTransmission, func(err *ProcessingError) error {
		log.Printf("Attempting transmission recovery for worker %d", err.WorkerID)
		// Transmission recovery might involve retrying file transmission
		return nil
	})
}

// GetErrorLog returns a copy of the error log
func (eh *ErrorHandler) GetErrorLog() []ProcessingError {
	eh.errorMutex.RLock()
	defer eh.errorMutex.RUnlock()

	logCopy := make([]ProcessingError, len(eh.errorLog))
	copy(logCopy, eh.errorLog)
	return logCopy
}

// GetErrorsByType returns errors of a specific type
func (eh *ErrorHandler) GetErrorsByType(errorType ErrorType) []ProcessingError {
	eh.errorMutex.RLock()
	defer eh.errorMutex.RUnlock()

	var errors []ProcessingError
	for _, err := range eh.errorLog {
		if err.Type == errorType {
			errors = append(errors, err)
		}
	}
	return errors
}

// GetErrorsBySeverity returns errors of a specific severity
func (eh *ErrorHandler) GetErrorsBySeverity(severity ErrorSeverity) []ProcessingError {
	eh.errorMutex.RLock()
	defer eh.errorMutex.RUnlock()

	var errors []ProcessingError
	for _, err := range eh.errorLog {
		if err.Severity == severity {
			errors = append(errors, err)
		}
	}
	return errors
}

// GetErrorsByWorker returns errors for a specific worker
func (eh *ErrorHandler) GetErrorsByWorker(workerID int) []ProcessingError {
	eh.errorMutex.RLock()
	defer eh.errorMutex.RUnlock()

	var errors []ProcessingError
	for _, err := range eh.errorLog {
		if err.WorkerID == workerID {
			errors = append(errors, err)
		}
	}
	return errors
}

// ClearErrorLog clears the error log
func (eh *ErrorHandler) ClearErrorLog() {
	eh.errorMutex.Lock()
	defer eh.errorMutex.Unlock()

	eh.errorLog = make([]ProcessingError, 0)
	log.Printf("Error log cleared")
}

// GetErrorStatistics returns statistics about errors
func (eh *ErrorHandler) GetErrorStatistics() map[string]interface{} {
	eh.errorMutex.RLock()
	defer eh.errorMutex.RUnlock()

	stats := map[string]interface{}{
		"total_errors": len(eh.errorLog),
		"by_type":      make(map[ErrorType]int),
		"by_severity":  make(map[ErrorSeverity]int),
		"by_worker":    make(map[int]int),
		"recoverable":  0,
	}

	for _, err := range eh.errorLog {
		stats["by_type"].(map[ErrorType]int)[err.Type]++
		stats["by_severity"].(map[ErrorSeverity]int)[err.Severity]++
		stats["by_worker"].(map[int]int)[err.WorkerID]++
		if err.Recoverable {
			stats["recoverable"] = stats["recoverable"].(int) + 1
		}
	}

	return stats
}

// PropagateErrorToServer sends error information to the main server
func PropagateErrorToServer(workerID int, err *ProcessingError, sendFunc func(*messaging.Message) error) error {
	// Create error message
	errorMsg := messaging.CreateMessage(messaging.MsgTypeError, workerID, "", []byte(err.Error()))

	// Add error details to metadata
	errorMsg.Metadata["error_type"] = string(err.Type)
	errorMsg.Metadata["error_severity"] = string(err.Severity)
	errorMsg.Metadata["operation"] = err.Operation
	errorMsg.Metadata["recoverable"] = err.Recoverable
	errorMsg.Metadata["retry_count"] = err.RetryCount
	errorMsg.Metadata["timestamp"] = err.Timestamp.Format(time.RFC3339)

	// Add context if available
	if err.Context != nil {
		for key, value := range err.Context {
			errorMsg.Metadata[fmt.Sprintf("context_%s", key)] = value
		}
	}

	// Send the error message
	if sendErr := sendFunc(errorMsg); sendErr != nil {
		return fmt.Errorf("failed to propagate error to server: %w", sendErr)
	}

	log.Printf("Error propagated to server for worker %d: %s", workerID, err.Error())
	return nil
}

// CreateConnectionError creates a connection-related error
func CreateConnectionError(workerID int, operation, message string, cause error) *ProcessingError {
	severity := SeverityHigh
	if cause != nil {
		// Determine severity based on the underlying error
		switch cause.Error() {
		case "connection refused", "network unreachable":
			severity = SeverityCritical
		case "timeout":
			severity = SeverityMedium
		}
	}

	return NewProcessingError(ErrorTypeConnection, severity, workerID, operation, message, cause)
}

// CreateProcessingError creates a processing-related error
func CreateProcessingError(workerID int, operation, message string, cause error) *ProcessingError {
	severity := SeverityMedium
	if cause != nil {
		// Determine severity based on the underlying error
		errorStr := cause.Error()
		if contains(errorStr, "timeout") {
			severity = SeverityHigh
		} else if contains(errorStr, "permission denied") || contains(errorStr, "no such file") {
			severity = SeverityHigh
		}
	}

	return NewProcessingError(ErrorTypeProcessing, severity, workerID, operation, message, cause)
}

// CreateTransmissionError creates a transmission-related error
func CreateTransmissionError(workerID int, operation, message string, cause error) *ProcessingError {
	severity := SeverityMedium
	if cause != nil {
		// Determine severity based on the underlying error
		errorStr := cause.Error()
		if contains(errorStr, "connection") {
			severity = SeverityHigh
		}
	}

	return NewProcessingError(ErrorTypeTransmission, severity, workerID, operation, message, cause)
}

// CreateValidationError creates a validation-related error
func CreateValidationError(workerID int, operation, message string, cause error) *ProcessingError {
	return NewProcessingError(ErrorTypeValidation, SeverityMedium, workerID, operation, message, cause)
}

// CreateTimeoutError creates a timeout-related error
func CreateTimeoutError(workerID int, operation, message string, timeout time.Duration) *ProcessingError {
	err := NewProcessingError(ErrorTypeTimeout, SeverityHigh, workerID, operation, message, nil)
	err.AddContext("timeout_duration", timeout.String())
	return err
}

// CreateSystemError creates a system-related error
func CreateSystemError(workerID int, operation, message string, cause error) *ProcessingError {
	severity := SeverityCritical
	if cause != nil {
		// System errors are generally critical
		errorStr := cause.Error()
		if contains(errorStr, "disk space") || contains(errorStr, "memory") {
			severity = SeverityCritical
		}
	}

	return NewProcessingError(ErrorTypeSystem, severity, workerID, operation, message, cause)
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ErrorAggregator aggregates errors from multiple workers
type ErrorAggregator struct {
	errors    map[int][]ProcessingError
	mutex     sync.RWMutex
	threshold int // Threshold for triggering alerts
	alertFunc func(workerID int, errors []ProcessingError)
}

// NewErrorAggregator creates a new error aggregator
func NewErrorAggregator(threshold int, alertFunc func(int, []ProcessingError)) *ErrorAggregator {
	return &ErrorAggregator{
		errors:    make(map[int][]ProcessingError),
		threshold: threshold,
		alertFunc: alertFunc,
	}
}

// AddError adds an error to the aggregator
func (ea *ErrorAggregator) AddError(err *ProcessingError) {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()

	if ea.errors[err.WorkerID] == nil {
		ea.errors[err.WorkerID] = make([]ProcessingError, 0)
	}
	ea.errors[err.WorkerID] = append(ea.errors[err.WorkerID], *err)

	// Check if threshold is exceeded
	if len(ea.errors[err.WorkerID]) >= ea.threshold && ea.alertFunc != nil {
		go ea.alertFunc(err.WorkerID, ea.errors[err.WorkerID])
	}
}

// GetErrors returns errors for a specific worker
func (ea *ErrorAggregator) GetErrors(workerID int) []ProcessingError {
	ea.mutex.RLock()
	defer ea.mutex.RUnlock()

	if errors, exists := ea.errors[workerID]; exists {
		result := make([]ProcessingError, len(errors))
		copy(result, errors)
		return result
	}
	return nil
}

// GetAllErrors returns all aggregated errors
func (ea *ErrorAggregator) GetAllErrors() map[int][]ProcessingError {
	ea.mutex.RLock()
	defer ea.mutex.RUnlock()

	result := make(map[int][]ProcessingError)
	for workerID, errors := range ea.errors {
		result[workerID] = make([]ProcessingError, len(errors))
		copy(result[workerID], errors)
	}
	return result
}

// ClearErrors clears errors for a specific worker
func (ea *ErrorAggregator) ClearErrors(workerID int) {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()

	delete(ea.errors, workerID)
}

// ClearAllErrors clears all errors
func (ea *ErrorAggregator) ClearAllErrors() {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()

	ea.errors = make(map[int][]ProcessingError)
}
