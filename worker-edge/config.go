package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LoadConfig loads configuration from a JSON file
func LoadConfig(configPath string) (*Config, error) {
	// Start with default configuration
	config := DefaultConfig()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Config file %s not found, using defaults\n", configPath)
		return config, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// ValidateEnvironment checks if the required environment is available
func ValidateEnvironment(config *Config) error {
	// Check if Python is available
	if _, err := exec.LookPath("python"); err != nil {
		if _, err := exec.LookPath("python3"); err != nil {
			return fmt.Errorf("python interpreter not found in PATH")
		}
	}

	// Check if Python script exists
	if _, err := os.Stat(config.PythonScript); os.IsNotExist(err) {
		return fmt.Errorf("python script not found: %s", config.PythonScript)
	}

	// Create batch directory if it doesn't exist
	if err := os.MkdirAll(config.BatchDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create batch directory: %w", err)
	}

	// Create weights directory if it doesn't exist
	weightsDir := filepath.Dir(config.WeightsPath)
	if err := os.MkdirAll(weightsDir, 0755); err != nil {
		return fmt.Errorf("failed to create weights directory: %w", err)
	}

	return nil
}

// SaveConfig saves the current configuration to a file
func SaveConfig(config *Config, configPath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
