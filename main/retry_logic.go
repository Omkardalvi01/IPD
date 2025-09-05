package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RetryStrategy defines different retry strategies
type RetryStrategy string

const (
	RetryStrategyFixed       RetryStrategy = "FIXED"
	RetryStrategyExponential RetryStrategy = "EXPONENTIAL"
	RetryStrategyLinear      RetryStrategy = "LINEAR"
	RetryStrategyCustom      RetryStrategy = "CUSTOM"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	Strategy        RetryStrategy `json:"strategy"`
	Multiplier      float64       `json:"multiplier"`       // For exponential backoff
	Jitter          bool          `json:"jitter"`           // Add randomness to delays
	JitterRange     float64       `json:"jitter_range"`     // Jitter range (0.0 to 1.0)
	RetryableErrors []ErrorType   `json:"retryable_errors"` // Which error types to retry
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Strategy:     RetryStrategyExponential,
		Multiplier:   2.0,
		Jitter:       true,
		JitterRange:  0.1,
		RetryableErrors: []ErrorType{
			ErrorTypeConnection,
			ErrorTypeTransmission,
			ErrorTypeTimeout,
		},
	}
}

// Validate validates the retry configuration
func (rc *RetryConfig) Validate() error {
	if rc.MaxAttempts < 1 {
		return fmt.Errorf("max attempts must be at least 1")
	}
	if rc.InitialDelay <= 0 {
		return fmt.Errorf("initial delay must be positive")
	}
	if rc.MaxDelay <= 0 {
		return fmt.Errorf("max delay must be positive")
	}
	if rc.MaxDelay < rc.InitialDelay {
		return fmt.Errorf("max delay must be greater than or equal to initial delay")
	}
	if rc.Multiplier <= 1.0 && rc.Strategy == RetryStrategyExponential {
		return fmt.Errorf("multiplier must be greater than 1.0 for exponential strategy")
	}
	if rc.JitterRange < 0.0 || rc.JitterRange > 1.0 {
		return fmt.Errorf("jitter range must be between 0.0 and 1.0")
	}
	return nil
}

// RetryableOperation defines an operation that can be retried
type RetryableOperation func(ctx context.Context, attempt int) error

// RetryResult contains the result of a retry operation
type RetryResult struct {
	Success      bool            `json:"success"`
	Attempts     int             `json:"attempts"`
	TotalTime    time.Duration   `json:"total_time"`
	LastError    error           `json:"last_error,omitempty"`
	AttemptTimes []time.Duration `json:"attempt_times"`
}

// RetryManager manages retry operations
type RetryManager struct {
	config      *RetryConfig
	operations  map[string]*RetryOperation
	mutex       sync.RWMutex
	metrics     *RetryMetrics
	customDelay func(attempt int, baseDelay time.Duration) time.Duration
}

// RetryOperation tracks an ongoing retry operation
type RetryOperation struct {
	ID          string
	WorkerID    int
	Operation   string
	StartTime   time.Time
	Attempts    int
	LastAttempt time.Time
	LastError   error
	Context     context.Context
	Cancel      context.CancelFunc
}

// RetryMetrics tracks retry statistics
type RetryMetrics struct {
	TotalOperations   int64         `json:"total_operations"`
	SuccessfulRetries int64         `json:"successful_retries"`
	FailedRetries     int64         `json:"failed_retries"`
	AverageAttempts   float64       `json:"average_attempts"`
	AverageRetryTime  time.Duration `json:"average_retry_time"`
	mutex             sync.RWMutex
}

// NewRetryManager creates a new retry manager
func NewRetryManager(config *RetryConfig) (*RetryManager, error) {
	if config == nil {
		config = DefaultRetryConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid retry configuration: %w", err)
	}

	return &RetryManager{
		config:     config,
		operations: make(map[string]*RetryOperation),
		metrics:    &RetryMetrics{},
	}, nil
}

