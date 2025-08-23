package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"github.com/Omkardalvi01/IPD/networking"
	"github.com/pion/webrtc/v3"
)

const(
	Role string = "E"
	END string = "EOF"
)

type Edge struct {
	yoloTrainer *YOLOTrainer
	trainingStats struct {
		totalImages     int
		batchesCompleted int
		totalTrainingTime float64
		avgLoss         float64
		avgMAP          float64
	}
}

func main(){
	var dir_name string
	fmt.Println("Name of dir you want to copy into:")
	fmt.Scan(&dir_name)
	
	err := os.MkdirAll(dir_name, 0755)
	if err != nil{
		log.Fatal("Error while make dir")
	}

	// Initialize YOLO trainer
	edge := &Edge{
		yoloTrainer: NewYOLOTrainer(),
	}

	conn , err := networking.Createconnection()
	if err != nil{
		return
	} 
	
	var uid string
	fmt.Print("Give the unique_id: ")
	fmt.Scan(&uid)
	
	err = networking.Forward(conn, Role)
	if err != nil{
		log.Fatal("Error while forwarding role",err)
	}

	err = networking.Forward(conn, uid)
	if err != nil{
		log.Fatal("Error while forwarding uid",err)
	}

	pc , err := webrtc.NewPeerConnection(networking.Webconfig)
	if err != nil{
		log.Fatal("Error while intializing peer connectrion", err)
	}
	
	var file_name string
	var f *os.File
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s\n", dc.Label())

		dc.OnOpen(func() {
			fmt.Println("Connected to peer. Ready to receive files for YOLO training:")
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {

			if msg.IsString{
				
				if string(msg.Data) == END{
					fmt.Printf("Download %s Complete\n",f.Name())
					if err := f.Close(); err != nil{
						log.Fatal("Error while closing file", err)
					}
				}else{
					file_name = string(msg.Data)
					file_path := filepath.Join(dir_name,file_name)
					f , err= os.Create(file_path)
					if err != nil{
						log.Fatal("Error while creating file", err)
					}
				}
				
			}else{
				_ , err = io.Copy(f, bytes.NewBuffer(msg.Data))
				if err != nil{
					log.Fatal("Error while copying file", err)
				}
				
				// After file is received, process it for YOLO training if it's an image
				if isImageFile(file_name) {
					// Read the file data for YOLO processing
					file_path := filepath.Join(dir_name, file_name)
					fileData, readErr := os.ReadFile(file_path)
					if readErr != nil {
						log.Printf("Warning: failed to read file for YOLO training: %v", readErr)
					} else {
						// Process image with YOLO trainer
						if processErr := edge.handleReceivedFile(file_name, fileData); processErr != nil {
							log.Printf("Warning: YOLO processing failed: %v", processErr)
						}
					}
				}
			}
		
		})
	})

	offer , err := networking.Recieve(conn)
	if err != nil{
		log.Fatal("Error while recieveing answer",err)
	}

	offer_SDP := webrtc.SessionDescription{
		SDP: offer,
		Type: webrtc.SDPTypeOffer,
	}

	err = pc.SetRemoteDescription(offer_SDP)
	if err != nil{
		log.Fatal("Error at setting remote description", err)
	}

	
	answer , err := pc.CreateAnswer(nil)
	if err != nil{
		log.Fatal("Error at creating answer")
	}

	err = pc.SetLocalDescription(answer)
	if err != nil{
		log.Fatal("Error at setting local description")
	}

	<-webrtc.GatheringCompletePromise(pc)

    finalAnswer := pc.LocalDescription()

    fmt.Print(finalAnswer.SDP)
	err = networking.Forward(conn, finalAnswer.SDP)
	if err != nil{
		log.Fatal("Error while forwarding answer",err)
	}

	select{}

}

func (e *Edge) handleReceivedFile(filename string, data []byte) error {
	log.Printf("Received file: %s (%d bytes)", filename, len(data))
	
	if isImageFile(filename) {
		log.Printf("Adding image to YOLO training batch...")
		
		result, err := e.yoloTrainer.TrainOnImage(data, filename)
		if err != nil {
			log.Printf("YOLO training failed: %v", err)
			return err
		}
		
		// Update training statistics
		e.trainingStats.totalImages++
		if result.BatchCompleted {
			e.trainingStats.batchesCompleted++
			e.trainingStats.totalTrainingTime += result.TrainingTime
			if result.TrainingLoss > 0 {
				e.trainingStats.avgLoss = (e.trainingStats.avgLoss*float64(e.trainingStats.batchesCompleted-1) + result.TrainingLoss) / float64(e.trainingStats.batchesCompleted)
			}
			if result.ValidationMAP > 0 {
				e.trainingStats.avgMAP = (e.trainingStats.avgMAP*float64(e.trainingStats.batchesCompleted-1) + result.ValidationMAP) / float64(e.trainingStats.batchesCompleted)
			}
		}
		
		log.Printf("YOLO Training Status for %s:", filename)
		log.Printf("  Batch Status: %d/%d images", result.BatchInfo.CurrentBatchSize, result.BatchInfo.MaxBatchSize)
		
		if result.BatchCompleted {
			log.Printf("  ✓ BATCH TRAINING COMPLETED!")
			log.Printf("  Training Loss: %.6f", result.TrainingLoss)
			log.Printf("  Validation mAP: %.6f", result.ValidationMAP)
			log.Printf("  Training Time: %.3f seconds", result.TrainingTime)
			log.Printf("  Model Weights File: %s", result.WeightsFilePath)
		}
		
		log.Printf("  Total Stats - Images: %d, Batches: %d, Avg Loss: %.6f, Avg mAP: %.6f", 
			e.trainingStats.totalImages, e.trainingStats.batchesCompleted, 
			e.trainingStats.avgLoss, e.trainingStats.avgMAP)
		
		return nil
	}
	
	log.Printf("Non-image file received: %s", filename)
	return nil
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"}
	
	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}