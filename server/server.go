package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
        return true 
    },
}

type comms_channel struct{
	id_channel map[string] []chan string
	mu sync.RWMutex
}
const(
	RoleWorker string= "W"
    RoleEdge   string= "E"
)
var Peer = comms_channel{id_channel: make(map[string] []chan string)}

func exchangeSDP(conn *websocket.Conn){
	defer conn.Close()
	_ , ct , err := conn.ReadMessage()
	if err != nil{
		log.Print("Error while reading id",err)
	}	
	role := string(ct)

	_ , id , err := conn.ReadMessage()
	if err != nil{
		log.Print("Error while reading id",err)
	}	
	conn_id := string(id)
	fmt.Println("id: ",conn_id) 

	var comms_chl chan string

	switch role{
	case RoleWorker:

		comms_chl = make(chan string)
		Peer.mu.Lock()
		comms_channels := Peer.id_channel[conn_id]
		comms_channels = append(comms_channels, comms_chl)
		Peer.id_channel[conn_id] = comms_channels
		Peer.mu.Unlock()

	case RoleEdge:
		
		Peer.mu.Lock()
		n := len(Peer.id_channel[conn_id]) 
		if n == 0{
			log.Println("No worker remaining")
			Peer.mu.Unlock()
			return
		}
		comms_chl = Peer.id_channel[conn_id][n-1]
		Peer.id_channel[conn_id] = Peer.id_channel[conn_id][:n-1]
		Peer.mu.Unlock()
	}
	
	select{
	case offer := <-comms_chl:
		err := conn.WriteMessage(websocket.TextMessage, []byte(offer))
		if err != nil{
			log.Print("Error forwarding offer ", err)
			return 
		}
		fmt.Println("Directing offer to peerB",string(offer))

		_ , answer , err := conn.ReadMessage()
		if err != nil{
			log.Print("Error reading answer ", err)
			return 
		}
		fmt.Println("Sending answer to peerA", string(answer))
		comms_chl <- string(answer)
		return
	
	default:
		_ , offer, err := conn.ReadMessage()
		if err != nil{
			log.Print("Error reading offer ", err)
			return 
		}
		fmt.Println("Recieved Offer ", string(offer))
		comms_chl <- string(offer)

		answer := <-comms_chl
		err = conn.WriteMessage(websocket.TextMessage, []byte(answer)) 
		if err != nil{
			log.Print("Error at writing answer ", err)
			return 
		}
		fmt.Println("Recieved answer from peerB", string(answer))
	}

	
}

func wshandler(w http.ResponseWriter, r *http.Request){
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil{
		log.Println("Couldnt upgrade to websocket ",err)
		return
	}

	exchangeSDP(conn)
}

func main(){

	http.HandleFunc("/", wshandler)
	log.Fatal(http.ListenAndServe(":10000",nil))
}