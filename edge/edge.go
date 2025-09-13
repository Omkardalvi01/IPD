package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
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

var(
	 ALGO_LIVE_LINK = "wss://ipd-allocator-yz2k.onrender.com"
 	ALGO_TEST_LINK = "ws://localhost:13000/join"
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
	
	// Send file marker and name
	sendfilename := fmt.Sprintf("%s.txt",uid)
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
	err = dc.SendText("FILE_END:" + sendfilename)
	if err != nil {
		log.Printf("Failed to send file end marker: %v", err)
		return fmt.Errorf("failed to send file end marker: %v", err)
	}

	log.Printf("Successfully sent file: %s (%d bytes)", fileName, totalSent)
	return nil
}

func triggerPythonScript(dirName, edgeID string, dc *webrtc.DataChannel) error {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting current working directory: %v", err)
		return fmt.Errorf("failed to get current working directory: %v", err)
	}

	// Path to the virtual environment
	venvPath := filepath.Join(cwd, "venv")
	pythonPath := filepath.Join(venvPath, "bin", "python")
	
	// Check if virtual environment exists, if not create it
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		log.Println("Creating Python virtual environment...")
		cmd := exec.Command("python3", "-m", "venv", venvPath)
		cmd.Dir = cwd
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Error creating virtual environment: %v\nOutput: %s", err, string(output))
			return fmt.Errorf("failed to create virtual environment: %v", err)
		}

		// Install required packages
		reqPath := filepath.Join(cwd, "requirements.txt")
		
		// Upgrade pip first
		pipUpgradeCmd := exec.Command(pythonPath, "-m", "pip", "install", "--upgrade", "pip")
		pipUpgradeCmd.Dir = cwd
		if output, err := pipUpgradeCmd.CombinedOutput(); err != nil {
			log.Printf("Error upgrading pip: %v\nOutput: %s", err, string(output))
			return fmt.Errorf("failed to upgrade pip: %v", err)
		}

		// Install requirements
		pipInstallCmd := exec.Command(pythonPath, "-m", "pip", "install", "-r", reqPath)
		pipInstallCmd.Dir = cwd
		if output, err := pipInstallCmd.CombinedOutput(); err != nil {
			log.Printf("Error installing requirements: %v\nOutput: %s", err, string(output))
			return fmt.Errorf("failed to install requirements: %v", err)
		}
	}

	// Prepare the command to run the Python script
	pythonScript := filepath.Join(cwd, "process_images.py")
	cmd := exec.Command(pythonPath, pythonScript, "--data_dir", dirName)
	cmd.Dir = cwd
	
	// Set up output buffers
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Start the command
	log.Printf("Starting Python script to process images in: %s", dirName)
	err = cmd.Run()

	// Process the output
	output := stdoutBuf.String()
	errOutput := stderrBuf.String()

	if err != nil {
		log.Printf("Python script error: %v\nStdout: %s\nStderr: %s", err, output, errOutput)
		// Send error back via WebRTC if needed
		if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
			dc.SendText(fmt.Sprintf("ERROR: %v - %s", err, errOutput))
		}
		return fmt.Errorf("python script failed: %v", err)
	}

	log.Printf("Python script completed successfully\nOutput: %s", output)

	// If the script produces an output file, send it back to the server
	// Look for the OUTPUT_FILE line in the output
	var outputPath string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "OUTPUT_FILE:") {
			outputPath = strings.TrimSpace(strings.TrimPrefix(line, "OUTPUT_FILE:"))
			break
		}
	}

	if outputPath != "" {
		log.Printf("Found output file: %s", outputPath)
		// Send the output file back to the server if data channel is open
		if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
			err = sendFileToServer(edgeID, outputPath, dc)
			if err != nil {
				return fmt.Errorf("failed to send file to server: %v", err)
			}
			log.Printf("Successfully sent output file to server: %s", outputPath)
		} else {
			log.Printf("Warning: Data channel not open, could not send output file")
		}
	} else {
		log.Println("No output file path found in script output")
	}

	return nil
}

func main() {
	var dir_name string
	fmt.Println("Name of dir you want to copy into:")
	fmt.Scan(&dir_name)

	err := os.MkdirAll(dir_name, 0755)
	if err != nil {
		log.Fatalf("Error creating directory: %v", err)
	}

	edge_id := id_maker()
	fmt.Printf("Edge id %s\n", edge_id)

	var room_id string
	fmt.Print("Give the room_id: ")
	fmt.Scan(&room_id)

	// Create output directory if it doesn't exist
	err = os.MkdirAll("outputs", 0755)
	if err != nil {
		log.Fatalf("Error creating outputs directory: %v", err)
	}

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
	var dcGlobal *webrtc.DataChannel
	var dcMutex sync.Mutex

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s\n", dc.Label())
		
		dcMutex.Lock()
		dcGlobal = dc
		dcMutex.Unlock()

		dc.OnOpen(func() {
			fmt.Println("Data channel connected")
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
							// Keep the data channel alive
							ticker := time.NewTicker(2 * time.Second)
							defer ticker.Stop()
							
							// Channel to signal script completion
							done := make(chan struct{})
							
							// Start a goroutine to keep the connection alive
							go func() {
								for {
									select {
									case <-ticker.C:
										dcMutex.Lock()
										if dcGlobal != nil && dcGlobal.ReadyState() == webrtc.DataChannelStateOpen {
											dcGlobal.SendText("KEEPALIVE")
										}
										dcMutex.Unlock()
									case <-done:
										return
									}
								}
							}()
							
							// Run the Python script
							err := triggerPythonScript(dir_name, edge_id, dcGlobal)
							if err != nil {
								log.Printf("Error triggering Python script: %v", err)
							}
							
							// Signal the keepalive goroutine to exit
							close(done)
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

	// Keep the main goroutine alive
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	log.Println("Shutting down...")
	
	// Clean up
	if pc != nil {
		if err := pc.Close(); err != nil {
			log.Printf("Error closing peer connection: %v", err)
		}
	}
	if algo_service != nil {
		algo_service.Close()
	}
	
	log.Println("Cleanup complete")

}