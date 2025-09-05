package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Note: ProcessingConfig is defined in worker.go to avoid circular dependencies

// SystemConfig holds system-wide configuration parameters
type SystemConfig struct {
	MaxWorkers          int           `json:"max_workers"`
	HeartbeatInterval   time.Duration `json:"heartbeat_interval"`
	ProcessingTimeout   time.Duration `json:"processing_timeout"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`
	RetryAttempts       int           `json:"retry_attempts"`
	ExternalScriptPath  string        `json:"external_script_path"`
	InputBaseDirectory  string        `json:"input_base_directory"`
	OutputBaseDirectory string        `json:"output_base_directory"`
	SignalingServerURL  string        `json:"signaling_server_url"`
	LogLevel            string        `json:"log_level"`
	EnableMetrics       bool          `json:"enable_metrics"`
	MetricsPort         int           `json:"metrics_port"`
}

// NetworkConfig holds network-related configuration
type NetworkConfig struct {
	STUNServers          []string      `json:"stun_servers"`
	TURNServers          []string      `json:"turn_servers"`
	DataChannelTimeout   time.Duration `json:"data_channel_timeout"`
	MaxReconnectAttempts int           `json:"max_reconnect_attempts"`
	ReconnectDelay       time.Duration `json:"reconnect_delay"`
}

// FileTransmissionConfig holds file transmission configuration
type FileTransmissionConfig struct {
	ChunkSize           int           `json:"chunk_size"`
	MaxConcurrentFiles  int           `json:"max_concurrent_files"`
	TransmissionTimeout time.Duration `json:"transmission_timeout"`
	RetryAttempts       int           `json:"retry_attempts"`
	EnableChecksum      bool          `json:"enable_checksum"`
}

// AppConfig represents the complete application configuration
type AppConfig struct {
	System           SystemConfig           `json:"system"`
	Network          NetworkConfig          `json:"network"`
	FileTransmission FileTransmissionConfig `json:"file_transmission"`
}

// Note: DefaultProcessingConfig is defined in worker.go

// DefaultSystemConfig returns default system configuration
func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		MaxWorkers:          4,
		HeartbeatInterval:   30 * time.Second,
		ProcessingTimeout:   10 * time.Minute,
		ConnectionTimeout:   30 * time.Second,
		RetryAttempts:       3,
		ExternalScriptPath:  "./scripts/process.py",
		InputBaseDirectory:  "./data/input",
		OutputBaseDirectory: "./data/output",
		SignalingServerURL:  "ws://localhost:8080",
		LogLevel:            "INFO",
		EnableMetrics:       false,
		MetricsPort:         9090,
	}
}

// DefaultNetworkConfig returns default network configuration
func DefaultNetworkConfig() *NetworkConfig {
	return &NetworkConfig{
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		TURNServers:          []string{},
		DataChannelTimeout:   30 * time.Second,
		MaxReconnectAttempts: 5,
		ReconnectDelay:       2 * time.Second,
	}
}

// DefaultFileTransmissionConfig returns default file transmission configuration
func DefaultFileTransmissionConfig() *FileTransmissionConfig {
	return &FileTransmissionConfig{
		ChunkSize:           64 * 1024, // 64KB chunks
		MaxConcurrentFiles:  3,
		TransmissionTimeout: 2 * time.Minute,
		RetryAttempts:       3,
		EnableChecksum:      true,
	}
}

// DefaultAppConfig returns default application configuration
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		System:           *DefaultSystemConfig(),
		Network:          *DefaultNetworkConfig(),
		FileTransmission: *DefaultFileTransmissionConfig(),
	}
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(configPath string) (*AppConfig, error) {
	// Start with default configuration
	config := DefaultAppConfig()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config file doesn't exist, return default config
		return config, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration in %s: %w", configPath, err)
	}

	return config, nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() (*AppConfig, error) {
	config := DefaultAppConfig()

	// Note: Processing configuration environment variables are handled separately
	// in worker.go to maintain separation of concerns

	// System configuration
	if val := os.Getenv("IPD_MAX_WORKERS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal > 0 {
			config.System.MaxWorkers = intVal
		}
	}
	if val := os.Getenv("IPD_CONNECTION_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.System.ConnectionTimeout = duration
		}
	}
	if val := os.Getenv("IPD_RETRY_ATTEMPTS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal >= 0 {
			config.System.RetryAttempts = intVal
		}
	}
	if val := os.Getenv("IPD_EXTERNAL_SCRIPT_PATH"); val != "" {
		config.System.ExternalScriptPath = val
	}
	if val := os.Getenv("IPD_INPUT_BASE_DIRECTORY"); val != "" {
		config.System.InputBaseDirectory = val
	}
	if val := os.Getenv("IPD_OUTPUT_BASE_DIRECTORY"); val != "" {
		config.System.OutputBaseDirectory = val
	}
	if val := os.Getenv("IPD_SIGNALING_SERVER_URL"); val != "" {
		config.System.SignalingServerURL = val
	}
	if val := os.Getenv("IPD_LOG_LEVEL"); val != "" {
		config.System.LogLevel = val
	}
	if val := os.Getenv("IPD_ENABLE_METRICS"); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			config.System.EnableMetrics = boolVal
		}
	}
	if val := os.Getenv("IPD_METRICS_PORT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal > 0 && intVal <= 65535 {
			config.System.MetricsPort = intVal
		}
	}

	// Network configuration
	if val := os.Getenv("IPD_STUN_SERVERS"); val != "" {
		// Parse comma-separated list
		servers := parseCommaSeparatedList(val)
		if len(servers) > 0 {
			config.Network.STUNServers = servers
		}
	}
	if val := os.Getenv("IPD_TURN_SERVERS"); val != "" {
		servers := parseCommaSeparatedList(val)
		config.Network.TURNServers = servers
	}
	if val := os.Getenv("IPD_DATA_CHANNEL_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Network.DataChannelTimeout = duration
		}
	}
	if val := os.Getenv("IPD_MAX_RECONNECT_ATTEMPTS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal >= 0 {
			config.Network.MaxReconnectAttempts = intVal
		}
	}
	if val := os.Getenv("IPD_RECONNECT_DELAY"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Network.ReconnectDelay = duration
		}
	}

	// File transmission configuration
	if val := os.Getenv("IPD_CHUNK_SIZE"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal > 0 {
			config.FileTransmission.ChunkSize = intVal
		}
	}
	if val := os.Getenv("IPD_MAX_CONCURRENT_FILES"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal > 0 {
			config.FileTransmission.MaxConcurrentFiles = intVal
		}
	}
	if val := os.Getenv("IPD_TRANSMISSION_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.FileTransmission.TransmissionTimeout = duration
		}
	}
	if val := os.Getenv("IPD_FILE_RETRY_ATTEMPTS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil && intVal >= 0 {
			config.FileTransmission.RetryAttempts = intVal
		}
	}
	if val := os.Getenv("IPD_ENABLE_CHECKSUM"); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			config.FileTransmission.EnableChecksum = boolVal
		}
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid environment configuration: %w", err)
	}

	return config, nil
}

