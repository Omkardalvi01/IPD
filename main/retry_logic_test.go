package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRetryConfig(t *testing.T) {
	t.Run("DefaultRetryConfig", func(t *testing.T) {
		config := DefaultRetryConfig()

		if config.MaxAttempts != 3 {
			t.Errorf("Expected max attempts 3, got %d", config.MaxAttempts)
		}
		if config.InitialDelay != 1*time.Second {
			t.Errorf("Expected initial delay 1s, got %v", config.InitialDelay)
		}
		if config.Strategy != RetryStrategyExponential {
			t.Errorf("Expected exponential strategy, got %s", config.Strategy)
		}
		if !config.Jitter {
			t.Error("Expected jitter to be enabled")
		}
	})

	t.Run("ValidateValidConfig", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    500 * time.Millisecond,
			MaxDelay:        10 * time.Second,
			Strategy:        RetryStrategyExponential,
			Multiplier:      2.0,
			Jitter:          true,
			JitterRange:     0.1,
			RetryableErrors: []ErrorType{ErrorTypeConnection},
		}

		if err := config.Validate(); err != nil {
			t.Errorf("Expected valid config to pass validation, got: %v", err)
		}
	})

	t.Run("ValidateInvalidConfigs", func(t *testing.T) {
		testCases := []struct {
			name   string
			config *RetryConfig
		}{
			{
				name: "ZeroMaxAttempts",
				config: &RetryConfig{
					MaxAttempts:  0,
					InitialDelay: 1 * time.Second,
					MaxDelay:     10 * time.Second,
				},
			},
			{
				name: "NegativeInitialDelay",
				config: &RetryConfig{
					MaxAttempts:  3,
					InitialDelay: -1 * time.Second,
					MaxDelay:     10 * time.Second,
				},
			},
			{
				name: "MaxDelayLessThanInitial",
				config: &RetryConfig{
					MaxAttempts:  3,
					InitialDelay: 10 * time.Second,
					MaxDelay:     5 * time.Second,
				},
			},
			{
				name: "InvalidMultiplierForExponential",
				config: &RetryConfig{
					MaxAttempts:  3,
					InitialDelay: 1 * time.Second,
					MaxDelay:     10 * time.Second,
					Strategy:     RetryStrategyExponential,
					Multiplier:   0.5,
				},
			},
			{
				name: "InvalidJitterRange",
				config: &RetryConfig{
					MaxAttempts:  3,
					InitialDelay: 1 * time.Second,
					MaxDelay:     10 * time.Second,
					JitterRange:  1.5,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if err := tc.config.Validate(); err == nil {
					t.Error("Expected validation to fail for invalid config")
				}
			})
		}
	})
}

func TestRetryManager(t *testing.T) {
	t.Run("NewRetryManager", func(t *testing.T) {
		config := DefaultRetryConfig()
		rm, err := NewRetryManager(config)

		if err != nil {
			t.Fatalf("Expected no error creating retry manager, got: %v", err)
		}
		if rm == nil {
			t.Fatal("Expected non-nil retry manager")
		}
		if rm.config != config {
			t.Error("Expected config to be set")
		}
	})

	t.Run("NewRetryManagerWithNilConfig", func(t *testing.T) {
		rm, err := NewRetryManager(nil)

		if err != nil {
			t.Fatalf("Expected no error with nil config, got: %v", err)
		}
		if rm.config == nil {
			t.Error("Expected default config to be used")
		}
	})

	t.Run("NewRetryManagerWithInvalidConfig", func(t *testing.T) {
		invalidConfig := &RetryConfig{
			MaxAttempts: 0, // Invalid
		}

		_, err := NewRetryManager(invalidConfig)
		if err == nil {
			t.Error("Expected error with invalid config")
		}
	})
}

