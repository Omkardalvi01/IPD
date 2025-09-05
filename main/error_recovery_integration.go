package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// ErrorRecoverySystem integrates all error handling components
type ErrorRecoverySystem struct {
	errorHandler       *ErrorHandler
	retryManager       *RetryManager
	degradationManager *GracefulDegradationManager
	logger             *EnhancedLogger
	errorAggregator    *ErrorAggregator
	recoveryStrategies map[ErrorType]*RecoveryStrategy
	systemMetrics      *SystemMetrics
	alertThresholds    *AlertThresholds
	mutex              sync.RWMutex
	isActive           bool
	monitoringInterval time.Duration
	stopChan           chan struct{}
	wg                 sync.WaitGroup
}

// RecoveryStrategy defines how to recover from specific error types
type RecoveryStrategy struct {
	MaxRetries          int           `json:"max_retries"`
	RetryConfig         *RetryConfig  `json:"retry_config"`
	EscalationThreshold int           `json:"escalation_threshold"`
	RecoveryTimeout     time.Duration `json:"recovery_timeout"`
	FallbackAction      string        `json:"fallback_action"`
}

// SystemMetrics tracks system-wide error and recovery metrics
type SystemMetrics struct {
	TotalErrors          int64                   `json:"total_errors"`
	ErrorsByType         map[ErrorType]int64     `json:"errors_by_type"`
	ErrorsBySeverity     map[ErrorSeverity]int64 `json:"errors_by_severity"`
	RecoveryAttempts     int64                   `json:"recovery_attempts"`
	SuccessfulRecoveries int64                   `json:"successful_recoveries"`
	FailedRecoveries     int64                   `json:"failed_recoveries"`
	SystemUptime         time.Duration           `json:"system_uptime"`
	LastErrorTime        time.Time               `json:"last_error_time"`
	mutex                sync.RWMutex
}

// AlertThresholds defines when to trigger alerts
type AlertThresholds struct {
	ErrorsPerMinute     int              `json:"errors_per_minute"`
	CriticalErrorCount  int              `json:"critical_error_count"`
	FailedWorkerPercent float64          `json:"failed_worker_percent"`
	RecoveryFailureRate float64          `json:"recovery_failure_rate"`
	DegradationLevel    DegradationLevel `json:"degradation_level"`
}

