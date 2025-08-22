package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// DataHandler manages image batch reception and storage
type DataHandler struct {
	config *Config
}

// NewDataHandler creates a new data handler
func NewDataHandler(config *Config) *DataHandler {
	return &DataHandler{
		config: config,
	}
}

// ReceiveImageBatch handles receiving and storing image batches
func (dh *DataHandler) ReceiveImageBatch(batchID string) (string, error) {
	log.Printf("Starting to receive image batch: %s", batchID)

	// Create batch directory
	batchDir := filepath.Join(dh.config.BatchDirectory, batchID)
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create batch directory: %w", err)
	}

	log.Printf("Created batch directory: %s", batchDir)
	return batchDir, nil
}

// ValidateImageBatch validates the received image batch
func (dh *DataHandler) ValidateImageBatch(batchDir string) error {
	log.Printf("Validating image batch in: %s", batchDir)

	// Check if directory exists
	if _, err := os.Stat(batchDir); os.IsNotExist(err) {
		return fmt.Errorf("batch directory does not exist: %s", batchDir)
	}

	// Count files in batch directory
	files, err := os.ReadDir(batchDir)
	if err != nil {
		return fmt.Errorf("failed to read batch directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("batch directory is empty: %s", batchDir)
	}

	log.Printf("Batch validation successful: %d files found", len(files))
	return nil
}

// CleanupBatch removes the processed batch directory
func (dh *DataHandler) CleanupBatch(batchDir string) error {
	log.Printf("Cleaning up batch directory: %s", batchDir)

	if err := os.RemoveAll(batchDir); err != nil {
		return fmt.Errorf("failed to cleanup batch directory: %w", err)
	}

	log.Printf("Batch cleanup completed: %s", batchDir)
	return nil
}

// GetBatchStats returns statistics about a batch
func (dh *DataHandler) GetBatchStats(batchDir string) (int, int64, error) {
	files, err := os.ReadDir(batchDir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read batch directory: %w", err)
	}

	var totalSize int64
	fileCount := 0

	for _, file := range files {
		if !file.IsDir() {
			info, err := file.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
			fileCount++
		}
	}

	return fileCount, totalSize, nil
}
