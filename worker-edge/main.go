package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var configPath string
	var workerID string

	flag.StringVar(&configPath, "config", "config.json", "Path to configuration file")
	flag.StringVar(&workerID, "worker-id", "", "Worker ID for this edge node")
	flag.Parse()

	if workerID == "" {
		log.Fatal("Worker ID is required. Use -worker-id flag")
	}

	fmt.Printf("Starting Worker Edge Node: %s\n", workerID)
	fmt.Printf("Using config file: %s\n", configPath)

	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	config.WorkerID = workerID

	// Validate environment
	if err := ValidateEnvironment(config); err != nil {
		log.Fatalf("Environment validation failed: %v", err)
	}

	// Create worker instance
	worker := NewWorker(config)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal, gracefully shutting down...")
		worker.Shutdown()
		os.Exit(0)
	}()

	// Start worker
	if err := worker.Start(); err != nil {
		log.Fatalf("Worker failed to start: %v", err)
	}
}
