package main

import (
	"fmt"
	"testing"
	"time"
)

func TestProcessingError(t *testing.T) {
	t.Run("NewProcessingError", func(t *testing.T) {
		err := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "test_operation", "test message", nil)

		if err.Type != ErrorTypeConnection {
			t.Errorf("Expected error type %s, got %s", ErrorTypeConnection, err.Type)
		}
		if err.Severity != SeverityHigh {
			t.Errorf("Expected severity %s, got %s", SeverityHigh, err.Severity)
		}
		if err.WorkerID != 1 {
			t.Errorf("Expected worker ID 1, got %d", err.WorkerID)
		}
		if err.Operation != "test_operation" {
			t.Errorf("Expected operation 'test_operation', got '%s'", err.Operation)
		}
		if err.Message != "test message" {
			t.Errorf("Expected message 'test message', got '%s'", err.Message)
		}
		if !err.Recoverable {
			t.Error("Expected error to be recoverable for connection type with high severity")
		}
	})

	t.Run("ErrorInterface", func(t *testing.T) {
		cause := fmt.Errorf("underlying error")
		err := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 2, "process", "failed", cause)

		errorStr := err.Error()
		expectedSubstrings := []string{"PROCESSING", "MEDIUM", "Worker 2", "process", "failed", "underlying error"}

		for _, substr := range expectedSubstrings {
			if !contains(errorStr, substr) {
				t.Errorf("Error string should contain '%s', got: %s", substr, errorStr)
			}
		}
	})

	t.Run("AddContext", func(t *testing.T) {
		err := NewProcessingError(ErrorTypeValidation, SeverityLow, 3, "validate", "invalid data", nil)
		err.AddContext("field", "username")
		err.AddContext("value", "invalid_user")

		if err.Context["field"] != "username" {
			t.Errorf("Expected context field 'username', got %v", err.Context["field"])
		}
		if err.Context["value"] != "invalid_user" {
			t.Errorf("Expected context value 'invalid_user', got %v", err.Context["value"])
		}
	})

	t.Run("IncrementRetryCount", func(t *testing.T) {
		err := NewProcessingError(ErrorTypeTimeout, SeverityMedium, 4, "timeout", "operation timed out", nil)

		if err.RetryCount != 0 {
			t.Errorf("Expected initial retry count 0, got %d", err.RetryCount)
		}

		err.IncrementRetryCount()
		if err.RetryCount != 1 {
			t.Errorf("Expected retry count 1 after increment, got %d", err.RetryCount)
		}
	})
}

