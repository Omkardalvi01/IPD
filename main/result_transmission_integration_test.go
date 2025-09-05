package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

// TestResultCollectionAndTransmissionIntegration tests the complete workflow
// of collecting processed files and transmitting them
func TestResultCollectionAndTransmissionIntegration(t *testing.T) {
	// Create temporary directories
	inputDir, err := os.MkdirTemp("", "test_input_*")
	if err != nil {
		t.Fatalf("Failed to create input temp directory: %v", err)
	}
	defer os.RemoveAll(inputDir)

	outputDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create output temp directory: %v", err)
	}
	defer os.RemoveAll(outputDir)

	// Create test input files
	inputFiles := []string{"input1.txt", "input2.jpg", "input3.png"}
	inputContent := []string{"Input text 1", "fake jpeg data", "fake png data"}

	for i, filename := range inputFiles {
		filePath := filepath.Join(inputDir, filename)
		if err := os.WriteFile(filePath, []byte(inputContent[i]), 0644); err != nil {
			t.Fatalf("Failed to create input file %s: %v", filename, err)
		}
	}

	// Create script executor
	scriptExecutor := NewScriptExecutor(inputDir, outputDir, 30*time.Second)

	// Execute processing (this will create output files)
	execResult, err := scriptExecutor.Execute()
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	if !execResult.Success {
		t.Fatalf("Script execution was not successful: %s", execResult.ErrorMsg)
	}

	if len(execResult.OutputFiles) == 0 {
		t.Fatal("Expected output files to be created")
	}

	t.Logf("Script execution completed: %d input files, %d output files, duration: %v",
		len(execResult.InputFiles), len(execResult.OutputFiles), execResult.Duration)

	// Create result collector
	workerID := 1
	resultCollector := NewResultCollector(workerID, outputDir)

	// Scan output directory
	newFiles, err := resultCollector.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("Failed to scan output directory: %v", err)
	}

	if len(newFiles) == 0 {
		t.Fatal("Expected processed files to be found")
	}

	t.Logf("Result collector found %d processed files", len(newFiles))

	// Verify file metadata collection
	collectedFiles := resultCollector.GetCollectedFiles()
	for filename, metadata := range collectedFiles {
		t.Logf("Collected file: %s (size: %d, checksum: %s, chunks: %d)",
			filename, metadata.Size, metadata.Checksum[:8], metadata.ChunkCount)

		if metadata.Filename == "" {
			t.Errorf("File %s has empty filename", filename)
		}

		if metadata.Size <= 0 {
			t.Errorf("File %s has invalid size: %d", filename, metadata.Size)
		}

		if metadata.Checksum == "" {
			t.Errorf("File %s has empty checksum", filename)
		}

		if metadata.ChunkCount <= 0 {
			t.Errorf("File %s has invalid chunk count: %d", filename, metadata.ChunkCount)
		}
	}

	// Create processing result
	processingResult := resultCollector.CreateProcessingResult(
		len(execResult.InputFiles),
		execResult.Duration,
		messaging.StatusSuccess,
	)

	// Verify processing result
	if processingResult.WorkerID != workerID {
		t.Errorf("Expected worker ID %d, got %d", workerID, processingResult.WorkerID)
	}

	if processingResult.Status != messaging.StatusSuccess {
		t.Errorf("Expected status success, got %v", processingResult.Status)
	}

	if processingResult.InputFileCount != len(execResult.InputFiles) {
		t.Errorf("Expected input file count %d, got %d",
			len(execResult.InputFiles), processingResult.InputFileCount)
	}

	if processingResult.OutputFileCount != len(newFiles) {
		t.Errorf("Expected output file count %d, got %d",
			len(newFiles), processingResult.OutputFileCount)
	}

	if len(processingResult.OutputFiles) != len(newFiles) {
		t.Errorf("Expected %d output files in result, got %d",
			len(newFiles), len(processingResult.OutputFiles))
	}

	t.Logf("Processing result created successfully: %d input files, %d output files",
		processingResult.InputFileCount, processingResult.OutputFileCount)

	// Test file preparation for transmission
	for filename := range collectedFiles {
		chunks, err := resultCollector.PrepareFileForTransmission(filename)
		if err != nil {
			t.Errorf("Failed to prepare file %s for transmission: %v", filename, err)
			continue
		}

		if len(chunks) == 0 {
			t.Errorf("Expected chunks for file %s, got none", filename)
			continue
		}

		t.Logf("File %s prepared for transmission: %d chunks", filename, len(chunks))

		// Verify chunk integrity
		totalSize := int64(0)
		for i, chunk := range chunks {
			if chunk.Filename != filename {
				t.Errorf("Chunk %d has wrong filename: expected %s, got %s",
					i, filename, chunk.Filename)
			}

			if chunk.ChunkIndex != i {
				t.Errorf("Chunk has wrong index: expected %d, got %d",
					i, chunk.ChunkIndex)
			}

			if chunk.TotalChunks != len(chunks) {
				t.Errorf("Chunk has wrong total count: expected %d, got %d",
					len(chunks), chunk.TotalChunks)
			}

			if chunk.IsLast != (i == len(chunks)-1) {
				t.Errorf("Chunk %d has wrong IsLast flag", i)
			}

			if len(chunk.Data) == 0 {
				t.Errorf("Chunk %d has no data", i)
			}

			if chunk.Checksum == "" {
				t.Errorf("Chunk %d has no checksum", i)
			}

			totalSize += int64(len(chunk.Data))
		}

		// Verify total size matches file size
		metadata, _ := resultCollector.GetFileMetadata(filename)
		if totalSize != metadata.Size {
			t.Errorf("Total chunk size %d doesn't match file size %d for %s",
				totalSize, metadata.Size, filename)
		}
	}

	// Test transmission status tracking
	pendingFiles := resultCollector.GetPendingFiles()
	if len(pendingFiles) != len(newFiles) {
		t.Errorf("Expected %d pending files, got %d", len(newFiles), len(pendingFiles))
	}

	// Mark some files as transmitted
	transmittedCount := 0
	for filename := range collectedFiles {
		resultCollector.MarkFileAsTransmitted(filename, true, "")
		transmittedCount++
		if transmittedCount >= 2 { // Mark only first 2 as transmitted
			break
		}
	}

	// Check transmission status
	transmissionStatus := resultCollector.GetTransmissionStatus()
	actualTransmitted := 0
	for _, transmitted := range transmissionStatus {
		if transmitted {
			actualTransmitted++
		}
	}

	if actualTransmitted != transmittedCount {
		t.Errorf("Expected %d transmitted files, got %d", transmittedCount, actualTransmitted)
	}

	// Check pending files after marking some as transmitted
	pendingFiles = resultCollector.GetPendingFiles()
	expectedPending := len(newFiles) - transmittedCount
	if len(pendingFiles) != expectedPending {
		t.Errorf("Expected %d pending files after transmission, got %d",
			expectedPending, len(pendingFiles))
	}

	// Check transmission log
	transmissionLog := resultCollector.GetTransmissionLog()
	if len(transmissionLog) != transmittedCount {
		t.Errorf("Expected %d transmission log entries, got %d",
			transmittedCount, len(transmissionLog))
	}

	for _, logEntry := range transmissionLog {
		if !logEntry.Success {
			t.Errorf("Expected log entry for %s to indicate success", logEntry.Filename)
		}

		if logEntry.ErrorMessage != "" {
			t.Errorf("Expected no error message for successful transmission, got: %s",
				logEntry.ErrorMessage)
		}
	}

	// Test statistics
	stats := resultCollector.GetStatistics()
	if stats["total_files"] != len(newFiles) {
		t.Errorf("Expected total_files %d, got %v", len(newFiles), stats["total_files"])
	}

	if stats["transmitted_files"] != transmittedCount {
		t.Errorf("Expected transmitted_files %d, got %v", transmittedCount, stats["transmitted_files"])
	}

	if stats["pending_files"] != expectedPending {
		t.Errorf("Expected pending_files %d, got %v", expectedPending, stats["pending_files"])
	}

	t.Logf("Integration test completed successfully")
	t.Logf("Statistics: %+v", stats)
}

