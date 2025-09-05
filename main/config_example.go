package main

import (
	"fmt"
	"log"
	"os"
)

// ConfigExample demonstrates how to use the configuration management system
func ConfigExample() {
	fmt.Println("=== IPD Configuration Management System Example ===")

	// 1. Load default configuration
	fmt.Println("\n1. Loading default configuration...")
	defaultConfig := DefaultAppConfig()
	fmt.Printf("   Default max workers: %d\n", defaultConfig.System.MaxWorkers)
	fmt.Printf("   Default log level: %s\n", defaultConfig.System.LogLevel)
	fmt.Printf("   Default chunk size: %d bytes\n", defaultConfig.FileTransmission.ChunkSize)

	// 2. Validate default configuration
	fmt.Println("\n2. Validating default configuration...")
	if err := ValidateConfig(defaultConfig); err != nil {
		log.Printf("   Validation failed: %v", err)
	} else {
		fmt.Println("   ✓ Default configuration is valid")
	}

	// 3. Create a default configuration file
	fmt.Println("\n3. Creating default configuration file...")
	configPath := "./example_config.json"
	if err := CreateDefaultConfigFile(configPath); err != nil {
		log.Printf("   Failed to create config file: %v", err)
	} else {
		fmt.Printf("   ✓ Created configuration file: %s\n", configPath)
	}

	// 4. Load configuration from file
	fmt.Println("\n4. Loading configuration from file...")
	fileConfig, err := LoadConfigFromFile(configPath)
	if err != nil {
		log.Printf("   Failed to load config from file: %v", err)
	} else {
		fmt.Printf("   ✓ Loaded configuration from file\n")
		fmt.Printf("   File max workers: %d\n", fileConfig.System.MaxWorkers)
	}

	// 5. Demonstrate environment variable override
	fmt.Println("\n5. Demonstrating environment variable override...")

	// Set some environment variables
	os.Setenv("IPD_MAX_WORKERS", "8")
	os.Setenv("IPD_LOG_LEVEL", "DEBUG")
	os.Setenv("IPD_CHUNK_SIZE", "32768")

	envConfig, err := LoadConfigFromEnv()
	if err != nil {
		log.Printf("   Failed to load config from environment: %v", err)
	} else {
		fmt.Printf("   ✓ Loaded configuration from environment\n")
		fmt.Printf("   Env max workers: %d (from IPD_MAX_WORKERS)\n", envConfig.System.MaxWorkers)
		fmt.Printf("   Env log level: %s (from IPD_LOG_LEVEL)\n", envConfig.System.LogLevel)
		fmt.Printf("   Env chunk size: %d (from IPD_CHUNK_SIZE)\n", envConfig.FileTransmission.ChunkSize)
	}

	// 6. Load combined configuration (file + environment)
	fmt.Println("\n6. Loading combined configuration (file + environment)...")
	combinedConfig, err := LoadConfig(configPath)
	if err != nil {
		log.Printf("   Failed to load combined config: %v", err)
	} else {
		fmt.Printf("   ✓ Loaded combined configuration\n")
		fmt.Printf("   Combined max workers: %d\n", combinedConfig.System.MaxWorkers)
		fmt.Printf("   Combined log level: %s\n", combinedConfig.System.LogLevel)
	}

	// 7. Demonstrate ProcessingConfig from environment
	fmt.Println("\n7. Loading ProcessingConfig from environment...")

	// Set processing-specific environment variables
	os.Setenv("IPD_PYTHON_SCRIPT_PATH", "/usr/bin/python3")
	os.Setenv("IPD_INPUT_DIRECTORY", "/data/input")
	os.Setenv("IPD_OUTPUT_DIRECTORY", "/data/output")
	os.Setenv("IPD_PROCESSING_TIMEOUT", "15m")

	processingConfig := LoadProcessingConfigFromEnv()
	fmt.Printf("   ✓ Loaded ProcessingConfig from environment\n")
	fmt.Printf("   Python path: %s\n", processingConfig.PythonScriptPath)
	fmt.Printf("   Input directory: %s\n", processingConfig.InputDirectory)
	fmt.Printf("   Output directory: %s\n", processingConfig.OutputDirectory)
	fmt.Printf("   Processing timeout: %v\n", processingConfig.ProcessingTimeout)

	// 8. Validate ProcessingConfig
	fmt.Println("\n8. Validating ProcessingConfig...")
	if err := ValidateProcessingConfig(processingConfig); err != nil {
		log.Printf("   Validation failed: %v", err)
	} else {
		fmt.Println("   ✓ ProcessingConfig is valid")
	}

	// 9. Save modified configuration
	fmt.Println("\n9. Saving modified configuration...")
	modifiedConfig := DefaultAppConfig()
	modifiedConfig.System.MaxWorkers = 16
	modifiedConfig.System.LogLevel = "DEBUG"
	modifiedConfig.FileTransmission.ChunkSize = 128 * 1024 // 128KB

	modifiedPath := "./modified_config.json"
	if err := SaveConfigToFile(modifiedConfig, modifiedPath); err != nil {
		log.Printf("   Failed to save modified config: %v", err)
	} else {
		fmt.Printf("   ✓ Saved modified configuration to: %s\n", modifiedPath)
	}

	// Clean up environment variables
	fmt.Println("\n10. Cleaning up...")
	envVarsToClean := []string{
		"IPD_MAX_WORKERS", "IPD_LOG_LEVEL", "IPD_CHUNK_SIZE",
		"IPD_PYTHON_SCRIPT_PATH", "IPD_INPUT_DIRECTORY",
		"IPD_OUTPUT_DIRECTORY", "IPD_PROCESSING_TIMEOUT",
	}

	for _, envVar := range envVarsToClean {
		os.Unsetenv(envVar)
	}

	// Clean up config files
	os.Remove(configPath)
	os.Remove(modifiedPath)

	fmt.Println("   ✓ Cleaned up environment variables and files")
	fmt.Println("\n=== Configuration Management Example Complete ===")
}

// Uncomment the main function below to run this example
/*
func main() {
	ConfigExample()
}
*/
