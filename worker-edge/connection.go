package main

import (
	"fmt"
	"log"

	"github.com/Omkardalvi01/IPD/networking"
	"github.com/pion/webrtc/v3"
)

// ConnectionManager handles WebRTC connections
type ConnectionManager struct {
	config *Config
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config *Config) *ConnectionManager {
	return &ConnectionManager{
		config: config,
	}
}

// EstablishConnection establishes a WebRTC connection with the master edge
func (cm *ConnectionManager) EstablishConnection(workerID string) (*webrtc.PeerConnection, *webrtc.DataChannel, error) {
	log.Printf("Establishing connection for worker %s", workerID)

	// Create WebSocket connection to signaling server
	conn, err := networking.Createconnection()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create signaling connection: %w", err)
	}
	defer conn.Close()

	// Send role as worker edge
	err = networking.Forward(conn, "WE") // Worker Edge role
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send role: %w", err)
	}

	// Send worker ID
	err = networking.Forward(conn, workerID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send worker ID: %w", err)
	}

	// Create peer connection
	peerConn, err := webrtc.NewPeerConnection(networking.Webconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Set up connection state change handler
	peerConn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State changed to: %s", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			log.Printf("ICE connection failed")
		}
	})

	// Wait for offer from master edge
	offerSDP, err := networking.Recieve(conn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to receive offer: %w", err)
	}

	// Set remote description (offer)
	offer := webrtc.SessionDescription{
		SDP:  offerSDP,
		Type: webrtc.SDPTypeOffer,
	}

	err = peerConn.SetRemoteDescription(offer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set remote description: %w", err)
	}

	// Create answer
	answer, err := peerConn.CreateAnswer(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create answer: %w", err)
	}

	// Set local description (answer)
	err = peerConn.SetLocalDescription(answer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set local description: %w", err)
	}

	// Wait for ICE gathering to complete
	<-webrtc.GatheringCompletePromise(peerConn)

	// Send answer back to master
	err = networking.Forward(conn, peerConn.LocalDescription().SDP)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send answer: %w", err)
	}

	// Wait for data channel from master edge
	var dataChannel *webrtc.DataChannel
	dataChannelReady := make(chan struct{})

	peerConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("Received data channel: %s", dc.Label())
		dataChannel = dc
		close(dataChannelReady)
	})

	// Wait for data channel to be established
	<-dataChannelReady

	if dataChannel == nil {
		return nil, nil, fmt.Errorf("data channel not established")
	}

	log.Println("WebRTC connection established successfully")
	return peerConn, dataChannel, nil
}

// Close closes the connection manager
func (cm *ConnectionManager) Close() {
	log.Println("Closing connection manager")
}
