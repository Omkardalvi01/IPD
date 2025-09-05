package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfigurations(t *testing.T) {
	// Note: ProcessingConfig tests are in worker_test.go to avoid circular dependencies

	t.Run("DefaultSystemConfig", func(t *testing.T) {
		config := DefaultSystemConfig()

		if config.MaxWorkers != 4 {
			t.Errorf("Expected MaxWorkers to be 4, got %d", config.MaxWorkers)
		}
		if config.HeartbeatInterval != 30*time.Second {
			t.Errorf("Expected HeartbeatInterval to be 30s, got %v", config.HeartbeatInterval)
		}
		if config.ProcessingTimeout != 10*time.Minute {
			t.Errorf("Expected ProcessingTimeout to be 10m, got %v", config.ProcessingTimeout)
		}
		if config.ConnectionTimeout != 30*time.Second {
			t.Errorf("Expected ConnectionTimeout to be 30s, got %v", config.ConnectionTimeout)
		}
		if config.RetryAttempts != 3 {
			t.Errorf("Expected RetryAttempts to be 3, got %d", config.RetryAttempts)
		}
		if config.SignalingServerURL != "ws://localhost:8080" {
			t.Errorf("Expected SignalingServerURL to be 'ws://localhost:8080', got '%s'", config.SignalingServerURL)
		}
		if config.LogLevel != "INFO" {
			t.Errorf("Expected LogLevel to be 'INFO', got '%s'", config.LogLevel)
		}
		if config.EnableMetrics != false {
			t.Errorf("Expected EnableMetrics to be false, got %v", config.EnableMetrics)
		}
		if config.MetricsPort != 9090 {
			t.Errorf("Expected MetricsPort to be 9090, got %d", config.MetricsPort)
		}
	})

	t.Run("DefaultNetworkConfig", func(t *testing.T) {
		config := DefaultNetworkConfig()

		expectedSTUNServers := []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		}
		if len(config.STUNServers) != len(expectedSTUNServers) {
			t.Errorf("Expected %d STUN servers, got %d", len(expectedSTUNServers), len(config.STUNServers))
		}
		for i, expected := range expectedSTUNServers {
			if i < len(config.STUNServers) && config.STUNServers[i] != expected {
				t.Errorf("Expected STUN server %d to be '%s', got '%s'", i, expected, config.STUNServers[i])
			}
		}
		if len(config.TURNServers) != 0 {
			t.Errorf("Expected no TURN servers by default, got %d", len(config.TURNServers))
		}
		if config.DataChannelTimeout != 30*time.Second {
			t.Errorf("Expected DataChannelTimeout to be 30s, got %v", config.DataChannelTimeout)
		}
		if config.MaxReconnectAttempts != 5 {
			t.Errorf("Expected MaxReconnectAttempts to be 5, got %d", config.MaxReconnectAttempts)
		}
		if config.ReconnectDelay != 2*time.Second {
			t.Errorf("Expected ReconnectDelay to be 2s, got %v", config.ReconnectDelay)
		}
	})

	t.Run("DefaultFileTransmissionConfig", func(t *testing.T) {
		config := DefaultFileTransmissionConfig()

		if config.ChunkSize != 64*1024 {
			t.Errorf("Expected ChunkSize to be 65536, got %d", config.ChunkSize)
		}
		if config.MaxConcurrentFiles != 3 {
			t.Errorf("Expected MaxConcurrentFiles to be 3, got %d", config.MaxConcurrentFiles)
		}
		if config.TransmissionTimeout != 2*time.Minute {
			t.Errorf("Expected TransmissionTimeout to be 2m, got %v", config.TransmissionTimeout)
		}
		if config.RetryAttempts != 3 {
			t.Errorf("Expected RetryAttempts to be 3, got %d", config.RetryAttempts)
		}
		if config.EnableChecksum != true {
			t.Errorf("Expected EnableChecksum to be true, got %v", config.EnableChecksum)
		}
	})

	t.Run("DefaultAppConfig", func(t *testing.T) {
		config := DefaultAppConfig()

		// Verify all sub-configurations are present
		if config.System.MaxWorkers == 0 {
			t.Error("System configuration not properly initialized")
		}
		if len(config.Network.STUNServers) == 0 {
			t.Error("Network configuration not properly initialized")
		}
		if config.FileTransmission.ChunkSize == 0 {
			t.Error("FileTransmission configuration not properly initialized")
		}
	})
}

