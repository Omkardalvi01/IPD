package networking

import (
	"github.com/pion/webrtc/v3"
)

var Webconfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}