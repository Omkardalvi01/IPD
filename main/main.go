package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/messaging"
)

func main() {
	// Check if running in worker node mode
	if os.Getenv("IPD_MODE") == "worker-node" {
		fmt.Println("🚀 Starting in Worker Node Mode")
		runWorkerNodeMode()
		return
	}

	var wg sync.WaitGroup

	// Load configuration
	configPath := GetConfigPath()
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Printf("Warning: Failed to load configuration from %s: %v", configPath, err)
		log.Println("Using default configuration...")
		config = DefaultAppConfig()
	}

	// Create default config file if it doesn't exist
	if err := CreateDefaultConfigFile(configPath); err != nil {
		log.Printf("Warning: Failed to create default config file: %v", err)
	}

	fmt.Printf("IPD Enhanced Worker System\n")
	fmt.Printf("Configuration loaded from: %s\n", configPath)
	fmt.Printf("Max workers: %d, Heartbeat interval: %v\n",
		config.System.MaxWorkers, config.System.HeartbeatInterval)

	// Get input directory
	var dir string
	if len(os.Args) > 1 {
		dir = os.Args[1]
	} else {
		dir = "./train"
	}

	f, err := os.Open(dir)
	if err != nil {
		log.Fatal("Error while opening file", err)
	}
	defer f.Close()

	n, files, err := get_data(f)
	if err != nil {
		log.Fatal("Error while reading dir", err)
	}

	// Get number of workers with configuration limits
	var numWorkers int
	MaxWorkers := runtime.NumCPU()
	if MaxWorkers > config.System.MaxWorkers {
		MaxWorkers = config.System.MaxWorkers
	}

	fmt.Printf("Enter number of workers (recommended less than %d for your device, max %d from config)\nWorkers: ",
		MaxWorkers, config.System.MaxWorkers)
	fmt.Scan(&numWorkers)

	if numWorkers > config.System.MaxWorkers {
		fmt.Printf("Limiting workers to configured maximum: %d\n", config.System.MaxWorkers)
		numWorkers = config.System.MaxWorkers
	}

	uid := create_uid()
	fmt.Println("Connection_id:", uid)

	// Create processing configuration from system config
	processingConfig := &ProcessingConfig{
		PythonScriptPath:  config.System.ExternalScriptPath,
		InputDirectory:    config.System.InputBaseDirectory,
		OutputDirectory:   config.System.OutputBaseDirectory,
		ProcessingTimeout: config.System.ProcessingTimeout,
		HeartbeatInterval: config.System.HeartbeatInterval,
	}

	// Initialize enhanced result collection system
	sharedResultsDir := "./shared_results"
	serverCollector := NewServerResultCollector(numWorkers, sharedResultsDir)

	// Initialize connection lifecycle manager with configuration
	lifecycleManager := NewConnectionLifecycleManager(
		config.System.ConnectionTimeout,
		config.System.ConnectionTimeout*2,
	)

	// Set up connection lifecycle handlers
	lifecycleManager.SetCompletionHandler(func(workerID int, reason messaging.ConnectionCloseReason) {
		fmt.Printf("🔗 Worker %d connection closed: %s\n", workerID, reason)
	})

	lifecycleManager.SetErrorHandler(func(workerID int, err error) {
		fmt.Printf("❌ Worker %d connection error: %v\n", workerID, err)
	})

	// Initialize server message handler with enhanced integration
	messageHandler := NewServerMessageHandler(serverCollector, lifecycleManager)

	// Start result collection monitoring
	go monitorResultCollection(serverCollector)

	// Start heartbeat monitoring
	go monitorHeartbeats(lifecycleManager, config.System.HeartbeatInterval)

	// Initialize persistent worker pool instead of legacy worker pool
	resultchan := make(chan Result)
	persistentPool := NewPersistentWorkerPool(resultchan, processingConfig)

	// Set up integration between components
	setupComponentIntegration(persistentPool, messageHandler, lifecycleManager)

	// Start persistent worker pool
	fmt.Printf("🚀 Starting %d persistent workers...\n", numWorkers)
	channel_pool, err := persistentPool.StartPool(numWorkers, uid, &wg)
	if err != nil {
		log.Fatalf("Failed to start persistent worker pool: %v", err)
	}

	// Monitor worker results with enhanced logging
	go func() {
		i := 1
		for result := range resultchan {
			fmt.Printf("📤 Worker: %d status: %v uploaded: %d/%d\n",
				result.worker_id, result.result, i, n)
			i++
		}
	}()

	// Wait for workers to initialize
	wg.Wait()
	fmt.Printf("✅ All %d persistent workers initialized\n", numWorkers)

	// Distribute images to workers
	fmt.Printf("📂 Distributing %d images to workers...\n", len(files))
	for i, file_entries := range files {
		file_path := filepath.Join(dir, file_entries.Name())

		file, err := os.Open(file_path)
		if err != nil {
			log.Printf("Error while reading file %s error %v\n", file_path, err)
			continue
		}

		channel_pool[i%numWorkers] <- Request{f: file}
	}

	// Close request channels to signal end of image distribution
	for i := 0; i < len(channel_pool); i++ {
		close(channel_pool[i])
	}

	fmt.Printf("📡 Image distribution complete. Workers maintaining connections for processing...\n")

	// Monitor worker states during processing
	go monitorWorkerStates(persistentPool, config.System.HeartbeatInterval)

	// Wait for all workers to complete their processing and result transmission
	fmt.Println("⏳ Waiting for all workers to complete processing and result transmission...")

	// Set up timeout for overall processing
	processingTimeout := time.After(config.System.ProcessingTimeout * time.Duration(numWorkers))

	select {
	case <-serverCollector.GetAllCompleteChannel():
		fmt.Printf("🎉 All workers completed! Results stored in: %s\n",
			serverCollector.GetSharedResultsDirectory())

		// Print final statistics
		printFinalStatistics(serverCollector, persistentPool)

	case <-processingTimeout:
		fmt.Printf("⏰ Processing timeout reached. Stopping remaining workers...\n")
		persistentPool.StopAll()

		// Print partial results
		fmt.Printf("⚠️  Partial results available in: %s\n",
			serverCollector.GetSharedResultsDirectory())
	}

	// Graceful shutdown
	fmt.Println("🛑 Initiating graceful shutdown...")
	persistentPool.StopAll()

	// Give time for cleanup
	time.Sleep(2 * time.Second)
	fmt.Println("✅ Shutdown complete")
}