func TestErrorHandler(t *testing.T) {
	t.Run("NewErrorHandler", func(t *testing.T) {
		config := DefaultRetryConfig()
		handler := NewErrorHandler(config)

		if handler == nil {
			t.Fatal("Expected non-nil error handler")
		}
		if handler.retryConfig != config {
			t.Error("Expected retry config to be set")
		}
	})

	t.Run("HandleError", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())
		err := NewProcessingError(ErrorTypeConnection, SeverityMedium, 1, "connect", "connection failed", nil)

		handleErr := handler.HandleError(err)
		if handleErr != nil {
			t.Errorf("Expected no error from HandleError, got: %v", handleErr)
		}

		// Check that error was logged
		errorLog := handler.GetErrorLog()
		if len(errorLog) != 1 {
			t.Errorf("Expected 1 error in log, got %d", len(errorLog))
		}
		if errorLog[0].Type != ErrorTypeConnection {
			t.Errorf("Expected logged error type %s, got %s", ErrorTypeConnection, errorLog[0].Type)
		}
	})

	t.Run("GetErrorsByType", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		// Add different types of errors
		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 2, "process", "failed", nil)
		err3 := NewProcessingError(ErrorTypeConnection, SeverityLow, 3, "connect", "failed", nil)

		handler.HandleError(err1)
		handler.HandleError(err2)
		handler.HandleError(err3)

		connectionErrors := handler.GetErrorsByType(ErrorTypeConnection)
		if len(connectionErrors) != 2 {
			t.Errorf("Expected 2 connection errors, got %d", len(connectionErrors))
		}

		processingErrors := handler.GetErrorsByType(ErrorTypeProcessing)
		if len(processingErrors) != 1 {
			t.Errorf("Expected 1 processing error, got %d", len(processingErrors))
		}
	})

	t.Run("GetErrorsBySeverity", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityHigh, 2, "process", "failed", nil)
		err3 := NewProcessingError(ErrorTypeTimeout, SeverityMedium, 3, "timeout", "failed", nil)

		handler.HandleError(err1)
		handler.HandleError(err2)
		handler.HandleError(err3)

		highSeverityErrors := handler.GetErrorsBySeverity(SeverityHigh)
		if len(highSeverityErrors) != 2 {
			t.Errorf("Expected 2 high severity errors, got %d", len(highSeverityErrors))
		}

		mediumSeverityErrors := handler.GetErrorsBySeverity(SeverityMedium)
		if len(mediumSeverityErrors) != 1 {
			t.Errorf("Expected 1 medium severity error, got %d", len(mediumSeverityErrors))
		}
	})

	t.Run("GetErrorsByWorker", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 1, "process", "failed", nil)
		err3 := NewProcessingError(ErrorTypeTimeout, SeverityLow, 2, "timeout", "failed", nil)

		handler.HandleError(err1)
		handler.HandleError(err2)
		handler.HandleError(err3)

		worker1Errors := handler.GetErrorsByWorker(1)
		if len(worker1Errors) != 2 {
			t.Errorf("Expected 2 errors for worker 1, got %d", len(worker1Errors))
		}

		worker2Errors := handler.GetErrorsByWorker(2)
		if len(worker2Errors) != 1 {
			t.Errorf("Expected 1 error for worker 2, got %d", len(worker2Errors))
		}
	})

	t.Run("ErrorStatistics", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		// Add various errors
		errors := []*ProcessingError{
			NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil),
			NewProcessingError(ErrorTypeConnection, SeverityMedium, 2, "connect", "failed", nil),
			NewProcessingError(ErrorTypeProcessing, SeverityLow, 1, "process", "failed", nil),
		}

		for _, err := range errors {
			handler.HandleError(err)
		}

		stats := handler.GetErrorStatistics()

		if stats["total_errors"] != 3 {
			t.Errorf("Expected 3 total errors, got %v", stats["total_errors"])
		}

		byType := stats["by_type"].(map[ErrorType]int)
		if byType[ErrorTypeConnection] != 2 {
			t.Errorf("Expected 2 connection errors, got %d", byType[ErrorTypeConnection])
		}
		if byType[ErrorTypeProcessing] != 1 {
			t.Errorf("Expected 1 processing error, got %d", byType[ErrorTypeProcessing])
		}

		bySeverity := stats["by_severity"].(map[ErrorSeverity]int)
		if bySeverity[SeverityHigh] != 1 {
			t.Errorf("Expected 1 high severity error, got %d", bySeverity[SeverityHigh])
		}
		if bySeverity[SeverityMedium] != 1 {
			t.Errorf("Expected 1 medium severity error, got %d", bySeverity[SeverityMedium])
		}
		if bySeverity[SeverityLow] != 1 {
			t.Errorf("Expected 1 low severity error, got %d", bySeverity[SeverityLow])
		}
	})

	t.Run("ClearErrorLog", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		err := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		handler.HandleError(err)

		if len(handler.GetErrorLog()) != 1 {
			t.Error("Expected 1 error in log before clear")
		}

		handler.ClearErrorLog()

		if len(handler.GetErrorLog()) != 0 {
			t.Error("Expected 0 errors in log after clear")
		}
	})
}