func TestRetryExecution(t *testing.T) {
	t.Run("SuccessfulOperationFirstAttempt", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		attemptCount := 0
		operation := func(ctx context.Context, attempt int) error {
			attemptCount++
			return nil // Success on first attempt
		}

		ctx := context.Background()
		result := rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		if !result.Success {
			t.Error("Expected operation to succeed")
		}
		if result.Attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", result.Attempts)
		}
		if attemptCount != 1 {
			t.Errorf("Expected operation to be called once, got %d", attemptCount)
		}
	})

	t.Run("SuccessfulOperationAfterRetries", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			Strategy:        RetryStrategyFixed,
			Jitter:          false,
			RetryableErrors: []ErrorType{ErrorTypeConnection},
		}
		rm, _ := NewRetryManager(config)

		attemptCount := 0
		operation := func(ctx context.Context, attempt int) error {
			attemptCount++
			if attempt < 3 {
				return CreateConnectionError(1, "connect", "connection failed", fmt.Errorf("network error"))
			}
			return nil // Success on third attempt
		}

		ctx := context.Background()
		result := rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		if !result.Success {
			t.Error("Expected operation to succeed after retries")
		}
		if result.Attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", result.Attempts)
		}
		if attemptCount != 3 {
			t.Errorf("Expected operation to be called 3 times, got %d", attemptCount)
		}
	})

	t.Run("FailedOperationAllAttempts", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:     2,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			Strategy:        RetryStrategyFixed,
			Jitter:          false,
			RetryableErrors: []ErrorType{ErrorTypeConnection},
		}
		rm, _ := NewRetryManager(config)

		attemptCount := 0
		operation := func(ctx context.Context, attempt int) error {
			attemptCount++
			return CreateConnectionError(1, "connect", "connection failed", fmt.Errorf("network error"))
		}

		ctx := context.Background()
		result := rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		if result.Success {
			t.Error("Expected operation to fail after all attempts")
		}
		if result.Attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", result.Attempts)
		}
		if attemptCount != 2 {
			t.Errorf("Expected operation to be called 2 times, got %d", attemptCount)
		}
		if result.LastError == nil {
			t.Error("Expected last error to be set")
		}
	})

	t.Run("NonRetryableError", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		attemptCount := 0
		operation := func(ctx context.Context, attempt int) error {
			attemptCount++
			return CreateValidationError(1, "validate", "validation failed", nil)
		}

		ctx := context.Background()
		result := rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		if result.Success {
			t.Error("Expected operation to fail")
		}
		if result.Attempts != 1 {
			t.Errorf("Expected 1 attempt for non-retryable error, got %d", result.Attempts)
		}
		if attemptCount != 1 {
			t.Errorf("Expected operation to be called once, got %d", attemptCount)
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    100 * time.Millisecond,
			MaxDelay:        1 * time.Second,
			Strategy:        RetryStrategyFixed,
			Jitter:          false,
			RetryableErrors: []ErrorType{ErrorTypeConnection},
		}
		rm, _ := NewRetryManager(config)

		attemptCount := 0
		operation := func(ctx context.Context, attempt int) error {
			attemptCount++
			return CreateConnectionError(1, "connect", "connection failed", fmt.Errorf("network error"))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		result := rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		if result.Success {
			t.Error("Expected operation to fail due to context cancellation")
		}
		if result.LastError != context.DeadlineExceeded {
			t.Errorf("Expected context deadline exceeded error, got: %v", result.LastError)
		}
	})
}