func TestConfigValidation(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		config := DefaultAppConfig()
		if err := ValidateConfig(config); err != nil {
			t.Errorf("Default config should be valid, got error: %v", err)
		}
	})

	t.Run("InvalidMaxWorkers", func(t *testing.T) {
		config := DefaultAppConfig()
		config.System.MaxWorkers = 0
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for zero max workers")
		}

		config.System.MaxWorkers = 101
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for too many max workers")
		}
	})

	t.Run("InvalidRetryAttempts", func(t *testing.T) {
		config := DefaultAppConfig()
		config.System.RetryAttempts = -1
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for negative retry attempts")
		}

		config.System.RetryAttempts = 11
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for too many retry attempts")
		}
	})

	t.Run("EmptySignalingServerURL", func(t *testing.T) {
		config := DefaultAppConfig()
		config.System.SignalingServerURL = ""
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for empty signaling server URL")
		}
	})

	t.Run("InvalidLogLevel", func(t *testing.T) {
		config := DefaultAppConfig()
		config.System.LogLevel = "INVALID"
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for invalid log level")
		}
	})

	t.Run("InvalidMetricsPort", func(t *testing.T) {
		config := DefaultAppConfig()
		config.System.MetricsPort = 0
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for zero metrics port")
		}

		config.System.MetricsPort = 65536
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for metrics port too high")
		}
	})

	t.Run("NoSTUNServers", func(t *testing.T) {
		config := DefaultAppConfig()
		config.Network.STUNServers = []string{}
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for no STUN servers")
		}
	})

	t.Run("InvalidChunkSize", func(t *testing.T) {
		config := DefaultAppConfig()
		config.FileTransmission.ChunkSize = 0
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for zero chunk size")
		}

		config.FileTransmission.ChunkSize = 2 * 1024 * 1024 // 2MB
		if err := ValidateConfig(config); err == nil {
			t.Error("Expected validation error for chunk size too large")
		}
	})
}

func TestFileOperations(t *testing.T) {
	// Create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "test_config.json")

	t.Run("SaveAndLoadConfig", func(t *testing.T) {
		originalConfig := DefaultAppConfig()
		originalConfig.System.MaxWorkers = 8
		originalConfig.System.SignalingServerURL = "ws://custom:8080"

		// Save config
		if err := SaveConfigToFile(originalConfig, configPath); err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}

		// Load config
		loadedConfig, err := LoadConfigFromFile(configPath)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Verify values
		if loadedConfig.System.MaxWorkers != 8 {
			t.Errorf("Expected MaxWorkers to be 8, got %d", loadedConfig.System.MaxWorkers)
		}
		if loadedConfig.System.SignalingServerURL != "ws://custom:8080" {
			t.Errorf("Expected SignalingServerURL to be 'ws://custom:8080', got '%s'", loadedConfig.System.SignalingServerURL)
		}
	})

	t.Run("LoadNonExistentConfig", func(t *testing.T) {
		nonExistentPath := filepath.Join(tempDir, "nonexistent.json")
		config, err := LoadConfigFromFile(nonExistentPath)
		if err != nil {
			t.Fatalf("Loading non-existent config should return default config, got error: %v", err)
		}

		// Should return default config
		defaultConfig := DefaultAppConfig()
		if config.System.MaxWorkers != defaultConfig.System.MaxWorkers {
			t.Error("Non-existent config should return default values")
		}
	})

	t.Run("LoadInvalidJSON", func(t *testing.T) {
		invalidJSONPath := filepath.Join(tempDir, "invalid.json")
		if err := os.WriteFile(invalidJSONPath, []byte("invalid json"), 0644); err != nil {
			t.Fatalf("Failed to write invalid JSON file: %v", err)
		}

		_, err := LoadConfigFromFile(invalidJSONPath)
		if err == nil {
			t.Error("Expected error when loading invalid JSON")
		}
	})

	t.Run("CreateDefaultConfigFile", func(t *testing.T) {
		defaultConfigPath := filepath.Join(tempDir, "default_config.json")

		if err := CreateDefaultConfigFile(defaultConfigPath); err != nil {
			t.Fatalf("Failed to create default config file: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
			t.Error("Default config file was not created")
		}

		// Load and verify it's valid
		config, err := LoadConfigFromFile(defaultConfigPath)
		if err != nil {
			t.Fatalf("Failed to load created default config: %v", err)
		}

		if err := ValidateConfig(config); err != nil {
			t.Errorf("Created default config is invalid: %v", err)
		}
	})
}