// LoadConfig loads configuration from file first, then overrides with environment variables
func LoadConfig(configPath string) (*AppConfig, error) {
	// Load from file first
	config, err := LoadConfigFromFile(configPath)
	if err != nil {
		return nil, err
	}

	// Override with environment variables
	envConfig, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}

	// Merge configurations (environment takes precedence)
	mergedConfig := mergeConfigs(config, envConfig)

	return mergedConfig, nil
}

// SaveConfigToFile saves configuration to a JSON file
func SaveConfigToFile(config *AppConfig, configPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfig validates the configuration parameters
func ValidateConfig(config *AppConfig) error {
	// Note: Processing configuration validation is handled in worker.go

	// Validate system configuration
	if config.System.MaxWorkers <= 0 {
		return fmt.Errorf("max workers must be positive")
	}
	if config.System.MaxWorkers > 100 {
		return fmt.Errorf("max workers cannot exceed 100")
	}
	if config.System.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection timeout must be positive")
	}
	if config.System.RetryAttempts < 0 {
		return fmt.Errorf("retry attempts cannot be negative")
	}
	if config.System.RetryAttempts > 10 {
		return fmt.Errorf("retry attempts cannot exceed 10")
	}
	if config.System.SignalingServerURL == "" {
		return fmt.Errorf("signaling server URL cannot be empty")
	}
	if config.System.LogLevel != "" {
		validLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
		valid := false
		for _, level := range validLevels {
			if config.System.LogLevel == level {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid log level: %s (must be one of: DEBUG, INFO, WARN, ERROR)", config.System.LogLevel)
		}
	}
	if config.System.MetricsPort <= 0 || config.System.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535")
	}

	// Validate network configuration
	if len(config.Network.STUNServers) == 0 {
		return fmt.Errorf("at least one STUN server must be configured")
	}
	if config.Network.DataChannelTimeout <= 0 {
		return fmt.Errorf("data channel timeout must be positive")
	}
	if config.Network.MaxReconnectAttempts < 0 {
		return fmt.Errorf("max reconnect attempts cannot be negative")
	}
	if config.Network.ReconnectDelay <= 0 {
		return fmt.Errorf("reconnect delay must be positive")
	}

	// Validate file transmission configuration
	if config.FileTransmission.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	if config.FileTransmission.ChunkSize > 1024*1024 { // 1MB max
		return fmt.Errorf("chunk size cannot exceed 1MB")
	}
	if config.FileTransmission.MaxConcurrentFiles <= 0 {
		return fmt.Errorf("max concurrent files must be positive")
	}
	if config.FileTransmission.TransmissionTimeout <= 0 {
		return fmt.Errorf("transmission timeout must be positive")
	}
	if config.FileTransmission.RetryAttempts < 0 {
		return fmt.Errorf("file transmission retry attempts cannot be negative")
	}

	return nil
}

// parseCommaSeparatedList parses a comma-separated string into a slice
func parseCommaSeparatedList(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// mergeConfigs merges two configurations, with the second taking precedence
func mergeConfigs(base, override *AppConfig) *AppConfig {
	// For simplicity, we'll return the override config since environment
	// variables should take precedence over file configuration
	// In a more sophisticated implementation, we could merge field by field
	return override
}

// GetConfigPath returns the default configuration file path
func GetConfigPath() string {
	// Check for custom config path in environment
	if path := os.Getenv("IPD_CONFIG_PATH"); path != "" {
		return path
	}

	// Default to config.json in current directory
	return "./config.json"
}

// CreateDefaultConfigFile creates a default configuration file if it doesn't exist
func CreateDefaultConfigFile(configPath string) error {
	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		return nil // File already exists
	}

	// Create default configuration
	defaultConfig := DefaultAppConfig()

	// Save to file
	return SaveConfigToFile(defaultConfig, configPath)
}
