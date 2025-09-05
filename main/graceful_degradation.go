package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// DegradationLevel represents the level of system degradation
type DegradationLevel int

const (
	DegradationNone DegradationLevel = iota
	DegradationMinor
	DegradationModerate
	DegradationSevere
	DegradationCritical
)

// String returns the string representation of DegradationLevel
func (dl DegradationLevel) String() string {
	switch dl {
	case DegradationNone:
		return "NONE"
	case DegradationMinor:
		return "MINOR"
	case DegradationModerate:
		return "MODERATE"
	case DegradationSevere:
		return "SEVERE"
	case DegradationCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// FailureMode represents different types of failures that can occur
type FailureMode string

const (
	FailureModeWorkerUnresponsive FailureMode = "WORKER_UNRESPONSIVE"
	FailureModeConnectionLoss     FailureMode = "CONNECTION_LOSS"
	FailureModeProcessingFailure  FailureMode = "PROCESSING_FAILURE"
	FailureModeTransmissionError  FailureMode = "TRANSMISSION_ERROR"
	FailureModeResourceExhaustion FailureMode = "RESOURCE_EXHAUSTION"
	FailureModePartialResults     FailureMode = "PARTIAL_RESULTS"
)

// DegradationStrategy defines how to handle different levels of degradation
type DegradationStrategy struct {
	Level                DegradationLevel `json:"level"`
	MaxFailedWorkers     int              `json:"max_failed_workers"`
	MinSuccessfulWorkers int              `json:"min_successful_workers"`
	TimeoutMultiplier    float64          `json:"timeout_multiplier"`
	RetryReduction       float64          `json:"retry_reduction"`
	EnablePartialResults bool             `json:"enable_partial_results"`
	FallbackActions      []string         `json:"fallback_actions"`
}

// GracefulDegradationManager manages system degradation and recovery
type GracefulDegradationManager struct {
	strategies         map[DegradationLevel]*DegradationStrategy
	currentLevel       DegradationLevel
	failedWorkers      map[int]FailureMode
	successfulWorkers  map[int]bool
	totalWorkers       int
	mutex              sync.RWMutex
	degradationHistory []DegradationEvent
	alertCallbacks     []DegradationCallback
	recoveryCallbacks  []RecoveryCallback
	partialResults     map[int]*messaging.ProcessingResult
	startTime          time.Time
	lastDegradation    time.Time
}

// DegradationEvent represents a degradation event
type DegradationEvent struct {
	Timestamp    time.Time        `json:"timestamp"`
	Level        DegradationLevel `json:"level"`
	Trigger      string           `json:"trigger"`
	FailedCount  int              `json:"failed_count"`
	SuccessCount int              `json:"success_count"`
	TotalCount   int              `json:"total_count"`
}

// DegradationCallback is called when degradation level changes
type DegradationCallback func(level DegradationLevel, event DegradationEvent)

// RecoveryCallback is called when system recovers
type RecoveryCallback func(previousLevel DegradationLevel, currentLevel DegradationLevel)

// NewGracefulDegradationManager creates a new graceful degradation manager
func NewGracefulDegradationManager(totalWorkers int) *GracefulDegradationManager {
	gdm := &GracefulDegradationManager{
		strategies:         make(map[DegradationLevel]*DegradationStrategy),
		currentLevel:       DegradationNone,
		failedWorkers:      make(map[int]FailureMode),
		successfulWorkers:  make(map[int]bool),
		totalWorkers:       totalWorkers,
		degradationHistory: make([]DegradationEvent, 0),
		alertCallbacks:     make([]DegradationCallback, 0),
		recoveryCallbacks:  make([]RecoveryCallback, 0),
		partialResults:     make(map[int]*messaging.ProcessingResult),
		startTime:          time.Now(),
	}

	// Initialize default strategies
	gdm.initializeDefaultStrategies()

	return gdm
}

// initializeDefaultStrategies sets up default degradation strategies
func (gdm *GracefulDegradationManager) initializeDefaultStrategies() {
	// Minor degradation: 1-2 workers failed
	gdm.strategies[DegradationMinor] = &DegradationStrategy{
		Level:                DegradationMinor,
		MaxFailedWorkers:     2,
		MinSuccessfulWorkers: gdm.totalWorkers - 2,
		TimeoutMultiplier:    1.2,
		RetryReduction:       0.9,
		EnablePartialResults: false,
		FallbackActions:      []string{"increase_timeouts", "reduce_retries"},
	}

	// Moderate degradation: 3-5 workers failed or 20-40% failure rate
	maxModerate := max(5, gdm.totalWorkers*40/100)
	gdm.strategies[DegradationModerate] = &DegradationStrategy{
		Level:                DegradationModerate,
		MaxFailedWorkers:     maxModerate,
		MinSuccessfulWorkers: gdm.totalWorkers - maxModerate,
		TimeoutMultiplier:    1.5,
		RetryReduction:       0.7,
		EnablePartialResults: true,
		FallbackActions:      []string{"enable_partial_results", "increase_timeouts", "reduce_retries"},
	}

	// Severe degradation: 40-70% failure rate
	maxSevere := max(maxModerate+1, gdm.totalWorkers*70/100)
	gdm.strategies[DegradationSevere] = &DegradationStrategy{
		Level:                DegradationSevere,
		MaxFailedWorkers:     maxSevere,
		MinSuccessfulWorkers: gdm.totalWorkers - maxSevere,
		TimeoutMultiplier:    2.0,
		RetryReduction:       0.5,
		EnablePartialResults: true,
		FallbackActions:      []string{"enable_partial_results", "emergency_timeouts", "minimal_retries"},
	}

	// Critical degradation: >70% failure rate
	gdm.strategies[DegradationCritical] = &DegradationStrategy{
		Level:                DegradationCritical,
		MaxFailedWorkers:     gdm.totalWorkers,
		MinSuccessfulWorkers: max(1, gdm.totalWorkers*30/100),
		TimeoutMultiplier:    3.0,
		RetryReduction:       0.3,
		EnablePartialResults: true,
		FallbackActions:      []string{"emergency_mode", "accept_any_results", "maximum_timeouts"},
	}
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ReportWorkerFailure reports a worker failure and updates degradation level
func (gdm *GracefulDegradationManager) ReportWorkerFailure(workerID int, failureMode FailureMode, details string) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()

	// Record the failure
	gdm.failedWorkers[workerID] = failureMode
	delete(gdm.successfulWorkers, workerID)

	log.Printf("Worker %d failed with mode %s: %s", workerID, failureMode, details)

	// Evaluate degradation level
	previousLevel := gdm.currentLevel
	newLevel := gdm.evaluateDegradationLevel()

	if newLevel != previousLevel {
		gdm.updateDegradationLevel(newLevel, fmt.Sprintf("Worker %d failure: %s", workerID, failureMode))
	}
}

// ReportWorkerSuccess reports a worker success and potentially improves degradation level
func (gdm *GracefulDegradationManager) ReportWorkerSuccess(workerID int, result *messaging.ProcessingResult) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()

	// Record the success
	gdm.successfulWorkers[workerID] = true
	delete(gdm.failedWorkers, workerID)

	// Store partial result if enabled
	if result != nil {
		gdm.partialResults[workerID] = result
	}

	log.Printf("Worker %d completed successfully", workerID)

	// Evaluate degradation level (might improve)
	previousLevel := gdm.currentLevel
	newLevel := gdm.evaluateDegradationLevel()

	if newLevel != previousLevel {
		gdm.updateDegradationLevel(newLevel, fmt.Sprintf("Worker %d success", workerID))
	}
}

// evaluateDegradationLevel determines the current degradation level based on failures
func (gdm *GracefulDegradationManager) evaluateDegradationLevel() DegradationLevel {
	failedCount := len(gdm.failedWorkers)
	successCount := len(gdm.successfulWorkers)

	// Check each degradation level from most severe to least
	levels := []DegradationLevel{DegradationCritical, DegradationSevere, DegradationModerate, DegradationMinor}

	for _, level := range levels {
		strategy := gdm.strategies[level]
		if strategy != nil {
			if failedCount >= strategy.MaxFailedWorkers || successCount < strategy.MinSuccessfulWorkers {
				return level
			}
		}
	}

	return DegradationNone
}

// updateDegradationLevel updates the current degradation level and triggers callbacks
func (gdm *GracefulDegradationManager) updateDegradationLevel(newLevel DegradationLevel, trigger string) {
	previousLevel := gdm.currentLevel
	gdm.currentLevel = newLevel
	gdm.lastDegradation = time.Now()

	// Create degradation event
	event := DegradationEvent{
		Timestamp:    time.Now(),
		Level:        newLevel,
		Trigger:      trigger,
		FailedCount:  len(gdm.failedWorkers),
		SuccessCount: len(gdm.successfulWorkers),
		TotalCount:   gdm.totalWorkers,
	}

	gdm.degradationHistory = append(gdm.degradationHistory, event)

	log.Printf("Degradation level changed: %s -> %s (trigger: %s)",
		previousLevel, newLevel, trigger)

	// Execute callbacks
	if newLevel > previousLevel {
		// Degradation occurred
		for _, callback := range gdm.alertCallbacks {
			go callback(newLevel, event)
		}
	} else if newLevel < previousLevel {
		// Recovery occurred
		for _, callback := range gdm.recoveryCallbacks {
			go callback(previousLevel, newLevel)
		}
	}

	// Apply degradation strategy
	gdm.applyDegradationStrategy(newLevel)
}

// applyDegradationStrategy applies the strategy for the current degradation level
func (gdm *GracefulDegradationManager) applyDegradationStrategy(level DegradationLevel) {
	strategy := gdm.strategies[level]
	if strategy == nil {
		return
	}

	log.Printf("Applying degradation strategy for level %s", level)

	// Execute fallback actions
	for _, action := range strategy.FallbackActions {
		gdm.executeFallbackAction(action, strategy)
	}
}

// executeFallbackAction executes a specific fallback action
func (gdm *GracefulDegradationManager) executeFallbackAction(action string, strategy *DegradationStrategy) {
	switch action {
	case "increase_timeouts":
		log.Printf("Increasing timeouts by factor %.1f", strategy.TimeoutMultiplier)
		// This would be implemented by the calling code

	case "reduce_retries":
		log.Printf("Reducing retry attempts by factor %.1f", strategy.RetryReduction)
		// This would be implemented by the calling code

	case "enable_partial_results":
		log.Printf("Enabling partial results collection")
		// Partial results are already being collected

	case "emergency_timeouts":
		log.Printf("Applying emergency timeout settings")
		// Extended timeouts for critical situations

	case "minimal_retries":
		log.Printf("Reducing to minimal retry attempts")
		// Minimal retries to conserve resources

	case "emergency_mode":
		log.Printf("Entering emergency mode - accepting any available results")
		// Emergency mode handling

	case "accept_any_results":
		log.Printf("Accepting any partial results available")
		// Accept whatever results we have

	case "maximum_timeouts":
		log.Printf("Applying maximum timeout settings")
		// Maximum timeouts for critical recovery

	default:
		log.Printf("Unknown fallback action: %s", action)
	}
}

// GetCurrentLevel returns the current degradation level
func (gdm *GracefulDegradationManager) GetCurrentLevel() DegradationLevel {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()
	return gdm.currentLevel
}

// GetStrategy returns the strategy for a specific degradation level
func (gdm *GracefulDegradationManager) GetStrategy(level DegradationLevel) *DegradationStrategy {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()
	return gdm.strategies[level]
}

// GetCurrentStrategy returns the strategy for the current degradation level
func (gdm *GracefulDegradationManager) GetCurrentStrategy() *DegradationStrategy {
	return gdm.GetStrategy(gdm.GetCurrentLevel())
}

// GetFailedWorkers returns a copy of failed workers map
func (gdm *GracefulDegradationManager) GetFailedWorkers() map[int]FailureMode {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	result := make(map[int]FailureMode)
	for workerID, mode := range gdm.failedWorkers {
		result[workerID] = mode
	}
	return result
}

// GetSuccessfulWorkers returns a list of successful worker IDs
func (gdm *GracefulDegradationManager) GetSuccessfulWorkers() []int {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	result := make([]int, 0, len(gdm.successfulWorkers))
	for workerID := range gdm.successfulWorkers {
		result = append(result, workerID)
	}
	return result
}

// GetPartialResults returns collected partial results
func (gdm *GracefulDegradationManager) GetPartialResults() map[int]*messaging.ProcessingResult {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	result := make(map[int]*messaging.ProcessingResult)
	for workerID, partialResult := range gdm.partialResults {
		result[workerID] = partialResult
	}
	return result
}

// ShouldAcceptPartialResults determines if partial results should be accepted
func (gdm *GracefulDegradationManager) ShouldAcceptPartialResults() bool {
	strategy := gdm.GetCurrentStrategy()
	return strategy != nil && strategy.EnablePartialResults
}

// GetTimeoutMultiplier returns the timeout multiplier for the current degradation level
func (gdm *GracefulDegradationManager) GetTimeoutMultiplier() float64 {
	strategy := gdm.GetCurrentStrategy()
	if strategy != nil {
		return strategy.TimeoutMultiplier
	}
	return 1.0
}

// GetRetryReduction returns the retry reduction factor for the current degradation level
func (gdm *GracefulDegradationManager) GetRetryReduction() float64 {
	strategy := gdm.GetCurrentStrategy()
	if strategy != nil {
		return strategy.RetryReduction
	}
	return 1.0
}

// RegisterDegradationCallback registers a callback for degradation events
func (gdm *GracefulDegradationManager) RegisterDegradationCallback(callback DegradationCallback) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()
	gdm.alertCallbacks = append(gdm.alertCallbacks, callback)
}

