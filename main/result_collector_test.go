package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

func TestNewResultCollector(t *testing.T) {
	workerID := 1
	outputDir := "./test_output"

	rc := NewResultCollector(workerID, outputDir)

	if rc.workerID != workerID {
		t.Errorf("Expected worker ID %d, got %d", workerID, rc.workerID)
	}

	if rc.outputDir != outputDir {
		t.Errorf("Expected output directory %s, got %s", outputDir, rc.outputDir)
	}

	if rc.chunkSize != 1024*1024 {
		t.Errorf("Expected default chunk size %d, got %d", 1024*1024, rc.chunkSize)
	}

	if rc.maxFileSize != 512*1024 {
		t.Errorf("Expected default max file size %d, got %d", 512*1024, rc.maxFileSize)
	}
}

func TestResultCollectorSetters(t *testing.T) {
	rc := NewResultCollector(1, "./test_output")

	// Test SetChunkSize
	newChunkSize := int64(2 * 1024 * 1024)
	rc.SetChunkSize(newChunkSize)
	if rc.chunkSize != newChunkSize {
		t.Errorf("Expected chunk size %d, got %d", newChunkSize, rc.chunkSize)
	}

	// Test SetMaxInlineFileSize
	newMaxSize := int64(1024 * 1024)
	rc.SetMaxInlineFileSize(newMaxSize)
	if rc.maxFileSize != newMaxSize {
		t.Errorf("Expected max file size %d, got %d", newMaxSize, rc.maxFileSize)
	}
}

func TestScanOutputDirectoryEmpty(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	rc := NewResultCollector(1, tempDir)

	files, err := rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
}

func TestScanOutputDirectoryWithFiles(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := []string{"test1.txt", "test2.jpg", "test3.png"}
	testContent := []string{"Hello World", "fake image data", "fake png data"}

	for i, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(testContent[i]), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	rc := NewResultCollector(1, tempDir)

	files, err := rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	if len(files) != len(testFiles) {
		t.Errorf("Expected %d files, got %d", len(testFiles), len(files))
	}

	// Check that all test files were found
	collectedFiles := rc.GetCollectedFiles()
	for _, expectedFile := range testFiles {
		if _, exists := collectedFiles[expectedFile]; !exists {
			t.Errorf("Expected file %s not found in collected files", expectedFile)
		}
	}
}

func TestGetFileMetadata(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := "test.txt"
	testContent := "Hello World"
	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Get file metadata
	metadata, exists := rc.GetFileMetadata(testFile)
	if !exists {
		t.Fatalf("File metadata not found for %s", testFile)
	}

	if metadata.Filename != testFile {
		t.Errorf("Expected filename %s, got %s", testFile, metadata.Filename)
	}

	if metadata.Size != int64(len(testContent)) {
		t.Errorf("Expected file size %d, got %d", len(testContent), metadata.Size)
	}

	if metadata.Checksum == "" {
		t.Error("Expected non-empty checksum")
	}

	if metadata.ContentType != "text/plain" {
		t.Errorf("Expected content type text/plain, got %s", metadata.ContentType)
	}
}

func TestCreateProcessingResult(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := []string{"test1.txt", "test2.jpg"}
	testContent := []string{"Hello World", "fake image data"}

	for i, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(testContent[i]), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Create processing result
	inputFileCount := 3
	processingTime := 5 * time.Second
	status := messaging.StatusSuccess

	result := rc.CreateProcessingResult(inputFileCount, processingTime, status)

	if result.WorkerID != 1 {
		t.Errorf("Expected worker ID 1, got %d", result.WorkerID)
	}

	if result.Status != status {
		t.Errorf("Expected status %v, got %v", status, result.Status)
	}

	if result.ProcessingTime != processingTime {
		t.Errorf("Expected processing time %v, got %v", processingTime, result.ProcessingTime)
	}

	if result.InputFileCount != inputFileCount {
		t.Errorf("Expected input file count %d, got %d", inputFileCount, result.InputFileCount)
	}

	if result.OutputFileCount != len(testFiles) {
		t.Errorf("Expected output file count %d, got %d", len(testFiles), result.OutputFileCount)
	}

	if len(result.OutputFiles) != len(testFiles) {
		t.Errorf("Expected %d output files, got %d", len(testFiles), len(result.OutputFiles))
	}
}

func TestPrepareFileForTransmissionSmallFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create small test file
	testFile := "small.txt"
	testContent := "Hello World"
	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Prepare file for transmission
	chunks, err := rc.PrepareFileForTransmission(testFile)
	if err != nil {
		t.Fatalf("PrepareFileForTransmission failed: %v", err)
	}

	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for small file, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Filename != testFile {
		t.Errorf("Expected filename %s, got %s", testFile, chunk.Filename)
	}

	if chunk.ChunkIndex != 0 {
		t.Errorf("Expected chunk index 0, got %d", chunk.ChunkIndex)
	}

	if chunk.TotalChunks != 1 {
		t.Errorf("Expected total chunks 1, got %d", chunk.TotalChunks)
	}

	if !chunk.IsLast {
		t.Error("Expected chunk to be marked as last")
	}

	if string(chunk.Data) != testContent {
		t.Errorf("Expected chunk data %s, got %s", testContent, string(chunk.Data))
	}
}

func TestPrepareFileForTransmissionLargeFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create large test file (larger than default chunk size to get multiple chunks)
	testFile := "large.txt"
	testContent := make([]byte, 3*1024*1024) // 3MB
	for i := range testContent {
		testContent[i] = byte('A' + (i % 26))
	}

	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Prepare file for transmission
	chunks, err := rc.PrepareFileForTransmission(testFile)
	if err != nil {
		t.Fatalf("PrepareFileForTransmission failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks for large file, got %d", len(chunks))
	}

	// Verify chunk properties
	totalData := make([]byte, 0)
	for i, chunk := range chunks {
		if chunk.Filename != testFile {
			t.Errorf("Expected filename %s, got %s", testFile, chunk.Filename)
		}

		if chunk.ChunkIndex != i {
			t.Errorf("Expected chunk index %d, got %d", i, chunk.ChunkIndex)
		}

		if chunk.TotalChunks != len(chunks) {
			t.Errorf("Expected total chunks %d, got %d", len(chunks), chunk.TotalChunks)
		}

		if chunk.IsLast != (i == len(chunks)-1) {
			t.Errorf("Chunk %d IsLast flag incorrect", i)
		}

		totalData = append(totalData, chunk.Data...)
	}

	// Verify reconstructed data matches original
	if len(totalData) != len(testContent) {
		t.Errorf("Expected reconstructed data length %d, got %d", len(testContent), len(totalData))
	}

	for i, b := range totalData {
		if b != testContent[i] {
			t.Errorf("Data mismatch at byte %d", i)
			break
		}
	}
}

func TestMarkFileAsTransmitted(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := "test.txt"
	testContent := "Hello World"
	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Initially file should not be transmitted
	if rc.IsAllFilesTransmitted() {
		t.Error("Expected files not to be transmitted initially")
	}

	// Mark file as transmitted
	rc.MarkFileAsTransmitted(testFile, true, "")

	// Check transmission status
	status := rc.GetTransmissionStatus()
	if !status[testFile] {
		t.Errorf("Expected file %s to be marked as transmitted", testFile)
	}

	if !rc.IsAllFilesTransmitted() {
		t.Error("Expected all files to be transmitted")
	}

	// Check transmission log
	log := rc.GetTransmissionLog()
	if len(log) != 1 {
		t.Errorf("Expected 1 transmission log entry, got %d", len(log))
	}

	if log[0].Filename != testFile {
		t.Errorf("Expected log filename %s, got %s", testFile, log[0].Filename)
	}

	if !log[0].Success {
		t.Error("Expected log entry to indicate success")
	}
}

func TestGetPendingFiles(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := []string{"test1.txt", "test2.txt", "test3.txt"}
	for _, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Initially all files should be pending
	pending := rc.GetPendingFiles()
	if len(pending) != len(testFiles) {
		t.Errorf("Expected %d pending files, got %d", len(testFiles), len(pending))
	}

	// Mark one file as transmitted
	rc.MarkFileAsTransmitted(testFiles[0], true, "")

	// Check pending files
	pending = rc.GetPendingFiles()
	if len(pending) != len(testFiles)-1 {
		t.Errorf("Expected %d pending files, got %d", len(testFiles)-1, len(pending))
	}

	// Verify the transmitted file is not in pending list
	for _, filename := range pending {
		if filename == testFiles[0] {
			t.Errorf("Transmitted file %s should not be in pending list", testFiles[0])
		}
	}
}

func TestGetStatistics(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_output_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files of different sizes
	smallFile := filepath.Join(tempDir, "small.txt")
	largeFile := filepath.Join(tempDir, "large.txt")

	if err := os.WriteFile(smallFile, []byte("small"), 0644); err != nil {
		t.Fatalf("Failed to create small file: %v", err)
	}

	largeContent := make([]byte, 1024*1024) // 1MB
	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	rc := NewResultCollector(1, tempDir)

	// Scan directory
	_, err = rc.ScanOutputDirectory()
	if err != nil {
		t.Fatalf("ScanOutputDirectory failed: %v", err)
	}

	// Mark one file as transmitted
	rc.MarkFileAsTransmitted("small.txt", true, "")

	// Get statistics
	stats := rc.GetStatistics()

	if stats["total_files"] != 2 {
		t.Errorf("Expected total_files 2, got %v", stats["total_files"])
	}

	if stats["transmitted_files"] != 1 {
		t.Errorf("Expected transmitted_files 1, got %v", stats["transmitted_files"])
	}

	if stats["pending_files"] != 1 {
		t.Errorf("Expected pending_files 1, got %v", stats["pending_files"])
	}

	if stats["inline_files"] != 1 {
		t.Errorf("Expected inline_files 1, got %v", stats["inline_files"])
	}

	if stats["chunked_files"] != 1 {
		t.Errorf("Expected chunked_files 1, got %v", stats["chunked_files"])
	}
}