// NewErrorRecoverySystem creates a new integrated error recovery system
func NewErrorRecoverySystem(totalWorkers int, loggerConfig *LoggerConfig) (*ErrorRecoverySystem, error) {
	// Initialize logger
	logger, err := NewEnhancedLogger(loggerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize retry manager
	retryManager, err := NewRetryManager(DefaultRetryConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize retry manager: %w", err)
	}

	// Initialize error handler
	errorHandler := NewErrorHandler(DefaultRetryConfig())

	// Initialize degradation manager
	degradationManager := NewGracefulDegradationManager(totalWorkers)

	// Initialize error aggregator with alert threshold
	errorAggregator := NewErrorAggregator(5, func(workerID int, errors []ProcessingError) {
		logger.WarnWithContext("Worker error threshold exceeded", workerID, "error_aggregator", "threshold_alert",
			nil, map[string]interface{}{"error_count": len(errors)})
	})

	ers := &ErrorRecoverySystem{
		errorHandler:       errorHandler,
		retryManager:       retryManager,
		degradationManager: degradationManager,
		logger:             logger,
		errorAggregator:    errorAggregator,
		recoveryStrategies: make(map[ErrorType]*RecoveryStrategy),
		systemMetrics: &SystemMetrics{
			ErrorsByType:     make(map[ErrorType]int64),
			ErrorsBySeverity: make(map[ErrorSeverity]int64),
		},
		alertThresholds: &AlertThresholds{
			ErrorsPerMinute:     10,
			CriticalErrorCount:  3,
			FailedWorkerPercent: 0.3,
			RecoveryFailureRate: 0.5,
			DegradationLevel:    DegradationModerate,
		},
		monitoringInterval: 30 * time.Second,
		stopChan:           make(chan struct{}),
	}

	// Initialize default recovery strategies
	ers.initializeDefaultRecoveryStrategies()

	// Register callbacks
	ers.registerCallbacks()

	return ers, nil
}

// initializeDefaultRecoveryStrategies sets up default recovery strategies
func (ers *ErrorRecoverySystem) initializeDefaultRecoveryStrategies() {
	// Connection error recovery
	ers.recoveryStrategies[ErrorTypeConnection] = &RecoveryStrategy{
		MaxRetries:          5,
		RetryConfig:         QuickRetryConfig,
		EscalationThreshold: 3,
		RecoveryTimeout:     2 * time.Minute,
		FallbackAction:      "reconnect_worker",
	}

	// Processing error recovery
	ers.recoveryStrategies[ErrorTypeProcessing] = &RecoveryStrategy{
		MaxRetries:          3,
		RetryConfig:         StandardRetryConfig,
		EscalationThreshold: 2,
		RecoveryTimeout:     5 * time.Minute,
		FallbackAction:      "restart_processing",
	}

	// Transmission error recovery
	ers.recoveryStrategies[ErrorTypeTransmission] = &RecoveryStrategy{
		MaxRetries:          4,
		RetryConfig:         QuickRetryConfig,
		EscalationThreshold: 2,
		RecoveryTimeout:     3 * time.Minute,
		FallbackAction:      "retry_transmission",
	}

	// Timeout error recovery
	ers.recoveryStrategies[ErrorTypeTimeout] = &RecoveryStrategy{
		MaxRetries:          2,
		RetryConfig:         SlowRetryConfig,
		EscalationThreshold: 1,
		RecoveryTimeout:     10 * time.Minute,
		FallbackAction:      "extend_timeouts",
	}
}

// registerCallbacks registers callbacks for various components
func (ers *ErrorRecoverySystem) registerCallbacks() {
	// Register degradation callbacks
	ers.degradationManager.RegisterDegradationCallback(func(level DegradationLevel, event DegradationEvent) {
		ers.logger.LogDegradationEvent(level, event)
		ers.handleDegradationAlert(level, event)
	})

	ers.degradationManager.RegisterRecoveryCallback(func(previousLevel, currentLevel DegradationLevel) {
		ers.logger.InfoWithContext("System recovery detected", 0, "degradation", "recovery",
			map[string]interface{}{
				"previous_level": previousLevel.String(),
				"current_level":  currentLevel.String(),
			})
	})

	// Register error handler callbacks
	ers.errorHandler.RegisterErrorCallback(SeverityCritical, func(err *ProcessingError) {
		ers.handleCriticalError(err)
	})

	ers.errorHandler.RegisterErrorCallback(SeverityHigh, func(err *ProcessingError) {
		ers.handleHighSeverityError(err)
	})
}

// Start starts the error recovery system
func (ers *ErrorRecoverySystem) Start() {
	ers.mutex.Lock()
	if ers.isActive {
		ers.mutex.Unlock()
		return
	}
	ers.isActive = true
	ers.mutex.Unlock()

	ers.logger.Info("Error recovery system starting")

	// Start monitoring goroutine
	ers.wg.Add(1)
	go ers.monitoringLoop()

	ers.logger.Info("Error recovery system started")
}

// Stop stops the error recovery system
func (ers *ErrorRecoverySystem) Stop() {
	ers.mutex.Lock()
	if !ers.isActive {
		ers.mutex.Unlock()
		return
	}
	ers.isActive = false
	ers.mutex.Unlock()

	ers.logger.Info("Error recovery system stopping")

	// Stop monitoring
	close(ers.stopChan)
	ers.wg.Wait()

	// Cancel all retry operations
	ers.retryManager.CancelAllOperations()

	// Close logger
	ers.logger.Close()

	ers.logger.Info("Error recovery system stopped")
}

// HandleError processes an error through the integrated recovery system
func (ers *ErrorRecoverySystem) HandleError(err *ProcessingError) error {
	// Update metrics
	ers.updateMetrics(err)

	// Log the error
	ers.logger.LogProcessingError(err)

	// Add to aggregator
	ers.errorAggregator.AddError(err)

	// Report to degradation manager
	ers.reportToDegradationManager(err)

	// Handle through error handler
	if handlerErr := ers.errorHandler.HandleError(err); handlerErr != nil {
		ers.logger.ErrorWithContext("Error handler failed", err.WorkerID, "error_handler", "handle_error",
			handlerErr, map[string]interface{}{"original_error": err.Error()})
	}

	// Attempt recovery if error is recoverable
	if err.Recoverable {
		return ers.attemptRecovery(err)
	}

	return nil
}

// attemptRecovery attempts to recover from an error using the appropriate strategy
func (ers *ErrorRecoverySystem) attemptRecovery(err *ProcessingError) error {
	strategy, exists := ers.recoveryStrategies[err.Type]
	if !exists {
		ers.logger.WarnWithContext("No recovery strategy found", err.WorkerID, "recovery", "strategy_lookup",
			nil, map[string]interface{}{"error_type": string(err.Type)})
		return fmt.Errorf("no recovery strategy for error type: %s", err.Type)
	}

	ers.logger.InfoWithContext("Attempting error recovery", err.WorkerID, "recovery", "attempt",
		map[string]interface{}{
			"error_type": string(err.Type),
			"strategy":   strategy.FallbackAction,
		})

	// Create recovery context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), strategy.RecoveryTimeout)
	defer cancel()

	// Create recovery operation
	operationID := fmt.Sprintf("recovery_%s_%d_%d", err.Type, err.WorkerID, time.Now().UnixNano())

	recoveryOperation := func(ctx context.Context, attempt int) error {
		ers.logger.LogRetryAttempt(err.WorkerID, string(err.Type), attempt, strategy.MaxRetries, err)

		// Execute recovery action based on strategy
		return ers.executeRecoveryAction(ctx, err, strategy.FallbackAction)
	}

	// Execute recovery with retry
	result := ers.retryManager.ExecuteWithRetry(ctx, operationID, err.WorkerID, recoveryOperation)

	// Update recovery metrics
	ers.updateRecoveryMetrics(result.Success)

	if result.Success {
		ers.logger.InfoWithContext("Error recovery successful", err.WorkerID, "recovery", "success",
			map[string]interface{}{
				"attempts":   result.Attempts,
				"total_time": result.TotalTime.String(),
				"error_type": string(err.Type),
			})
		return nil
	} else {
		ers.logger.ErrorWithContext("Error recovery failed", err.WorkerID, "recovery", "failure",
			result.LastError, map[string]interface{}{
				"attempts":   result.Attempts,
				"total_time": result.TotalTime.String(),
				"error_type": string(err.Type),
			})
		return result.LastError
	}
}

// executeRecoveryAction executes a specific recovery action
func (ers *ErrorRecoverySystem) executeRecoveryAction(ctx context.Context, err *ProcessingError, action string) error {
	switch action {
	case "reconnect_worker":
		return ers.reconnectWorker(ctx, err.WorkerID)
	case "restart_processing":
		return ers.restartProcessing(ctx, err.WorkerID)
	case "retry_transmission":
		return ers.retryTransmission(ctx, err.WorkerID)
	case "extend_timeouts":
		return ers.extendTimeouts(ctx, err.WorkerID)
	default:
		return fmt.Errorf("unknown recovery action: %s", action)
	}
}

// reconnectWorker attempts to reconnect a worker
func (ers *ErrorRecoverySystem) reconnectWorker(ctx context.Context, workerID int) error {
	ers.logger.InfoWithContext("Attempting worker reconnection", workerID, "recovery", "reconnect", nil)

	// This would be implemented by the calling code to actually reconnect the worker
	// For now, we simulate the reconnection process
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		// Simulate reconnection success
		ers.logger.InfoWithContext("Worker reconnection completed", workerID, "recovery", "reconnect", nil)
		return nil
	}
}

