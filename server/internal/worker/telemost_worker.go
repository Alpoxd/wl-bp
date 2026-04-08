package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"server/internal/browser"
	"server/internal/db"
)

type TelemostWorker struct {
	sessionID int
	account   *db.HostAccount
	mode      string
	db        *db.DB
	browser   *browser.Browser

	tmSession *browser.TelemostSession
	startTime time.Time
	statsMu   sync.RWMutex
	bytesTX   int64
	bytesRX   int64

	onLink func(string)
}

func NewTelemostWorker(sessionID int, account *db.HostAccount, mode string, database *db.DB, b *browser.Browser) *TelemostWorker {
	return &TelemostWorker{
		sessionID: sessionID,
		account:   account,
		mode:      mode,
		db:        database,
		browser:   b,
		startTime: time.Now(),
	}
}

func (w *TelemostWorker) Start(ctx context.Context) error {
	log.Printf("[tm_worker:%d] Starting for account %s...", w.sessionID, w.account.Label)
	
	w.tmSession = browser.NewTelemostSession(w.browser)
	
	link, err := w.tmSession.CreateCall(ctx, w.account.Cookies)
	if err != nil {
		return err
	}

	if w.onLink != nil {
		w.onLink(link)
	}

	// Wait until context is canceled
	<-ctx.Done()
	return ctx.Err()
}

func (w *TelemostWorker) Close() error {
	if w.tmSession != nil {
		w.tmSession.Close()
	}
	return nil
}

func (w *TelemostWorker) GetStats() *Stats {
	w.statsMu.RLock()
	defer w.statsMu.RUnlock()

	return &Stats{
		Uptime:  time.Since(w.startTime),
		BytesTX: w.bytesTX,
		BytesRX: w.bytesRX,
		State:   "stub", 
	}
}
