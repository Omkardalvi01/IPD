package main

import (
	"crypto/sha256"
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

// ResultCollector handles collection and transmission of processed files
type ResultCollector struct {
	workerID        int
	outputDir       string
	chunkSize       int64 // Size for chunked transmission
	maxFileSize     int64 // Maximum file size to transmit inline
	mutex           sync.RWMutex
	collectedFiles  map[string]*FileMetadata
	transmissionLog []TransmissionRecord
}

// FileMetadata contains metadata about a processed file
type FileMetadata struct {
	Filename      string
	FullPath      string
	Size          int64
	Checksum      string
	ModTime       time.Time
	IsTransmitted bool
	ChunkCount    int
	ContentType   string
}

// TransmissionRecord tracks file transmission history
type TransmissionRecord struct {
	Filename     string
	Timestamp    time.Time
	Success      bool
	ChunksTotal  int
	ChunksSent   int
	ErrorMessage string
}

// ChunkInfo represents a file chunk for transmission
type ChunkInfo struct {
	Filename    string
	ChunkIndex  int
	TotalChunks int
	Data        []byte
	Checksum    string
	IsLast      bool
}

// NewResultCollector creates a new ResultCollector instance
func NewResultCollector(workerID int, outputDir string) *ResultCollector {
	return &ResultCollector{
		workerID:        workerID,
		outputDir:       outputDir,
		chunkSize:       1024 * 1024, // 1MB chunks by default
		maxFileSize:     512 * 1024,  // 512KB max for inline transmission
		collectedFiles:  make(map[string]*FileMetadata),
		transmissionLog: make([]TransmissionRecord, 0),
	}
}

// SetChunkSize sets the chunk size for file transmission
func (rc *ResultCollector) SetChunkSize(size int64) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.chunkSize = size
}

// SetMaxInlineFileSize sets the maximum file size for inline transmission
func (rc *ResultCollector) SetMaxInlineFileSize(size int64) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.maxFileSize = size
}

// ScanOutputDirectory scans the output directory for processed files
func (rc *ResultCollector) ScanOutputDirectory() ([]string, error) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	log.Printf("Worker %d scanning output directory: %s", rc.workerID, rc.outputDir)

	// Ensure output directory exists
	if _, err := os.Stat(rc.outputDir); os.IsNotExist(err) {
		log.Printf("Worker %d output directory does not exist: %s", rc.workerID, rc.outputDir)
		return []string{}, nil
	}

	var newFiles []string
	err := filepath.Walk(rc.outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Worker %d error walking path %s: %v", rc.workerID, path, err)
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		filename := info.Name()

		// Check if we've already collected this file
		if existingFile, exists := rc.collectedFiles[filename]; exists {
			// Check if file has been modified since last collection
			if !info.ModTime().After(existingFile.ModTime) {
				return nil // File hasn't changed
			}
			log.Printf("Worker %d detected file modification: %s", rc.workerID, filename)
		}

		// Calculate file checksum
		checksum, err := rc.calculateFileChecksum(path)
		if err != nil {
			log.Printf("Worker %d failed to calculate checksum for %s: %v", rc.workerID, path, err)
			return nil // Continue with other files
		}

		// Determine content type
		contentType := rc.determineContentType(filename)

		// Create file metadata
		metadata := &FileMetadata{
			Filename:      filename,
			FullPath:      path,
			Size:          info.Size(),
			Checksum:      checksum,
			ModTime:       info.ModTime(),
			IsTransmitted: false,
			ContentType:   contentType,
		}

		// Calculate chunk count for large files
		if info.Size() > rc.maxFileSize {
			metadata.ChunkCount = int((info.Size() + rc.chunkSize - 1) / rc.chunkSize)
		} else {
			metadata.ChunkCount = 1
		}

		rc.collectedFiles[filename] = metadata
		newFiles = append(newFiles, filename)

		log.Printf("Worker %d collected file: %s (size: %d bytes, chunks: %d, checksum: %s)",
			rc.workerID, filename, info.Size(), metadata.ChunkCount, checksum[:8])

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan output directory: %w", err)
	}

	log.Printf("Worker %d found %d files in output directory (%d new/modified)",
		rc.workerID, len(rc.collectedFiles), len(newFiles))

	return newFiles, nil
}

