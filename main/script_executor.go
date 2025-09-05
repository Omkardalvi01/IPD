package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// ScriptExecutor handles external script execution with mock implementation
type ScriptExecutor struct {
	inputDir        string
	outputDir       string
	timeout         time.Duration
	isRunning       bool
	runningMutex    sync.RWMutex
	stopChan        chan struct{}
	watcherStopChan chan struct{}
}

// ExecutionResult represents the result of script execution
type ExecutionResult struct {
	Success     bool
	Duration    time.Duration
	OutputFiles []string
	ErrorMsg    string
	InputFiles  []string
}

// NewScriptExecutor creates a new ScriptExecutor instance
func NewScriptExecutor(inputDir, outputDir string, timeout time.Duration) *ScriptExecutor {
	return &ScriptExecutor{
		inputDir:        inputDir,
		outputDir:       outputDir,
		timeout:         timeout,
		isRunning:       false,
		stopChan:        make(chan struct{}),
		watcherStopChan: make(chan struct{}),
	}
}

// Execute performs mock processing by copying/renaming files to simulate processing
func (se *ScriptExecutor) Execute() (*ExecutionResult, error) {
	se.runningMutex.Lock()
	if se.isRunning {
		se.runningMutex.Unlock()
		return nil, fmt.Errorf("script executor is already running")
	}
	se.isRunning = true
	se.runningMutex.Unlock()

	defer func() {
		se.runningMutex.Lock()
		se.isRunning = false
		se.runningMutex.Unlock()
	}()

	startTime := time.Now()
	log.Printf("Starting mock script execution - Input: %s, Output: %s", se.inputDir, se.outputDir)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(se.outputDir, 0755); err != nil {
		return &ExecutionResult{
			Success:  false,
			Duration: time.Since(startTime),
			ErrorMsg: fmt.Sprintf("failed to create output directory: %v", err),
		}, err
	}

	// Get list of input files
	inputFiles, err := se.getInputFiles()
	if err != nil {
		return &ExecutionResult{
			Success:    false,
			Duration:   time.Since(startTime),
			ErrorMsg:   fmt.Sprintf("failed to read input files: %v", err),
			InputFiles: []string{},
		}, err
	}

	if len(inputFiles) == 0 {
		log.Printf("No input files found in directory: %s", se.inputDir)
		return &ExecutionResult{
			Success:     true,
			Duration:    time.Since(startTime),
			OutputFiles: []string{},
			InputFiles:  []string{},
		}, nil
	}

	log.Printf("Found %d input files to process", len(inputFiles))

	// Execute with timeout
	resultChan := make(chan *ExecutionResult, 1)
	errorChan := make(chan error, 1)

	go func() {
		result, err := se.performMockProcessing(inputFiles, startTime)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	select {
	case result := <-resultChan:
		log.Printf("Mock script execution completed successfully in %v", result.Duration)
		return result, nil
	case err := <-errorChan:
		return &ExecutionResult{
			Success:    false,
			Duration:   time.Since(startTime),
			ErrorMsg:   err.Error(),
			InputFiles: inputFiles,
		}, err
	case <-time.After(se.timeout):
		log.Printf("Mock script execution timed out after %v", se.timeout)
		return &ExecutionResult{
			Success:    false,
			Duration:   time.Since(startTime),
			ErrorMsg:   fmt.Sprintf("execution timed out after %v", se.timeout),
			InputFiles: inputFiles,
		}, fmt.Errorf("execution timeout")
	case <-se.stopChan:
		log.Printf("Mock script execution stopped by user")
		return &ExecutionResult{
			Success:    false,
			Duration:   time.Since(startTime),
			ErrorMsg:   "execution stopped by user",
			InputFiles: inputFiles,
		}, fmt.Errorf("execution stopped")
	}
}

// performMockProcessing simulates processing by copying and renaming files
func (se *ScriptExecutor) performMockProcessing(inputFiles []string, startTime time.Time) (*ExecutionResult, error) {
	outputFiles := make([]string, 0, len(inputFiles))

	// Simulate processing time (1-3 seconds per file)
	processingDelay := time.Duration(len(inputFiles)) * 500 * time.Millisecond
	if processingDelay > 5*time.Second {
		processingDelay = 5 * time.Second
	}

	log.Printf("Simulating processing delay of %v for %d files", processingDelay, len(inputFiles))
	time.Sleep(processingDelay)

	for i, inputFile := range inputFiles {
		// Check if execution was stopped
		select {
		case <-se.stopChan:
			return nil, fmt.Errorf("processing interrupted")
		default:
		}

		// Generate output filename with "processed_" prefix and timestamp
		baseName := filepath.Base(inputFile)
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)

		timestamp := time.Now().Format("20060102_150405")
		outputFileName := fmt.Sprintf("processed_%s_%s_%d%s", nameWithoutExt, timestamp, i, ext)
		outputPath := filepath.Join(se.outputDir, outputFileName)

		// Copy file to output directory with new name (simulating processing)
		if err := se.copyFile(inputFile, outputPath); err != nil {
			log.Printf("Failed to process file %s: %v", inputFile, err)
			continue
		}

		// Add some mock processing metadata to the file
		if err := se.addProcessingMetadata(outputPath); err != nil {
			log.Printf("Failed to add metadata to %s: %v", outputPath, err)
			// Continue anyway, this is just mock processing
		}

		outputFiles = append(outputFiles, outputPath)
		log.Printf("Mock processed file %d/%d: %s -> %s", i+1, len(inputFiles), baseName, outputFileName)
	}

	return &ExecutionResult{
		Success:     true,
		Duration:    time.Since(startTime),
		OutputFiles: outputFiles,
		InputFiles:  inputFiles,
	}, nil
}