// SetCustomDelayFunction sets a custom delay calculation function
func (rm *RetryManager) SetCustomDelayFunction(delayFunc func(int, time.Duration) time.Duration) {
	rm.customDelay = delayFunc
}

// ExecuteWithRetry executes an operation with retry logic
func (rm *RetryManager) ExecuteWithRetry(ctx context.Context, operationID string, workerID int, operation RetryableOperation) *RetryResult {
	startTime := time.Now()
	result := &RetryResult{
		Success:      false,
		Attempts:     0,
		AttemptTimes: make([]time.Duration, 0),
	}

	// Create retry operation tracking
	retryCtx, cancel := context.WithCancel(ctx)
	retryOp := &RetryOperation{
		ID:        operationID,
		WorkerID:  workerID,
		Operation: operationID,
		StartTime: startTime,
		Context:   retryCtx,
		Cancel:    cancel,
	}

	rm.registerOperation(retryOp)
	defer rm.unregisterOperation(operationID)
	defer cancel()

	log.Printf("Starting retry operation %s for worker %d", operationID, workerID)

	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		attemptStart := time.Now()
		result.Attempts = attempt
		retryOp.Attempts = attempt
		retryOp.LastAttempt = attemptStart

		// Check if context is cancelled
		select {
		case <-retryCtx.Done():
			result.LastError = retryCtx.Err()
			log.Printf("Retry operation %s cancelled after %d attempts", operationID, attempt-1)
			return result
		default:
		}

		// Execute the operation
		err := operation(retryCtx, attempt)
		attemptDuration := time.Since(attemptStart)
		result.AttemptTimes = append(result.AttemptTimes, attemptDuration)

		if err == nil {
			// Success
			result.Success = true
			result.TotalTime = time.Since(startTime)
			rm.updateMetrics(true, attempt, result.TotalTime)
			log.Printf("Retry operation %s succeeded on attempt %d", operationID, attempt)
			return result
		}

		// Operation failed
		result.LastError = err
		retryOp.LastError = err

		log.Printf("Retry operation %s failed on attempt %d: %v", operationID, attempt, err)

		// Check if error is retryable
		if !rm.isRetryableError(err) {
			log.Printf("Error is not retryable, stopping retry operation %s", operationID)
			break
		}

		// Don't wait after the last attempt
		if attempt < rm.config.MaxAttempts {
			delay := rm.calculateDelay(attempt)
			log.Printf("Waiting %v before retry attempt %d for operation %s", delay, attempt+1, operationID)

			select {
			case <-retryCtx.Done():
				result.LastError = retryCtx.Err()
				log.Printf("Retry operation %s cancelled during delay", operationID)
				return result
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	// All attempts failed
	result.TotalTime = time.Since(startTime)
	rm.updateMetrics(false, result.Attempts, result.TotalTime)
	log.Printf("Retry operation %s failed after %d attempts", operationID, result.Attempts)
	return result
}

// calculateDelay calculates the delay for the next retry attempt
func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch rm.config.Strategy {
	case RetryStrategyFixed:
		delay = rm.config.InitialDelay

	case RetryStrategyLinear:
		delay = time.Duration(attempt) * rm.config.InitialDelay

	case RetryStrategyExponential:
		delay = time.Duration(float64(rm.config.InitialDelay) * math.Pow(rm.config.Multiplier, float64(attempt-1)))

	case RetryStrategyCustom:
		if rm.customDelay != nil {
			delay = rm.customDelay(attempt, rm.config.InitialDelay)
		} else {
			delay = rm.config.InitialDelay
		}

	default:
		delay = rm.config.InitialDelay
	}

	// Apply maximum delay limit
	if delay > rm.config.MaxDelay {
		delay = rm.config.MaxDelay
	}

	// Apply jitter if enabled
	if rm.config.Jitter {
		jitterAmount := float64(delay) * rm.config.JitterRange
		jitter := (rand.Float64() - 0.5) * 2 * jitterAmount // Random value between -jitterAmount and +jitterAmount
		delay = time.Duration(float64(delay) + jitter)

		// Ensure delay is not negative
		if delay < 0 {
			delay = time.Millisecond
		}
	}

	return delay
}

// isRetryableError checks if an error is retryable based on configuration
func (rm *RetryManager) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a ProcessingError
	if procErr, ok := err.(*ProcessingError); ok {
		for _, retryableType := range rm.config.RetryableErrors {
			if procErr.Type == retryableType {
				return procErr.Recoverable
			}
		}
		return false
	}

	// For non-ProcessingError types, check error message patterns
	errorMsg := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"network unreachable",
		"connection reset",
		"broken pipe",
	}

	for _, pattern := range retryablePatterns {
		if contains(errorMsg, pattern) {
			return true
		}
	}

	return false
}