// GetCollectedFiles returns a copy of collected file metadata
func (rc *ResultCollector) GetCollectedFiles() map[string]*FileMetadata {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	files := make(map[string]*FileMetadata)
	for name, metadata := range rc.collectedFiles {
		// Create a copy to avoid race conditions
		fileCopy := *metadata
		files[name] = &fileCopy
	}
	return files
}

// GetFileMetadata returns metadata for a specific file
func (rc *ResultCollector) GetFileMetadata(filename string) (*FileMetadata, bool) {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	metadata, exists := rc.collectedFiles[filename]
	if !exists {
		return nil, false
	}

	// Return a copy
	fileCopy := *metadata
	return &fileCopy, true
}

// CreateProcessingResult creates a ProcessingResult from collected files
func (rc *ResultCollector) CreateProcessingResult(inputFileCount int, processingTime time.Duration, status messaging.ProcessingStatus) *messaging.ProcessingResult {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	processedFiles := make([]messaging.ProcessedFile, 0, len(rc.collectedFiles))

	for _, metadata := range rc.collectedFiles {
		// Read file data for small files only
		var fileData []byte
		if metadata.Size <= rc.maxFileSize {
			data, err := os.ReadFile(metadata.FullPath)
			if err != nil {
				log.Printf("Worker %d failed to read file data for %s: %v", rc.workerID, metadata.Filename, err)
			} else {
				fileData = data
			}
		}

		processedFile := messaging.ProcessedFile{
			Filename:    metadata.Filename,
			Size:        metadata.Size,
			Checksum:    metadata.Checksum,
			ProcessedAt: metadata.ModTime,
			Data:        fileData,
		}

		processedFiles = append(processedFiles, processedFile)
	}

	metadata := map[string]interface{}{
		"output_directory":     rc.outputDir,
		"collection_timestamp": time.Now(),
		"worker_id":            rc.workerID,
		"total_files":          len(rc.collectedFiles),
		"chunked_files":        rc.getChunkedFileCount(),
		"inline_files":         rc.getInlineFileCount(),
	}

	return &messaging.ProcessingResult{
		WorkerID:        rc.workerID,
		Status:          status,
		ProcessingTime:  processingTime,
		InputFileCount:  inputFileCount,
		OutputFileCount: len(rc.collectedFiles),
		OutputFiles:     processedFiles,
		Metadata:        metadata,
	}
}

// PrepareFileForTransmission prepares a file for chunked transmission
func (rc *ResultCollector) PrepareFileForTransmission(filename string) ([]*ChunkInfo, error) {
	rc.mutex.RLock()
	metadata, exists := rc.collectedFiles[filename]
	rc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("file not found in collection: %s", filename)
	}

	// For small files, send as single chunk
	if metadata.Size <= rc.maxFileSize {
		data, err := os.ReadFile(metadata.FullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
		}

		chunk := &ChunkInfo{
			Filename:    filename,
			ChunkIndex:  0,
			TotalChunks: 1,
			Data:        data,
			Checksum:    rc.calculateDataChecksum(data),
			IsLast:      true,
		}

		return []*ChunkInfo{chunk}, nil
	}

	// For large files, create chunks
	file, err := os.Open(metadata.FullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	totalChunks := metadata.ChunkCount
	chunks := make([]*ChunkInfo, 0, totalChunks)

	buffer := make([]byte, rc.chunkSize)
	chunkIndex := 0

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read chunk from file %s: %w", filename, err)
		}

		if bytesRead == 0 {
			break
		}

		chunkData := make([]byte, bytesRead)
		copy(chunkData, buffer[:bytesRead])

		chunk := &ChunkInfo{
			Filename:    filename,
			ChunkIndex:  chunkIndex,
			TotalChunks: totalChunks,
			Data:        chunkData,
			Checksum:    rc.calculateDataChecksum(chunkData),
			IsLast:      chunkIndex == totalChunks-1,
		}

		chunks = append(chunks, chunk)
		chunkIndex++

		if err == io.EOF {
			break
		}
	}

	log.Printf("Worker %d prepared %d chunks for file %s", rc.workerID, len(chunks), filename)
	return chunks, nil
}

