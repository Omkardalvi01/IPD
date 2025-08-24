package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"sync"
)

type YOLOTrainer struct {
	pythonPath       string
	scriptPath       string  
	tempDir          string
	weightsDir       string
	trainingLogDir   string
	batchSize        int
	currentBatchSize int
	mu              sync.Mutex
}

type YOLOTrainingResult struct {
	ImageID          string  `json:"image_id"`
	BatchCompleted   bool    `json:"batch_completed"`
	TrainingLoss     float64 `json:"training_loss,omitempty"`
	ValidationMAP    float64 `json:"validation_map,omitempty"`
	ModelWeights     string  `json:"model_weights,omitempty"`
	WeightsFilePath  string  `json:"weights_file_path,omitempty"`
	TrainingTime     float64 `json:"training_time,omitempty"`
	BatchInfo        BatchInfo `json:"batch_info"`
	Timestamp        float64 `json:"timestamp"`
	Error            string  `json:"error,omitempty"`
}

type BatchInfo struct {
	CurrentBatchSize int `json:"current_batch_size"`
	MaxBatchSize     int `json:"max_batch_size"`
	TotalBatches     int `json:"total_batches_trained"`
	ImagesProcessed  int `json:"images_processed_total"`
}

func NewYOLOTrainer() *YOLOTrainer {
	// Set directories
	tempDir := "./temp_images"
	weightsDir := "./yolo_weights"
	trainingLogDir := "./yolo_logs"
	
	// Create directories with 0755 permissions
	os.MkdirAll(tempDir, 0755)
	os.MkdirAll(weightsDir, 0755)
	os.MkdirAll(trainingLogDir, 0755)
	
	return &YOLOTrainer{
		pythonPath: "python3",
		scriptPath: "../ml/yolo_trainer.py",
		tempDir: tempDir,
		weightsDir: weightsDir,
		trainingLogDir: trainingLogDir,
		batchSize: 8,
		currentBatchSize: 0,
	}
}

func (yt *YOLOTrainer) TrainOnImage(imageData []byte, imageID string) (*YOLOTrainingResult, error) {
	yt.mu.Lock()
	defer yt.mu.Unlock()
	
	log.Printf("Adding image to YOLO batch: %s (batch: %d/%d)", imageID, yt.currentBatchSize+1, yt.batchSize)
	
	// Create temp file path
	tempFile := filepath.Join(yt.tempDir, imageID)
	
	// Write image data to temp file
	err := ioutil.WriteFile(tempFile, imageData, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write temp file: %v", err)
	}
	
	// Cleanup temp file after processing
	defer func() {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			log.Printf("Warning: failed to remove temp file %s: %v", tempFile, removeErr)
		}
	}()
	
	// Run YOLO training
	result, err := yt.runYOLOTraining(tempFile, imageID)
	if err != nil {
		return nil, err
	}
	
	// Update batch size tracking
	if result.BatchCompleted {
		yt.currentBatchSize = 0
	} else {
		yt.currentBatchSize++
	}
	
	// Log training result
	if logErr := yt.logTrainingResult(result); logErr != nil {
		log.Printf("Warning: failed to log training result: %v", logErr)
	}
	
	return result, nil
}

func (yt *YOLOTrainer) runYOLOTraining(imagePath, imageID string) (*YOLOTrainingResult, error) {
	// Create command to run Python script
	cmd := exec.Command(yt.pythonPath, yt.scriptPath, imagePath, imageID)
	
	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Record start time and run command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime).Seconds()
	
	// Log stderr for YOLO training progress information
	if stderr.Len() > 0 {
		log.Printf("YOLO Training Progress: %s", stderr.String())
	}
	
	if err != nil {
		return nil, fmt.Errorf("YOLO training command failed: %v", err)
	}
	
	// Parse JSON result from stdout
	var result YOLOTrainingResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse YOLO training result: %v", unmarshalErr)
	}
	
	// Check for errors in result
	if result.Error != "" {
		return nil, fmt.Errorf("YOLO training error: %s", result.Error)
	}
	
	// Set timestamp and image ID
	result.Timestamp = float64(time.Now().Unix())
	result.ImageID = imageID
	result.TrainingTime = duration
	
	// Update batch tracking
	if result.BatchCompleted {
		yt.currentBatchSize = 0
	} else {
		yt.currentBatchSize++
	}
	
	// Log training metrics if batch completed
	if result.BatchCompleted {
		log.Printf("Batch training completed - Loss: %.6f, mAP: %.6f, Time: %.3fs", 
			result.TrainingLoss, result.ValidationMAP, result.TrainingTime)
	}
	
	return &result, nil
}

func (yt *YOLOTrainer) logTrainingResult(result *YOLOTrainingResult) error {
	// Create log filename with timestamp
	logFilename := fmt.Sprintf("yolo_training_log_%s_%d.json", result.ImageID, int64(result.Timestamp))
	logPath := filepath.Join(yt.trainingLogDir, logFilename)
	
	// Marshal result with pretty formatting
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal training result: %v", err)
	}
	
	// Write to log file
	err = ioutil.WriteFile(logPath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write training log: %v", err)
	}
	
	return nil
}

func (yt *YOLOTrainer) GetTrainingStats() map[string]interface{} {
	yt.mu.Lock()
	defer yt.mu.Unlock()
	
	return map[string]interface{}{
		"current_batch_size": yt.currentBatchSize,
		"max_batch_size":     yt.batchSize,
		"temp_dir":           yt.tempDir,
		"weights_dir":        yt.weightsDir,
		"training_log_dir":   yt.trainingLogDir,
	}
}