// RegisterRecoveryCallback registers a callback for recovery events
func (gdm *GracefulDegradationManager) RegisterRecoveryCallback(callback RecoveryCallback) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()
	gdm.recoveryCallbacks = append(gdm.recoveryCallbacks, callback)
}

// GetDegradationHistory returns the history of degradation events
func (gdm *GracefulDegradationManager) GetDegradationHistory() []DegradationEvent {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	result := make([]DegradationEvent, len(gdm.degradationHistory))
	copy(result, gdm.degradationHistory)
	return result
}

// GetSystemStatus returns comprehensive system status
func (gdm *GracefulDegradationManager) GetSystemStatus() map[string]interface{} {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	return map[string]interface{}{
		"current_level":      gdm.currentLevel.String(),
		"total_workers":      gdm.totalWorkers,
		"failed_workers":     len(gdm.failedWorkers),
		"successful_workers": len(gdm.successfulWorkers),
		"partial_results":    len(gdm.partialResults),
		"uptime":             time.Since(gdm.startTime).String(),
		"last_degradation":   gdm.lastDegradation.Format(time.RFC3339),
		"degradation_events": len(gdm.degradationHistory),
	}
}

// Reset resets the degradation manager for a new batch of workers
func (gdm *GracefulDegradationManager) Reset(totalWorkers int) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()

	gdm.totalWorkers = totalWorkers
	gdm.currentLevel = DegradationNone
	gdm.failedWorkers = make(map[int]FailureMode)
	gdm.successfulWorkers = make(map[int]bool)
	gdm.partialResults = make(map[int]*messaging.ProcessingResult)
	gdm.startTime = time.Now()

	// Reinitialize strategies for new worker count
	gdm.initializeDefaultStrategies()

	log.Printf("Graceful degradation manager reset for %d workers", totalWorkers)
}

