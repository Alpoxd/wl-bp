package accounts

import (
	"fmt"
	"log"
	"sync"
	"time"

	"server/internal/db"
)

type AccountPool struct {
	mu sync.Mutex
	db *db.DB
}

func NewAccountPool(database *db.DB) *AccountPool {
	return &AccountPool{
		db: database,
	}
}

// Acquire selects the best available account for the given platform.
// For VK, it selects an account with active_workers < max_concurrent_calls.
// For Yandex, it selects the account with the lowest active_workers (least-loaded).
func (p *AccountPool) Acquire(platform string) (*db.HostAccount, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	accounts, err := p.db.ListHostAccounts()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var bestAccount *db.HostAccount

	for _, acc := range accounts {
		if acc.Platform != platform || acc.Status != "active" {
			continue
		}

		// VK uses max_concurrent_calls (usually 1)
		// Yandex uses 0 (unlimited)
		if acc.MaxConcurrentCalls > 0 && acc.ActiveWorkers >= acc.MaxConcurrentCalls {
			continue
		}

		if bestAccount == nil {
			bestAccount = acc
			continue
		}

		// Strategy: least-loaded (lowest active_workers), then LRU (oldest last_used)
		if acc.ActiveWorkers < bestAccount.ActiveWorkers {
			bestAccount = acc
		} else if acc.ActiveWorkers == bestAccount.ActiveWorkers {
			if bestAccount.LastUsed == nil {
				// Keep bestAccount, since it has never been used
			} else if acc.LastUsed == nil {
				bestAccount = acc
			} else if acc.LastUsed.Before(*bestAccount.LastUsed) {
				bestAccount = acc
			}
		}
	}

	if bestAccount == nil {
		return nil, fmt.Errorf("no available accounts for platform %s", platform)
	}

	// Update the account state
	newActiveWorkers := bestAccount.ActiveWorkers + 1
	var newStatus = "active"
	
	if bestAccount.MaxConcurrentCalls > 0 && newActiveWorkers >= bestAccount.MaxConcurrentCalls {
		newStatus = "busy"
	}

	_, err = p.db.Exec(`UPDATE host_accounts SET active_workers = ?, status = ?, last_used = CURRENT_TIMESTAMP WHERE id = ?`, newActiveWorkers, newStatus, bestAccount.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update account state: %w", err)
	}
	
	bestAccount.ActiveWorkers = newActiveWorkers
	bestAccount.Status = newStatus

	log.Printf("[pool] Acquired %s account ID:%d (%s) - workers:%d status:%s", platform, bestAccount.ID, bestAccount.Label, newActiveWorkers, newStatus)
	return bestAccount, nil
}

// Release releases a worker for the given account.
func (p *AccountPool) Release(accountID int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	acc, err := p.db.GetHostAccount(accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account not found")
	}

	newActiveWorkers := acc.ActiveWorkers - 1
	if newActiveWorkers < 0 {
		newActiveWorkers = 0
	}

	newStatus := acc.Status
	// If it was busy because of limits, it becomes active again once a worker leaves
	if acc.Status == "busy" && (acc.MaxConcurrentCalls == 0 || newActiveWorkers < acc.MaxConcurrentCalls) {
		newStatus = "active"
	}

	_, err = p.db.Exec(`UPDATE host_accounts SET active_workers = ?, status = ? WHERE id = ?`, newActiveWorkers, newStatus, accountID)
	if err != nil {
		return err
	}

	log.Printf("[pool] Released account ID:%d (%s) - workers:%d status:%s", acc.ID, acc.Label, newActiveWorkers, newStatus)
	return nil
}

// MarkFailed records a failure for an account and updates its status if needed.
func (p *AccountPool) MarkFailed(accountID int, isBan bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	acc, err := p.db.GetHostAccount(accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account not found")
	}

	if isBan {
		_, err = p.db.Exec(`UPDATE host_accounts SET status = 'banned', banned_at = CURRENT_TIMESTAMP WHERE id = ?`, accountID)
		log.Printf("[pool] ❌ Account ID:%d (%s) marked as BANNED", acc.ID, acc.Label)
		return err
	}

	newFailCount := acc.FailCount + 1
	status := acc.Status

	if newFailCount >= 3 {
		status = "cooldown"
		log.Printf("[pool] ⚠️ Account ID:%d (%s) moved to COOLDOWN (fails: %d)", acc.ID, acc.Label, newFailCount)
	}

	_, err = p.db.Exec(`UPDATE host_accounts SET fail_count = ?, status = ? WHERE id = ?`, newFailCount, status, accountID)
	return err
}

// StartRecoveryRoutine periodically clears cooldowns.
func (p *AccountPool) StartRecoveryRoutine(cooldownMinutes int) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			p.recoverCooldowns(cooldownMinutes)
		}
	}()
}

func (p *AccountPool) recoverCooldowns(cooldownMinutes int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find accounts in cooldown that haven't been used recently
	result, err := p.db.Exec(fmt.Sprintf(`UPDATE host_accounts SET status = 'active', fail_count = 0 WHERE status = 'cooldown' AND last_used <= datetime('now', '-%d minutes')`, cooldownMinutes))
	if err == nil {
		rows, _ := result.RowsAffected()
		if rows > 0 {
			log.Printf("[pool] ♻️ Recovered %d accounts from cooldown", rows)
		}
	}
}
