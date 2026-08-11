package web

import (
	"encoding/json"
	"net/http"

	"sahiy-agent/internal/store"
)

// Server statistika/tarixni ko'rsatadigan dashboard.
type Server struct {
	store *store.Store
	addr  string
}

// New yangi web server.
func New(st *store.Store, addr string) *Server {
	return &Server{store: st, addr: addr}
}

// Start HTTP serverni ishga tushiradi (blokirovka qiladi).
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/history", s.handleHistory)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, st)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Recent(100)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
