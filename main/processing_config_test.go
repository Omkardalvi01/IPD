package main

import (
	"os"
	"testing"
	"time"
)

func TestProcessingConfigFunctions(t *testing.T) {
	t.Run("DefaultProcessingConfig", func(t *testing.T) {
		config := DefaultProcessingConfig()

		if config.PythonScriptPath != "python3" {
			t.Errorf("Expected PythonScriptPath to be 'python3', got '%s'", config.PythonScriptPath)
		}
		if config.InputDirectory != "./input" {
			t.Errorf("Expected InputDirectory to be './input', got '%s'", config.InputDirectory)
		}
		if config.OutputDirectory != "./output" {
			t.Errorf("Expected OutputDirectory to be './output', got '%s'", config.OutputDirectory)
		}
		if config.ProcessingTimeout != 5*time.Minute {
			t.Errorf("Expected ProcessingTimeout to be 5m, got %v", config.ProcessingTimeout)
		}
		if config.HeartbeatInterval != 30*time.Second {
			t.Errorf("Expected HeartbeatInterval to be 30s, got %v", config.HeartbeatInterval)
		}
	})

	t.Run("LoadProcessingConfigFromEnv", func(t *testing.T) {
		// Save original environment
		originalEnv := make(map[string]string)
		envVars := []string{
			"IPD_PYTHON_SCRIPT_PATH", "IPD_INPUT_DIRECTORY", "IPD_OUTPUT_DIRECTORY",
			"IPD_PROCESSING_TIMEOUT", "IPD_HEARTBEAT_INTERVAL",
		}

		for _, envVar := range envVars {
			originalEnv[envVar] = os.Getenv(envVar)
		}

		// Clean up environment after test
		defer func() {
			for _, envVar := range envVars {
				if originalValue, exists := originalEnv[envVar]; exists && originalValue != "" {
					os.Setenv(envVar, originalValue)
				} else {
					os.Unsetenv(envVar)
				}
			}
		}()

		// Set test environment variables
		os.Setenv("IPD_PYTHON_SCRIPT_PATH", "/custom/python")
		os.Setenv("IPD_INPUT_DIRECTORY", "/custom/input")
		os.Setenv("IPD_OUTPUT_DIRECTORY", "/custom/output")
		os.Setenv("IPD_PROCESSING_TIMEOUT", "10m")
		os.Setenv("IPD_HEARTBEAT_INTERVAL", "60s")

		config := LoadProcessingConfigFromEnv()

		// Verify values
		if config.PythonScriptPath != "/custom/python" {
			t.Errorf("Expected PythonScriptPath '/custom/python', got '%s'", config.PythonScriptPath)
		}
		if config.InputDirectory != "/custom/input" {
			t.Errorf("Expected InputDirectory '/custom/input', got '%s'", config.InputDirectory)
		}
		if config.OutputDirectory != "/custom/output" {
			t.Errorf("Expected OutputDirectory '/custom/output', got '%s'", config.OutputDirectory)
		}
		if config.ProcessingTimeout != 10*time.Minute {
			t.Errorf("Expected ProcessingTimeout 10m, got %v", config.ProcessingTimeout)
		}
		if config.HeartbeatInterval != 60*time.Second {
			t.Errorf("Expected HeartbeatInterval 60s, got %v", config.HeartbeatInterval)
		}
	})

	t.Run("LoadProcessingConfigFromEnvWithDefaults", func(t *testing.T) {
		// Clear all environment variables
		envVars := []string{
			"IPD_PYTHON_SCRIPT_PATH", "IPD_INPUT_DIRECTORY", "IPD_OUTPUT_DIRECTORY",
			"IPD_PROCESSING_TIMEOUT", "IPD_HEARTBEAT_INTERVAL",
		}

		for _, envVar := range envVars {
			os.Unsetenv(envVar)
		}

		config := LoadProcessingConfigFromEnv()

		// Should return default values
		defaultConfig := DefaultProcessingConfig()
		if config.PythonScriptPath != defaultConfig.PythonScriptPath {
			t.Error("Should use default PythonScriptPath when env var not set")
		}
		if config.InputDirectory != defaultConfig.InputDirectory {
			t.Error("Should use default InputDirectory when env var not set")
		}
		if config.OutputDirectory != defaultConfig.OutputDirectory {
			t.Error("Should use default OutputDirectory when env var not set")
		}
		if config.ProcessingTimeout != defaultConfig.ProcessingTimeout {
			t.Error("Should use default ProcessingTimeout when env var not set")
		}
		if config.HeartbeatInterval != defaultConfig.HeartbeatInterval {
			t.Error("Should use default HeartbeatInterval when env var not set")
		}
	})

	t.Run("ValidateProcessingConfig", func(t *testing.T) {
		// Valid config
		config := DefaultProcessingConfig()
		if err := ValidateProcessingConfig(config); err != nil {
			t.Errorf("Default config should be valid, got error: %v", err)
		}

		// Invalid processing timeout
		config.ProcessingTimeout = -1 * time.Second
		if err := ValidateProcessingConfig(config); err == nil {
			t.Error("Expected validation error for negative processing timeout")
		}

		// Reset and test invalid heartbeat interval
		config = DefaultProcessingConfig()
		config.HeartbeatInterval = 0
		if err := ValidateProcessingConfig(config); err == nil {
			t.Error("Expected validation error for zero heartbeat interval")
		}

		// Reset and test empty input directory
		config = DefaultProcessingConfig()
		config.InputDirectory = ""
		if err := ValidateProcessingConfig(config); err == nil {
			t.Error("Expected validation error for empty input directory")
		}

		// Reset and test empty output directory
		config = DefaultProcessingConfig()
		config.OutputDirectory = ""
		if err := ValidateProcessingConfig(config); err == nil {
			t.Error("Expected validation error for empty output directory")
		}

		// Reset and test empty python script path
		config = DefaultProcessingConfig()
		config.PythonScriptPath = ""
		if err := ValidateProcessingConfig(config); err == nil {
			t.Error("Expected validation error for empty python script path")
		}
	})

	t.Run("LoadProcessingConfigFromEnvInvalidValues", func(t *testing.T) {
		// Save original environment
		originalEnv := make(map[string]string)
		envVars := []string{"IPD_PROCESSING_TIMEOUT", "IPD_HEARTBEAT_INTERVAL"}

		for _, envVar := range envVars {
			originalEnv[envVar] = os.Getenv(envVar)
		}

		// Clean up environment after test
		defer func() {
			for _, envVar := range envVars {
				if originalValue, exists := originalEnv[envVar]; exists && originalValue != "" {
					os.Setenv(envVar, originalValue)
				} else {
					os.Unsetenv(envVar)
				}
			}
		}()

		// Set invalid duration values
		os.Setenv("IPD_PROCESSING_TIMEOUT", "invalid")
		os.Setenv("IPD_HEARTBEAT_INTERVAL", "invalid")

		config := LoadProcessingConfigFromEnv()

		// Should use default values for invalid durations
		defaultConfig := DefaultProcessingConfig()
		if config.ProcessingTimeout != defaultConfig.ProcessingTimeout {
			t.Error("Should use default ProcessingTimeout for invalid env value")
		}
		if config.HeartbeatInterval != defaultConfig.HeartbeatInterval {
			t.Error("Should use default HeartbeatInterval for invalid env value")
		}
	})
}
