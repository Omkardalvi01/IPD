package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
	"github.com/pion/webrtc/v3"
)

// FileTransmitter handles transmission of files through WebRTC data channels
type FileTransmitter struct {
	workerID        int
	dataChannel     *webrtc.DataChannel
	resultCollector *ResultCollector
	mutex           sync.RWMutex
	isTransmitting  bool
	ackTimeout      time.Duration
	retryAttempts   int
	ackWaitMap      map[string]chan bool
	ackMutex        sync.RWMutex
}

// TransmissionOptions configures file transmission behavior
type TransmissionOptions struct {
	AckTimeout    time.Duration
	RetryAttempts int
	ChunkDelay    time.Duration // Delay between chunk transmissions
}

// DefaultTransmissionOptions returns default transmission options
func DefaultTransmissionOptions() *TransmissionOptions {
	return &TransmissionOptions{
		AckTimeout:    30 * time.Second,
		RetryAttempts: 3,
		ChunkDelay:    100 * time.Millisecond,
	}
}

// NewFileTransmitter creates a new FileTransmitter instance
func NewFileTransmitter(workerID int, dataChannel *webrtc.DataChannel, resultCollector *ResultCollector) *FileTransmitter {
	return &FileTransmitter{
		workerID:        workerID,
		dataChannel:     dataChannel,
		resultCollector: resultCollector,
		isTransmitting:  false,
		ackTimeout:      30 * time.Second,
		retryAttempts:   3,
		ackWaitMap:      make(map[string]chan bool),
	}
}

// SetTransmissionOptions configures transmission behavior
func (ft *FileTransmitter) SetTransmissionOptions(options *TransmissionOptions) {
	ft.mutex.Lock()
	defer ft.mutex.Unlock()

	if options.AckTimeout > 0 {
		ft.ackTimeout = options.AckTimeout
	}
	if options.RetryAttempts >= 0 {
		ft.retryAttempts = options.RetryAttempts
	}
}

// IsTransmitting returns whether the transmitter is currently transmitting
func (ft *FileTransmitter) IsTransmitting() bool {
	ft.mutex.RLock()
	defer ft.mutex.RUnlock()
	return ft.isTransmitting
}

// setTransmitting sets the transmission state
func (ft *FileTransmitter) setTransmitting(transmitting bool) {
	ft.mutex.Lock()
	defer ft.mutex.Unlock()
	ft.isTransmitting = transmitting
}

// TransmitAllFiles transmits all collected files to the main server
func (ft *FileTransmitter) TransmitAllFiles(options *TransmissionOptions) error {
	if ft.IsTransmitting() {
		return fmt.Errorf("transmission already in progress")
	}

	if options == nil {
		options = DefaultTransmissionOptions()
	}

	ft.setTransmitting(true)
	defer ft.setTransmitting(false)

	log.Printf("Worker %d starting transmission of all collected files", ft.workerID)

	// Get pending files
	pendingFiles := ft.resultCollector.GetPendingFiles()
	if len(pendingFiles) == 0 {
		log.Printf("Worker %d no files to transmit", ft.workerID)
		return nil
	}

	log.Printf("Worker %d transmitting %d files", ft.workerID, len(pendingFiles))

	// Send transmission start notification
	if err := ft.sendTransmissionStart(len(pendingFiles)); err != nil {
		return fmt.Errorf("failed to send transmission start notification: %w", err)
	}

	successCount := 0
	errorCount := 0

	// Transmit each file
	for _, filename := range pendingFiles {
		log.Printf("Worker %d transmitting file: %s", ft.workerID, filename)

		if err := ft.TransmitFile(filename, options); err != nil {
			log.Printf("Worker %d failed to transmit file %s: %v", ft.workerID, filename, err)
			ft.resultCollector.MarkFileAsTransmitted(filename, false, err.Error())
			errorCount++
		} else {
			log.Printf("Worker %d successfully transmitted file: %s", ft.workerID, filename)
			ft.resultCollector.MarkFileAsTransmitted(filename, true, "")
			successCount++
		}

		// Add delay between files if specified
		if options.ChunkDelay > 0 {
			time.Sleep(options.ChunkDelay)
		}
	}

	// Send transmission end notification
	if err := ft.sendTransmissionEnd(successCount, errorCount); err != nil {
		log.Printf("Worker %d failed to send transmission end notification: %v", ft.workerID, err)
	}

	log.Printf("Worker %d transmission completed: %d successful, %d failed", ft.workerID, successCount, errorCount)

	if errorCount > 0 {
		return fmt.Errorf("transmission completed with %d errors out of %d files", errorCount, len(pendingFiles))
	}

	return nil
}

