package networking

import (
	"fmt"
	"log"

	"github.com/pion/webrtc/v3"
)
const(
	Role string = "W"
)

func Peerconnection(uid	string) (*webrtc.PeerConnection, *webrtc.DataChannel, error) {

	conn, err := Createconnection()
	if err != nil{
		return nil, nil, err
	}
	defer conn.Close()

	err = Forward(conn, Role)
	if err != nil{
		return nil, nil,  err
	}

	err = Forward(conn, uid)
	if err != nil{
		return nil, nil,  err
	}
	
	peer_conn, err := webrtc.NewPeerConnection(Webconfig)
	if err != nil {
		log.Println("error while creaating peer connection in user_comms.go")
		return nil, nil, err 
	}
	
	peer_conn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed to: %s\n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed{
			fmt.Printf("connection has failed to the given candidate")
			if closeErr := peer_conn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}
	})

	dc, err := peer_conn.CreateDataChannel("data", nil)
	if err != nil{
		log.Println("error while creating data channel in user_comms.go")
		return nil, nil, err 
	}

	offer , err := peer_conn.CreateOffer(nil)
	if err != nil {
		log.Println("error while creating offer in user_comms.go")
		return nil, nil, err 
	}


	err = peer_conn.SetLocalDescription(offer)
	if err != nil {
		log.Println("error setting local description user_comms.go")
		return nil, nil, err 
	}

	<-webrtc.GatheringCompletePromise(peer_conn)

	err = Forward(conn, peer_conn.LocalDescription().SDP)
	if err != nil{
		return nil, nil, err
	}
	
	resp , err := Recieve(conn)
	if err != nil{
		return nil, nil, err
	}

	answer_sdp := webrtc.SessionDescription{
		SDP: resp,
		Type: webrtc.SDPTypeAnswer,
	}

	err = peer_conn.SetRemoteDescription(answer_sdp)
	if err != nil {
		log.Println("error setting setting remote description user_comms.go")
		return nil, nil, err  
	}
	fmt.Print(peer_conn.RemoteDescription().SDP)

	return peer_conn , dc, nil

}