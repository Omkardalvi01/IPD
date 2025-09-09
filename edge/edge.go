package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
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

func sendFileToServer(filePath string, dc *webrtc.DataChannel) error {
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
	
	// Send file marker and name
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
	// Path to your Python script
	cmd := exec.Command("python3", "process_images.py", dirName, edgeID)
	cmd.Dir = filepath.Dir(dirName) // Set working directory to the parent of dirName

	// Create a pipe to capture stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting python script: %v", err)
	}

	// Read and process the output line by line
	scanner := bufio.NewScanner(stdout)
	var outputPath string
	foundOutput := false

	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("Python output: %s", line) // Log the output for debugging

		// Look for the OUTPUT_FILE line
		if strings.HasPrefix(line, "OUTPUT_FILE:") {
			outputPath = strings.TrimSpace(strings.TrimPrefix(line, "OUTPUT_FILE:"))
			foundOutput = true
		}
	}

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("python script failed: %v", err)
	}

	if !foundOutput {
		return fmt.Errorf("could not find output file path in python script output")
	}

	log.Printf("Python script completed, output file: %s", outputPath)

	// Send the output file back to the server
	if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
		err = sendFileToServer(outputPath, dc)
		if err != nil {
			return fmt.Errorf("failed to send file to server: %v", err)
		}
		log.Printf("Successfully sent output file to server: %s", outputPath)
	} else {
		log.Printf("Warning: Data channel not open, could not send output file")
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

	var room_id string
	fmt.Print("Give the room_id: ")
	fmt.Scan(&room_id)

	postBody := map[string]string{
		"role":    Role,
		"room_id": room_id,
		"edge_id": edge_id,
	}

	algo_service, _, err := websocket.DefaultDialer.Dial("ws://localhost:5000/join", nil)
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
					filesMutex.Unlock()
					
					// Check if all files have been received
					filesMutex.Lock()
					if receivedFiles == totalFiles {
						// All files received, trigger Python script
						go func() {
							err := triggerPythonScript(dir_name, edge_id, dc)
							if err != nil {
								log.Printf("Error triggering Python script: %v", err)
							}
						}()
					}
					filesMutex.Unlock()
				} else {
					totalFiles++
					file_name = msgText
					file_path := filepath.Join(dir_name, file_name)
					f, err = os.Create(file_path)
					if err != nil {
						log.Fatal("Error while creating file", err)
					}
				}

			} else {
				_, err = io.Copy(f, bytes.NewBuffer(msg.Data))
				if err != nil {
					log.Fatal("Error while copying file", err)
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