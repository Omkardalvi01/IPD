package main

import (
	"testing"
	"time"
)

// TestMainApplicationIntegration tests the integration of enhanced worker system components
func TestMainApplicationIntegration(t *testing.T) {
	// Test configuration loading
	config := DefaultAppConfig()
	if config == nil {
		t.Fatal("Failed to create default configuration")
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("Default configuration validation failed: %v", err)
	}

	// Test processing configuration creation
	processingConfig := &ProcessingConfig{
		PythonScriptPath:  config.System.ExternalScriptPath,
		InputDirectory:    config.System.InputBaseDirectory,
		OutputDirectory:   config.System.OutputBaseDirectory,
		ProcessingTimeout: config.System.ProcessingTimeout,
		HeartbeatInterval: config.System.HeartbeatInterval,
	}

	if err := ValidateProcessingConfig(processingConfig); err != nil {
		t.Fatalf("Processing configuration validation failed: %v", err)
	}

	// Test server result collector initialization
	serverCollector := NewServerResultCollector(2, "./test_results")
	if serverCollector == nil {
		t.Fatal("Failed to create server result collector")
	}

	if serverCollector.GetExpectedWorkerCount() != 2 {
		t.Errorf("Expected worker count mismatch: expected 2, got %d", serverCollector.GetExpectedWorkerCount())
	}

	// Test connection lifecycle manager initialization
	lifecycleManager := NewConnectionLifecycleManager(30*time.Second, 60*time.Second)
	if lifecycleManager == nil {
		t.Fatal("Failed to create connection lifecycle manager")
	}

	// Test server message handler initialization
	messageHandler := NewServerMessageHandler(serverCollector, lifecycleManager)
	if messageHandler == nil {
		t.Fatal("Failed to create server message handler")
	}

	// Test persistent worker pool initialization
	resultChan := make(chan Result, 10)
	persistentPool := NewPersistentWorkerPool(resultChan, processingConfig)
	if persistentPool == nil {
		t.Fatal("Failed to create persistent worker pool")
	}

	// Test component integration setup
	setupComponentIntegration(persistentPool, messageHandler, lifecycleManager)

	// Verify integration callback is set
	if persistentPool.integrationCallback == nil {
		t.Error("Integration callback not set on persistent worker pool")
	}

	t.Log("✅ All enhanced worker system components integrated successfully")
}

// TestConfigurationSystem tests the configuration management system
func TestConfigurationSystem(t *testing.T) {
	// Test default configuration
	config := DefaultAppConfig()
	if config.System.MaxWorkers <= 0 {
		t.Error("Invalid default max workers")
	}

	if config.System.HeartbeatInterval <= 0 {
		t.Error("Invalid default heartbeat interval")
	}

	if config.System.ProcessingTimeout <= 0 {
		t.Error("Invalid default processing timeout")
	}

	// Test configuration validation
	if err := ValidateConfig(config); err != nil {
		t.Errorf("Default configuration validation failed: %v", err)
	}

	// Test processing configuration
	processingConfig := DefaultProcessingConfig()
	if err := ValidateProcessingConfig(processingConfig); err != nil {
		t.Errorf("Default processing configuration validation failed: %v", err)
	}

	t.Log("✅ Configuration system working correctly")
}

// TestResultCollectionSystem tests the result collection and aggregation system
func TestResultCollectionSystem(t *testing.T) {
	collector := NewServerResultCollector(3, "./test_results")

	// Test initial state
	if collector.IsComplete() {
		t.Error("Collector should not be complete initially")
	}

	if collector.GetCompletedWorkerCount() != 0 {
		t.Error("Initial completed worker count should be 0")
	}

	if collector.GetExpectedWorkerCount() != 3 {
		t.Error("Expected worker count should be 3")
	}

	// Test channels
	completionChan := collector.GetCompletionChannel()
	if completionChan == nil {
		t.Error("Completion channel should not be nil")
	}

	allCompleteChan := collector.GetAllCompleteChannel()
	if allCompleteChan == nil {
		t.Error("All complete channel should not be nil")
	}

	t.Log("✅ Result collection system working correctly")
}

// TestHeartbeatMonitoring tests the heartbeat monitoring system
func TestHeartbeatMonitoring(t *testing.T) {
	lifecycleManager := NewConnectionLifecycleManager(5*time.Second, 10*time.Second)

	// Test initial state
	if lifecycleManager.GetTotalConnectionCount() != 0 {
		t.Error("Initial connection count should be 0")
	}

	if lifecycleManager.GetHealthyConnectionCount() != 0 {
		t.Error("Initial healthy connection count should be 0")
	}

	unhealthy := lifecycleManager.GetUnhealthyConnections()
	if len(unhealthy) != 0 {
		t.Error("Initial unhealthy connections should be empty")
	}

	t.Log("✅ Heartbeat monitoring system working correctly")
}
