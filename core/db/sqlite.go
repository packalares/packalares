package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	database *sql.DB
	dbOnce   sync.Once
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		totp_secret TEXT DEFAULT '',
		totp_enabled INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS mounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		mount_type TEXT NOT NULL,
		address TEXT NOT NULL,
		share_name TEXT DEFAULT '',
		username TEXT DEFAULT '',
		mount_path TEXT NOT NULL,
		active INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token)`,
	`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id)`,
}

func Init(dbPath string) error {
	var initErr error
	dbOnce.Do(func() {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = fmt.Errorf("create db directory: %w", err)
			return
		}

		db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
		if err != nil {
			initErr = fmt.Errorf("open database: %w", err)
			return
		}

		db.SetMaxOpenConns(1)

		if err := db.Ping(); err != nil {
			initErr = fmt.Errorf("ping database: %w", err)
			return
		}

		for i, m := range migrations {
			if _, err := db.Exec(m); err != nil {
				initErr = fmt.Errorf("migration %d: %w", i, err)
				return
			}
		}

		database = db
	})
	return initErr
}

func Get() *sql.DB {
	return database
}
