package db

import (
	"database/sql"
	"time"
)

type HostAccount struct {
	ID                 int
	Platform           string
	Label              string
	Cookies            string
	Status             string
	MaxConcurrentCalls int
	ActiveWorkers      int
	BannedAt           *time.Time
	LastUsed           *time.Time
	FailCount          int
	TotalCalls         int
	CreatedAt          time.Time
}

func (db *DB) CreateHostAccount(platform, label, cookies string, maxConcurrent int) (int, error) {
	result, err := db.Exec(
		`INSERT INTO host_accounts (platform, label, cookies, max_concurrent_calls) VALUES (?, ?, ?, ?)`,
		platform, label, cookies, maxConcurrent,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetHostAccount(id int) (*HostAccount, error) {
	row := db.QueryRow(`SELECT id, platform, label, cookies, status, max_concurrent_calls, active_workers, banned_at, last_used, fail_count, total_calls, created_at FROM host_accounts WHERE id = ?`, id)
	return scanHostAccount(row)
}

func (db *DB) ListHostAccounts() ([]*HostAccount, error) {
	rows, err := db.Query(`SELECT id, platform, label, cookies, status, max_concurrent_calls, active_workers, banned_at, last_used, fail_count, total_calls, created_at FROM host_accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*HostAccount
	for rows.Next() {
		acc, err := scanHostAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func (db *DB) UpdateHostAccountStatus(id int, status string) error {
	_, err := db.Exec(`UPDATE host_accounts SET status = ? WHERE id = ?`, status, id)
	return err
}

func (db *DB) IncrementFailCount(id int) error {
	_, err := db.Exec(`UPDATE host_accounts SET fail_count = fail_count + 1 WHERE id = ?`, id)
	return err
}

func (db *DB) RecordCall(id int) error {
	_, err := db.Exec(`UPDATE host_accounts SET total_calls = total_calls + 1, last_used = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (db *DB) ResetFailCount(id int) error {
    _, err := db.Exec(`UPDATE host_accounts SET fail_count = 0 WHERE id = ?`, id)
    return err
}

func (db *DB) DeleteHostAccount(id int) error {
    _, err := db.Exec(`DELETE FROM host_accounts WHERE id = ?`, id)
    return err
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanHostAccount(s rowScanner) (*HostAccount, error) {
	var acc HostAccount
	var bannedAt, lastUsed *time.Time
	err := s.Scan(
		&acc.ID, &acc.Platform, &acc.Label, &acc.Cookies, &acc.Status, &acc.MaxConcurrentCalls, &acc.ActiveWorkers,
		&bannedAt, &lastUsed, &acc.FailCount, &acc.TotalCalls, &acc.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	acc.BannedAt = bannedAt
	acc.LastUsed = lastUsed
	return &acc, nil
}
