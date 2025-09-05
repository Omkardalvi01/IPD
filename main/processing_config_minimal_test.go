package main

import (
	"os"
	"testing"
	"time"
)

// Minimal ProcessingConfig for testing (avoiding worker.go dependencies)
type TestProcessingConfig struct {
	PythonScriptPath  string
	InputDirectory    string
	OutputDirectory   string
	ProcessingTimeout time.Duration
	HeartbeatInterval time.Duration
}

// defaultTestProcessingConfig returns default processing configuration for testing
func defaultTestProcessingConfig() *TestProcessingConfig {
	return &TestProcessingConfig{
		PythonScriptPath:  "python3",
		InputDirectory:    "./input",
		OutputDirectory:   "./output",
		ProcessingTimeout: 5 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
	}
}

// loadTestProcessingConfigFromEnv loads ProcessingConfig from environment variables for testing
func loadTestProcessingConfigFromEnv() *TestProcessingConfig {
	config := defaultTestProcessingConfig()

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

func TestProcessingConfigEnvironmentLoading(t *testing.T) {
	t.Run("LoadFromEnvironment", func(t *testing.T) {
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

		config := loadTestProcessingConfigFromEnv()

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

	t.Run("LoadWithDefaults", func(t *testing.T) {
		// Clear all environment variables
		envVars := []string{
			"IPD_PYTHON_SCRIPT_PATH", "IPD_INPUT_DIRECTORY", "IPD_OUTPUT_DIRECTORY",
			"IPD_PROCESSING_TIMEOUT", "IPD_HEARTBEAT_INTERVAL",
		}

		originalEnv := make(map[string]string)
		for _, envVar := range envVars {
			originalEnv[envVar] = os.Getenv(envVar)
			os.Unsetenv(envVar)
		}

		// Clean up environment after test
		defer func() {
			for _, envVar := range envVars {
				if originalValue, exists := originalEnv[envVar]; exists && originalValue != "" {
					os.Setenv(envVar, originalValue)
				}
			}
		}()

		config := loadTestProcessingConfigFromEnv()

		// Should return default values
		defaultConfig := defaultTestProcessingConfig()
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

	t.Run("LoadWithInvalidValues", func(t *testing.T) {
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

		config := loadTestProcessingConfigFromEnv()

		// Should use default values for invalid durations
		defaultConfig := defaultTestProcessingConfig()
		if config.ProcessingTimeout != defaultConfig.ProcessingTimeout {
			t.Error("Should use default ProcessingTimeout for invalid env value")
		}
		if config.HeartbeatInterval != defaultConfig.HeartbeatInterval {
			t.Error("Should use default HeartbeatInterval for invalid env value")
		}
	})
}
