package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

// String returns the string representation of LogLevel
func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	WorkerID  int                    `json:"worker_id,omitempty"`
	Component string                 `json:"component,omitempty"`
	Operation string                 `json:"operation,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Function  string                 `json:"function,omitempty"`
}

// LoggerConfig holds configuration for the enhanced logger
type LoggerConfig struct {
	Level         LogLevel      `json:"level"`
	EnableConsole bool          `json:"enable_console"`
	EnableFile    bool          `json:"enable_file"`
	EnableJSON    bool          `json:"enable_json"`
	LogDirectory  string        `json:"log_directory"`
	MaxFileSize   int64         `json:"max_file_size"`  // in bytes
	MaxFiles      int           `json:"max_files"`      // number of log files to keep
	EnableCaller  bool          `json:"enable_caller"`  // include file/line info
	BufferSize    int           `json:"buffer_size"`    // buffer size for async logging
	FlushInterval time.Duration `json:"flush_interval"` // how often to flush buffers
}

// DefaultLoggerConfig returns a default logger configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:         LogLevelInfo,
		EnableConsole: true,
		EnableFile:    true,
		EnableJSON:    true,
		LogDirectory:  "./logs",
		MaxFileSize:   10 * 1024 * 1024, // 10MB
		MaxFiles:      5,
		EnableCaller:  true,
		BufferSize:    1000,
		FlushInterval: 5 * time.Second,
	}
}

// EnhancedLogger provides structured logging with multiple outputs
type EnhancedLogger struct {
	config        *LoggerConfig
	fileWriter    io.Writer
	consoleWriter io.Writer
	logBuffer     chan LogEntry
	mutex         sync.RWMutex
	stopChan      chan struct{}
	wg            sync.WaitGroup
	currentFile   *os.File
	currentSize   int64
	fileIndex     int
}

// NewEnhancedLogger creates a new enhanced logger
func NewEnhancedLogger(config *LoggerConfig) (*EnhancedLogger, error) {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	logger := &EnhancedLogger{
		config:        config,
		consoleWriter: os.Stdout,
		logBuffer:     make(chan LogEntry, config.BufferSize),
		stopChan:      make(chan struct{}),
	}

	// Create log directory if file logging is enabled
	if config.EnableFile {
		if err := os.MkdirAll(config.LogDirectory, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		if err := logger.rotateLogFile(); err != nil {
			return nil, fmt.Errorf("failed to create initial log file: %w", err)
		}
	}

	// Start background logging goroutine
	logger.wg.Add(1)
	go logger.processLogEntries()

	// Start periodic flush goroutine
	logger.wg.Add(1)
	go logger.periodicFlush()

	return logger, nil
}

// processLogEntries processes log entries from the buffer
func (el *EnhancedLogger) processLogEntries() {
	defer el.wg.Done()

	for {
		select {
		case entry := <-el.logBuffer:
			el.writeLogEntry(entry)
		case <-el.stopChan:
			// Process remaining entries
			for {
				select {
				case entry := <-el.logBuffer:
					el.writeLogEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// periodicFlush periodically flushes log buffers
func (el *EnhancedLogger) periodicFlush() {
	defer el.wg.Done()

	ticker := time.NewTicker(el.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			el.flush()
		case <-el.stopChan:
			el.flush()
			return
		}
	}
}

// writeLogEntry writes a log entry to configured outputs
func (el *EnhancedLogger) writeLogEntry(entry LogEntry) {
	// Check if entry level meets minimum level
	if entry.Level < el.config.Level {
		return
	}

	var output string
	if el.config.EnableJSON {
		jsonData, err := json.Marshal(entry)
		if err != nil {
			// Fallback to simple format if JSON marshaling fails
			output = el.formatSimpleEntry(entry)
		} else {
			output = string(jsonData)
		}
	} else {
		output = el.formatSimpleEntry(entry)
	}

	output += "\n"

	// Write to console
	if el.config.EnableConsole {
		el.consoleWriter.Write([]byte(output))
	}

	// Write to file
	if el.config.EnableFile && el.currentFile != nil {
		el.mutex.Lock()
		n, err := el.currentFile.WriteString(output)
		if err == nil {
			el.currentSize += int64(n)
			// Check if file rotation is needed
			if el.currentSize >= el.config.MaxFileSize {
				el.rotateLogFile()
			}
		}
		el.mutex.Unlock()
	}
}

// formatSimpleEntry formats a log entry in a simple text format
func (el *EnhancedLogger) formatSimpleEntry(entry LogEntry) string {
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05.000")

	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", timestamp))
	parts = append(parts, fmt.Sprintf("[%s]", entry.Level.String()))

	if entry.WorkerID > 0 {
		parts = append(parts, fmt.Sprintf("[Worker:%d]", entry.WorkerID))
	}

	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
	}

	if entry.Operation != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Operation))
	}

	if el.config.EnableCaller && entry.File != "" {
		parts = append(parts, fmt.Sprintf("[%s:%d]", filepath.Base(entry.File), entry.Line))
	}

	parts = append(parts, entry.Message)

	if entry.Error != "" {
		parts = append(parts, fmt.Sprintf("Error: %s", entry.Error))
	}

	return fmt.Sprintf("%s", joinStrings(parts, " "))
}

// joinStrings joins strings with a separator
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// rotateLogFile rotates the current log file
func (el *EnhancedLogger) rotateLogFile() error {
	// Close current file if open
	if el.currentFile != nil {
		el.currentFile.Close()
	}

	// Generate new filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("ipd_%s_%d.log", timestamp, el.fileIndex)
	filepath := filepath.Join(el.config.LogDirectory, filename)

	// Create new file
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	el.currentFile = file
	el.currentSize = 0
	el.fileIndex++

	// Clean up old files
	el.cleanupOldFiles()

	return nil
}

// cleanupOldFiles removes old log files beyond the configured limit
func (el *EnhancedLogger) cleanupOldFiles() {
	files, err := filepath.Glob(filepath.Join(el.config.LogDirectory, "ipd_*.log"))
	if err != nil {
		return
	}

	if len(files) <= el.config.MaxFiles {
		return
	}

	// Sort files by modification time (oldest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var fileInfos []fileInfo
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, fileInfo{
			path:    file,
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time
	for i := 0; i < len(fileInfos)-1; i++ {
		for j := i + 1; j < len(fileInfos); j++ {
			if fileInfos[i].modTime.After(fileInfos[j].modTime) {
				fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
			}
		}
	}

	// Remove oldest files
	filesToRemove := len(fileInfos) - el.config.MaxFiles
	for i := 0; i < filesToRemove; i++ {
		os.Remove(fileInfos[i].path)
	}
}

// createLogEntry creates a log entry with caller information
func (el *EnhancedLogger) createLogEntry(level LogLevel, message string, workerID int, component, operation string, err error, context map[string]interface{}) LogEntry {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		WorkerID:  workerID,
		Component: component,
		Operation: operation,
		Context:   context,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	// Add caller information if enabled
	if el.config.EnableCaller {
		if pc, file, line, ok := runtime.Caller(3); ok {
			entry.File = file
			entry.Line = line
			if fn := runtime.FuncForPC(pc); fn != nil {
				entry.Function = fn.Name()
			}
		}
	}

	return entry
}

// log sends a log entry to the buffer
func (el *EnhancedLogger) log(entry LogEntry) {
	select {
	case el.logBuffer <- entry:
		// Entry buffered successfully
	default:
		// Buffer full, write directly to avoid blocking
		el.writeLogEntry(entry)
	}
}

// Debug logs a debug message
func (el *EnhancedLogger) Debug(message string) {
	entry := el.createLogEntry(LogLevelDebug, message, 0, "", "", nil, nil)
	el.log(entry)
}

// DebugWithContext logs a debug message with context
func (el *EnhancedLogger) DebugWithContext(message string, workerID int, component, operation string, context map[string]interface{}) {
	entry := el.createLogEntry(LogLevelDebug, message, workerID, component, operation, nil, context)
	el.log(entry)
}

// Info logs an info message
func (el *EnhancedLogger) Info(message string) {
	entry := el.createLogEntry(LogLevelInfo, message, 0, "", "", nil, nil)
	el.log(entry)
}

// InfoWithContext logs an info message with context
func (el *EnhancedLogger) InfoWithContext(message string, workerID int, component, operation string, context map[string]interface{}) {
	entry := el.createLogEntry(LogLevelInfo, message, workerID, component, operation, nil, context)
	el.log(entry)
}

// Warn logs a warning message
func (el *EnhancedLogger) Warn(message string) {
	entry := el.createLogEntry(LogLevelWarn, message, 0, "", "", nil, nil)
	el.log(entry)
}

// WarnWithContext logs a warning message with context
func (el *EnhancedLogger) WarnWithContext(message string, workerID int, component, operation string, err error, context map[string]interface{}) {
	entry := el.createLogEntry(LogLevelWarn, message, workerID, component, operation, err, context)
	el.log(entry)
}

// Error logs an error message
func (el *EnhancedLogger) Error(message string, err error) {
	entry := el.createLogEntry(LogLevelError, message, 0, "", "", err, nil)
	el.log(entry)
}

// ErrorWithContext logs an error message with context
func (el *EnhancedLogger) ErrorWithContext(message string, workerID int, component, operation string, err error, context map[string]interface{}) {
	entry := el.createLogEntry(LogLevelError, message, workerID, component, operation, err, context)
	el.log(entry)
}

// Fatal logs a fatal message and exits
func (el *EnhancedLogger) Fatal(message string, err error) {
	entry := el.createLogEntry(LogLevelFatal, message, 0, "", "", err, nil)
	el.log(entry)
	el.flush()
	os.Exit(1)
}

// LogProcessingError logs a ProcessingError with full context
func (el *EnhancedLogger) LogProcessingError(procErr *ProcessingError) {
	context := map[string]interface{}{
		"error_type":  string(procErr.Type),
		"severity":    string(procErr.Severity),
		"recoverable": procErr.Recoverable,
		"retry_count": procErr.RetryCount,
	}

	// Add custom context
	for key, value := range procErr.Context {
		context[key] = value
	}

	var level LogLevel
	switch procErr.Severity {
	case SeverityLow:
		level = LogLevelInfo
	case SeverityMedium:
		level = LogLevelWarn
	case SeverityHigh:
		level = LogLevelError
	case SeverityCritical:
		level = LogLevelError
	default:
		level = LogLevelError
	}

	entry := el.createLogEntry(level, procErr.Message, procErr.WorkerID,
		string(procErr.Type), procErr.Operation, procErr.Cause, context)
	el.log(entry)
}

// LogWorkerState logs worker state changes
func (el *EnhancedLogger) LogWorkerState(workerID int, oldState, newState string, context map[string]interface{}) {
	message := fmt.Sprintf("Worker state changed: %s -> %s", oldState, newState)
	el.InfoWithContext(message, workerID, "worker", "state_change", context)
}

// LogConnectionEvent logs connection-related events
func (el *EnhancedLogger) LogConnectionEvent(workerID int, event string, context map[string]interface{}) {
	el.InfoWithContext(event, workerID, "connection", "event", context)
}

// LogProcessingEvent logs processing-related events
func (el *EnhancedLogger) LogProcessingEvent(workerID int, event string, context map[string]interface{}) {
	el.InfoWithContext(event, workerID, "processing", "event", context)
}

// LogTransmissionEvent logs transmission-related events
func (el *EnhancedLogger) LogTransmissionEvent(workerID int, event string, context map[string]interface{}) {
	el.InfoWithContext(event, workerID, "transmission", "event", context)
}

// LogRetryAttempt logs retry attempts
func (el *EnhancedLogger) LogRetryAttempt(workerID int, operation string, attempt int, maxAttempts int, err error) {
	context := map[string]interface{}{
		"attempt":      attempt,
		"max_attempts": maxAttempts,
	}

	message := fmt.Sprintf("Retry attempt %d/%d for operation: %s", attempt, maxAttempts, operation)
	el.WarnWithContext(message, workerID, "retry", operation, err, context)
}

// LogDegradationEvent logs system degradation events
func (el *EnhancedLogger) LogDegradationEvent(level DegradationLevel, event DegradationEvent) {
	context := map[string]interface{}{
		"degradation_level": level.String(),
		"trigger":           event.Trigger,
		"failed_count":      event.FailedCount,
		"success_count":     event.SuccessCount,
		"total_count":       event.TotalCount,
	}

	message := fmt.Sprintf("System degradation: %s", level.String())

	var logLevel LogLevel
	switch level {
	case DegradationMinor:
		logLevel = LogLevelWarn
	case DegradationModerate:
		logLevel = LogLevelWarn
	case DegradationSevere:
		logLevel = LogLevelError
	case DegradationCritical:
		logLevel = LogLevelError
	default:
		logLevel = LogLevelInfo
	}

	entry := el.createLogEntry(logLevel, message, 0, "degradation", "level_change", nil, context)
	el.log(entry)
}

// flush flushes any buffered log entries
func (el *EnhancedLogger) flush() {
	if el.currentFile != nil {
		el.mutex.Lock()
		el.currentFile.Sync()
		el.mutex.Unlock()
	}
}

// Close closes the logger and flushes remaining entries
func (el *EnhancedLogger) Close() {
	close(el.stopChan)
	el.wg.Wait()

	if el.currentFile != nil {
		el.currentFile.Close()
	}
}

// SetLevel sets the minimum log level
func (el *EnhancedLogger) SetLevel(level LogLevel) {
	el.mutex.Lock()
	defer el.mutex.Unlock()
	el.config.Level = level
}

// GetLevel returns the current log level
func (el *EnhancedLogger) GetLevel() LogLevel {
	el.mutex.RLock()
	defer el.mutex.RUnlock()
	return el.config.Level
}

// GetLogStats returns logging statistics
func (el *EnhancedLogger) GetLogStats() map[string]interface{} {
	return map[string]interface{}{
		"buffer_size":       cap(el.logBuffer),
		"buffer_used":       len(el.logBuffer),
		"current_level":     el.config.Level.String(),
		"file_logging":      el.config.EnableFile,
		"console_logging":   el.config.EnableConsole,
		"json_format":       el.config.EnableJSON,
		"current_file_size": el.currentSize,
		"max_file_size":     el.config.MaxFileSize,
	}
}

// Global logger instance
var globalLogger *EnhancedLogger
var loggerOnce sync.Once

// InitializeGlobalLogger initializes the global logger
func InitializeGlobalLogger(config *LoggerConfig) error {
	var err error
	loggerOnce.Do(func() {
		globalLogger, err = NewEnhancedLogger(config)
	})
	return err
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *EnhancedLogger {
	if globalLogger == nil {
		// Initialize with default config if not already initialized
		InitializeGlobalLogger(nil)
	}
	return globalLogger
}

// Convenience functions using global logger
func LogDebug(message string) {
	GetGlobalLogger().Debug(message)
}

func LogInfo(message string) {
	GetGlobalLogger().Info(message)
}

func LogWarn(message string) {
	GetGlobalLogger().Warn(message)
}

func LogError(message string, err error) {
	GetGlobalLogger().Error(message, err)
}

func LogProcessingError(procErr *ProcessingError) {
	GetGlobalLogger().LogProcessingError(procErr)
}

func LogWorkerState(workerID int, oldState, newState string, context map[string]interface{}) {
	GetGlobalLogger().LogWorkerState(workerID, oldState, newState, context)
}

func LogRetryAttempt(workerID int, operation string, attempt int, maxAttempts int, err error) {
	GetGlobalLogger().LogRetryAttempt(workerID, operation, attempt, maxAttempts, err)
}
