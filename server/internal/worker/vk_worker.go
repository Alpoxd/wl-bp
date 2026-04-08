package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"server/internal/db"
	"server/internal/tunnel"
)

type VKWorker struct {
	sessionID int
	account   *db.HostAccount
	mode      string
	db        *db.DB

	bridge   *tunnel.Bridge
	callInfo *tunnel.CallInfo
	
	startTime time.Time
	statsMu   sync.RWMutex
	bytesTX   int64
	bytesRX   int64

	onLink func(string)
}

func NewVKWorker(sessionID int, account *db.HostAccount, mode string, database *db.DB) *VKWorker {
	return &VKWorker{
		sessionID: sessionID,
		account:   account,
		mode:      mode,
		db:        database,
		startTime: time.Now(),
	}
}

func (w *VKWorker) Start(ctx context.Context) error {
	log.Printf("[vk_worker:%d] Starting for account %s...", w.sessionID, w.account.Label)

	// Fetch dynamic config
	cfg, err := tunnel.FetchVKConfig()
	if err != nil {
		return err
	}

	// Create call
	w.callInfo, err = tunnel.CreateAndJoinCall(w.account.Cookies, "", cfg)
	if err != nil {
		return err
	}

	iceServers := tunnel.BuildICEServers(w.callInfo)

	// Create bridge
	w.bridge = tunnel.NewBridge(func() tunnel.Relay {
		ur := tunnel.NewTunnelRelay()
		// TODO: Configure buffers based on .env
		ur.ReadBufSize = 32768
		ur.MaxDCBuf = 4 * 1024 * 1024
		ur.OnConnected = func(t *tunnel.VP8DataTunnel) {
			tunnel.NewRelayBridge(t, "creator", log.Printf)
		}
		ur.OnDataStats = func(tx, rx int64) {
			w.statsMu.Lock()
			w.bytesTX += tx
			w.bytesRX += rx
			w.statsMu.Unlock()
		}
		return ur
	})

	w.bridge.OnCallCreated = w.onLink

	// Block until context is canceled or unrecoverable error occurs
	return w.bridge.Run(ctx, w.callInfo, cfg, iceServers)
}

func (w *VKWorker) Close() error {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	return nil
}

func (w *VKWorker) GetStats() *Stats {
	w.statsMu.RLock()
	defer w.statsMu.RUnlock()

	return &Stats{
		Uptime:  time.Since(w.startTime),
		BytesTX: w.bytesTX,
		BytesRX: w.bytesRX,
		State:   "active", // We can enhance this by polling bridge state
	}
}
