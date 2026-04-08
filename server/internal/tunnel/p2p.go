package tunnel

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

type P2PHandler struct {
	mu     sync.Mutex
	bridge *Bridge
	pc     *webrtc.PeerConnection
	dc     *webrtc.DataChannel
}

func NewP2PHandler(bridge *Bridge) *P2PHandler {
	return &P2PHandler{
		bridge: bridge,
	}
}

func (h *P2PHandler) Init() {
}

func (h *P2PHandler) OnRegisteredPeer(peerID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Printf("[p2p] Registered peer %d, creating connection", peerID)

	// Since we are creating the tunnel from the inside, we act as the caller (offerer)
	// We'll let the user initiate the offer from the app side later,
	// or we can establish it directly here if needed.
	// For now, this is a placeholder to setup variables if needed for direct calling.
}

func (h *P2PHandler) OnTransmittedData(data map[string]interface{}) {
	// P2P signaling comes over WS through transmitted-data
	if msgType, ok := data["type"].(string); ok {
		switch msgType {
		case "offer":
			if sdp, ok := data["sdp"].(string); ok {
				h.handleOffer(sdp)
			}
		case "answer":
			if sdp, ok := data["sdp"].(string); ok {
				h.handleAnswer(sdp)
			}
		case "candidate":
			if cand, ok := data["candidate"].(string); ok {
				sdpMLineIndex := uint16(data["sdpMLineIndex"].(float64))
				sdpMid := data["sdpMid"].(string)
				h.handleCandidate(cand, sdpMLineIndex, sdpMid)
			}
		}
	}
}

func (h *P2PHandler) handleOffer(sdp string) {
	log.Printf("[p2p] Handling offer SDP")
	// For actual implementation, setup PC here and reply with answer
}

func (h *P2PHandler) handleAnswer(sdp string) {
	log.Printf("[p2p] Handling answer SDP")
	if h.pc != nil {
		h.pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  sdp,
		})
	}
}

func (h *P2PHandler) handleCandidate(candidate string, sdpMLineIndex uint16, sdpMid string) {
	if h.pc != nil {
		h.pc.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:        candidate,
			SDPMLineIndex:    &sdpMLineIndex,
			SDPMid:           &sdpMid,
		})
	}
}

// SendSignalingMessage sends signaling data back to the peer
func (h *P2PHandler) SendSignalingMessage(participantId int64, data interface{}) {
	h.bridge.vkSend("transmit-data", map[string]interface{}{
		"participantId": participantId,
		"data":          data,
	})
}
