package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/networking"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var (
	fileTransferMutex sync.Mutex
	activeTransfers   = make(map[string]bool)
)

const (
	Role string = "E"
	END  string = "EOF"
)

func id_maker() string {
	u, _ := uuid.NewUUID()
	return u.String()
}

const(
	ALGO_LIVE_LINK = "wss://ipd-allocator-1.onrender.com/ws"
 	ALGO_TEST_LINK = "ws://localhost:13000/join"
	ML_MODEL = "ws://localhost:8765"
)

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
	err = dc.SendText("FILE:" + fileName)
	
	if err != nil {
		log.Printf("Failed to send file marker: %v", err)
		return fmt.Errorf("failed to send file marker: %v", err)
	}

	// Wait a bit to ensure the marker is processed
	time.Sleep(100 * time.Millisecond)

	// Send file data in chunks
	buffer := make([]byte, 16*1024) // Smaller chunks for better reliability
	totalSent := 0
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
		log.Printf("Sent %d/%d bytes (%.1f%%)", totalSent, fileInfo.Size(), float64(totalSent)/float64(fileInfo.Size())*100)
	}

	// Send end of file marker
	err = dc.SendText("FILE_END:" + fileName)
	if err != nil {
		log.Printf("Failed to send file end marker: %v", err)
		return fmt.Errorf("failed to send file end marker: %v", err)
	}

	log.Printf("Successfully sent file: %s (%d bytes)", fileName, totalSent)
	return nil
}

func triggerPythonScript(dirName, edgeID string, dc *webrtc.DataChannel) error {
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

	log.Printf("Sending directory path to ML service: %s", absPath)
	err = model.WriteMessage(websocket.TextMessage, []byte(absPath))
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
	
	outputPath := strings.TrimSpace(string(resp))
	log.Printf("Received response from ML service: %s", outputPath)
	
	// Check if the response is an error message
	if strings.HasPrefix(outputPath, "ERROR:") {
		log.Printf("ML service returned error: %s", outputPath)
		return fmt.Errorf("ML service error: %s", outputPath)
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

	postBody := map[string]string{
		"role":    Role,
		"room_id": room_id,
		"edge_id": edge_id,
	}

	algo_service, _, err := websocket.DefaultDialer.Dial(ALGO_TEST_LINK, nil)
	if err != nil {
		log.Fatal("Error while creating connection to algorithm service", err)
	}

	err = algo_service.WriteJSON(postBody)
	if err != nil {
		log.Fatal("Error while writing to connection", err)
	}

	_, resp, err := algo_service.ReadMessage()
	if err != nil {
		log.Println("Error while reading from connection ", err)
	}
	fmt.Println("Response from algo service ", string(resp))

	conn, err := networking.Createconnection()
	if err != nil {
		return
	}

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
		totalFiles    int
		receivedFiles int
		filesMutex   sync.Mutex
	)

	var file_name string
	var f *os.File
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
					log.Printf("Received file %d of %d", receivedFiles, totalFiles)
					
					// Check if all files have been received
					if receivedFiles == totalFiles {
						filesMutex.Unlock()
						// All files received, trigger Python script
						log.Printf("All %d files received. Triggering ML training...", totalFiles)
						go func() {
							err := triggerPythonScript(dir_name, edge_id, dc)
							if err != nil {
								log.Printf("Error triggering Python script: %v", err)
							}
						}()
					} else {
						filesMutex.Unlock()
					}
				} else {
					filesMutex.Lock()
					totalFiles++
					file_name = msgText
					file_path := filepath.Join(dir_name, file_name)
					log.Printf("Starting to receive file %d: %s", totalFiles, file_name)
					f, err = os.Create(file_path)
					if err != nil {
						log.Fatal("Error while creating file", err)
					}
					filesMutex.Unlock()
				}

			} else {
				if f != nil {
					_, err = io.Copy(f, bytes.NewBuffer(msg.Data))
					if err != nil {
						log.Fatal("Error while copying file", err)
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
		SDP: offer,
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

	fmt.Print(finalAnswer.SDP)
	err = networking.Forward(conn, finalAnswer.SDP)
	if err != nil {
		log.Fatal("Error while forwarding answer", err)
	}

	select {}

}