// SetCustomStrategy allows setting a custom strategy for a degradation level
func (gdm *GracefulDegradationManager) SetCustomStrategy(level DegradationLevel, strategy *DegradationStrategy) {
	gdm.mutex.Lock()
	defer gdm.mutex.Unlock()

	gdm.strategies[level] = strategy
	log.Printf("Custom strategy set for degradation level %s", level)
}

// CanProceedWithPartialResults determines if the system can proceed with current partial results
func (gdm *GracefulDegradationManager) CanProceedWithPartialResults(minRequiredResults int) bool {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	availableResults := len(gdm.partialResults)
	strategy := gdm.strategies[gdm.currentLevel]

	if strategy != nil && strategy.EnablePartialResults {
		return availableResults >= minRequiredResults
	}

	return false
}

// WaitForMinimumResults waits for a minimum number of results or timeout
func (gdm *GracefulDegradationManager) WaitForMinimumResults(ctx context.Context, minResults int, checkInterval time.Duration) error {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			gdm.mutex.RLock()
			currentResults := len(gdm.partialResults)
			currentLevel := gdm.currentLevel
			gdm.mutex.RUnlock()

			if currentResults >= minResults {
				log.Printf("Minimum results achieved: %d/%d", currentResults, minResults)
				return nil
			}

			// Check if we should accept partial results due to degradation
			if gdm.ShouldAcceptPartialResults() && currentLevel >= DegradationModerate {
				log.Printf("Accepting partial results due to degradation level %s: %d/%d",
					currentLevel, currentResults, minResults)
				return nil
			}
		}
	}
}