// getInputFiles returns a list of files in the input directory
func (se *ScriptExecutor) getInputFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(se.inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Only include common image/data file extensions for mock processing
		ext := strings.ToLower(filepath.Ext(info.Name()))
		validExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".txt", ".dat", ".bin"}

		for _, validExt := range validExtensions {
			if ext == validExt {
				files = append(files, path)
				break
			}
		}

		return nil
	})

	return files, err
}

// copyFile copies a file from src to dst
func (se *ScriptExecutor) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// Sync to ensure data is written
	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// addProcessingMetadata adds mock processing metadata to a file
func (se *ScriptExecutor) addProcessingMetadata(filePath string) error {
	// Open file in append mode to add metadata
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Add mock metadata comment (for text files) or binary marker
	metadata := fmt.Sprintf("\n# Mock Processing Metadata\n# Processed at: %s\n# Processor: ScriptExecutor Mock\n# Version: 1.0\n",
		time.Now().Format(time.RFC3339))

	// For binary files, we'll skip adding text metadata to avoid corruption
	ext := strings.ToLower(filepath.Ext(filePath))
	binaryExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".bin", ".dat"}

	for _, binExt := range binaryExtensions {
		if ext == binExt {
			// For binary files, just touch the file to update timestamp
			return os.Chtimes(filePath, time.Now(), time.Now())
		}
	}

	// For text files, add metadata
	_, err = file.WriteString(metadata)
	return err
}

// WatchOutputDirectory monitors the output directory for new files
func (se *ScriptExecutor) WatchOutputDirectory() (<-chan string, error) {
	fileChan := make(chan string, 10)

	// Ensure output directory exists
	if err := os.MkdirAll(se.outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get initial file list to track new files
	initialFiles, err := se.getOutputFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial file list: %w", err)
	}

	initialFileSet := make(map[string]bool)
	for _, file := range initialFiles {
		initialFileSet[file] = true
	}

	go func() {
		defer close(fileChan)

		ticker := time.NewTicker(1 * time.Second) // Check every second
		defer ticker.Stop()

		log.Printf("Started watching output directory: %s", se.outputDir)

		for {
			select {
			case <-ticker.C:
				currentFiles, err := se.getOutputFiles()
				if err != nil {
					log.Printf("Error reading output directory: %v", err)
					continue
				}

				// Check for new files
				for _, file := range currentFiles {
					if !initialFileSet[file] {
						log.Printf("New output file detected: %s", file)

						// Wait a bit to ensure file is completely written
						time.Sleep(100 * time.Millisecond)

						select {
						case fileChan <- file:
							initialFileSet[file] = true
						case <-se.watcherStopChan:
							return
						}
					}
				}

			case <-se.watcherStopChan:
				log.Printf("Stopped watching output directory: %s", se.outputDir)
				return
			}
		}
	}()

	return fileChan, nil
}