func TestErrorAggregator(t *testing.T) {
	t.Run("NewErrorAggregator", func(t *testing.T) {
		alertFunc := func(workerID int, errors []ProcessingError) {
			// Alert function for testing
		}

		aggregator := NewErrorAggregator(3, alertFunc)
		if aggregator == nil {
			t.Fatal("Expected non-nil error aggregator")
		}
		if aggregator.threshold != 3 {
			t.Errorf("Expected threshold 3, got %d", aggregator.threshold)
		}
	})

	t.Run("AddError", func(t *testing.T) {
		alertCalled := false
		var alertWorkerID int
		var alertErrors []ProcessingError

		alertFunc := func(workerID int, errors []ProcessingError) {
			alertCalled = true
			alertWorkerID = workerID
			alertErrors = errors
		}

		aggregator := NewErrorAggregator(2, alertFunc)

		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed 1", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 1, "process", "failed 2", nil)

		// Add first error - should not trigger alert
		aggregator.AddError(err1)
		if alertCalled {
			t.Error("Alert should not be called after first error")
		}

		// Add second error - should trigger alert
		aggregator.AddError(err2)

		// Give some time for the goroutine to execute
		time.Sleep(10 * time.Millisecond)

		if !alertCalled {
			t.Error("Alert should be called after reaching threshold")
		}
		if alertWorkerID != 1 {
			t.Errorf("Expected alert for worker 1, got worker %d", alertWorkerID)
		}
		if len(alertErrors) != 2 {
			t.Errorf("Expected 2 errors in alert, got %d", len(alertErrors))
		}
	})

	t.Run("GetErrors", func(t *testing.T) {
		aggregator := NewErrorAggregator(5, nil)

		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 2, "process", "failed", nil)

		aggregator.AddError(err1)
		aggregator.AddError(err2)

		worker1Errors := aggregator.GetErrors(1)
		if len(worker1Errors) != 1 {
			t.Errorf("Expected 1 error for worker 1, got %d", len(worker1Errors))
		}

		worker2Errors := aggregator.GetErrors(2)
		if len(worker2Errors) != 1 {
			t.Errorf("Expected 1 error for worker 2, got %d", len(worker2Errors))
		}

		worker3Errors := aggregator.GetErrors(3)
		if worker3Errors != nil {
			t.Errorf("Expected nil for worker 3, got %v", worker3Errors)
		}
	})

	t.Run("ClearErrors", func(t *testing.T) {
		aggregator := NewErrorAggregator(5, nil)

		err := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		aggregator.AddError(err)

		if len(aggregator.GetErrors(1)) != 1 {
			t.Error("Expected 1 error before clear")
		}

		aggregator.ClearErrors(1)

		if aggregator.GetErrors(1) != nil {
			t.Error("Expected nil errors after clear")
		}
	})

	t.Run("ClearAllErrors", func(t *testing.T) {
		aggregator := NewErrorAggregator(5, nil)

		err1 := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		err2 := NewProcessingError(ErrorTypeProcessing, SeverityMedium, 2, "process", "failed", nil)

		aggregator.AddError(err1)
		aggregator.AddError(err2)

		allErrors := aggregator.GetAllErrors()
		if len(allErrors) != 2 {
			t.Errorf("Expected 2 workers with errors, got %d", len(allErrors))
		}

		aggregator.ClearAllErrors()

		allErrors = aggregator.GetAllErrors()
		if len(allErrors) != 0 {
			t.Errorf("Expected 0 workers with errors after clear, got %d", len(allErrors))
		}
	})
}

func TestErrorCreationFunctions(t *testing.T) {
	t.Run("CreateConnectionError", func(t *testing.T) {
		cause := fmt.Errorf("connection refused")
		err := CreateConnectionError(1, "connect", "failed to connect", cause)

		if err.Type != ErrorTypeConnection {
			t.Errorf("Expected connection error type, got %s", err.Type)
		}
		if err.Severity != SeverityCritical {
			t.Errorf("Expected critical severity for 'connection refused', got %s", err.Severity)
		}
		if err.WorkerID != 1 {
			t.Errorf("Expected worker ID 1, got %d", err.WorkerID)
		}
		if err.Cause != cause {
			t.Error("Expected cause to be set")
		}
	})

	t.Run("CreateProcessingError", func(t *testing.T) {
		cause := fmt.Errorf("timeout occurred")
		err := CreateProcessingError(2, "process", "processing failed", cause)

		if err.Type != ErrorTypeProcessing {
			t.Errorf("Expected processing error type, got %s", err.Type)
		}
		if err.Severity != SeverityHigh {
			t.Errorf("Expected high severity for 'timeout', got %s", err.Severity)
		}
		if err.WorkerID != 2 {
			t.Errorf("Expected worker ID 2, got %d", err.WorkerID)
		}
	})

	t.Run("CreateTransmissionError", func(t *testing.T) {
		cause := fmt.Errorf("network error")
		err := CreateTransmissionError(3, "transmit", "transmission failed", cause)

		if err.Type != ErrorTypeTransmission {
			t.Errorf("Expected transmission error type, got %s", err.Type)
		}
		if err.WorkerID != 3 {
			t.Errorf("Expected worker ID 3, got %d", err.WorkerID)
		}
	})

	t.Run("CreateValidationError", func(t *testing.T) {
		err := CreateValidationError(4, "validate", "validation failed", nil)

		if err.Type != ErrorTypeValidation {
			t.Errorf("Expected validation error type, got %s", err.Type)
		}
		if err.Severity != SeverityMedium {
			t.Errorf("Expected medium severity for validation error, got %s", err.Severity)
		}
		if err.WorkerID != 4 {
			t.Errorf("Expected worker ID 4, got %d", err.WorkerID)
		}
	})

	t.Run("CreateTimeoutError", func(t *testing.T) {
		timeout := 30 * time.Second
		err := CreateTimeoutError(5, "timeout_op", "operation timed out", timeout)

		if err.Type != ErrorTypeTimeout {
			t.Errorf("Expected timeout error type, got %s", err.Type)
		}
		if err.Severity != SeverityHigh {
			t.Errorf("Expected high severity for timeout error, got %s", err.Severity)
		}
		if err.WorkerID != 5 {
			t.Errorf("Expected worker ID 5, got %d", err.WorkerID)
		}

		if err.Context["timeout_duration"] != timeout.String() {
			t.Errorf("Expected timeout duration in context, got %v", err.Context["timeout_duration"])
		}
	})

	t.Run("CreateSystemError", func(t *testing.T) {
		cause := fmt.Errorf("disk space full")
		err := CreateSystemError(6, "system_op", "system error", cause)

		if err.Type != ErrorTypeSystem {
			t.Errorf("Expected system error type, got %s", err.Type)
		}
		if err.Severity != SeverityCritical {
			t.Errorf("Expected critical severity for system error, got %s", err.Severity)
		}
		if err.WorkerID != 6 {
			t.Errorf("Expected worker ID 6, got %d", err.WorkerID)
		}
	})
}