func TestDelayCalculation(t *testing.T) {
	t.Run("FixedStrategy", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyFixed,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Jitter:       false,
		}
		rm, _ := NewRetryManager(config)

		delay1 := rm.calculateDelay(1)
		delay2 := rm.calculateDelay(2)
		delay3 := rm.calculateDelay(3)

		if delay1 != config.InitialDelay {
			t.Errorf("Expected delay %v, got %v", config.InitialDelay, delay1)
		}
		if delay2 != config.InitialDelay {
			t.Errorf("Expected delay %v, got %v", config.InitialDelay, delay2)
		}
		if delay3 != config.InitialDelay {
			t.Errorf("Expected delay %v, got %v", config.InitialDelay, delay3)
		}
	})

	t.Run("LinearStrategy", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyLinear,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Jitter:       false,
		}
		rm, _ := NewRetryManager(config)

		delay1 := rm.calculateDelay(1)
		delay2 := rm.calculateDelay(2)
		delay3 := rm.calculateDelay(3)

		expected1 := 1 * config.InitialDelay
		expected2 := 2 * config.InitialDelay
		expected3 := 3 * config.InitialDelay

		if delay1 != expected1 {
			t.Errorf("Expected delay %v, got %v", expected1, delay1)
		}
		if delay2 != expected2 {
			t.Errorf("Expected delay %v, got %v", expected2, delay2)
		}
		if delay3 != expected3 {
			t.Errorf("Expected delay %v, got %v", expected3, delay3)
		}
	})

	t.Run("ExponentialStrategy", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyExponential,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
			Jitter:       false,
		}
		rm, _ := NewRetryManager(config)

		delay1 := rm.calculateDelay(1)
		delay2 := rm.calculateDelay(2)
		delay3 := rm.calculateDelay(3)

		expected1 := config.InitialDelay
		expected2 := time.Duration(float64(config.InitialDelay) * 2.0)
		expected3 := time.Duration(float64(config.InitialDelay) * 4.0)

		if delay1 != expected1 {
			t.Errorf("Expected delay %v, got %v", expected1, delay1)
		}
		if delay2 != expected2 {
			t.Errorf("Expected delay %v, got %v", expected2, delay2)
		}
		if delay3 != expected3 {
			t.Errorf("Expected delay %v, got %v", expected3, delay3)
		}
	})

	t.Run("MaxDelayLimit", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyExponential,
			InitialDelay: 1 * time.Second,
			MaxDelay:     2 * time.Second,
			Multiplier:   10.0,
			Jitter:       false,
		}
		rm, _ := NewRetryManager(config)

		delay := rm.calculateDelay(5) // Should be capped at MaxDelay

		if delay != config.MaxDelay {
			t.Errorf("Expected delay to be capped at %v, got %v", config.MaxDelay, delay)
		}
	})

	t.Run("CustomStrategy", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyCustom,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Jitter:       false,
		}
		rm, _ := NewRetryManager(config)

		customDelayCalled := false
		rm.SetCustomDelayFunction(func(attempt int, baseDelay time.Duration) time.Duration {
			customDelayCalled = true
			return time.Duration(attempt*attempt) * baseDelay
		})

		delay := rm.calculateDelay(3)

		if !customDelayCalled {
			t.Error("Expected custom delay function to be called")
		}

		expected := time.Duration(3*3) * config.InitialDelay
		if delay != expected {
			t.Errorf("Expected delay %v, got %v", expected, delay)
		}
	})

	t.Run("JitterEnabled", func(t *testing.T) {
		config := &RetryConfig{
			Strategy:     RetryStrategyFixed,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Jitter:       true,
			JitterRange:  0.1,
		}
		rm, _ := NewRetryManager(config)

		// Test multiple times to ensure jitter is working
		delays := make([]time.Duration, 10)
		for i := 0; i < 10; i++ {
			delays[i] = rm.calculateDelay(1)
		}

		// Check that not all delays are the same (jitter should introduce variation)
		allSame := true
		for i := 1; i < len(delays); i++ {
			if delays[i] != delays[0] {
				allSame = false
				break
			}
		}

		if allSame {
			t.Error("Expected jitter to introduce variation in delays")
		}

		// Check that all delays are within reasonable bounds
		minExpected := time.Duration(float64(config.InitialDelay) * (1.0 - config.JitterRange))
		maxExpected := time.Duration(float64(config.InitialDelay) * (1.0 + config.JitterRange))

		for i, delay := range delays {
			if delay < minExpected || delay > maxExpected {
				t.Errorf("Delay %d (%v) is outside expected range [%v, %v]",
					i, delay, minExpected, maxExpected)
			}
		}
	})
}