// getOutputFiles returns a list of files in the output directory
func (se *ScriptExecutor) getOutputFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(se.outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// Stop stops the script executor and any running operations
func (se *ScriptExecutor) Stop() {
	log.Printf("Stopping script executor")

	// Stop execution
	select {
	case se.stopChan <- struct{}{}:
	default:
	}

	// Stop watcher
	select {
	case se.watcherStopChan <- struct{}{}:
	default:
	}
}

// IsRunning returns whether the script executor is currently running
func (se *ScriptExecutor) IsRunning() bool {
	se.runningMutex.RLock()
	defer se.runningMutex.RUnlock()
	return se.isRunning
}

// GetInputDirectory returns the input directory path
func (se *ScriptExecutor) GetInputDirectory() string {
	return se.inputDir
}

// GetOutputDirectory returns the output directory path
func (se *ScriptExecutor) GetOutputDirectory() string {
	return se.outputDir
}

// GetTimeout returns the execution timeout
func (se *ScriptExecutor) GetTimeout() time.Duration {
	return se.timeout
}

// SetTimeout updates the execution timeout
func (se *ScriptExecutor) SetTimeout(timeout time.Duration) {
	se.timeout = timeout
}

// CreateProcessingResult creates a ProcessingResult from ExecutionResult
func (se *ScriptExecutor) CreateProcessingResult(workerID int, execResult *ExecutionResult) *messaging.ProcessingResult {
	status := messaging.StatusSuccess
	if !execResult.Success {
		status = messaging.StatusError
	}

	processedFiles := make([]messaging.ProcessedFile, 0, len(execResult.OutputFiles))

	for _, outputFile := range execResult.OutputFiles {
		// Get file info
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			log.Printf("Failed to get file info for %s: %v", outputFile, err)
			continue
		}

		// Calculate checksum
		checksum, err := se.calculateFileChecksum(outputFile)
		if err != nil {
			log.Printf("Failed to calculate checksum for %s: %v", outputFile, err)
			checksum = "unknown"
		}

		// Read file data (for small files only, to avoid memory issues)
		var fileData []byte
		if fileInfo.Size() < 1024*1024 { // Only read files smaller than 1MB
			data, err := os.ReadFile(outputFile)
			if err != nil {
				log.Printf("Failed to read file data for %s: %v", outputFile, err)
			} else {
				fileData = data
			}
		}

		processedFile := messaging.ProcessedFile{
			Filename:    filepath.Base(outputFile),
			Size:        fileInfo.Size(),
			Checksum:    checksum,
			ProcessedAt: fileInfo.ModTime(),
			Data:        fileData,
		}

		processedFiles = append(processedFiles, processedFile)
	}

	metadata := map[string]interface{}{
		"input_directory":  se.inputDir,
		"output_directory": se.outputDir,
		"execution_type":   "mock_processing",
		"processor":        "ScriptExecutor",
		"version":          "1.0",
	}

	return &messaging.ProcessingResult{
		WorkerID:        workerID,
		Status:          status,
		ProcessingTime:  execResult.Duration,
		InputFileCount:  len(execResult.InputFiles),
		OutputFileCount: len(execResult.OutputFiles),
		OutputFiles:     processedFiles,
		ErrorMessage:    execResult.ErrorMsg,
		Metadata:        metadata,
	}
}

// calculateFileChecksum calculates MD5 checksum of a file
func (se *ScriptExecutor) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// CleanupOutputDirectory removes all files from the output directory
func (se *ScriptExecutor) CleanupOutputDirectory() error {
	files, err := se.getOutputFiles()
	if err != nil {
		return fmt.Errorf("failed to get output files: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Printf("Failed to remove file %s: %v", file, err)
		}
	}

	log.Printf("Cleaned up %d files from output directory", len(files))
	return nil
}

// GetFileCount returns the number of files in input and output directories
func (se *ScriptExecutor) GetFileCount() (inputCount, outputCount int, err error) {
	inputFiles, err := se.getInputFiles()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count input files: %w", err)
	}

	outputFiles, err := se.getOutputFiles()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count output files: %w", err)
	}

	return len(inputFiles), len(outputFiles), nil
}
