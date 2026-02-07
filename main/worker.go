package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Omkardalvi01/IPD/networking"
	"github.com/pion/webrtc/v3"
)

type result_state int

const (
	SUCCESS        result_state = 0
	FAILURE        result_state = -1
	MODEL_RECEIVED result_state = 1
	END            string       = "EOF"
)

type Result struct {
	worker_id int
	result    result_state
}

type Request struct {
	f            string
	remote_name  string
	is_params    bool
	params       string
	is_trigger   bool
	round_folder string
}

type Worker struct {
	req_chan      chan Request
	res_chan      chan<- Result
	conn_id       string
	worker_id     int
	current       int
	weight        int
	outputDirBase string
}

func (w *Worker) start(wg1, wg2 *sync.WaitGroup, total_files int) {
	defer wg2.Done()

	peer, dc, err := networking.Peerconnection(w.conn_id)
	if err != nil {
		log.Printf("Error with peer connection in worker %d", w.worker_id)
		return
	}
	defer dc.Close()
	defer peer.Close()

	wg1.Done()

	done := make(chan struct{})

	// Default output directory
	w.outputDirBase = "./outputs"

	dc.OnOpen(func() {
		fmt.Printf("Worker %d: Data channel Open\n", w.worker_id)

		weight := (w.weight * total_files) / 100

		// Send weight as metadata
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(weight))
		dc.Send(buf)

		// Send allocation percentage explicitly
		perc_msg := fmt.Sprintf("WEIGHT_PERCENTAGE:%d", w.weight)
		dc.SendText(perc_msg)

		for r := range w.req_chan {
			if r.is_params {
				if r.round_folder != "" {
					w.outputDirBase = filepath.Join("./outputs", r.round_folder)
					if err := os.MkdirAll(w.outputDirBase, 0755); err != nil {
						log.Printf("Worker %d: Error creating round folder %s: %v", w.worker_id, w.outputDirBase, err)
					} else {
						log.Printf("Worker %d: Output directory updated to: %s", w.worker_id, w.outputDirBase)
					}
				}
				// Send hyperparameters
				dc.SendText("HYPERPARAMS:" + r.params)
				continue
			}

			if r.is_trigger {
				log.Printf("Worker %d: Sending BATCH_ENDED signal", w.worker_id)
				dc.SendText("BATCH_ENDED")
				continue
			}

			if r.f != "" {
				f, err := os.Open(r.f)
				if err != nil {
					log.Printf("Worker %d: Error opening file %s: %v", w.worker_id, r.f, err)
					continue
				}

				var send_file_name string
				if r.remote_name != "" {
					send_file_name = r.remote_name
				} else {
					// Use only the base filename to keep edge storage clean
					send_file_name = filepath.Base(r.f)
				}

				dc.SendText(send_file_name)
				log.Printf("Worker %d: Sending file: %s (Source: %s)", w.worker_id, send_file_name, r.f)

				// Send in 16KB chunks
				const chunkSize = 16 * 1024
				buffer := make([]byte, chunkSize)
				chunkCount := 0
				startTime := time.Now()

				for {
					n, err := f.Read(buffer)
					if err != nil {
						if err == io.EOF {
							break
						}
						log.Printf("Worker %d: Error reading %s: %v", w.worker_id, send_file_name, err)
						break
					}

					if err := dc.Send(buffer[:n]); err != nil {
						log.Printf("Worker %d: Error sending chunk %d for %s: %v", w.worker_id, chunkCount, send_file_name, err)
						break
					}
					chunkCount++
					if chunkCount%50 == 0 {
						log.Printf("Worker %d: Sent %d chunks for %s...", w.worker_id, chunkCount, send_file_name)
					}
				}

				dc.SendText(END)
				f.Close()
				log.Printf("Worker %d: Finished sending %s (%d chunks) in %v", w.worker_id, send_file_name, chunkCount, time.Since(startTime))

				w.res_chan <- Result{worker_id: w.worker_id, result: SUCCESS}
			}
		}
		close(done)
	})

	var (
		currentFile     *os.File
		currentFileName string
		totalReceived   int64
		receiveChunks   int64
	)

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			msgText := string(msg.Data)
			switch {
			case strings.HasPrefix(msgText, "DHAK-DHAK"):
				log.Printf("Worker %d: Heartbeat from edge: %s", w.worker_id, msgText)

			case strings.HasPrefix(msgText, "FILE:"):
				if currentFile != nil {
					currentFile.Close()
				}

				currentFileName = strings.TrimPrefix(msgText, "FILE:")
				filePath := filepath.Join(w.outputDirBase, currentFileName)

				log.Printf("Worker %d: Starting to receive file: %s (Target: %s)", w.worker_id, currentFileName, filePath)

				var err error
				currentFile, err = os.Create(filePath)
				if err != nil {
					log.Printf("Worker %d: Error creating file %s: %v", w.worker_id, filePath, err)
					return
				}
				totalReceived = 0
				receiveChunks = 0

			case strings.HasPrefix(msgText, "FILE_END:"):
				if currentFile != nil {
					currentFile.Sync()
					currentFile.Close()
					log.Printf("Worker %d: Successfully received file: %s (%d bytes, %d chunks)", w.worker_id, currentFileName, totalReceived, receiveChunks)
					currentFile = nil

					w.res_chan <- Result{worker_id: w.worker_id, result: MODEL_RECEIVED}
				}

			default:
				log.Printf("Worker %d Received message: %s", w.worker_id, msgText)
			}
		} else {
			if currentFile != nil {
				n, err := currentFile.Write(msg.Data)
				if err != nil {
					log.Printf("Worker %d: Error writing chunk: %v", w.worker_id, err)
					return
				}
				totalReceived += int64(n)
				receiveChunks++
				if receiveChunks%50 == 0 {
					log.Printf("Worker %d: Received %d chunks for %s...", w.worker_id, receiveChunks, currentFileName)
				}
			}
		}
	})

	dc.OnClose(func() {
		log.Printf("Worker %d: Data channel closed", w.worker_id)
		if currentFile != nil {
			currentFile.Close()
		}
	})

	<-done
}

type Workerpool struct {
	resultchan  chan<- Result
	workers     []*Worker
	num_workers int
}

func (wp *Workerpool) start_pool(n, total_files int, id []string, weights map[string]int, edges, worker_done *sync.WaitGroup) {
	wp.num_workers = n
	for i := 0; i < n; i++ {
		w := Worker{worker_id: i, req_chan: make(chan Request), res_chan: wp.resultchan, conn_id: id[i], weight: weights[id[i]], current: 0}
		wp.workers = append(wp.workers, &w)
		edges.Add(1)
		worker_done.Add(1)
		go w.start(edges, worker_done, total_files)
	}
}

func (wp *Workerpool) pickWorker() *Worker {
	var best *Worker
	total := 0

	for _, w := range wp.workers {
		w.current += w.weight
		total += w.weight
		if best == nil || w.current > best.current {
			best = w
		}
	}

	best.current -= total
	return best
}