// monitorResultCollection monitors the result collection process
func monitorResultCollection(collector *ServerResultCollector) {
	completionChan := collector.GetCompletionChannel()
	allCompleteChan := collector.GetAllCompleteChannel()

	for {
		select {
		case workerID := <-completionChan:
			fmt.Printf("✓ Worker %d completed result transmission (%d/%d)\n",
				workerID, collector.GetCompletedWorkerCount(), collector.GetExpectedWorkerCount())

		case <-allCompleteChan:
			fmt.Println("🎉 All workers have completed result transmission!")

			// Print validation errors if any
			validationErrors := collector.GetValidationErrors()
			if len(validationErrors) > 0 {
				fmt.Printf("⚠️  Validation errors found for %d workers:\n", len(validationErrors))
				for workerID, errors := range validationErrors {
					fmt.Printf("  Worker %d: %d errors\n", workerID, len(errors))
					for _, err := range errors {
						fmt.Printf("    - %s\n", err)
					}
				}
			} else {
				fmt.Println("✅ No validation errors found")
			}

			return

		case <-time.After(30 * time.Second):
			// Periodic status update
			if !collector.IsComplete() {
				fmt.Printf("📊 Result collection status: %d/%d workers completed\n",
					collector.GetCompletedWorkerCount(), collector.GetExpectedWorkerCount())
			}
		}
	}
}