// restartProcessing attempts to restart processing for a worker
func (ers *ErrorRecoverySystem) restartProcessing(ctx context.Context, workerID int) error {
	ers.logger.InfoWithContext("Attempting processing restart", workerID, "recovery", "restart_processing", nil)

	// This would be implemented by the calling code to restart processing
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		ers.logger.InfoWithContext("Processing restart completed", workerID, "recovery", "restart_processing", nil)
		return nil
	}
}

// retryTransmission attempts to retry transmission for a worker
func (ers *ErrorRecoverySystem) retryTransmission(ctx context.Context, workerID int) error {
	ers.logger.InfoWithContext("Attempting transmission retry", workerID, "recovery", "retry_transmission", nil)

	// This would be implemented by the calling code to retry transmission
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		ers.logger.InfoWithContext("Transmission retry completed", workerID, "recovery", "retry_transmission", nil)
		return nil
	}
}

// extendTimeouts extends timeouts for a worker
func (ers *ErrorRecoverySystem) extendTimeouts(ctx context.Context, workerID int) error {
	ers.logger.InfoWithContext("Extending timeouts", workerID, "recovery", "extend_timeouts", nil)

	// This would be implemented by the calling code to extend timeouts
	multiplier := ers.degradationManager.GetTimeoutMultiplier()
	ers.logger.InfoWithContext("Timeouts extended", workerID, "recovery", "extend_timeouts",
		map[string]interface{}{"multiplier": multiplier})
	return nil
}

