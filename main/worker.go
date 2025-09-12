package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Omkardalvi01/IPD/networking"
	"github.com/pion/webrtc/v3"
)

type result_state int

const(
	SUCCESS result_state = 0
	FAILURE result_state = -1 
	END string = "EOF"
)

type Result struct{
	worker_id int
	result result_state
}

type Request struct{
	f string
}

type Worker struct{
	req_chan chan Request
	res_chan chan<- Result
	conn_id string
	worker_id int
	current int
	weight int
}

func (w Worker) start(wg1, wg2 *sync.WaitGroup ){
	defer wg2.Done()

	stop_worker := make(chan struct{})

	peer ,dc , err := networking.Peerconnection(w.conn_id)
	if err != nil{
		log.Printf("Error with peer connection in worker %d", w.worker_id)
		return 
	}
	defer dc.Close()
	defer peer.Close()

	wg1.Done()

	dc.OnOpen(func() {
		fmt.Println("Data channel Open")
		for r := range w.req_chan {
			
			f, err := os.Open(r.f)
			if err != nil{
				log.Print("Error while opening file ",err)
			}

			file_name := strings.Join(strings.Split(f.Name(), "/")[1:], "/")
			send_file_name := strings.ReplaceAll(file_name, "/", "#")
			dc.SendText(send_file_name)

			img , err := get_img_data(f.Name()) 
			if err != nil{
				log.Fatal("Error while get image data", err)
			}

			err = dc.Send(img)
			if err != nil{
				log.Fatal("Error while sending image data", err)
			}
			
			dc.SendText(END)

			w.res_chan <- Result{worker_id: w.worker_id, result: SUCCESS}
			
			f.Close()
		}
		
	})
	// Create output directory if it doesn't exist
	outputDir := "./outputs"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("Failed to create output directory: %v", err)
	} else {
		log.Printf("Output directory ready: %s", outputDir)
	}

	var (
		currentFile     *os.File
		currentFileName string
		totalReceived   int64
	)

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			msgText := string(msg.Data)
			switch {
			case strings.HasPrefix(msgText, "DHAK-DHAK"):
				// This is a heartbeat message
				log.Printf("Heartbeat received: %s", msgText)

			case strings.HasPrefix(msgText, "FILE:"):
				// Start of a new file
				if currentFile != nil {
					log.Printf("Warning: Previous file %s not properly closed", currentFileName)
					currentFile.Close()
					currentFile = nil
				}

				currentFileName = strings.TrimPrefix(msgText, "FILE:")
				filePath := filepath.Join(outputDir, currentFileName)
				
				
				
				log.Printf("Starting to receive file: %s", currentFileName)
				
				var err error
				currentFile, err = os.Create(filePath)
				if err != nil {
					log.Printf("Error creating file %s: %v", filePath, err)
					return
				}
				totalReceived = 0
				log.Printf("Ready to receive data for file: %s", filePath)

			case strings.HasPrefix(msgText, "FILE_END:"):
				// End of file
				expectedFile := strings.TrimPrefix(msgText, "FILE_END:")
				if currentFile != nil {
					if currentFileName != expectedFile {
						log.Printf("Warning: File end marker mismatch. Expected %s, got %s", currentFileName, expectedFile)
					}
					err := currentFile.Sync() // Ensure all data is written to disk
					if err != nil {
						log.Printf("Error syncing file %s: %v", currentFileName, err)
					}
					currentFile.Close()
					log.Printf("Successfully received file: %s (%d bytes)", currentFileName, totalReceived)
					currentFile = nil
					totalReceived = 0
					stop_worker <- struct{}{}
				} else {
					log.Printf("Received FILE_END but no file is currently being received")
				}

				
			default:
				log.Printf("Received message: %s", msgText)
			}
		} else {
			// Handle binary data (file chunks)
			if currentFile != nil {
				n, err := currentFile.Write(msg.Data)
				if err != nil {
					log.Printf("Error writing to file: %v", err)
					currentFile.Close()
					currentFile = nil
					return
				}
				totalReceived += int64(n)
				if totalReceived%(1024*1024) == 0 { // Log every 1MB
					log.Printf("Received %d bytes for %s", totalReceived, currentFileName)
				}
			} else {
				log.Printf("Received unexpected binary data without FILE: prefix, size: %d bytes", len(msg.Data))
			}
		}
	})

	// Clean up on connection close
	dc.OnClose(func() {
		log.Printf("Data channel closed")
		if currentFile != nil {
			currentFile.Close()
			currentFile = nil
		}
	})

	<-stop_worker
}

type Workerpool struct{
	resultchan chan<- Result
	workers []*Worker
	num_workers int
}

func (wp *Workerpool) start_pool(n int, id []string, weights map[string]int, edges , worker_done *sync.WaitGroup) {
	wp.num_workers = n
	for i := 0 ; i < n ; i++ {
		w := Worker{worker_id: i, req_chan: make(chan Request), res_chan: wp.resultchan, conn_id: id[i], weight: weights[id[i]], current: 0}
		wp.workers = append(wp.workers, &w)
		edges.Add(1)
		worker_done.Add(1)
		go w.start(edges, worker_done)
	}
}

func (wp *Workerpool) pickWorker() *Worker{
	var best *Worker
	total := 0

	for _, w := range wp.workers{
		w.current += w.weight
		total += w.weight
		if best == nil || w.current > best.current{
			best = w
		}
	}

	best.current -= total
	return best

}
