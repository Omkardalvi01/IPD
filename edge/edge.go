package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/networking"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)



const (
	Role string = "E"
	END  string = "EOF"
)

func id_maker() string {
	u, _ := uuid.NewUUID()
	return u.String()
}

const (
	ALGO_TEST_LINK = "ws://localhost:13000/join"
	ALGO_LIVE_LINK = "wss://ipd-allocator-1.onrender.com/ws"
	ML_MODEL       = "ws://localhost:8765"
)

// sendfiletouser
func sendFileToServer(uid, filePath string, dc *webrtc.DataChannel) error {
	log.Printf("Attempting to send file: %s", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Get file info for logging
	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Failed to get file info: %v", err)
		return fmt.Errorf("failed to get file info: %v", err)
	}

	// Send the filename first
	fileName := filepath.Base(filePath)
	log.Printf("Sending file metadata - Name: %s, Size: %d bytes", fileName, fileInfo.Size())

	// Send file marker and name - use the actual filename from Python output
	// Use .pth for model weights
	sendfilename := fmt.Sprintf("%s.txt", uid)
	err = dc.SendText("FILE:" + sendfilename)

	if err != nil {
		log.Printf("Failed to send file marker: %v", err)
		return fmt.Errorf("failed to send file marker: %v", err)
	}

	// Wait a bit to ensure the marker is processed
	time.Sleep(100 * time.Millisecond)

	// Send file data in chunks
	buffer := make([]byte, 16*1024) // Smaller chunks for better reliability
	totalSent := 0
	chunkCount := 0
	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file chunk: %v", err)
			return fmt.Errorf("error reading file: %v", err)
		}
		if n == 0 {
			break
		}

		sendErr := dc.Send(buffer[:n])
		if sendErr != nil {
			log.Printf("Error sending file chunk: %v", sendErr)
			return fmt.Errorf("error sending file data: %v", sendErr)
		}
		totalSent += n
		chunkCount++
		if chunkCount%50 == 0 || totalSent == int(fileInfo.Size()) {
			log.Printf("Sent %d/%d bytes (%.1f%%) - %d chunks", totalSent, fileInfo.Size(), float64(totalSent)/float64(fileInfo.Size())*100, chunkCount)
		}
	}

	// Send end of file marker
	err = dc.SendText("FILE_END:" + sendfilename)
	if err != nil {
		log.Printf("Failed to send file end marker: %v", err)
		return fmt.Errorf("failed to send file end marker: %v", err)
	}

	log.Printf("Successfully sent file: %s (%d bytes, %d chunks)", fileName, totalSent, chunkCount)
	return nil
}


func triggerPythonScript(dirName, edgeID string, percentage int, hyperparams map[string]interface{}, dc *webrtc.DataChannel) error {
	// Convert relative directory name to absolute path
	absPath, err := filepath.Abs(dirName)
	if err != nil {
		log.Printf("Error getting absolute path for %s: %v", dirName, err)
		return fmt.Errorf("error getting absolute path: %v", err)
	}

	log.Printf("Connecting to ML model service at %s", ML_MODEL)
	model, _, err := websocket.DefaultDialer.Dial(ML_MODEL, nil)
	if err != nil {
		log.Printf("Error while creating connection to ML model service: %v", err)
		return fmt.Errorf("failed to connect to ML service: %v", err)
	}
	defer model.Close()

	// Create JSON message with directory, edge ID, and percentage
	message := map[string]interface{}{
		"data_dir":   absPath,
		"edge_id":    edgeID,
		"percentage": percentage,
	}

	// Merge hyperparameters if present
	for k, v := range hyperparams {
		// Special handling for model_path: make it absolute if it's in the data dir
		if k == "model_path" {
			if pathStr, ok := v.(string); ok && pathStr != "" {
				// Check if file exists in data dir (transferred global model)
				localModelPath := filepath.Join(absPath, pathStr)
				if _, err := os.Stat(localModelPath); err == nil {
					message[k] = localModelPath
					log.Printf("Resolved absolute model path: %s", localModelPath)
				} else {
					// Fallback to original value (maybe absolute path on Edge device?)
					message[k] = v
				}
			}
		} else {
			message[k] = v
		}
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error creating JSON message: %v", err)
		return fmt.Errorf("failed to create JSON message: %v", err)
	}

	log.Printf("Sending directory path and edge ID to ML service: %s", string(jsonMessage))
	err = model.WriteMessage(websocket.TextMessage, jsonMessage)
	if err != nil {
		log.Printf("Error while writing to ML service connection: %v", err)
		return fmt.Errorf("failed to send data to ML service: %v", err)
	}

	log.Println("Waiting for response from ML service...")
	_, resp, err := model.ReadMessage()
	if err != nil {
		log.Printf("Error while reading from ML service connection: %v", err)
		return fmt.Errorf("failed to read response from ML service: %v", err)
	}

	// Parse the JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		log.Printf("Error parsing JSON response: %v", err)
		return fmt.Errorf("invalid response format from ML service: %v", err)
	}

	log.Printf("Received response from ML service: %+v", result)

	// Check if the response indicates an error
	if success, ok := result["success"].(bool); ok && !success {
		errMsg := "unknown error occurred"
		if msg, ok := result["error"].(string); ok {
			errMsg = msg
		}
		log.Printf("ML service returned error: %s", errMsg)
		return fmt.Errorf("ML service error: %s", errMsg)
	}

	// Get the output file path
	outputPath, ok := result["output_file_path"].(string)
	if !ok || outputPath == "" {
		errMsg := "no output file path in response"
		log.Print(errMsg)
		return fmt.Errorf("ML service error: %s", errMsg)
	}

	// Verify that the output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		log.Printf("Output file does not exist: %s", outputPath)
		return fmt.Errorf("output file does not exist: %s", outputPath)
	}

	log.Printf("ML training completed successfully. Output file: %s", outputPath)

	// Send the output file back to the server
	if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
		log.Printf("Sending output file to server via DataChannel...")
		err = sendFileToServer(edgeID, outputPath, dc)
		if err != nil {
			log.Printf("Failed to send file to server: %v", err)
			return fmt.Errorf("failed to send file to server: %v", err)
		}
		log.Printf("Successfully sent output file to server: %s", outputPath)
	} else {
		log.Printf("Warning: Data channel not open (state: %v), could not send output file", dc.ReadyState())
		return fmt.Errorf("data channel not available for sending output file")
	}

	return nil
}