// reportToDegradationManager reports errors to the degradation manager
func (ers *ErrorRecoverySystem) reportToDegradationManager(err *ProcessingError) {
	var failureMode FailureMode

	switch err.Type {
	case ErrorTypeConnection:
		failureMode = FailureModeConnectionLoss
	case ErrorTypeProcessing:
		failureMode = FailureModeProcessingFailure
	case ErrorTypeTransmission:
		failureMode = FailureModeTransmissionError
	case ErrorTypeTimeout:
		failureMode = FailureModeWorkerUnresponsive
	default:
		failureMode = FailureModeProcessingFailure
	}

	ers.degradationManager.ReportWorkerFailure(err.WorkerID, failureMode, err.Message)
}

// ReportWorkerSuccess reports successful worker completion
func (ers *ErrorRecoverySystem) ReportWorkerSuccess(workerID int, result *messaging.ProcessingResult) {
	ers.degradationManager.ReportWorkerSuccess(workerID, result)
	ers.logger.InfoWithContext("Worker completed successfully", workerID, "worker", "completion",
		map[string]interface{}{
			"processing_time": result.ProcessingTime.String(),
			"output_files":    result.OutputFileCount,
		})
}

// updateMetrics updates system metrics
func (ers *ErrorRecoverySystem) updateMetrics(err *ProcessingError) {
	ers.systemMetrics.mutex.Lock()
	defer ers.systemMetrics.mutex.Unlock()

	ers.systemMetrics.TotalErrors++
	ers.systemMetrics.ErrorsByType[err.Type]++
	ers.systemMetrics.ErrorsBySeverity[err.Severity]++
	ers.systemMetrics.LastErrorTime = time.Now()
}

// updateRecoveryMetrics updates recovery metrics
func (ers *ErrorRecoverySystem) updateRecoveryMetrics(success bool) {
	ers.systemMetrics.mutex.Lock()
	defer ers.systemMetrics.mutex.Unlock()

	ers.systemMetrics.RecoveryAttempts++
	if success {
		ers.systemMetrics.SuccessfulRecoveries++
	} else {
		ers.systemMetrics.FailedRecoveries++
	}
}

// monitoringLoop runs the system monitoring loop
func (ers *ErrorRecoverySystem) monitoringLoop() {
	defer ers.wg.Done()

	ticker := time.NewTicker(ers.monitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ers.performHealthCheck()
		case <-ers.stopChan:
			return
		}
	}
}