func TestRetryableOperations(t *testing.T) {
	t.Run("CreateRetryableConnectionOperation", func(t *testing.T) {
		connectCalled := false
		connectFunc := func() error {
			connectCalled = true
			return nil
		}

		operation := CreateRetryableConnectionOperation(1, connectFunc)

		ctx := context.Background()
		err := operation(ctx, 1)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !connectCalled {
			t.Error("Expected connect function to be called")
		}
	})

	t.Run("CreateRetryableProcessingOperation", func(t *testing.T) {
		processCalled := false
		processFunc := func() error {
			processCalled = true
			return fmt.Errorf("processing failed")
		}

		operation := CreateRetryableProcessingOperation(2, processFunc)

		ctx := context.Background()
		err := operation(ctx, 1)

		if err == nil {
			t.Error("Expected error from processing function")
		}
		if !processCalled {
			t.Error("Expected process function to be called")
		}

		// Check that it returns a ProcessingError
		if procErr, ok := err.(*ProcessingError); ok {
			if procErr.Type != ErrorTypeProcessing {
				t.Errorf("Expected processing error type, got %s", procErr.Type)
			}
			if procErr.WorkerID != 2 {
				t.Errorf("Expected worker ID 2, got %d", procErr.WorkerID)
			}
		} else {
			t.Error("Expected ProcessingError type")
		}
	})

	t.Run("CreateRetryableTransmissionOperation", func(t *testing.T) {
		transmitCalled := false
		transmitFunc := func() error {
			transmitCalled = true
			return nil
		}

		operation := CreateRetryableTransmissionOperation(3, transmitFunc)

		ctx := context.Background()
		err := operation(ctx, 2)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !transmitCalled {
			t.Error("Expected transmit function to be called")
		}
	})
}

func TestRetryOperationManagement(t *testing.T) {
	t.Run("GetActiveOperations", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		// Start a long-running operation
		operation := func(ctx context.Context, attempt int) error {
			time.Sleep(100 * time.Millisecond)
			return fmt.Errorf("always fails")
		}

		go func() {
			ctx := context.Background()
			rm.ExecuteWithRetry(ctx, "long_op", 1, operation)
		}()

		// Give it time to start
		time.Sleep(10 * time.Millisecond)

		activeOps := rm.GetActiveOperations()
		if len(activeOps) != 1 {
			t.Errorf("Expected 1 active operation, got %d", len(activeOps))
		}

		if op, exists := activeOps["long_op"]; exists {
			if op.WorkerID != 1 {
				t.Errorf("Expected worker ID 1, got %d", op.WorkerID)
			}
		} else {
			t.Error("Expected to find 'long_op' in active operations")
		}

		// Wait for operation to complete
		time.Sleep(200 * time.Millisecond)

		activeOps = rm.GetActiveOperations()
		if len(activeOps) != 0 {
			t.Errorf("Expected 0 active operations after completion, got %d", len(activeOps))
		}
	})

	t.Run("CancelOperation", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:     10,
			InitialDelay:    50 * time.Millisecond,
			MaxDelay:        1 * time.Second,
			Strategy:        RetryStrategyFixed,
			Jitter:          false,
			RetryableErrors: []ErrorType{ErrorTypeConnection},
		}
		rm, _ := NewRetryManager(config)

		operation := func(ctx context.Context, attempt int) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return CreateConnectionError(1, "connect", "failed", fmt.Errorf("network error"))
			}
		}

		resultChan := make(chan *RetryResult, 1)
		go func() {
			ctx := context.Background()
			result := rm.ExecuteWithRetry(ctx, "cancel_test", 1, operation)
			resultChan <- result
		}()

		// Give it time to start
		time.Sleep(10 * time.Millisecond)

		// Cancel the operation
		cancelled := rm.CancelOperation("cancel_test")
		if !cancelled {
			t.Error("Expected operation to be cancelled")
		}

		// Wait for result
		select {
		case result := <-resultChan:
			if result.Success {
				t.Error("Expected operation to fail due to cancellation")
			}
			if result.LastError != context.Canceled {
				t.Errorf("Expected context cancelled error, got: %v", result.LastError)
			}
		case <-time.After(1 * time.Second):
			t.Error("Operation should have completed quickly after cancellation")
		}
	})

	t.Run("CancelAllOperations", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		operation := func(ctx context.Context, attempt int) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return fmt.Errorf("always fails")
			}
		}

		// Start multiple operations
		for i := 0; i < 3; i++ {
			go func(id int) {
				ctx := context.Background()
				rm.ExecuteWithRetry(ctx, fmt.Sprintf("op_%d", id), id, operation)
			}(i)
		}

		// Give them time to start
		time.Sleep(10 * time.Millisecond)

		activeOps := rm.GetActiveOperations()
		if len(activeOps) != 3 {
			t.Errorf("Expected 3 active operations, got %d", len(activeOps))
		}

		// Cancel all operations
		cancelled := rm.CancelAllOperations()
		if cancelled != 3 {
			t.Errorf("Expected 3 operations to be cancelled, got %d", cancelled)
		}

		// Wait a bit and check that operations are no longer active
		time.Sleep(50 * time.Millisecond)

		activeOps = rm.GetActiveOperations()
		if len(activeOps) != 0 {
			t.Errorf("Expected 0 active operations after cancel all, got %d", len(activeOps))
		}
	})
}