// TransmitFile transmits a single file with chunked transmission support
func (ft *FileTransmitter) TransmitFile(filename string, options *TransmissionOptions) error {
	if options == nil {
		options = DefaultTransmissionOptions()
	}

	// Get file metadata
	metadata, exists := ft.resultCollector.GetFileMetadata(filename)
	if !exists {
		return fmt.Errorf("file not found in collection: %s", filename)
	}

	log.Printf("Worker %d transmitting file %s (size: %d bytes, chunks: %d)",
		ft.workerID, filename, metadata.Size, metadata.ChunkCount)

	// Prepare file chunks
	chunks, err := ft.resultCollector.PrepareFileForTransmission(filename)
	if err != nil {
		return fmt.Errorf("failed to prepare file for transmission: %w", err)
	}

	// Send file start notification
	if err := ft.sendFileStart(filename, metadata); err != nil {
		return fmt.Errorf("failed to send file start notification: %w", err)
	}

	// Transmit chunks with retry logic
	for i, chunk := range chunks {
		var lastErr error
		transmitted := false

		for attempt := 0; attempt <= ft.retryAttempts; attempt++ {
			if attempt > 0 {
				log.Printf("Worker %d retrying chunk %d/%d for file %s (attempt %d/%d)",
					ft.workerID, i+1, len(chunks), filename, attempt, ft.retryAttempts)
				time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			}

			if err := ft.transmitChunk(chunk, options); err != nil {
				lastErr = err
				log.Printf("Worker %d failed to transmit chunk %d/%d for file %s: %v",
					ft.workerID, i+1, len(chunks), filename, err)
				continue
			}

			transmitted = true
			break
		}

		if !transmitted {
			return fmt.Errorf("failed to transmit chunk %d after %d attempts: %w", i, ft.retryAttempts+1, lastErr)
		}

		log.Printf("Worker %d transmitted chunk %d/%d for file %s", ft.workerID, i+1, len(chunks), filename)

		// Add delay between chunks if specified
		if options.ChunkDelay > 0 && i < len(chunks)-1 {
			time.Sleep(options.ChunkDelay)
		}
	}

	// Send file end notification
	if err := ft.sendFileEnd(filename, len(chunks)); err != nil {
		return fmt.Errorf("failed to send file end notification: %w", err)
	}

	log.Printf("Worker %d completed transmission of file %s (%d chunks)", ft.workerID, filename, len(chunks))
	return nil
}

// transmitChunk transmits a single chunk with acknowledgment
func (ft *FileTransmitter) transmitChunk(chunk *ChunkInfo, options *TransmissionOptions) error {
	// Create chunk message
	chunkMsg := messaging.CreateMessage(messaging.MsgTypeResult, ft.workerID, chunk.Filename, chunk.Data)

	// Add chunk metadata
	chunkMsg.Metadata["message_subtype"] = "file_chunk"
	chunkMsg.Metadata["chunk_index"] = chunk.ChunkIndex
	chunkMsg.Metadata["total_chunks"] = chunk.TotalChunks
	chunkMsg.Metadata["chunk_checksum"] = chunk.Checksum
	chunkMsg.Metadata["is_last_chunk"] = chunk.IsLast
	chunkMsg.Metadata["chunk_size"] = len(chunk.Data)

	// Send chunk with acknowledgment
	return ft.sendMessageWithAck(chunkMsg, options.AckTimeout)
}

// sendTransmissionStart sends a notification that file transmission is starting
func (ft *FileTransmitter) sendTransmissionStart(fileCount int) error {
	msg := messaging.CreateMessage(messaging.MsgTypeResult, ft.workerID, "", nil)
	msg.Metadata["message_subtype"] = "transmission_start"
	msg.Metadata["file_count"] = fileCount
	msg.Metadata["worker_state"] = "SENDING"

	return ft.sendMessage(msg)
}

// sendTransmissionEnd sends a notification that file transmission has ended
func (ft *FileTransmitter) sendTransmissionEnd(successCount, errorCount int) error {
	msg := messaging.CreateMessage(messaging.MsgTypeResult, ft.workerID, "", nil)
	msg.Metadata["message_subtype"] = "transmission_end"
	msg.Metadata["success_count"] = successCount
	msg.Metadata["error_count"] = errorCount
	msg.Metadata["worker_state"] = "COMPLETE"

	return ft.sendMessage(msg)
}

// sendFileStart sends a notification that a file transmission is starting
func (ft *FileTransmitter) sendFileStart(filename string, metadata *FileMetadata) error {
	msg := messaging.CreateMessage(messaging.MsgTypeResult, ft.workerID, filename, nil)
	msg.Metadata["message_subtype"] = "file_start"
	msg.Metadata["file_size"] = metadata.Size
	msg.Metadata["file_checksum"] = metadata.Checksum
	msg.Metadata["chunk_count"] = metadata.ChunkCount
	msg.Metadata["content_type"] = metadata.ContentType
	msg.Metadata["modified_time"] = metadata.ModTime

	return ft.sendMessageWithAck(msg, ft.ackTimeout)
}

