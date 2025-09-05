package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// Example demonstrating how to use the server result collection system
func ExampleServerResultCollection() {
	fmt.Println("=== Server Result Collection Example ===")

	// Initialize server result collector for 3 workers
	sharedResultsDir := "./example_results"
	collector := NewServerResultCollector(3, sharedResultsDir)

	fmt.Printf("Initialized collector for %d workers\n", collector.GetExpectedWorkerCount())
	fmt.Printf("Results will be stored in: %s\n", collector.GetSharedResultsDirectory())

	// Start monitoring in background
	go func() {
		completionChan := collector.GetCompletionChannel()
		allCompleteChan := collector.GetAllCompleteChannel()

		for {
			select {
			case workerID := <-completionChan:
				fmt.Printf("✓ Worker %d completed (%d/%d)\n",
					workerID, collector.GetCompletedWorkerCount(), collector.GetExpectedWorkerCount())

			case <-allCompleteChan:
				fmt.Println("🎉 All workers completed!")

				// Print validation errors if any
				validationErrors := collector.GetValidationErrors()
				if len(validationErrors) > 0 {
					fmt.Printf("⚠️  Found validation errors for %d workers\n", len(validationErrors))
				} else {
					fmt.Println("✅ No validation errors")
				}
				return
			}
		}
	}()

	// Simulate results from 3 workers
	simulateWorkerResults(collector)

	// Wait for completion
	<-collector.GetAllCompleteChannel()

	fmt.Println("Example completed successfully!")
}

func simulateWorkerResults(collector *ServerResultCollector) {
	// Worker 1: Successful processing with 2 output files
	worker1Result := &messaging.ProcessingResult{
		WorkerID:        1,
		Status:          messaging.StatusSuccess,
		ProcessingTime:  time.Second * 3,
		InputFileCount:  5,
		OutputFileCount: 2,
		OutputFiles: []messaging.ProcessedFile{
			{
				Filename:    "processed_image_1.jpg",
				Size:        1024,
				Checksum:    "a1b2c3d4e5f6",
				ProcessedAt: time.Now(),
				Data:        make([]byte, 1024), // Simulated image data
			},
			{
				Filename:    "processed_image_2.jpg",
				Size:        2048,
				Checksum:    "f6e5d4c3b2a1",
				ProcessedAt: time.Now(),
				Data:        make([]byte, 2048), // Simulated image data
			},
		},
		Metadata: map[string]interface{}{
			"processing_algorithm": "edge_detection",
			"quality_score":        0.95,
		},
	}

	// Send worker 1 result
	sendWorkerResult(collector, 1, worker1Result)
	time.Sleep(100 * time.Millisecond)

	// Send worker 1 completion
	sendWorkerCompletion(collector, 1)
	time.Sleep(100 * time.Millisecond)

	// Worker 2: Partial success with 1 output file
	worker2Result := &messaging.ProcessingResult{
		WorkerID:        2,
		Status:          messaging.StatusPartialSuccess,
		ProcessingTime:  time.Second * 5,
		InputFileCount:  3,
		OutputFileCount: 1,
		OutputFiles: []messaging.ProcessedFile{
			{
				Filename:    "processed_document.pdf",
				Size:        4096,
				Checksum:    "1a2b3c4d5e6f",
				ProcessedAt: time.Now(),
				Data:        make([]byte, 4096), // Simulated PDF data
			},
		},
		ErrorMessage: "Some input files were corrupted",
		Metadata: map[string]interface{}{
			"processing_algorithm": "ocr_extraction",
			"success_rate":         0.67,
		},
	}

	// Send worker 2 result
	sendWorkerResult(collector, 2, worker2Result)
	time.Sleep(100 * time.Millisecond)

	// Send worker 2 completion
	sendWorkerCompletion(collector, 2)
	time.Sleep(100 * time.Millisecond)

	// Worker 3: Error case
	sendWorkerError(collector, 3, "Processing failed: Out of memory")
	time.Sleep(100 * time.Millisecond)
}

func sendWorkerResult(collector *ServerResultCollector, workerID int, result *messaging.ProcessingResult) {
	// Serialize result
	resultData, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to serialize result for worker %d: %v", workerID, err)
		return
	}

	// Create result message
	msg := &messaging.Message{
		Type:      messaging.MsgTypeResult,
		WorkerID:  workerID,
		Data:      resultData,
		Timestamp: time.Now(),
	}

	// Send to collector
	if err := collector.HandleResultMessage(workerID, msg); err != nil {
		log.Printf("Failed to handle result for worker %d: %v", workerID, err)
	}
}

func sendWorkerCompletion(collector *ServerResultCollector, workerID int) {
	// Create completion message
	msg := &messaging.Message{
		Type:      messaging.MsgTypeResultEnd,
		WorkerID:  workerID,
		Timestamp: time.Now(),
	}

	// Send to collector
	if err := collector.HandleResultMessage(workerID, msg); err != nil {
		log.Printf("Failed to handle completion for worker %d: %v", workerID, err)
	}
}

func sendWorkerError(collector *ServerResultCollector, workerID int, errorMsg string) {
	// Create error message
	msg := &messaging.Message{
		Type:      messaging.MsgTypeError,
		WorkerID:  workerID,
		Data:      []byte(errorMsg),
		Timestamp: time.Now(),
	}

	// Send to collector
	if err := collector.HandleResultMessage(workerID, msg); err != nil {
		log.Printf("Failed to handle error for worker %d: %v", workerID, err)
	}
}

// Uncomment the following to run the example:
// func main() {
//     ExampleServerResultCollection()
// }
