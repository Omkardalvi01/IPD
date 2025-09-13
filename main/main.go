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
ALGO_LIVE_LINK = "wss://ipd-allocator-yz2k.onrender.com"
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

	algo_service, _, err := websocket.DefaultDialer.Dial(ALGO_TEST_LINK, nil)
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
	wp.start_pool(numWorkers, edge_id, allocate.Uids, &edge_connection, &worker_done)

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

	// Prepare and send aggregation data
	agg_file := aggregator{
		NumEdges:     numWorkers,
		UIDandPercent: allocate.Uids,
	}

	agg_req_body, err := json.Marshal(agg_file)
	if err != nil {
		log.Fatal("Error while converting to JSON: ", err)
	}

	resp, err := http.Post("http://localhost:8000", "application/json", bytes.NewBuffer(agg_req_body))
	if err != nil {
		log.Fatal("Error while posting to aggregator: ", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response Status: %s\nResponse Body: %s\n", resp.Status, string(body))

	// Wait for interrupt signal
	<-signalChan
	log.Println("Shutting down...")


}