// registerOperation registers a retry operation for tracking
func (rm *RetryManager) registerOperation(op *RetryOperation) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	rm.operations[op.ID] = op
}

// unregisterOperation removes a retry operation from tracking
func (rm *RetryManager) unregisterOperation(operationID string) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	delete(rm.operations, operationID)
}

// GetActiveOperations returns currently active retry operations
func (rm *RetryManager) GetActiveOperations() map[string]*RetryOperation {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RetryOperation)
	for id, op := range rm.operations {
		// Create a copy to avoid race conditions
		opCopy := *op
		result[id] = &opCopy
	}
	return result
}

// CancelOperation cancels a specific retry operation
func (rm *RetryManager) CancelOperation(operationID string) bool {
	rm.mutex.RLock()
	op, exists := rm.operations[operationID]
	rm.mutex.RUnlock()

	if exists && op.Cancel != nil {
		op.Cancel()
		log.Printf("Cancelled retry operation %s", operationID)
		return true
	}
	return false
}

// CancelAllOperations cancels all active retry operations
func (rm *RetryManager) CancelAllOperations() int {
	rm.mutex.RLock()
	operations := make([]*RetryOperation, 0, len(rm.operations))
	for _, op := range rm.operations {
		operations = append(operations, op)
	}
	rm.mutex.RUnlock()

	cancelled := 0
	for _, op := range operations {
		if op.Cancel != nil {
			op.Cancel()
			cancelled++
		}
	}

	log.Printf("Cancelled %d retry operations", cancelled)
	return cancelled
}

// updateMetrics updates retry metrics
func (rm *RetryManager) updateMetrics(success bool, attempts int, totalTime time.Duration) {
	rm.metrics.mutex.Lock()
	defer rm.metrics.mutex.Unlock()

	rm.metrics.TotalOperations++
	if success {
		rm.metrics.SuccessfulRetries++
	} else {
		rm.metrics.FailedRetries++
	}

	// Update average attempts
	totalAttempts := rm.metrics.AverageAttempts*float64(rm.metrics.TotalOperations-1) + float64(attempts)
	rm.metrics.AverageAttempts = totalAttempts / float64(rm.metrics.TotalOperations)

	// Update average retry time
	totalTime64 := int64(rm.metrics.AverageRetryTime)*int64(rm.metrics.TotalOperations-1) + int64(totalTime)
	rm.metrics.AverageRetryTime = time.Duration(totalTime64 / int64(rm.metrics.TotalOperations))
}

// GetMetrics returns current retry metrics
func (rm *RetryManager) GetMetrics() RetryMetrics {
	rm.metrics.mutex.RLock()
	defer rm.metrics.mutex.RUnlock()

	return *rm.metrics
}

// ResetMetrics resets retry metrics
func (rm *RetryManager) ResetMetrics() {
	rm.metrics.mutex.Lock()
	defer rm.metrics.mutex.Unlock()

	rm.metrics.TotalOperations = 0
	rm.metrics.SuccessfulRetries = 0
	rm.metrics.FailedRetries = 0
	rm.metrics.AverageAttempts = 0
	rm.metrics.AverageRetryTime = 0

	log.Printf("Retry metrics reset")
}

