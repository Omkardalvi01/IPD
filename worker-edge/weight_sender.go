package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pion/webrtc/v3"
)

// WeightSender handles sending trained weights back to master edge
type WeightSender struct {
	config *Config
}

// NewWeightSender creates a new weight sender
func NewWeightSender(config *Config) *WeightSender {
	return &WeightSender{
		config: config,
	}
}

// SendWeights sends the weights file to the master edge
func (ws *WeightSender) SendWeights(dc *webrtc.DataChannel, weightsPath string) error {
	log.Printf("Sending weights from: %s", weightsPath)

	// Find the actual weights file (may have extensions)
	weightsFiles, err := filepath.Glob(weightsPath + "*")
	if err != nil {
		return fmt.Errorf("failed to find weights file: %w", err)
	}

	if len(weightsFiles) == 0 {
		return fmt.Errorf("no weights file found matching: %s", weightsPath)
	}

	// Use the first matching file
	actualWeightsPath := weightsFiles[0]

	// Read weights file
	weightsData, err := ws.ReadWeightsFile(actualWeightsPath)
	if err != nil {
		return fmt.Errorf("failed to read weights file: %w", err)
	}

	// Send weights data
	if err := dc.Send(weightsData); err != nil {
		return fmt.Errorf("failed to send weights data: %w", err)
	}

	log.Printf("Weights sent successfully: %d bytes", len(weightsData))
	return nil
}

// ReadWeightsFile reads the weights file and returns its contents
func (ws *WeightSender) ReadWeightsFile(weightsPath string) ([]byte, error) {
	log.Printf("Reading weights file: %s", weightsPath)

	// Check if file exists
	if _, err := os.Stat(weightsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("weights file does not exist: %s", weightsPath)
	}

	// Read file contents
	data, err := os.ReadFile(weightsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read weights file: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("weights file is empty: %s", weightsPath)
	}

	log.Printf("Weights file read successfully: %d bytes", len(data))
	return data, nil
}

// SignalTrainingComplete sends a training completion signal to master edge
func (ws *WeightSender) SignalTrainingComplete(dc *webrtc.DataChannel) error {
	log.Println("Signaling training completion to master edge")

	message := "TRAINING_COMPLETE"
	if err := dc.SendText(message); err != nil {
		return fmt.Errorf("failed to send training complete signal: %w", err)
	}

	log.Println("Training completion signal sent successfully")
	return nil
}

// GetWeightsFileInfo returns information about the weights file
func (ws *WeightSender) GetWeightsFileInfo(weightsPath string) (string, int64, error) {
	// Find the actual weights file
	weightsFiles, err := filepath.Glob(weightsPath + "*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to find weights file: %w", err)
	}

	if len(weightsFiles) == 0 {
		return "", 0, fmt.Errorf("no weights file found")
	}

	actualPath := weightsFiles[0]
	info, err := os.Stat(actualPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat weights file: %w", err)
	}

	return actualPath, info.Size(), nil
}