func TestRetryMetrics(t *testing.T) {
	t.Run("MetricsTracking", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		// Successful operation
		successOp := func(ctx context.Context, attempt int) error {
			return nil
		}

		// Failed operation
		failOp := func(ctx context.Context, attempt int) error {
			return CreateConnectionError(1, "connect", "failed", fmt.Errorf("network error"))
		}

		ctx := context.Background()

		// Execute successful operation
		rm.ExecuteWithRetry(ctx, "success_op", 1, successOp)

		// Execute failed operation
		rm.ExecuteWithRetry(ctx, "fail_op", 2, failOp)

		metrics := rm.GetMetrics()

		if metrics.TotalOperations != 2 {
			t.Errorf("Expected 2 total operations, got %d", metrics.TotalOperations)
		}
		if metrics.SuccessfulRetries != 1 {
			t.Errorf("Expected 1 successful retry, got %d", metrics.SuccessfulRetries)
		}
		if metrics.FailedRetries != 1 {
			t.Errorf("Expected 1 failed retry, got %d", metrics.FailedRetries)
		}
		if metrics.AverageAttempts == 0 {
			t.Error("Expected non-zero average attempts")
		}
		if metrics.AverageRetryTime == 0 {
			t.Error("Expected non-zero average retry time")
		}
	})

	t.Run("ResetMetrics", func(t *testing.T) {
		rm, _ := NewRetryManager(DefaultRetryConfig())

		operation := func(ctx context.Context, attempt int) error {
			return nil
		}

		ctx := context.Background()
		rm.ExecuteWithRetry(ctx, "test_op", 1, operation)

		metrics := rm.GetMetrics()
		if metrics.TotalOperations != 1 {
			t.Error("Expected metrics to be recorded")
		}

		rm.ResetMetrics()

		metrics = rm.GetMetrics()
		if metrics.TotalOperations != 0 {
			t.Error("Expected metrics to be reset")
		}
		if metrics.SuccessfulRetries != 0 {
			t.Error("Expected successful retries to be reset")
		}
		if metrics.FailedRetries != 0 {
			t.Error("Expected failed retries to be reset")
		}
	})
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("SuccessfulOperation", func(t *testing.T) {
		operation := func(ctx context.Context, attempt int) error {
			return nil
		}

		ctx := context.Background()
		err := RetryWithBackoff(ctx, operation, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("FailedOperation", func(t *testing.T) {
		operation := func(ctx context.Context, attempt int) error {
			return fmt.Errorf("always fails")
		}

		ctx := context.Background()
		err := RetryWithBackoff(ctx, operation, nil)

		if err == nil {
			t.Error("Expected error from failed operation")
		}
	})
}

// Benchmark tests
func BenchmarkRetryExecution(b *testing.B) {
	rm, _ := NewRetryManager(DefaultRetryConfig())

	operation := func(ctx context.Context, attempt int) error {
		return nil // Always succeed
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		rm.ExecuteWithRetry(ctx, fmt.Sprintf("bench_op_%d", i), i%10, operation)
	}
}

func BenchmarkDelayCalculation(b *testing.B) {
	rm, _ := NewRetryManager(DefaultRetryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm.calculateDelay(i%10 + 1)
	}
}