func TestRecoverabilityDetermination(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		severity  ErrorSeverity
		expected  bool
	}{
		{ErrorTypeConnection, SeverityCritical, false},
		{ErrorTypeConnection, SeverityHigh, true},
		{ErrorTypeConnection, SeverityMedium, true},
		{ErrorTypeConnection, SeverityLow, true},
		{ErrorTypeProcessing, SeverityCritical, false},
		{ErrorTypeProcessing, SeverityHigh, false},
		{ErrorTypeProcessing, SeverityMedium, true},
		{ErrorTypeTransmission, SeverityHigh, true},
		{ErrorTypeTimeout, SeverityMedium, true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_%s", tc.errorType, tc.severity), func(t *testing.T) {
			recoverable := determineRecoverability(tc.errorType, tc.severity)
			if recoverable != tc.expected {
				t.Errorf("Expected recoverability %v for %s/%s, got %v",
					tc.expected, tc.errorType, tc.severity, recoverable)
			}
		})
	}
}

// Benchmark tests
func BenchmarkErrorHandling(b *testing.B) {
	handler := NewErrorHandler(DefaultRetryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := NewProcessingError(ErrorTypeConnection, SeverityMedium, i%10, "benchmark", "test error", nil)
		handler.HandleError(err)
	}
}

func BenchmarkErrorAggregation(b *testing.B) {
	aggregator := NewErrorAggregator(1000, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := NewProcessingError(ErrorTypeProcessing, SeverityLow, i%100, "benchmark", "test error", nil)
		aggregator.AddError(err)
	}
}

// Helper function to test error callback registration
func TestErrorCallbacks(t *testing.T) {
	t.Run("RegisterErrorCallback", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		callbackCalled := false
		var callbackError *ProcessingError

		handler.RegisterErrorCallback(SeverityHigh, func(err *ProcessingError) {
			callbackCalled = true
			callbackError = err
		})

		err := NewProcessingError(ErrorTypeConnection, SeverityHigh, 1, "connect", "failed", nil)
		handler.HandleError(err)

		// Give some time for the goroutine to execute
		time.Sleep(10 * time.Millisecond)

		if !callbackCalled {
			t.Error("Expected callback to be called for high severity error")
		}
		if callbackError == nil || callbackError.Type != ErrorTypeConnection {
			t.Error("Expected callback to receive the correct error")
		}
	})

	t.Run("CallbackNotCalledForDifferentSeverity", func(t *testing.T) {
		handler := NewErrorHandler(DefaultRetryConfig())

		callbackCalled := false

		handler.RegisterErrorCallback(SeverityHigh, func(err *ProcessingError) {
			callbackCalled = true
		})

		err := NewProcessingError(ErrorTypeConnection, SeverityMedium, 1, "connect", "failed", nil)
		handler.HandleError(err)

		// Give some time for the goroutine to execute
		time.Sleep(10 * time.Millisecond)

		if callbackCalled {
			t.Error("Expected callback not to be called for medium severity error")
		}
	})
}
