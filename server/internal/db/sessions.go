package db

import (
	"database/sql"
	"time"
)

type Session struct {
	ID        int
	UserID    int
	AccountID int
	CallID    string
	JoinLink  string
	Platform  string
	Mode      string
	Status    string
	ErrorMsg  string
	BytesTX   int64
	BytesRX   int64
	CreatedAt time.Time
	ClosedAt  *time.Time
}

func (db *DB) CreateSession(userID, accountID int, platform, mode string) (int, error) {
	result, err := db.Exec(
		`INSERT INTO sessions (user_id, account_id, platform, mode, status) VALUES (?, ?, ?, ?, 'creating')`,
		userID, accountID, platform, mode,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) UpdateSessionActive(id int, callID, joinLink string) error {
	_, err := db.Exec(
		`UPDATE sessions SET status = 'active', call_id = ?, join_link = ? WHERE id = ?`,
		callID, joinLink, id,
	)
	return err
}

func (db *DB) UpdateSessionFailed(id int, errorMsg string) error {
	_, err := db.Exec(
		`UPDATE sessions SET status = 'failed', error_msg = ?, closed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		errorMsg, id,
	)
	return err
}

func (db *DB) CloseSession(id int, bytesTX, bytesRX int64) error {
	_, err := db.Exec(
		`UPDATE sessions SET status = 'closed', bytes_tx = ?, bytes_rx = ?, closed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		bytesTX, bytesRX, id,
	)
	return err
}

func (db *DB) GetActiveSessions() ([]*Session, error) {
	rows, err := db.Query(`SELECT id, user_id, account_id, call_id, join_link, platform, mode, status, error_msg, bytes_tx, bytes_rx, created_at, closed_at FROM sessions WHERE status IN ('creating', 'active')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func scanSession(s rowScanner) (*Session, error) {
	var sess Session
	var closedAt *time.Time
	var callId, joinLink, errorMsg sql.NullString

	err := s.Scan(
		&sess.ID, &sess.UserID, &sess.AccountID, &callId, &joinLink,
		&sess.Platform, &sess.Mode, &sess.Status, &errorMsg,
		&sess.BytesTX, &sess.BytesRX, &sess.CreatedAt, &closedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if callId.Valid {
		sess.CallID = callId.String
	}
	if joinLink.Valid {
		sess.JoinLink = joinLink.String
	}
	if errorMsg.Valid {
		sess.ErrorMsg = errorMsg.String
	}
	sess.ClosedAt = closedAt

	return &sess, nil
}
