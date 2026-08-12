// Package web — dashboard va kategoriya CRUD API'si.
// Frontend fayllari static/ katalogida (binarga embed qilinadi).
package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sahiy-agent/internal/category"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/store"
)

//go:embed static
var staticFS embed.FS

// Options — server sozlamalari.
type Options struct {
	Addr      string
	AdminUser string // bo'sh bo'lsa Basic Auth o'chiq
	AdminPass string
	Dev       bool   // true bo'lsa frontend diskdan o'qiladi (restart shart emas)
	MediaDir  string // chat rasmlari katalogi (/media/ orqali beriladi)
}

// Server statistika, tarix va kategoriyalarni ko'rsatadigan dashboard.
type Server struct {
	store *store.Store
	cats  *category.Store
	opt   Options
	files fs.FS
}

// New yangi web server.
func New(st *store.Store, cats *category.Store, opt Options) *Server {
	var files fs.FS
	if opt.Dev {
		files = os.DirFS("internal/web/static")
	} else {
		sub, err := fs.Sub(staticFS, "static")
		if err != nil {
			panic(err) // embed har doim mavjud
		}
		files = sub
	}
	return &Server{store: st, cats: cats, opt: opt, files: files}
}

// Start HTTP serverni ishga tushiradi (blokirovka qiladi).
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.files)))
	mux.HandleFunc("GET /{$}", s.page("index.html"))
	mux.HandleFunc("GET /categories", s.page("categories.html"))

	// Chat rasmlari — Basic Auth ostida qoladi (mijoz ma'lumoti).
	if s.opt.MediaDir != "" {
		mux.Handle("GET /media/", http.StripPrefix("/media/",
			http.FileServer(http.Dir(s.opt.MediaDir))))
	}

	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/images", s.handleImages)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/categories", s.handleCategoryList)
	mux.HandleFunc("POST /api/categories", s.handleCategoryCreate)
	mux.HandleFunc("PUT /api/categories/{id}", s.handleCategoryUpdate)
	mux.HandleFunc("DELETE /api/categories/{id}", s.handleCategoryDelete)

	return http.ListenAndServe(s.opt.Addr, s.auth(mux))
}

// auth — Basic Auth (ADMIN_USER/ADMIN_PASS o'rnatilgan bo'lsa).
func (s *Server) auth(next http.Handler) http.Handler {
	if s.opt.AdminUser == "" || s.opt.AdminPass == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		okUser := subtle.ConstantTimeCompare([]byte(user), []byte(s.opt.AdminUser)) == 1
		okPass := subtle.ConstantTimeCompare([]byte(pass), []byte(s.opt.AdminPass)) == 1
		if !ok || !okUser || !okPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Sahiy Agent"`)
			http.Error(w, "avtorizatsiya kerak", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// page — static katalogdagi HTML sahifani beradi.
func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(s.files, name)
		if err != nil {
			http.Error(w, "sahifa topilmadi", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

// --- statistika / tarix ---

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Recent(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

// handleImages oxirgi chat rasmlarini qaytaradi (dashboard uchun).
func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.RecentImages(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Path — serverdagi to'liq yo'l; tashqariga /media/ ga nisbiy yo'l beriladi.
	for i := range items {
		items[i].File = fmt.Sprintf("%d/%s", items[i].ConversationID, filepath.Base(items[i].Path))
	}
	writeJSON(w, items)
}

// --- kategoriyalar ---

func (s *Server) handleCategoryList(w http.ResponseWriter, r *http.Request) {
	items, err := s.cats.List(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleCategoryCreate(w http.ResponseWriter, r *http.Request) {
	c, err := decodeCategory(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cats.Create(c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

func (s *Server) handleCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, err := decodeCategory(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cats.Update(id, c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := s.cats.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, updated)
}

func (s *Server) handleCategoryDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cats.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- yordamchilar ---

func decodeCategory(r *http.Request) (*models.Category, error) {
	var c models.Category
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&c); err != nil {
		return nil, fmt.Errorf("json o'qib bo'lmadi: %w", err)
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Description = strings.TrimSpace(c.Description)
	c.Content = strings.TrimSpace(c.Content)
	return &c, nil
}

func pathID(r *http.Request) (uint, error) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("noto'g'ri id")
	}
	return uint(id), nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
