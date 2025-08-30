package networking

import (
	"log"

	"github.com/gorilla/websocket"
)

var TEST_LINK = "ws://localhost:10000/" 
var LIVE_LINK = "wss://sdp-server-1.onrender.com"

func Createconnection() (*websocket.Conn, error){
	conn, _ , err := websocket.DefaultDialer.Dial(TEST_LINK, nil)
	if err != nil{
		log.Println("Error while creating connection in signaling.go", err)
		return nil , err
	}

	return conn, nil
}

func Forward(conn *websocket.Conn, msg string) error{
	err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil{
		log.Println("Error while forwarding messge in signaling.go")
		return err
	}

	return nil
}

func Recieve(conn *websocket.Conn) (string,error){

	_ , resp , err := conn.ReadMessage()
	if err != nil {
		log.Println("Error while recieving in signaling.go err")
		return "",err
	}
	return string(resp),nil
}