// performHealthCheck performs a system health check
func (ers *ErrorRecoverySystem) performHealthCheck() {
	// Check error rates
	ers.checkErrorRates()

	// Check degradation level
	ers.checkDegradationLevel()

	// Check recovery success rate
	ers.checkRecoverySuccessRate()

	// Log system status
	ers.logSystemStatus()
}

// checkErrorRates checks if error rates exceed thresholds
func (ers *ErrorRecoverySystem) checkErrorRates() {
	ers.systemMetrics.mutex.RLock()
	recentErrors := ers.systemMetrics.TotalErrors // Simplified - would need time-based calculation
	ers.systemMetrics.mutex.RUnlock()

	if recentErrors > int64(ers.alertThresholds.ErrorsPerMinute) {
		ers.logger.WarnWithContext("High error rate detected", 0, "monitoring", "error_rate_check",
			nil, map[string]interface{}{
				"error_rate": recentErrors,
				"threshold":  ers.alertThresholds.ErrorsPerMinute,
			})
	}
}

// checkDegradationLevel checks if degradation level exceeds threshold
func (ers *ErrorRecoverySystem) checkDegradationLevel() {
	currentLevel := ers.degradationManager.GetCurrentLevel()
	if currentLevel >= ers.alertThresholds.DegradationLevel {
		ers.logger.WarnWithContext("System degradation threshold exceeded", 0, "monitoring", "degradation_check",
			nil, map[string]interface{}{
				"current_level": currentLevel.String(),
				"threshold":     ers.alertThresholds.DegradationLevel.String(),
			})
	}
}

// checkRecoverySuccessRate checks recovery success rate
func (ers *ErrorRecoverySystem) checkRecoverySuccessRate() {
	ers.systemMetrics.mutex.RLock()
	totalRecoveries := ers.systemMetrics.RecoveryAttempts
	successfulRecoveries := ers.systemMetrics.SuccessfulRecoveries
	ers.systemMetrics.mutex.RUnlock()

	if totalRecoveries > 0 {
		successRate := float64(successfulRecoveries) / float64(totalRecoveries)
		if successRate < (1.0 - ers.alertThresholds.RecoveryFailureRate) {
			ers.logger.WarnWithContext("Low recovery success rate", 0, "monitoring", "recovery_rate_check",
				nil, map[string]interface{}{
					"success_rate": successRate,
					"threshold":    1.0 - ers.alertThresholds.RecoveryFailureRate,
				})
		}
	}
}

// logSystemStatus logs current system status
func (ers *ErrorRecoverySystem) logSystemStatus() {
	status := ers.GetSystemStatus()
	ers.logger.DebugWithContext("System status", 0, "monitoring", "status_check", status)
}

// handleCriticalError handles critical errors
func (ers *ErrorRecoverySystem) handleCriticalError(err *ProcessingError) {
	ers.logger.ErrorWithContext("Critical error detected", err.WorkerID, "error_handler", "critical_error",
		err, map[string]interface{}{
			"error_type": string(err.Type),
			"operation":  err.Operation,
		})

	// Implement critical error handling logic
	// This might include immediate escalation, emergency procedures, etc.
}

// handleHighSeverityError handles high severity errors
func (ers *ErrorRecoverySystem) handleHighSeverityError(err *ProcessingError) {
	ers.logger.WarnWithContext("High severity error detected", err.WorkerID, "error_handler", "high_severity_error",
		err, map[string]interface{}{
			"error_type": string(err.Type),
			"operation":  err.Operation,
		})

	// Implement high severity error handling logic
}

// handleDegradationAlert handles degradation alerts
func (ers *ErrorRecoverySystem) handleDegradationAlert(level DegradationLevel, event DegradationEvent) {
	ers.logger.WarnWithContext("System degradation alert", 0, "degradation", "alert",
		nil, map[string]interface{}{
			"level":        level.String(),
			"trigger":      event.Trigger,
			"failed_count": event.FailedCount,
		})

	// Check if alert thresholds are exceeded
	if level >= ers.alertThresholds.DegradationLevel {
		ers.logger.ErrorWithContext("Critical degradation level reached", 0, "degradation", "critical_alert",
			nil, map[string]interface{}{
				"level":     level.String(),
				"threshold": ers.alertThresholds.DegradationLevel.String(),
			})
	}
}

