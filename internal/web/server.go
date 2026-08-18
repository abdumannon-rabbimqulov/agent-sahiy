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
	"strconv"
	"strings"

	"sahiy-agent/internal/escalation"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/prompts"
	"sahiy-agent/internal/settings"
	"sahiy-agent/internal/sources"
	"sahiy-agent/internal/store"
)

//go:embed static
var staticFS embed.FS

// Options — server sozlamalari.
type Options struct {
	Addr      string
	AdminUser string // bo'sh bo'lsa Basic Auth o'chiq
	AdminPass string
	Dev       bool // true bo'lsa frontend diskdan o'qiladi (restart shart emas)

	// SendReply — admin tekshirib tasdiqlagan javobni mijozga yuboradi.
	// main.go beradi (web paketi Sahiy API bilan bevosita ishlamaydi).
	// nil bo'lsa tasdiqlash endpointlari 503 qaytaradi.
	SendReply func(conversationID int64, text string) error

	// TryPrompt — /prompts dagi "Sinab ko'rish": saqlanmagan matnni
	// haqiqiy model orqali o'tkazadi. nil bo'lsa endpoint 503 qaytaradi.
	TryPrompt prompts.TryFunc

	// Lookup — tashqi manbalar (delivery / daigou / support) ustidagi
	// /api/source/* endpointlari uchun. main.go beradi; nil bo'lsa
	// o'sha endpointlar 503 qaytaradi.
	Lookup *sources.Sources
}

// Server statistika, tarix va kategoriyalarni ko'rsatadigan dashboard.
type Server struct {
	store   *store.Store
	esc     *escalation.Store
	set     *settings.Store
	prompts *prompts.Handler
	opt     Options
	files   fs.FS
}

// New yangi web server.
func New(st *store.Store, esc *escalation.Store,
	set *settings.Store, prm *prompts.Service, opt Options) *Server {
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
	h := prompts.NewHandler(prm)
	h.SetTry(opt.TryPrompt)
	return &Server{store: st, esc: esc, set: set,
		prompts: h, opt: opt, files: files}
}

// Start HTTP serverni ishga tushiradi (blokirovka qiladi).
func (s *Server) Start() error {
	return http.ListenAndServe(s.opt.Addr, s.Handler())
}

// Handler — barcha yo'llar va Basic Auth bilan tayyor handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.files)))
	mux.HandleFunc("GET /{$}", s.page("index.html"))
	mux.HandleFunc("GET /prompts", s.page("prompts.html"))

	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/escalations", s.handleEscalations)
	mux.HandleFunc("GET /api/costs", s.handleCosts)
	mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", s.handleSettingsSet)
	mux.HandleFunc("POST /api/interactions/{id}/send", s.handleApprove)
	mux.HandleFunc("POST /api/interactions/{id}/reject", s.handleReject)

	// Tashqi manbalar ustidagi o'z GET endpointlarimiz (sources.go).
	s.registerSources(mux)

	// Promptlar CRUD'i alohida paketda (internal/prompts/http.go).
	s.prompts.Register(mux)

	return s.auth(mux)
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