// TestResultCollectionWithDifferentFileSizes tests collection of files with various sizes
func TestResultCollectionWithDifferentFileSizes(t *testing.T) {
	// Create temporary output directory
	outputDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create output temp directory: %v", err)
	}
	defer os.RemoveAll(outputDir)

	// Create files of different sizes
	testFiles := map[string]int{
		"tiny.txt":   10,              // 10 bytes - inline
		"small.txt":  1024,            // 1KB - inline
		"medium.txt": 512 * 1024,      // 512KB - inline (at default limit)
		"large.txt":  1024 * 1024,     // 1MB - chunked (1 chunk, exactly chunk size)
		"huge.txt":   5 * 1024 * 1024, // 5MB - chunked (5 chunks)
	}

	for filename, size := range testFiles {
		content := make([]byte, size)
		for i := range content {
			content[i] = byte('A' + (i % 26))
		}

		filePath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Create result collector
	resultCollector := NewResultCollector(1, outputDir)

	// Scan directory
	newFiles, err := resultCollector.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("Failed to scan output directory: %v", err)
	}

	if len(newFiles) != len(testFiles) {
		t.Errorf("Expected %d files, got %d", len(testFiles), len(newFiles))
	}

	// Check file categorization
	stats := resultCollector.GetStatistics()
	inlineFiles := stats["inline_files"].(int)
	chunkedFiles := stats["chunked_files"].(int)

	expectedInline := 3  // tiny, small, medium
	expectedChunked := 2 // large, huge

	if inlineFiles != expectedInline {
		t.Errorf("Expected %d inline files, got %d", expectedInline, inlineFiles)
	}

	if chunkedFiles != expectedChunked {
		t.Errorf("Expected %d chunked files, got %d", expectedChunked, chunkedFiles)
	}

	// Test chunk preparation for different file sizes
	collectedFiles := resultCollector.GetCollectedFiles()
	for filename, metadata := range collectedFiles {
		expectedSize := testFiles[filename]
		if metadata.Size != int64(expectedSize) {
			t.Errorf("File %s: expected size %d, got %d", filename, expectedSize, metadata.Size)
		}

		chunks, err := resultCollector.PrepareFileForTransmission(filename)
		if err != nil {
			t.Errorf("Failed to prepare file %s for transmission: %v", filename, err)
			continue
		}

		// Verify chunk count matches metadata
		if len(chunks) != metadata.ChunkCount {
			t.Errorf("File %s: expected %d chunks, got %d",
				filename, metadata.ChunkCount, len(chunks))
		}

		// For small files, should be single chunk
		if expectedSize <= int(resultCollector.maxFileSize) {
			if len(chunks) != 1 {
				t.Errorf("Small file %s should have 1 chunk, got %d", filename, len(chunks))
			}
		} else {
			// For large files, chunk count depends on file size and chunk size
			expectedChunks := (expectedSize + int(resultCollector.chunkSize) - 1) / int(resultCollector.chunkSize)
			if len(chunks) != expectedChunks {
				t.Errorf("Large file %s should have %d chunks, got %d", filename, expectedChunks, len(chunks))
			}
		}

		t.Logf("File %s (%d bytes): %d chunks", filename, expectedSize, len(chunks))
	}
}

// TestErrorHandlingInResultCollection tests error scenarios in result collection
func TestErrorHandlingInResultCollection(t *testing.T) {
	// Test with non-existent directory
	resultCollector := NewResultCollector(1, "/non/existent/directory")

	files, err := resultCollector.ScanOutputDirectory()
	if err != nil {
		t.Errorf("ScanOutputDirectory should handle non-existent directory gracefully, got error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files for non-existent directory, got %d", len(files))
	}

	// Test file preparation for non-existent file
	_, err = resultCollector.PrepareFileForTransmission("non-existent.txt")
	if err == nil {
		t.Error("Expected error for non-existent file preparation")
	}

	// Test getting metadata for non-existent file
	_, exists := resultCollector.GetFileMetadata("non-existent.txt")
	if exists {
		t.Error("Expected false for non-existent file metadata")
	}
}
