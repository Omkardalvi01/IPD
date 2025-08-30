package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"github.com/Omkardalvi01/IPD/networking"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

const(
	Role string = "E"
	END string = "EOF"
)

func id_maker() string{
	u , _ := uuid.NewUUID()
	return u.String()
}

func main(){
	var dir_name string
	fmt.Println("Name of dir you want to copy into:")
	fmt.Scan(&dir_name)
	
	err := os.MkdirAll(dir_name, 0755)
	if err != nil{
		log.Fatal("Error while make dir")
	} 

	edge_id := id_maker()
	fmt.Printf("Edge id %s\n",edge_id)

	var room_id string
	fmt.Print("Give the room_id: ")
	fmt.Scan(&room_id)
	
	postBody := map[string]string{
		"role" : Role,
		"room_id":  room_id,
		"edge_id": edge_id,
   }

   algo_service , _, err := websocket.DefaultDialer.Dial("ws://localhost:5000/join", nil)
   if err != nil{
		log.Fatal("Error while creating connection to algorithm service", err)
	}

	err = algo_service.WriteJSON(postBody)
	if err != nil{
		log.Fatal("Error while writing to connection", err)
	}

	_, resp, err := algo_service.ReadMessage()
	if err != nil{
		log.Println("Error while reading from connection ", err)
	}
	fmt.Println("Response from algo service ", string(resp))

   conn , err := networking.Createconnection()
	if err != nil{
		return
	}
	
	err = networking.Forward(conn, Role)
	if err != nil{
		log.Fatal("Error while forwarding role",err)
	}

	err = networking.Forward(conn, edge_id)
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
			fmt.Println("Connected to peer. Type messages:")

			go func() {
				var msg string
				for {
					fmt.Scan(&msg)
					dc.SendText(msg)
				}
			}()
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