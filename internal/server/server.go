package server

import (
	"encoding/json"
	"github.com/stockyard-dev/stockyard-quarry/internal/store"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type Server struct {
	db      *store.DB
	mux     *http.ServeMux
	limits  Limits
	dataDir string
	pCfg    map[string]json.RawMessage
}

func New(db *store.DB, limits Limits, dataDir string) *Server {
	s := &Server{db: db, mux: http.NewServeMux(), limits: limits, dataDir: dataDir}

	// Ingest
	s.mux.HandleFunc("POST /api/ingest", s.ingest)
	s.mux.HandleFunc("POST /api/ingest/batch", s.ingestBatch)

	// Query
	s.mux.HandleFunc("GET /api/logs", s.queryLogs)
	s.mux.HandleFunc("GET /api/logs/tail", s.tail)

	// Sources
	s.mux.HandleFunc("GET /api/sources", s.listSources)

	// Saved searches
	s.mux.HandleFunc("GET /api/searches", s.listSearches)
	s.mux.HandleFunc("POST /api/searches", s.createSearch)
	s.mux.HandleFunc("DELETE /api/searches/{id}", s.deleteSearch)

	// Retention
	s.mux.HandleFunc("POST /api/prune", s.prune)

	// Meta
	s.mux.HandleFunc("GET /api/stats", s.stats)
	s.mux.HandleFunc("GET /api/levels", s.levels)
	s.mux.HandleFunc("GET /api/health", s.health)

	// Dashboard
	s.mux.HandleFunc("GET /ui", s.dashboard)
	s.mux.HandleFunc("GET /ui/", s.dashboard)
	s.mux.HandleFunc("GET /", s.root)
	s.mux.HandleFunc("GET /api/tier", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"tier": s.limits.Tier, "upgrade_url": "https://stockyard.dev/quarry/"})
	})

	s.loadPersonalConfig()
	s.mux.HandleFunc("GET /api/config", s.configHandler)
	s.mux.HandleFunc("GET /api/extras/{resource}", s.listExtras)
	s.mux.HandleFunc("GET /api/extras/{resource}/{id}", s.getExtras)
	s.mux.HandleFunc("PUT /api/extras/{resource}/{id}", s.putExtras)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui", http.StatusFound)
}

// ── Ingest ──

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var e store.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	if e.Message == "" {
		writeErr(w, 400, "message required")
		return
	}
	if err := s.db.Ingest(&e); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, e)
}

func (s *Server) ingestBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Logs []store.LogEntry `json:"logs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	count, err := s.db.IngestBatch(req.Logs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]int{"ingested": count})
}

// ── Query ──

func (s *Server) queryLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	f := store.LogFilter{
		Source: q.Get("source"),
		Level:  q.Get("level"),
		Search: q.Get("search"),
		After:  q.Get("after"),
		Before: q.Get("before"),
		Limit:  limit,
		Offset: offset,
	}
	logs, total := s.db.Query(f)
	writeJSON(w, 200, map[string]any{"logs": orEmpty(logs), "total": total})
}

func (s *Server) tail(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	writeJSON(w, 200, map[string]any{"logs": orEmpty(s.db.Tail(n))})
}

// ── Sources ──

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"sources": orEmpty(s.db.ListSources())})
}

// ── Saved Searches ──

func (s *Server) listSearches(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"searches": orEmpty(s.db.ListSavedSearches())})
}

func (s *Server) createSearch(w http.ResponseWriter, r *http.Request) {
	var ss store.SavedSearch
	if err := json.NewDecoder(r.Body).Decode(&ss); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	if ss.Name == "" {
		writeErr(w, 400, "name required")
		return
	}
	if err := s.db.CreateSavedSearch(&ss); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, ss)
}

func (s *Server) deleteSearch(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteSavedSearch(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"deleted": "ok"})
}

// ── Retention ──

func (s *Server) prune(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionDays int `json:"retention_days"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	pruned, err := s.db.Prune(req.RetentionDays)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"pruned": pruned})
}

// ── Meta ──

func (s *Server) levels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"levels": s.db.LevelCounts()})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.db.Stats())
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	st := s.db.Stats()
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "quarry", "total_logs": st.TotalLogs, "sources": st.Sources})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
func init() { log.SetFlags(log.LstdFlags | log.Lshortfile) }

// ─── personalization (auto-added) ──────────────────────────────────

func (s *Server) loadPersonalConfig() {
	path := filepath.Join(s.dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("%s: warning: could not parse config.json: %v", "quarry", err)
		return
	}
	s.pCfg = cfg
	log.Printf("%s: loaded personalization from %s", "quarry", path)
}

func (s *Server) configHandler(w http.ResponseWriter, r *http.Request) {
	if s.pCfg == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.pCfg)
}

func (s *Server) listExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	all := s.db.AllExtras(resource)
	out := make(map[string]json.RawMessage, len(all))
	for id, data := range all {
		out[id] = json.RawMessage(data)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) getExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	id := r.PathValue("id")
	data := s.db.GetExtras(resource, id)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(data))
}

func (s *Server) putExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	id := r.PathValue("id")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"read body"}`, 400)
		return
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, `{"error":"invalid json"}`, 400)
		return
	}
	if err := s.db.SetExtras(resource, id, string(body)); err != nil {
		http.Error(w, `{"error":"save failed"}`, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":"saved"}`))
}
