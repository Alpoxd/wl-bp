package db

import (
	"database/sql"
	"time"
)

type User struct {
	ID            int
	VkUserID      int64
	Label         string
	IsAdmin       bool
	IsAllowed     bool
	MaxConcurrent int
	TotalSessions int
	CreatedAt     time.Time
}

func (db *DB) CreateUser(vkUserID int64, label string, isAdmin bool) (int, error) {
	result, err := db.Exec(
		`INSERT INTO users (vk_user_id, label, is_admin) VALUES (?, ?, ?)`,
		vkUserID, label, isAdmin,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetUserByVkID(vkUserID int64) (*User, error) {
	row := db.QueryRow(`SELECT id, vk_user_id, label, is_admin, is_allowed, max_concurrent, total_sessions, created_at FROM users WHERE vk_user_id = ?`, vkUserID)
	return scanUser(row)
}

func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.Query(`SELECT id, vk_user_id, label, is_admin, is_allowed, max_concurrent, total_sessions, created_at FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (db *DB) UpdateUserAllowed(id int, isAllowed bool) error {
	_, err := db.Exec(`UPDATE users SET is_allowed = ? WHERE id = ?`, isAllowed, id)
	return err
}

func (db *DB) RecordUserSession(id int) error {
	_, err := db.Exec(`UPDATE users SET total_sessions = total_sessions + 1 WHERE id = ?`, id)
	return err
}

func scanUser(s rowScanner) (*User, error) {
	var u User
	err := s.Scan(
		&u.ID, &u.VkUserID, &u.Label, &u.IsAdmin, &u.IsAllowed, &u.MaxConcurrent, &u.TotalSessions, &u.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
