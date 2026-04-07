package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ db *sql.DB }

type LogEntry struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Level     string            `json:"level"` // debug, info, warn, error, fatal
	Message   string            `json:"message"`
	Meta      map[string]string `json:"meta,omitempty"`
	Timestamp string            `json:"timestamp"`
}

type Source struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}

type SavedSearch struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	Filters   string `json:"filters,omitempty"` // JSON of filter params
	CreatedAt string `json:"created_at"`
}

type LogFilter struct {
	Source string
	Level  string
	Search string
	After  string // RFC3339
	Before string // RFC3339
	Limit  int
	Offset int
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
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS logs (
			id TEXT PRIMARY KEY,
			source TEXT DEFAULT '',
			level TEXT DEFAULT 'info',
			message TEXT NOT NULL,
			meta_json TEXT DEFAULT '{}',
			timestamp TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS saved_searches (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			query TEXT DEFAULT '',
			filters TEXT DEFAULT '{}',
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_source ON logs(source)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_ts ON logs(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_message ON logs(message)`,
	} {
		if _, err := db.Exec(q); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS extras(resource TEXT NOT NULL,record_id TEXT NOT NULL,data TEXT NOT NULL DEFAULT '{}',PRIMARY KEY(resource, record_id))`)
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }
func genID() string        { return fmt.Sprintf("%d", time.Now().UnixNano()) }
func now() string          { return time.Now().UTC().Format(time.RFC3339) }

// ── Ingest ──

func (d *DB) Ingest(entry *LogEntry) error {
	entry.ID = genID()
	if entry.Timestamp == "" {
		entry.Timestamp = now()
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	if entry.Meta == nil {
		entry.Meta = map[string]string{}
	}
	mj, _ := json.Marshal(entry.Meta)
	_, err := d.db.Exec(`INSERT INTO logs (id,source,level,message,meta_json,timestamp) VALUES (?,?,?,?,?,?)`,
		entry.ID, entry.Source, entry.Level, entry.Message, string(mj), entry.Timestamp)
	return err
}

func (d *DB) IngestBatch(entries []LogEntry) (int, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO logs (id,source,level,message,meta_json,timestamp) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	count := 0
	for i := range entries {
		e := &entries[i]
		e.ID = genID()
		if e.Timestamp == "" {
			e.Timestamp = now()
		}
		if e.Level == "" {
			e.Level = "info"
		}
		if e.Meta == nil {
			e.Meta = map[string]string{}
		}
		mj, _ := json.Marshal(e.Meta)
		if _, err := stmt.Exec(e.ID, e.Source, e.Level, e.Message, string(mj), e.Timestamp); err == nil {
			count++
		}
		time.Sleep(time.Nanosecond) // unique IDs
	}
	tx.Commit()
	return count, nil
}

// ── Query ──

func (d *DB) Query(f LogFilter) ([]LogEntry, int) {
	where := []string{"1=1"}
	args := []any{}
	if f.Source != "" {
		where = append(where, "source=?")
		args = append(args, f.Source)
	}
	if f.Level != "" {
		where = append(where, "level=?")
		args = append(args, f.Level)
	}
	if f.Search != "" {
		where = append(where, "message LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	if f.After != "" {
		where = append(where, "timestamp>=?")
		args = append(args, f.After)
	}
	if f.Before != "" {
		where = append(where, "timestamp<=?")
		args = append(args, f.Before)
	}
	w := strings.Join(where, " AND ")
	var total int
	d.db.QueryRow("SELECT COUNT(*) FROM logs WHERE "+w, args...).Scan(&total)
	if f.Limit <= 0 {
		f.Limit = 100
	}
	q := fmt.Sprintf("SELECT id,source,level,message,meta_json,timestamp FROM logs WHERE %s ORDER BY timestamp DESC LIMIT ? OFFSET ?", w)
	args = append(args, f.Limit, f.Offset)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		var mj string
		if err := rows.Scan(&e.ID, &e.Source, &e.Level, &e.Message, &mj, &e.Timestamp); err != nil {
			continue
		}
		json.Unmarshal([]byte(mj), &e.Meta)
		out = append(out, e)
	}
	return out, total
}

// Tail returns the most recent N log entries
func (d *DB) Tail(n int) []LogEntry {
	if n <= 0 {
		n = 50
	}
	entries, _ := d.Query(LogFilter{Limit: n})
	return entries
}

// ── Sources ──

func (d *DB) ListSources() []Source {
	rows, err := d.db.Query(`SELECT source, COUNT(*) as cnt, MAX(timestamp) as last FROM logs GROUP BY source ORDER BY cnt DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.Name, &s.Count, &s.LastSeen); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ── Level counts ──

func (d *DB) LevelCounts() map[string]int {
	m := map[string]int{}
	rows, err := d.db.Query(`SELECT level, COUNT(*) FROM logs GROUP BY level`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var l string
		var c int
		rows.Scan(&l, &c)
		m[l] = c
	}
	return m
}

// ── Retention ──

func (d *DB) Prune(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format(time.RFC3339)
	res, err := d.db.Exec(`DELETE FROM logs WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ── Saved Searches ──

func (d *DB) CreateSavedSearch(ss *SavedSearch) error {
	ss.ID = genID()
	ss.CreatedAt = now()
	_, err := d.db.Exec(`INSERT INTO saved_searches (id,name,query,filters,created_at) VALUES (?,?,?,?,?)`,
		ss.ID, ss.Name, ss.Query, ss.Filters, ss.CreatedAt)
	return err
}

func (d *DB) ListSavedSearches() []SavedSearch {
	rows, err := d.db.Query(`SELECT id,name,query,filters,created_at FROM saved_searches ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []SavedSearch
	for rows.Next() {
		var ss SavedSearch
		if err := rows.Scan(&ss.ID, &ss.Name, &ss.Query, &ss.Filters, &ss.CreatedAt); err != nil {
			continue
		}
		out = append(out, ss)
	}
	return out
}

func (d *DB) DeleteSavedSearch(id string) error {
	_, err := d.db.Exec(`DELETE FROM saved_searches WHERE id=?`, id)
	return err
}

// ── Stats ──

type Stats struct {
	TotalLogs     int            `json:"total_logs"`
	Sources       int            `json:"sources"`
	ByLevel       map[string]int `json:"by_level"`
	Last24h       int            `json:"last_24h"`
	SavedSearches int            `json:"saved_searches"`
}

func (d *DB) Stats() Stats {
	var s Stats
	d.db.QueryRow(`SELECT COUNT(*) FROM logs`).Scan(&s.TotalLogs)
	d.db.QueryRow(`SELECT COUNT(DISTINCT source) FROM logs`).Scan(&s.Sources)
	since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	d.db.QueryRow(`SELECT COUNT(*) FROM logs WHERE timestamp>=?`, since).Scan(&s.Last24h)
	d.db.QueryRow(`SELECT COUNT(*) FROM saved_searches`).Scan(&s.SavedSearches)
	s.ByLevel = d.LevelCounts()
	return s
}

// ─── Extras: generic key-value storage for personalization custom fields ───

func (d *DB) GetExtras(resource, recordID string) string {
	var data string
	err := d.db.QueryRow(
		`SELECT data FROM extras WHERE resource=? AND record_id=?`,
		resource, recordID,
	).Scan(&data)
	if err != nil || data == "" {
		return "{}"
	}
	return data
}

func (d *DB) SetExtras(resource, recordID, data string) error {
	if data == "" {
		data = "{}"
	}
	_, err := d.db.Exec(
		`INSERT INTO extras(resource, record_id, data) VALUES(?, ?, ?)
		 ON CONFLICT(resource, record_id) DO UPDATE SET data=excluded.data`,
		resource, recordID, data,
	)
	return err
}

func (d *DB) DeleteExtras(resource, recordID string) error {
	_, err := d.db.Exec(
		`DELETE FROM extras WHERE resource=? AND record_id=?`,
		resource, recordID,
	)
	return err
}

func (d *DB) AllExtras(resource string) map[string]string {
	out := make(map[string]string)
	rows, _ := d.db.Query(
		`SELECT record_id, data FROM extras WHERE resource=?`,
		resource,
	)
	if rows == nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, data string
		rows.Scan(&id, &data)
		out[id] = data
	}
	return out
}