func main() {
	var dir_name string
	fmt.Println("Name of dir you want to copy into:")
	fmt.Scan(&dir_name)

	err := os.MkdirAll(dir_name, 0755)
	if err != nil {
		log.Fatal("Error while make dir")
	}

	edge_id := id_maker()
	fmt.Printf("Edge id %s\n", edge_id)

	// Set environment variable for Python script
	os.Setenv("EDGE_ID", edge_id)

	var room_id string
	fmt.Print("Give the room_id: ")
	fmt.Scan(&room_id)

	// Run benchmark script to get score
	fmt.Println("Running benchmark...")

	// Check where benchmark.py is
	benchPath := "benchmark.py"
	if _, err := os.Stat(benchPath); os.IsNotExist(err) {
		// Try parent directory
		benchPath = "../benchmark.py"
	}

	// Detect Python executable
	pythonExec := "python3"
	possibleVenvs := []string{
		"venv/bin/python3",
		"../venv/bin/python3",
		"edge/venv/bin/python3",
		"../edge/venv/bin/python3",
	}

	for _, venvPath := range possibleVenvs {
		if _, err := os.Stat(venvPath); err == nil {
			pythonExec = venvPath
			fmt.Printf("Using venv python: %s\n", pythonExec)
			break
		}
	}

	cmd := exec.Command(pythonExec, benchPath, "--score-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	// Capture stderr to debug if needed
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		log.Printf("Error running benchmark: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		// Fallback score if benchmark fails
		out.WriteString("1.0")
	}

	scoreStr := strings.TrimSpace(out.String())
	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil {
		log.Printf("Error parsing score: %v", err)
		score = 1.0
	}
	fmt.Printf("Benchmark Score: %.2f\n", score)

	postBody := map[string]interface{}{
		"role":            Role,
		"room_id":         room_id,
		"edge_id":         edge_id,
		"benchmark_score": score,
	}

	algo_service, _, err := websocket.DefaultDialer.Dial(ALGO_LIVE_LINK, nil)
	if err != nil {
		log.Fatal("Error while creating connection to algorithm service", err)
	}

	err = algo_service.WriteJSON(postBody)
	if err != nil {
		log.Fatal("Error while writing to connection", err)
	}

	// Keep the algorithm service connection alive in a goroutine
	go func() {
		defer algo_service.Close()
		for {
			_, _, err := algo_service.ReadMessage()
			if err != nil {
				log.Printf("Algorithm service connection closed: %v", err)
				return
			}
		}
	}()

	conn, err := networking.Createconnection()
	if err != nil {
		log.Printf("Error creating networking connection: %v", err)
		return
	}
	defer conn.Close()

	err = networking.Forward(conn, Role)
	if err != nil {
		log.Fatal("Error while forwarding role", err)
	}

	err = networking.Forward(conn, edge_id)
	if err != nil {
		log.Fatal("Error while forwarding uid", err)
	}

	pc, err := webrtc.NewPeerConnection(networking.Webconfig)
	if err != nil {
		log.Fatal("Error while intializing peer connectrion", err)
	}

	var (
		totalFiles          int
		receivedFiles       int
		allocatedPercentage int
		filesMutex          sync.Mutex
		hyperparams         map[string]interface{}
	)

	var file_name string
	var f *os.File
	var totalBytesReceived int64
	var rxChunkCount int
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s\n", dc.Label())

		dc.OnOpen(func() {
			fmt.Println("Connected to peer. Type messages:")
			tmx := time.NewTicker(time.Millisecond * 2000)

			for {
				<-tmx.C
				heartbeatMsg := fmt.Sprintf("DHAK-DHAK from edge %s", edge_id[:8]) // Using first 8 chars of UUID for brevity
				dc.SendText(heartbeatMsg)
			}
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {

			if msg.IsString {
				msgText := string(msg.Data)
				if msgText == END {
					filesMutex.Lock()
					receivedFiles++
					currentCount := receivedFiles // Copy for logging without holding lock
					filesMutex.Unlock()
					log.Printf("Received file %d of %d", currentCount, totalFiles)
				} else if msgText == "BATCH_ENDED" {
					log.Printf("Received BATCH_ENDED signal. All files received. Triggering ML training...")
					go func() {
						filesMutex.Lock()
						params := hyperparams
						filesMutex.Unlock()

						err := triggerPythonScript(dir_name, edge_id, allocatedPercentage, params, dc)
						if err != nil {
							log.Printf("Error triggering Python script: %v", err)
						}
					}()
				} else if strings.HasPrefix(msgText, "HYPERPARAMS:") {
					jsonStr := strings.TrimPrefix(msgText, "HYPERPARAMS:")
					var params map[string]interface{}
					if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
						log.Printf("Error parsing HYPERPARAMS: %v", err)
					} else {
						filesMutex.Lock()
						hyperparams = params
						receivedFiles = 0 // Reset counter for new round
						filesMutex.Unlock()
						log.Printf("Received hyperparameters: %+v", params)
					}
				} else if strings.HasPrefix(msgText, "WEIGHT_PERCENTAGE:") {
					// Parse allocation percentage
					percStr := strings.TrimPrefix(msgText, "WEIGHT_PERCENTAGE:")
					perc, err := strconv.Atoi(percStr)
					if err != nil {
						log.Printf("Error parsing weight percentage: %v", err)
					} else {
						filesMutex.Lock()
						allocatedPercentage = perc
						filesMutex.Unlock()
						log.Printf("Received allocation percentage: %d%%", perc)
					}
				} else {
					// This is a filename
					file_name = msgText
					file_path := filepath.Join(dir_name, file_name)

					if strings.HasSuffix(file_name, ".pth") {
						log.Printf("**************************************************")
						log.Printf("RECEIVING GLOBAL MODEL BROADCAST: %s", file_name)
						log.Printf("**************************************************")
					} else {
						log.Printf("STARTING TO RECEIVE FILE: %s", file_name)
					}
					totalBytesReceived = 0
					rxChunkCount = 0
					f, err = os.Create(file_path)
					if err != nil {
						log.Fatal("Error while creating file ", file_path, ": ", err)
					}
				}

			} else {
				// Check if this is a "weight"/"totalFiles" message
				if len(msg.Data) == 4 {
					// Decode integer
					weight := int(binary.BigEndian.Uint32(msg.Data))
					filesMutex.Lock()
					totalFiles = weight
					filesMutex.Unlock()
					log.Printf("Updated totalFiles to %d", totalFiles)
				} else {
					if f != nil {
						n, err := io.Copy(f, bytes.NewBuffer(msg.Data))
						if err != nil {
							log.Fatal("Error while copying file data: ", err)
						}
						totalBytesReceived += n
						rxChunkCount++
						if rxChunkCount%50 == 0 {
							log.Printf("Receiving data for %s: %d chunks (%d bytes) so far...", file_name, rxChunkCount, totalBytesReceived)
						}
					}
				}

			}

		})
	})

	offer, err := networking.Recieve(conn)
	if err != nil {
		log.Fatal("Error while recieveing answer", err)
	}

	offer_SDP := webrtc.SessionDescription{
		SDP:  offer,
		Type: webrtc.SDPTypeOffer,
	}

	err = pc.SetRemoteDescription(offer_SDP)
	if err != nil {
		log.Fatal("Error at setting remote description", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Fatal("Error at creating answer")
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		log.Fatal("Error at setting local description")
	}

	<-webrtc.GatheringCompletePromise(pc)

	finalAnswer := pc.LocalDescription()

	ld := pc.LocalDescription()
	ld.SDP = strings.ReplaceAll(ld.SDP, "a=max-message-size:65536", "a=max-message-size:262144")
	_ = pc.SetLocalDescription(*ld)

	fmt.Print(finalAnswer.SDP)
	err = networking.Forward(conn, finalAnswer.SDP)
	if err != nil {
		log.Fatal("Error while forwarding answer", err)
	}

	select {}

}