// GetSystemStatus returns comprehensive system status
func (ers *ErrorRecoverySystem) GetSystemStatus() map[string]interface{} {
	ers.systemMetrics.mutex.RLock()
	metrics := *ers.systemMetrics
	ers.systemMetrics.mutex.RUnlock()

	return map[string]interface{}{
		"is_active":             ers.isActive,
		"current_degradation":   ers.degradationManager.GetCurrentLevel().String(),
		"total_errors":          metrics.TotalErrors,
		"errors_by_type":        metrics.ErrorsByType,
		"errors_by_severity":    metrics.ErrorsBySeverity,
		"recovery_attempts":     metrics.RecoveryAttempts,
		"successful_recoveries": metrics.SuccessfulRecoveries,
		"failed_recoveries":     metrics.FailedRecoveries,
		"last_error_time":       metrics.LastErrorTime.Format(time.RFC3339),
		"retry_operations":      len(ers.retryManager.GetActiveOperations()),
		"degradation_status":    ers.degradationManager.GetSystemStatus(),
		"logger_stats":          ers.logger.GetLogStats(),
	}
}

// SetRecoveryStrategy sets a custom recovery strategy for an error type
func (ers *ErrorRecoverySystem) SetRecoveryStrategy(errorType ErrorType, strategy *RecoveryStrategy) {
	ers.mutex.Lock()
	defer ers.mutex.Unlock()
	ers.recoveryStrategies[errorType] = strategy
	ers.logger.InfoWithContext("Recovery strategy updated", 0, "recovery", "strategy_update",
		map[string]interface{}{
			"error_type": string(errorType),
			"strategy":   strategy.FallbackAction,
		})
}

// SetAlertThresholds sets custom alert thresholds
func (ers *ErrorRecoverySystem) SetAlertThresholds(thresholds *AlertThresholds) {
	ers.mutex.Lock()
	defer ers.mutex.Unlock()
	ers.alertThresholds = thresholds
	ers.logger.InfoWithContext("Alert thresholds updated", 0, "monitoring", "threshold_update",
		map[string]interface{}{
			"errors_per_minute":     thresholds.ErrorsPerMinute,
			"critical_error_count":  thresholds.CriticalErrorCount,
			"failed_worker_percent": thresholds.FailedWorkerPercent,
			"recovery_failure_rate": thresholds.RecoveryFailureRate,
			"degradation_level":     thresholds.DegradationLevel.String(),
		})
}

// Reset resets the error recovery system for a new batch of workers
func (ers *ErrorRecoverySystem) Reset(totalWorkers int) {
	ers.logger.InfoWithContext("Resetting error recovery system", 0, "system", "reset",
		map[string]interface{}{"total_workers": totalWorkers})

	// Reset degradation manager
	ers.degradationManager.Reset(totalWorkers)

	// Clear error aggregator
	ers.errorAggregator.ClearAllErrors()

	// Clear error handler log
	ers.errorHandler.ClearErrorLog()

	// Reset retry manager metrics
	ers.retryManager.ResetMetrics()

	// Reset system metrics
	ers.systemMetrics.mutex.Lock()
	ers.systemMetrics.TotalErrors = 0
	ers.systemMetrics.ErrorsByType = make(map[ErrorType]int64)
	ers.systemMetrics.ErrorsBySeverity = make(map[ErrorSeverity]int64)
	ers.systemMetrics.RecoveryAttempts = 0
	ers.systemMetrics.SuccessfulRecoveries = 0
	ers.systemMetrics.FailedRecoveries = 0
	ers.systemMetrics.mutex.Unlock()

	ers.logger.InfoWithContext("Error recovery system reset completed", 0, "system", "reset", nil)
}
