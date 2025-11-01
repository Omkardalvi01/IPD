package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)
const(
	Role = "C"
)

type allocation struct{
	Uids map[string]int `json:"allocation"`
}

type aggregator struct{
	NumEdges int `json:"numEdges"`
	UIDandPercent map[string]int `json:"file_and_weight"`
}

var ( 
ALGO_LIVE_LINK = "wss://ipd-allocator-1.onrender.com/ws"
ALGO_TEST_LINK = "ws://localhost:13000/join"
)

func main() {
	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	var edge_connection sync.WaitGroup
	var worker_done sync.WaitGroup
	
	// Set up directory
	dir := "./test"
	n, files, err := get_data(dir)
	if err != nil {
		log.Fatal("Error while reading dir: ", err)
	}

	// Get number of workers
	var numWorkers int
	MaxWorkers := runtime.NumCPU()
	fmt.Printf("Enter number of workers (recommended less than %d for your device)\nWorkers: ", MaxWorkers)
	fmt.Scan(&numWorkers)

	// Create room ID and connect to algorithm service
	room_id := create_uid()
	fmt.Println("Connection_id:", room_id)

	algo_service, _, err := websocket.DefaultDialer.Dial(ALGO_LIVE_LINK, nil)
	if err != nil {
		log.Fatal("Error while creating connection to algorithm service: ", err)
	}
	defer algo_service.Close()

	// Send initial connection message
	postbody := map[string]interface{}{
		"role":     Role,
		"room_id":  room_id,
		"num_edges": numWorkers,
	}

	if err := algo_service.WriteJSON(postbody); err != nil {
		log.Fatal("Error while writing to connection: ", err)
	}

	// Handle keepalive for algorithm service
	alive := make(chan struct{})
	defer close(alive)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := algo_service.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					log.Println("Failed to send ping: ", err)
					return
				}
			case <-alive:
				return
			}
		}
	}()

	// Read allocation from algorithm service
	var allocate allocation
	if err := algo_service.ReadJSON(&allocate); err != nil {
		log.Fatal("Error while reading from connection: ", err)
	}
	fmt.Println("Response from algo service: ", allocate)

	// Process allocation
	edge_id := make([]string, 0, len(allocate.Uids))
	for id := range allocate.Uids {
		edge_id = append(edge_id, id)
	}

	for _, id := range edge_id {
		fmt.Printf("ID: %s Weights: %d\n", id, allocate.Uids[id])
	}

	// Initialize worker pool
	resultchan := make(chan Result, n)
	wp := Workerpool{resultchan: resultchan}
	wp.start_pool(numWorkers, n, edge_id, allocate.Uids, &edge_connection, &worker_done)

	// Start result processor
	go func() {
		for i := 1; i <= n; i++ {
			result := <-resultchan
			fmt.Printf("worker: %d status: %v uploaded: %d/%d\n", result.worker_id, result.result, i, n)
		}
		close(resultchan)
	}()

	// Wait for workers to connect
	edge_connection.Wait()

	// Distribute files to workers
	for _, file_path := range files {
		worker := wp.pickWorker()
		worker.req_chan <- Request{f: file_path}
	}

	// Close worker channels
	for i := 0; i < numWorkers; i++ {
		close(wp.workers[i].req_chan)
	}

	// Wait for all workers to finish
	worker_done.Wait()

	// Prepare and send aggregation data to the API server
	// First, get the list of edge IDs that participated in this round
	edgeIDs := make([]string, 0, len(allocate.Uids))
	for id := range allocate.Uids {
		edgeIDs = append(edgeIDs, id)
	}

	// Prepare the request payload with UIDs
	requestData := map[string]interface{}{
		"uids": edgeIDs,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Fatal("Error while converting to JSON: ", err)
	}

	// Make the HTTP POST request to the aggregator API
	apiURL := "http://localhost:8000/receive_uids"
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatal("Error while posting to aggregator API: ", err)
	}
	defer resp.Body.Close()

	// Read and parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error reading response body: ", err)
	}

	// Parse the JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Fatal("Error parsing response: ", err)
	}

	// Log the response
	fmt.Printf("\n=== Aggregation Results ===\n")
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Message: %v\n", result["message"])
	
	// If successful, show model location
	if resp.StatusCode == http.StatusOK {
		if modelPath, ok := result["model_path"].(string); ok {
			fmt.Printf("Global model saved at: %s\n", modelPath)
		}
		if numClients, ok := result["num_clients"].(float64); ok {
			fmt.Printf("Number of clients aggregated: %.0f\n", numClients)
		}
	} else {
		// Show error details if the request failed
		fmt.Printf("Error details: %v\n", result)
	}

	// Wait for interrupt signal
	<-signalChan
	log.Println("Shutting down...")


}