// sendFileEnd sends a notification that a file transmission has ended
func (ft *FileTransmitter) sendFileEnd(filename string, chunkCount int) error {
	msg := messaging.CreateMessage(messaging.MsgTypeResult, ft.workerID, filename, nil)
	msg.Metadata["message_subtype"] = "file_end"
	msg.Metadata["chunks_sent"] = chunkCount

	return ft.sendMessageWithAck(msg, ft.ackTimeout)
}

// sendMessage sends a message through the data channel
func (ft *FileTransmitter) sendMessage(msg *messaging.Message) error {
	if ft.dataChannel == nil || ft.dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel is not open")
	}

	data, err := messaging.SerializeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	if err := ft.dataChannel.Send(data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// sendMessageWithAck sends a message and waits for acknowledgment
func (ft *FileTransmitter) sendMessageWithAck(msg *messaging.Message, timeout time.Duration) error {
	// Generate unique acknowledgment ID
	ackID := fmt.Sprintf("%d_%d_%s", ft.workerID, time.Now().UnixNano(), msg.Type)

	// Add acknowledgment ID to message metadata
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}
	msg.Metadata["ack_id"] = ackID

	// Create acknowledgment channel
	ackChan := make(chan bool, 1)

	ft.ackMutex.Lock()
	ft.ackWaitMap[ackID] = ackChan
	ft.ackMutex.Unlock()

	// Send the message
	if err := ft.sendMessage(msg); err != nil {
		ft.ackMutex.Lock()
		delete(ft.ackWaitMap, ackID)
		ft.ackMutex.Unlock()
		return err
	}

	// Wait for acknowledgment or timeout
	select {
	case success := <-ackChan:
		if !success {
			return fmt.Errorf("received negative acknowledgment")
		}
		return nil
	case <-time.After(timeout):
		ft.ackMutex.Lock()
		delete(ft.ackWaitMap, ackID)
		ft.ackMutex.Unlock()
		return fmt.Errorf("acknowledgment timeout for message type: %s", msg.Type)
	}
}

// HandleAcknowledgment handles acknowledgment messages from the server
func (ft *FileTransmitter) HandleAcknowledgment(msg *messaging.Message) error {
	// Extract acknowledgment ID from metadata
	ackID, exists := msg.Metadata["ack_id"]
	if !exists {
		return fmt.Errorf("acknowledgment message missing ack_id")
	}

	ackIDStr, ok := ackID.(string)
	if !ok {
		return fmt.Errorf("invalid ack_id type")
	}

	// Determine if acknowledgment is positive or negative
	success := true
	if ackStatus, exists := msg.Metadata["ack_status"]; exists {
		if status, ok := ackStatus.(string); ok && status == "error" {
			success = false
		}
	}

	ft.ackMutex.Lock()
	if ackChan, exists := ft.ackWaitMap[ackIDStr]; exists {
		select {
		case ackChan <- success:
		default:
		}
		delete(ft.ackWaitMap, ackIDStr)
	}
	ft.ackMutex.Unlock()

	log.Printf("Worker %d processed acknowledgment: %s (success: %t)", ft.workerID, ackIDStr, success)
	return nil
}

// GetTransmissionStatistics returns transmission statistics
func (ft *FileTransmitter) GetTransmissionStatistics() map[string]interface{} {
	ft.mutex.RLock()
	defer ft.mutex.RUnlock()

	stats := map[string]interface{}{
		"is_transmitting": ft.isTransmitting,
		"ack_timeout":     ft.ackTimeout.String(),
		"retry_attempts":  ft.retryAttempts,
		"pending_acks":    len(ft.ackWaitMap),
	}

	// Add result collector statistics
	if ft.resultCollector != nil {
		collectorStats := ft.resultCollector.GetStatistics()
		for key, value := range collectorStats {
			stats["collector_"+key] = value
		}
	}

	return stats
}

// Cleanup cleans up pending acknowledgments and resources
func (ft *FileTransmitter) Cleanup() {
	ft.ackMutex.Lock()
	defer ft.ackMutex.Unlock()

	// Signal failure to all pending acknowledgments
	for ackID, ackChan := range ft.ackWaitMap {
		select {
		case ackChan <- false:
		default:
		}
		delete(ft.ackWaitMap, ackID)
	}

	log.Printf("Worker %d file transmitter cleanup completed", ft.workerID)
}
