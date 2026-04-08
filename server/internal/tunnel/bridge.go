package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v4"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const TopologyDirect = "DIRECT"

type Bridge struct {
	mu         sync.Mutex
	vkWs       *websocket.Conn
	vkSeq      int
	iceServers []webrtc.ICEServer
	topology   string
	peers      map[int64]struct{}
	relay      Relay
	newRelay   func() Relay
	p2p        *P2PHandler

	OnCallCreated func(joinLink string)
	OnError       func(err error)
}

type ICEConfig struct {
	URLs       []string
	Username   string
	Credential string
}

func NewBridge(newRelay func() Relay) *Bridge {
	return &Bridge{
		newRelay: newRelay,
		peers:    make(map[int64]struct{}),
	}
}

func (b *Bridge) vkSend(command string, extra map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.vkWs == nil {
		return
	}
	b.vkSeq++
	seq := b.vkSeq
	var out []byte
	if pid, ok := extra["participantId"]; ok {
		dataJSON, _ := json.Marshal(extra["data"])
		out = []byte(fmt.Sprintf(`{"command":%q,"sequence":%d,"participantId":%v,"data":%s}`,
			command, seq, pid, dataJSON))
	} else {
		extra["command"] = command
		extra["sequence"] = seq
		out, _ = json.Marshal(extra)
	}
	b.vkWs.WriteMessage(websocket.TextMessage, out)
	log.Printf("[vk-ws] -> %s", command)
}

func (b *Bridge) handleVKMessage(raw []byte) error {
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}

	msgType, _ := msg["type"].(string)
	switch msgType {
	case "notification":
		notif, _ := msg["notification"].(string)
		log.Printf("[vk-ws] <- notification: %s", notif)

		switch notif {
		case "connection":
			log.Println("[vk-ws]    TURN creds received")

		case "transmitted-data":
			data, _ := msg["data"].(map[string]interface{})
			if data != nil && b.topology == TopologyDirect && b.p2p != nil {
				b.p2p.OnTransmittedData(data)
			}

		case "registered-peer":
			pid, _ := msg["participantId"].(float64)
			if b.topology == TopologyDirect && b.p2p != nil {
				b.p2p.OnRegisteredPeer(int64(pid))
			}

		case "topology-changed":
			topo, _ := msg["topology"].(string)
			log.Printf("[vk-ws]    Topology changed to %s", topo)
			b.topology = topo
			if topo != TopologyDirect {
				log.Printf("[vk-ws]    SFU not supported, kicking %d peers", len(b.peers))
				for pid := range b.peers {
					b.vkSend("remove-participant", map[string]interface{}{
						"participantId": pid,
						"ban":           false,
					})
				}
				return fmt.Errorf("SFU topology not supported")
			}

		case "participant-joined", "participant-added":
			if pid, ok := msg["participantId"].(float64); ok {
				b.peers[int64(pid)] = struct{}{}
				log.Printf("[vk-ws]    Participant %d joined (total: %d)", int64(pid), len(b.peers))
				if b.topology != TopologyDirect {
					log.Printf("[vk-ws]    Kicking peer %d (SFU topology)", int64(pid))
					b.vkSend("remove-participant", map[string]interface{}{
						"participantId": int64(pid),
						"ban":           false,
					})
					return fmt.Errorf("SFU topology not supported")
				}
			}

		case "participant-left":
			if pid, ok := msg["participantId"].(float64); ok {
				delete(b.peers, int64(pid))
				log.Printf("[vk-ws]    Participant %d left (total: %d)", int64(pid), len(b.peers))
			}

		case "hungup":
			if pid, ok := msg["participantId"].(float64); ok {
				delete(b.peers, int64(pid))
				log.Printf("[vk-ws]    Participant %d hung up (total: %d)", int64(pid), len(b.peers))
			} else {
				log.Println("[vk-ws]    Participant hung up")
			}
		}

	case "error":
		errMsg, _ := msg["message"].(string)
		errCode, _ := msg["error"].(string)
		log.Printf("[vk-ws] <- error: %s %s", errCode, errMsg)
		return fmt.Errorf("vk error: %s - %s", errCode, errMsg)
	}
	return nil
}

func (b *Bridge) connectVKWs(ctx context.Context, wsURL string) error {
	vkHeader := http.Header{}
	vkHeader.Set("User-Agent", userAgent)
	vkHeader.Set("Origin", "https://vk.com")
	vkDialer := websocket.Dialer{WriteBufferSize: rtpBufSize}

	vkWs, _, err := vkDialer.DialContext(ctx, wsURL, vkHeader)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.vkWs = vkWs
	b.vkSeq = 0
	b.mu.Unlock()
	return nil
}

func (b *Bridge) initRelay() {
	if b.relay != nil {
		b.relay.Close()
	}
	b.topology = TopologyDirect
	b.peers = make(map[int64]struct{})
	b.relay = b.newRelay()
	b.p2p = NewP2PHandler(b)
	b.p2p.Init()
}

func (b *Bridge) readLoop() error {
	for {
		_, msg, err := b.vkWs.ReadMessage()
		if err != nil {
			return err
		}
		if string(msg) == "ping" {
			b.vkWs.WriteMessage(websocket.TextMessage, []byte("pong"))
			continue
		}
		if err := b.handleVKMessage(msg); err != nil {
			return err
		}
	}
}

func (b *Bridge) Run(ctx context.Context, callInfo *CallInfo, cfg VKConfig, iceServers []webrtc.ICEServer) error {
	b.iceServers = iceServers

	wsURL := callInfo.WSEndpoint +
		"&platform=WEB" +
		"&appVersion=" + cfg.AppVersion +
		"&version=" + cfg.ProtocolVersion +
		"&device=browser&capabilities=0&clientType=VK&tgt=join"

	if b.OnCallCreated != nil {
		b.OnCallCreated(callInfo.JoinLink)
	}

	// Keepalive goroutine
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.mu.Lock()
				ws := b.vkWs
				b.mu.Unlock()
				if ws != nil {
					ws.WriteMessage(websocket.PingMessage, nil)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		b.initRelay()

		log.Println("[vk-ws] Connecting...")
		if err := b.connectVKWs(ctx, wsURL); err != nil {
			log.Printf("[vk-ws] Connect failed: %v, retrying in 5s...", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		log.Println("[vk-ws] Connected")

		b.vkSend("change-media-settings", map[string]interface{}{
			"mediaSettings": map[string]interface{}{
				"isAudioEnabled": false, "isVideoEnabled": true,
				"isScreenSharingEnabled": false, "isFastScreenSharingEnabled": false,
				"isAudioSharingEnabled": false, "isAnimojiEnabled": false,
			},
		})

		errCh := make(chan error, 1)
		go func() {
			errCh <- b.readLoop()
		}()

		var err error
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case err = <-errCh:
		}

		log.Printf("[vk-ws] Connection closed: %v", err)

		b.mu.Lock()
		if b.vkWs != nil {
			b.vkWs.Close()
			b.vkWs = nil
		}
		b.mu.Unlock()

		if b.OnError != nil && err != nil && err != context.Canceled {
			b.OnError(err)
		}

		if err == context.Canceled || err == context.DeadlineExceeded {
			return err
		}

		log.Println("[vk-ws] Reconnecting in 3s...")
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