// MarkFileAsTransmitted marks a file as successfully transmitted
func (rc *ResultCollector) MarkFileAsTransmitted(filename string, success bool, errorMsg string) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	if metadata, exists := rc.collectedFiles[filename]; exists {
		metadata.IsTransmitted = success

		// Add transmission record
		record := TransmissionRecord{
			Filename:     filename,
			Timestamp:    time.Now(),
			Success:      success,
			ChunksTotal:  metadata.ChunkCount,
			ChunksSent:   metadata.ChunkCount, // Assume all chunks sent if marking as transmitted
			ErrorMessage: errorMsg,
		}

		rc.transmissionLog = append(rc.transmissionLog, record)

		if success {
			log.Printf("Worker %d marked file as transmitted: %s", rc.workerID, filename)
		} else {
			log.Printf("Worker %d marked file transmission as failed: %s (error: %s)", rc.workerID, filename, errorMsg)
		}
	}
}

// GetTransmissionStatus returns transmission status for all files
func (rc *ResultCollector) GetTransmissionStatus() map[string]bool {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	status := make(map[string]bool)
	for filename, metadata := range rc.collectedFiles {
		status[filename] = metadata.IsTransmitted
	}
	return status
}

// GetTransmissionLog returns the transmission log
func (rc *ResultCollector) GetTransmissionLog() []TransmissionRecord {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	// Return a copy
	log := make([]TransmissionRecord, len(rc.transmissionLog))
	copy(log, rc.transmissionLog)
	return log
}

// IsAllFilesTransmitted checks if all collected files have been transmitted
func (rc *ResultCollector) IsAllFilesTransmitted() bool {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	for _, metadata := range rc.collectedFiles {
		if !metadata.IsTransmitted {
			return false
		}
	}
	return true
}

// GetPendingFiles returns files that haven't been transmitted yet
func (rc *ResultCollector) GetPendingFiles() []string {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	var pending []string
	for filename, metadata := range rc.collectedFiles {
		if !metadata.IsTransmitted {
			pending = append(pending, filename)
		}
	}
	return pending
}

// calculateFileChecksum calculates SHA256 checksum for a file
func (rc *ResultCollector) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// calculateDataChecksum calculates SHA256 checksum for data
func (rc *ResultCollector) calculateDataChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// determineContentType determines the content type based on file extension
func (rc *ResultCollector) determineContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	contentTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".tiff": "image/tiff",
		".txt":  "text/plain",
		".json": "application/json",
		".xml":  "application/xml",
		".csv":  "text/csv",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
	}

	if contentType, exists := contentTypes[ext]; exists {
		return contentType
	}

	return "application/octet-stream" // Default binary type
}

// getChunkedFileCount returns the number of files that require chunked transmission
func (rc *ResultCollector) getChunkedFileCount() int {
	count := 0
	for _, metadata := range rc.collectedFiles {
		if metadata.Size > rc.maxFileSize {
			count++
		}
	}
	return count
}

// getInlineFileCount returns the number of files that can be transmitted inline
func (rc *ResultCollector) getInlineFileCount() int {
	count := 0
	for _, metadata := range rc.collectedFiles {
		if metadata.Size <= rc.maxFileSize {
			count++
		}
	}
	return count
}

// GetStatistics returns collection and transmission statistics
func (rc *ResultCollector) GetStatistics() map[string]interface{} {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	totalSize := int64(0)
	transmittedCount := 0

	for _, metadata := range rc.collectedFiles {
		totalSize += metadata.Size
		if metadata.IsTransmitted {
			transmittedCount++
		}
	}

	return map[string]interface{}{
		"total_files":              len(rc.collectedFiles),
		"transmitted_files":        transmittedCount,
		"pending_files":            len(rc.collectedFiles) - transmittedCount,
		"total_size_bytes":         totalSize,
		"chunked_files":            rc.getChunkedFileCount(),
		"inline_files":             rc.getInlineFileCount(),
		"chunk_size":               rc.chunkSize,
		"max_inline_size":          rc.maxFileSize,
		"transmission_log_entries": len(rc.transmissionLog),
	}
}

// Reset clears all collected files and transmission log
func (rc *ResultCollector) Reset() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	rc.collectedFiles = make(map[string]*FileMetadata)
	rc.transmissionLog = make([]TransmissionRecord, 0)

	log.Printf("Worker %d result collector reset", rc.workerID)
}