func TestEnvironmentVariables(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"IPD_MAX_WORKERS", "IPD_CONNECTION_TIMEOUT", "IPD_RETRY_ATTEMPTS",
		"IPD_SIGNALING_SERVER_URL", "IPD_LOG_LEVEL", "IPD_ENABLE_METRICS",
		"IPD_METRICS_PORT", "IPD_STUN_SERVERS", "IPD_CHUNK_SIZE",
		"IPD_MAX_CONCURRENT_FILES",
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

	t.Run("LoadFromEnvironment", func(t *testing.T) {
		// Set test environment variables
		os.Setenv("IPD_MAX_WORKERS", "8")
		os.Setenv("IPD_CONNECTION_TIMEOUT", "45s")
		os.Setenv("IPD_RETRY_ATTEMPTS", "5")
		os.Setenv("IPD_SIGNALING_SERVER_URL", "ws://custom:8080")
		os.Setenv("IPD_LOG_LEVEL", "DEBUG")
		os.Setenv("IPD_ENABLE_METRICS", "true")
		os.Setenv("IPD_METRICS_PORT", "9091")
		os.Setenv("IPD_STUN_SERVERS", "stun:custom1.com:19302,stun:custom2.com:19302")
		os.Setenv("IPD_CHUNK_SIZE", "32768")
		os.Setenv("IPD_MAX_CONCURRENT_FILES", "5")

		config, err := LoadConfigFromEnv()
		if err != nil {
			t.Fatalf("Failed to load config from environment: %v", err)
		}

		// Verify values
		if config.System.MaxWorkers != 8 {
			t.Errorf("Expected MaxWorkers 8, got %d", config.System.MaxWorkers)
		}
		if config.System.ConnectionTimeout != 45*time.Second {
			t.Errorf("Expected ConnectionTimeout 45s, got %v", config.System.ConnectionTimeout)
		}
		if config.System.RetryAttempts != 5 {
			t.Errorf("Expected RetryAttempts 5, got %d", config.System.RetryAttempts)
		}
		if config.System.SignalingServerURL != "ws://custom:8080" {
			t.Errorf("Expected SignalingServerURL 'ws://custom:8080', got '%s'", config.System.SignalingServerURL)
		}
		if config.System.LogLevel != "DEBUG" {
			t.Errorf("Expected LogLevel 'DEBUG', got '%s'", config.System.LogLevel)
		}
		if config.System.EnableMetrics != true {
			t.Errorf("Expected EnableMetrics true, got %v", config.System.EnableMetrics)
		}
		if config.System.MetricsPort != 9091 {
			t.Errorf("Expected MetricsPort 9091, got %d", config.System.MetricsPort)
		}
		if len(config.Network.STUNServers) != 2 {
			t.Errorf("Expected 2 STUN servers, got %d", len(config.Network.STUNServers))
		}
		if config.FileTransmission.ChunkSize != 32768 {
			t.Errorf("Expected ChunkSize 32768, got %d", config.FileTransmission.ChunkSize)
		}
		if config.FileTransmission.MaxConcurrentFiles != 5 {
			t.Errorf("Expected MaxConcurrentFiles 5, got %d", config.FileTransmission.MaxConcurrentFiles)
		}
	})

	t.Run("InvalidEnvironmentValues", func(t *testing.T) {
		// Set invalid values
		os.Setenv("IPD_MAX_WORKERS", "invalid")
		os.Setenv("IPD_CONNECTION_TIMEOUT", "invalid")
		os.Setenv("IPD_ENABLE_METRICS", "invalid")

		config, err := LoadConfigFromEnv()
		if err != nil {
			t.Fatalf("LoadConfigFromEnv should handle invalid values gracefully: %v", err)
		}

		// Should use default values for invalid entries
		defaultConfig := DefaultAppConfig()
		if config.System.MaxWorkers != defaultConfig.System.MaxWorkers {
			t.Error("Invalid environment values should fall back to defaults")
		}
	})
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("ParseCommaSeparatedList", func(t *testing.T) {
		tests := []struct {
			input    string
			expected []string
		}{
			{"", []string{}},
			{"single", []string{"single"}},
			{"one,two,three", []string{"one", "two", "three"}},
			{"one, two , three ", []string{"one", "two", "three"}},
			{"one,,three", []string{"one", "three"}},
			{" , , ", []string{}},
		}

		for _, test := range tests {
			result := parseCommaSeparatedList(test.input)
			if len(result) != len(test.expected) {
				t.Errorf("Input '%s': expected %d items, got %d", test.input, len(test.expected), len(result))
				continue
			}
			for i, expected := range test.expected {
				if result[i] != expected {
					t.Errorf("Input '%s': expected item %d to be '%s', got '%s'", test.input, i, expected, result[i])
				}
			}
		}
	})

	t.Run("GetConfigPath", func(t *testing.T) {
		// Test default path
		originalEnv := os.Getenv("IPD_CONFIG_PATH")
		os.Unsetenv("IPD_CONFIG_PATH")

		defaultPath := GetConfigPath()
		if defaultPath != "./config.json" {
			t.Errorf("Expected default config path './config.json', got '%s'", defaultPath)
		}

		// Test custom path
		os.Setenv("IPD_CONFIG_PATH", "/custom/config.json")
		customPath := GetConfigPath()
		if customPath != "/custom/config.json" {
			t.Errorf("Expected custom config path '/custom/config.json', got '%s'", customPath)
		}

		// Restore original environment
		if originalEnv != "" {
			os.Setenv("IPD_CONFIG_PATH", originalEnv)
		} else {
			os.Unsetenv("IPD_CONFIG_PATH")
		}
	})
}

func TestJSONSerialization(t *testing.T) {
	t.Run("SerializeDeserialize", func(t *testing.T) {
		originalConfig := DefaultAppConfig()
		originalConfig.System.MaxWorkers = 16
		originalConfig.System.SignalingServerURL = "ws://test:8080"

		// Serialize to JSON
		jsonData, err := json.Marshal(originalConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config to JSON: %v", err)
		}

		// Deserialize from JSON
		var deserializedConfig AppConfig
		if err := json.Unmarshal(jsonData, &deserializedConfig); err != nil {
			t.Fatalf("Failed to unmarshal config from JSON: %v", err)
		}

		// Verify values
		if deserializedConfig.System.MaxWorkers != 16 {
			t.Errorf("Expected MaxWorkers 16, got %d", deserializedConfig.System.MaxWorkers)
		}
		if deserializedConfig.System.SignalingServerURL != "ws://test:8080" {
			t.Errorf("Expected SignalingServerURL 'ws://test:8080', got '%s'", deserializedConfig.System.SignalingServerURL)
		}
	})
}
