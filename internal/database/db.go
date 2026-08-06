package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	var err error
	DB, err = Open(filepath.Join(dataDir, "webhook-ui.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return Migrate()
}

// Open connects to a SQLite file with the settings the application depends on.
// Tests use it too: async executions write from their own goroutines while
// requests are being served, and a database opened without these settings
// fails those writes outright instead of waiting, which would make a test
// bench disagree with production about whether the code works.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	// SQLite takes one writer at a time. Letting database/sql hand out several
	// connections turns that into SQLITE_BUSY under concurrent executions;
	// funnelling through one connection makes them queue instead.
	db.SetMaxOpenConns(1)
	return db, nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
