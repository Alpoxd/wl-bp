package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"server/internal/accounts"
	"server/internal/browser"
	"server/internal/db"
)

type Worker interface {
	Start(ctx context.Context) error
	Close() error
	GetStats() *Stats
}

type Stats struct {
	Uptime    time.Duration
	BytesTX   int64
	BytesRX   int64
	State     string
}

type Manager struct {
	db         *db.DB
	pool       *accounts.AccountPool
	browser    *browser.Browser
	workers    map[int]*WorkerInstance
	mu         sync.RWMutex
	vkConfig   interface{} // We'll type this properly later
}

type WorkerInstance struct {
	SessionID int
	AccountID int
	Worker    Worker
	Cancel    context.CancelFunc
}

func NewManager(database *db.DB, pool *accounts.AccountPool, b *browser.Browser) *Manager {
	return &Manager{
		db:      database,
		pool:    pool,
		browser: b,
		workers: make(map[int]*WorkerInstance),
	}
}

// StartSession acquires an account and starts a worker for a user.
func (m *Manager) StartSession(userID int, platform, mode string) (int, string, error) {
	// 1. Acquire account
	acc, err := m.pool.Acquire(platform)
	if err != nil {
		return 0, "", fmt.Errorf("failed to acquire account: %w", err)
	}

	// 2. Create session in DB
	sessionID, err := m.db.CreateSession(userID, acc.ID, platform, mode)
	if err != nil {
		m.pool.Release(acc.ID)
		return 0, "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	// Channels for async result
	linkCh := make(chan string, 1)
	errCh := make(chan error, 1)

	var w Worker
	if platform == "vk" {
		w = NewVKWorker(sessionID, acc, mode, m.db)
	} else if platform == "yandex" {
		w = NewTelemostWorker(sessionID, acc, mode, m.db, m.browser)
	} else {
		m.pool.Release(acc.ID)
		m.db.UpdateSessionFailed(sessionID, "platform not implemented")
		cancel()
		return 0, "", fmt.Errorf("platform %s not implemented yet", platform)
	}

	// Start worker asynchronously
	go func() {
		// Provide callbacks through type assertion for now
		if vw, ok := w.(*VKWorker); ok {
			vw.onLink = func(link string) {
				m.db.UpdateSessionActive(sessionID, vw.callInfo.CallID, link)
				linkCh <- link
			}
		} else if tw, ok := w.(*TelemostWorker); ok {
			tw.onLink = func(link string) {
				// We don't have a specific callID from Telemost REST API, we can use the URL path
				m.db.UpdateSessionActive(sessionID, link, link)
				linkCh <- link
			}
		}

		err := w.Start(ctx)
		if err != nil && err != context.Canceled {
			log.Printf("[worker] Session %d failed: %v", sessionID, err)
			m.db.UpdateSessionFailed(sessionID, err.Error())
			m.pool.MarkFailed(acc.ID, false) // Mark fail count
			errCh <- err
		}
		
		// Cleanup when worker exits
		m.StopSession(sessionID)
	}()

	m.mu.Lock()
	m.workers[sessionID] = &WorkerInstance{
		SessionID: sessionID,
		AccountID: acc.ID,
		Worker:    w,
		Cancel:    cancel,
	}
	m.mu.Unlock()

	// Wait for link or error
	select {
	case link := <-linkCh:
		m.db.RecordCall(acc.ID)
		return sessionID, link, nil
	case err := <-errCh:
		return 0, "", err
	case <-time.After(15 * time.Second):
		m.StopSession(sessionID)
		return 0, "", fmt.Errorf("timeout waiting for join link")
	}
}

func (m *Manager) StopSession(sessionID int) {
	m.mu.Lock()
	inst, exists := m.workers[sessionID]
	if exists {
		delete(m.workers, sessionID)
	}
	m.mu.Unlock()

	if !exists {
		return
	}

	log.Printf("[manager] Stopping session %d", sessionID)
	inst.Cancel() // Cancel context
	
	// Get final stats before closing
	var tx, rx int64
	if stats := inst.Worker.GetStats(); stats != nil {
		tx = stats.BytesTX
		rx = stats.BytesRX
	}

	inst.Worker.Close()
	m.pool.Release(inst.AccountID)
	m.db.CloseSession(sessionID, tx, rx)
}

func (m *Manager) GetSessionStats(sessionID int) *Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if inst, exists := m.workers[sessionID]; exists {
		return inst.Worker.GetStats()
	}
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	sessions := make([]int, 0, len(m.workers))
	for id := range m.workers {
		sessions = append(sessions, id)
	}
	m.mu.Unlock()

	for _, id := range sessions {
		m.StopSession(id)
	}
}
