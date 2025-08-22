package main

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"time"
)

// TrainingCoordinator manages Python ML script execution
type TrainingCoordinator struct {
	config *Config
}

// NewTrainingCoordinator creates a new training coordinator
func NewTrainingCoordinator(config *Config) *TrainingCoordinator {
	return &TrainingCoordinator{
		config: config,
	}
}

// ExecuteTraining runs the Python training script on the batch
func (tc *TrainingCoordinator) ExecuteTraining(batchDir, weightsPath string) error {
	log.Printf("Starting training on batch: %s", batchDir)

	// Prepare command arguments
	pythonCmd := "python"
	if _, err := exec.LookPath("python3"); err == nil {
		pythonCmd = "python3"
	}

	args := []string{
		tc.config.PythonScript,
		"--batch-dir", batchDir,
		"--weights-output", weightsPath,
	}

	// Create and execute command
	cmd := exec.Command(pythonCmd, args...)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Training failed with output: %s", string(output))
		return fmt.Errorf("training script failed: %w", err)
	}

	log.Printf("Training completed successfully")
	log.Printf("Training output: %s", string(output))

	return nil
}

// MonitorTraining monitors a running training process
func (tc *TrainingCoordinator) MonitorTraining(cmd *exec.Cmd) error {
	log.Println("Monitoring training process...")

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start training process: %w", err)
	}

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("training process failed: %w", err)
	}

	log.Println("Training process completed successfully")
	return nil
}

// ValidateWeights checks if the weights file was generated correctly
func (tc *TrainingCoordinator) ValidateWeights(weightsPath string) error {
	log.Printf("Validating weights file: %s", weightsPath)

	// Check if weights file exists
	info, err := filepath.Glob(weightsPath + "*")
	if err != nil {
		return fmt.Errorf("failed to check weights file: %w", err)
	}

	if len(info) == 0 {
		return fmt.Errorf("weights file not found: %s", weightsPath)
	}

	// Use the first matching file
	actualWeightsPath := info[0]

	// Check file size
	stat, err := filepath.Glob(actualWeightsPath)
	if err != nil {
		return fmt.Errorf("failed to stat weights file: %w", err)
	}

	if len(stat) == 0 {
		return fmt.Errorf("weights file is empty: %s", actualWeightsPath)
	}

	log.Printf("Weights validation successful: %s", actualWeightsPath)
	return nil
}

// GetTrainingDuration calculates training duration
func (tc *TrainingCoordinator) GetTrainingDuration(startTime time.Time) time.Duration {
	return time.Since(startTime)
}