// monitorHeartbeats monitors heartbeat status across all workers
func monitorHeartbeats(lifecycleManager *ConnectionLifecycleManager, interval time.Duration) {
	ticker := time.NewTicker(interval * 2) // Check every 2x heartbeat interval
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get connection health status
			healthyConnections := lifecycleManager.GetHealthyConnectionCount()
			totalConnections := lifecycleManager.GetTotalConnectionCount()

			if totalConnections > 0 {
				fmt.Printf("💓 Heartbeat status: %d/%d connections healthy\n",
					healthyConnections, totalConnections)

				// Log unhealthy connections
				unhealthyConnections := lifecycleManager.GetUnhealthyConnections()
				for _, workerID := range unhealthyConnections {
					fmt.Printf("⚠️  Worker %d connection unhealthy\n", workerID)
				}
			}
		}
	}
}

// monitorWorkerStates monitors the state of all workers during processing
func monitorWorkerStates(pool *PersistentWorkerPool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			states := pool.GetWorkerStates()
			if len(states) == 0 {
				continue
			}

			// Count states
			stateCounts := make(map[WorkerState]int)
			for _, state := range states {
				stateCounts[state]++
			}

			// Print state summary
			fmt.Printf("🔄 Worker states: ")
			for state, count := range stateCounts {
				fmt.Printf("%s:%d ", state, count)
			}
			fmt.Println()

			// Check if all workers are complete
			if pool.IsAllWorkersComplete() {
				fmt.Println("✅ All workers have completed processing")
				return
			}
		}
	}
}

// printFinalStatistics prints final processing statistics
func printFinalStatistics(collector *ServerResultCollector, pool *PersistentWorkerPool) {
	fmt.Println("\n📈 Final Processing Statistics:")
	fmt.Printf("  Total workers: %d\n", collector.GetExpectedWorkerCount())
	fmt.Printf("  Completed workers: %d\n", collector.GetCompletedWorkerCount())
	fmt.Printf("  Healthy workers: %d\n", pool.GetHealthyWorkerCount())

	validationErrors := collector.GetValidationErrors()
	fmt.Printf("  Workers with validation errors: %d\n", len(validationErrors))

	// Print worker-specific statistics
	workers := pool.GetAllWorkers()
	for workerID, worker := range workers {
		status := worker.GetProcessingStatus()
		fmt.Printf("  Worker %d: %s", workerID, status["state"])

		if inputCount, ok := status["input_file_count"]; ok {
			fmt.Printf(" (input:%v", inputCount)
		}
		if outputCount, ok := status["output_file_count"]; ok {
			fmt.Printf(" output:%v)", outputCount)
		}
		fmt.Println()
	}

	fmt.Printf("  Results directory: %s\n", collector.GetSharedResultsDirectory())
}

// setupComponentIntegration sets up integration between persistent workers, message handler, and lifecycle manager
func setupComponentIntegration(pool *PersistentWorkerPool, messageHandler *ServerMessageHandler, lifecycleManager *ConnectionLifecycleManager) {
	log.Println("🔗 Setting up component integration...")

	// Set integration callback that will be called when each worker's data channel opens
	pool.SetIntegrationCallback(func(worker *PersistentWorker) {
		integrateWorkerWithServer(worker, messageHandler, lifecycleManager)
	})
}

// integrateWorkerWithServer integrates a worker with server-side components
func integrateWorkerWithServer(worker *PersistentWorker, messageHandler *ServerMessageHandler, lifecycleManager *ConnectionLifecycleManager) {
	workerID := worker.GetWorkerID()

	// Register worker's data channel with message handler
	if worker.data_channel != nil {
		messageHandler.RegisterWorkerChannel(workerID, worker.data_channel)
	}

	// Register connection with lifecycle manager
	if worker.data_channel != nil && worker.peer_conn != nil {
		lifecycleManager.RegisterConnection(workerID, worker.data_channel, worker.peer_conn)
	}

	log.Printf("🔗 Integrated worker %d with server components", workerID)
}

// handleWorkerResultMessage handles incoming result messages from workers
func handleWorkerResultMessage(collector *ServerResultCollector, workerID int, msg *messaging.Message) {
	if err := collector.HandleResultMessage(workerID, msg); err != nil {
		log.Printf("Error handling result message from worker %d: %v", workerID, err)
	}
}
