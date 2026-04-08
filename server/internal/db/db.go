package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Connect(dataSourceName string) (*DB, error) {
	db, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return &DB{db}, nil
}

func runMigrations(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS host_accounts (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			platform            TEXT NOT NULL DEFAULT 'vk',
			label               TEXT NOT NULL,
			cookies             TEXT NOT NULL,
			status              TEXT NOT NULL DEFAULT 'active',
			max_concurrent_calls INTEGER NOT NULL DEFAULT 1,
			active_workers      INTEGER NOT NULL DEFAULT 0,
			banned_at           TIMESTAMP,
			last_used           TIMESTAMP,
			fail_count          INTEGER DEFAULT 0,
			total_calls         INTEGER DEFAULT 0,
			created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			vk_user_id      INTEGER UNIQUE NOT NULL,
			label           TEXT,
			is_admin        BOOLEAN DEFAULT FALSE,
			is_allowed      BOOLEAN DEFAULT TRUE,
			max_concurrent  INTEGER DEFAULT 1,
			total_sessions  INTEGER DEFAULT 0,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id         INTEGER REFERENCES users(id),
			account_id      INTEGER REFERENCES host_accounts(id),
			call_id         TEXT,
			join_link       TEXT,
			platform        TEXT DEFAULT 'vk',
			mode            TEXT DEFAULT 'dc',
			status          TEXT DEFAULT 'creating',
			error_msg       TEXT,
			bytes_tx        INTEGER DEFAULT 0,
			bytes_rx        INTEGER DEFAULT 0,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			closed_at       TIMESTAMP
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	
	log.Println("[db] Migrations completed successfully")
	return nil
}
