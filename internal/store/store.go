package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct { db *sql.DB }

type LogEntry struct {
	ID           string   `json:"id"`
	Level        string   `json:"level"`
	Message      string   `json:"message"`
	Source       string   `json:"source"`
	Tags         string   `json:"tags"`
	CreatedAt    string   `json:"created_at"`
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	dsn := filepath.Join(dataDir, "quarry.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS logentrys (
			id TEXT PRIMARY KEY,\n\t\t\tlevel TEXT DEFAULT '',\n\t\t\tmessage TEXT DEFAULT '',\n\t\t\tsource TEXT DEFAULT '',\n\t\t\ttags TEXT DEFAULT '',
			created_at TEXT DEFAULT (datetime('now'))
		)`)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func genID() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

func (d *DB) Create(e *LogEntry) error {
	e.ID = genID()
	e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`INSERT INTO logentrys (id, level, message, source, tags, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		e.ID, e.Level, e.Message, e.Source, e.Tags, e.CreatedAt)
	return err
}

func (d *DB) Get(id string) *LogEntry {
	row := d.db.QueryRow(`SELECT id, level, message, source, tags, created_at FROM logentrys WHERE id=?`, id)
	var e LogEntry
	if err := row.Scan(&e.ID, &e.Level, &e.Message, &e.Source, &e.Tags, &e.CreatedAt); err != nil {
		return nil
	}
	return &e
}

func (d *DB) List() []LogEntry {
	rows, err := d.db.Query(`SELECT id, level, message, source, tags, created_at FROM logentrys ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.Level, &e.Message, &e.Source, &e.Tags, &e.CreatedAt); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result
}

func (d *DB) Delete(id string) error {
	_, err := d.db.Exec(`DELETE FROM logentrys WHERE id=?`, id)
	return err
}

func (d *DB) Count() int {
	var n int
	d.db.QueryRow(`SELECT COUNT(*) FROM logentrys`).Scan(&n)
	return n
}