// GenerateRecoveryPlan generates a recovery plan based on current failures
func (gdm *GracefulDegradationManager) GenerateRecoveryPlan() []string {
	gdm.mutex.RLock()
	defer gdm.mutex.RUnlock()

	var plan []string

	// Analyze failure modes
	failureModes := make(map[FailureMode]int)
	for _, mode := range gdm.failedWorkers {
		failureModes[mode]++
	}

	// Generate recovery actions based on failure patterns
	for mode, count := range failureModes {
		switch mode {
		case FailureModeWorkerUnresponsive:
			plan = append(plan, fmt.Sprintf("Restart %d unresponsive workers", count))
		case FailureModeConnectionLoss:
			plan = append(plan, fmt.Sprintf("Re-establish connections for %d workers", count))
		case FailureModeProcessingFailure:
			plan = append(plan, fmt.Sprintf("Investigate processing issues for %d workers", count))
		case FailureModeTransmissionError:
			plan = append(plan, fmt.Sprintf("Retry transmission for %d workers", count))
		case FailureModeResourceExhaustion:
			plan = append(plan, "Scale up resources or reduce workload")
		}
	}

	// Add general recovery actions based on degradation level
	switch gdm.currentLevel {
	case DegradationCritical:
		plan = append(plan, "Consider emergency shutdown and manual intervention")
	case DegradationSevere:
		plan = append(plan, "Implement emergency protocols and resource reallocation")
	case DegradationModerate:
		plan = append(plan, "Increase monitoring and prepare fallback procedures")
	case DegradationMinor:
		plan = append(plan, "Monitor closely and prepare for potential escalation")
	}

	return plan
}