// RetryWithBackoff is a convenience function for simple retry operations
func RetryWithBackoff(ctx context.Context, operation RetryableOperation, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	rm, err := NewRetryManager(config)
	if err != nil {
		return fmt.Errorf("failed to create retry manager: %w", err)
	}

	operationID := fmt.Sprintf("simple_retry_%d", time.Now().UnixNano())
	result := rm.ExecuteWithRetry(ctx, operationID, 0, operation)

	if !result.Success {
		return result.LastError
	}
	return nil
}

// CreateRetryableConnectionOperation creates a retryable connection operation
func CreateRetryableConnectionOperation(workerID int, connectFunc func() error) RetryableOperation {
	return func(ctx context.Context, attempt int) error {
		log.Printf("Connection attempt %d for worker %d", attempt, workerID)

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Attempt connection
		if err := connectFunc(); err != nil {
			return CreateConnectionError(workerID, "connection_attempt",
				fmt.Sprintf("Connection attempt %d failed", attempt), err)
		}

		log.Printf("Connection successful on attempt %d for worker %d", attempt, workerID)
		return nil
	}
}

// CreateRetryableProcessingOperation creates a retryable processing operation
func CreateRetryableProcessingOperation(workerID int, processFunc func() error) RetryableOperation {
	return func(ctx context.Context, attempt int) error {
		log.Printf("Processing attempt %d for worker %d", attempt, workerID)

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Attempt processing
		if err := processFunc(); err != nil {
			return CreateProcessingError(workerID, "processing_attempt",
				fmt.Sprintf("Processing attempt %d failed", attempt), err)
		}

		log.Printf("Processing successful on attempt %d for worker %d", attempt, workerID)
		return nil
	}
}

// CreateRetryableTransmissionOperation creates a retryable transmission operation
func CreateRetryableTransmissionOperation(workerID int, transmitFunc func() error) RetryableOperation {
	return func(ctx context.Context, attempt int) error {
		log.Printf("Transmission attempt %d for worker %d", attempt, workerID)

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Attempt transmission
		if err := transmitFunc(); err != nil {
			return CreateTransmissionError(workerID, "transmission_attempt",
				fmt.Sprintf("Transmission attempt %d failed", attempt), err)
		}

		log.Printf("Transmission successful on attempt %d for worker %d", attempt, workerID)
		return nil
	}
}

// RetryConfig presets for common scenarios
var (
	// QuickRetryConfig for fast operations that should retry quickly
	QuickRetryConfig = &RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        2 * time.Second,
		Strategy:        RetryStrategyExponential,
		Multiplier:      1.5,
		Jitter:          true,
		JitterRange:     0.1,
		RetryableErrors: []ErrorType{ErrorTypeConnection, ErrorTypeTransmission},
	}

	// StandardRetryConfig for normal operations
	StandardRetryConfig = DefaultRetryConfig()

	// SlowRetryConfig for long-running operations
	SlowRetryConfig = &RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    5 * time.Second,
		MaxDelay:        60 * time.Second,
		Strategy:        RetryStrategyExponential,
		Multiplier:      2.0,
		Jitter:          true,
		JitterRange:     0.2,
		RetryableErrors: []ErrorType{ErrorTypeProcessing, ErrorTypeTimeout},
	}

	// CriticalRetryConfig for critical operations that need more attempts
	CriticalRetryConfig = &RetryConfig{
		MaxAttempts:     10,
		InitialDelay:    500 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		Strategy:        RetryStrategyExponential,
		Multiplier:      1.8,
		Jitter:          true,
		JitterRange:     0.15,
		RetryableErrors: []ErrorType{ErrorTypeConnection, ErrorTypeTransmission, ErrorTypeTimeout},
	}
)