// handleEscalations xodimlar guruhiga yuborilgan muammolarni qaytaradi.
// ?status=pending — faqat javob kutayotganlari.
func (s *Server) handleEscalations(w http.ResponseWriter, r *http.Request) {
	var (
		items []models.Escalation
		err   error
	)
	if r.URL.Query().Get("status") == "pending" {
		items, err = s.esc.Pending(100)
	} else {
		items, err = s.esc.Recent(100)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

// costQuery — /api/costs so'rovining o'qilgan parametrlari.
type costQuery struct {
	Group string // day | client | conversation
	Days  int    // 0 — butun tarix
	Sort  string // cost | last
	Limit int
}

// parseCostQuery so'rov parametrlarini o'qiydi va chegaralaydi.
// Noto'g'ri qiymatlar jimgina default'ga tushadi.
func parseCostQuery(r *http.Request) costQuery {
	q := r.URL.Query()
	out := costQuery{Group: "day", Days: 30, Sort: "cost", Limit: 50}

	switch g := q.Get("group"); g {
	case "client", "conversation", "day":
		out.Group = g
	}
	if s := q.Get("sort"); s == "last" {
		out.Sort = s
	}
	// days: "all" yoki 0 — butun tarix.
	if v := q.Get("days"); v != "" {
		if strings.EqualFold(v, "all") {
			out.Days = 0
		} else if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			out.Days = min(n, 365)
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.Limit = min(n, 500)
		}
	}
	return out
}

// handleCosts AI xarajatini uch kesimda qaytaradi: kunlik, mijozlar bo'yicha
// yoki muammolar (suhbatlar) bo'yicha.
func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	q := parseCostQuery(r)

	var (
		items any
		err   error
	)
	switch q.Group {
	case "client":
		var rows []store.ClientCost
		if rows, err = s.store.ByClient(q.Days, q.Limit, q.Sort); rows == nil {
			rows = []store.ClientCost{}
		}
		items = rows
	case "conversation":
		var rows []store.ConversationCost
		if rows, err = s.store.ByConversation(q.Days, q.Limit, q.Sort); rows == nil {
			rows = []store.ConversationCost{}
		}
		items = rows
	default:
		// Daily o'zi days <= 0 ni "butun tarix" deb tushunadi.
		var rows []store.DailyCost
		if rows, err = s.store.Daily(q.Days); rows == nil {
			rows = []store.DailyCost{}
		}
		items = rows
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

// --- sozlamalar (AI yoqish/o'chirish) ---

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.set.All())
}

// handleSettingsSet bir yoki bir nechta sozlamani o'zgartiradi.
// Tanasi: {"ai_enabled": false, "auto_reply": true}
func (s *Server) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	var body map[string]bool
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "json o'qib bo'lmadi: "+err.Error(), http.StatusBadRequest)
		return
	}
	for k, v := range body {
		switch k {
		case settings.AIEnabled, settings.AutoReply:
			if err := s.set.Set(k, v); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "noma'lum sozlama: "+k, http.StatusBadRequest)
			return
		}
	}
	writeJSON(w, s.set.All())
}

// --- AI javobini tekshirib tasdiqlash ---

// handleApprove admin ko'rib chiqqan javobni mijozga yuboradi.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	in, ok := s.draft(w, r)
	if !ok {
		return
	}
	if s.opt.SendReply == nil {
		http.Error(w, "javob yuborish sozlanmagan", http.StatusServiceUnavailable)
		return
	}
	if err := s.opt.SendReply(in.ConversationID, in.AIReply); err != nil {
		http.Error(w, "mijozga yuborilmadi: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := s.store.MarkSent(in.ID, "AI + admin tasdiqladi"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": in.ID, "status": models.StatusAISent})
}

// handleReject javobni rad etadi — mijozga hech narsa yuborilmaydi.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	in, ok := s.draft(w, r)
	if !ok {
		return
	}
	if err := s.store.MarkRejected(in.ID, "Admin rad etdi"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": in.ID, "status": models.StatusRejected})
}

// draft — id bo'yicha yozuvni oladi va uni tasdiqlash mumkinligini tekshiradi.
// Faqat yuborilmagan AI javobi (ai_draft) tasdiqlanadi — ikki marta yuborish
// yoki xodim javobini qayta yuborishning oldini oladi.
func (s *Server) draft(w http.ResponseWriter, r *http.Request) (*models.Interaction, bool) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	in, err := s.store.Get(id)
	if err != nil {
		http.Error(w, "yozuv topilmadi", http.StatusNotFound)
		return nil, false
	}
	if in.Status != models.StatusAIDraft || in.Sent {
		http.Error(w, "bu javob tekshirishni kutmayapti (holat: "+models.StatusLabel(in.Status)+")",
			http.StatusConflict)
		return nil, false
	}
	if strings.TrimSpace(in.AIReply) == "" {
		http.Error(w, "javob matni bo'sh", http.StatusConflict)
		return nil, false
	}
	return in, true
}

// --- yordamchilar ---

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
