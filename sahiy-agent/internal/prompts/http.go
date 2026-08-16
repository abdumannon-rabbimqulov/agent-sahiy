package prompts

import (
	"encoding/json"
	"net/http"
	"strings"

	"sahiy-agent/internal/models"
)

// maxBody — so'rov tanasi chegarasi (prompt matni katta bo'lishi mumkin).
const maxBody = 2 << 20 // 2 MB

// Handler — promptlar uchun HTTP transport. Biznes qoidalari Service'da:
// bu qatlam faqat JSON o'qiydi/yozadi va xatoni status kodiga aylantiradi.
type Handler struct {
	svc *Service
}

// NewHandler yangi transport.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register marshrutlarni ro'yxatdan o'tkazadi.
//
// Kalit ichida ":" bo'ladi ("cat:order"), shuning uchun {key...} wildcard.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/prompts", h.list)
	mux.HandleFunc("POST /api/prompts", h.create)
	mux.HandleFunc("GET /api/prompts/{key...}", h.get)
	mux.HandleFunc("PUT /api/prompts/{key...}", h.update)
	mux.HandleFunc("PATCH /api/prompts/{key...}", h.update)
	mux.HandleFunc("DELETE /api/prompts/{key...}", h.delete)
}

// createRequest — POST /api/prompts tanasi.
// Enabled ko'rsatilmasa prompt yoqilgan holda yaratiladi.
type createRequest struct {
	Key     string `json:"key"`
	Content string `json:"content"`
	Enabled *bool  `json:"enabled"`
}

// updateRequest — PUT/PATCH tanasi. Har maydon ko'rsatkich: "berilmagan" va
// "bo'sh qiymat berilgan" farqlanadi. Key berilsa — kalit o'zgaradi.
type updateRequest struct {
	Key     *string `json:"key"`
	Content *string `json:"content"`
	Enabled *bool   `json:"enabled"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List()
	if err != nil {
		fail(w, err)
		return
	}
	if items == nil {
		items = []models.Prompt{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.ByKey(key(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if !decode(w, r, &body) {
		return
	}
	enabled := body.Enabled == nil || *body.Enabled
	p, err := h.svc.Create(body.Key, body.Content, enabled)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var body updateRequest
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.Update(key(r), Update{
		NewKey:  body.Key,
		Content: body.Content,
		Enabled: body.Enabled,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(key(r)); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- yordamchilar ---

// key — yo'ldagi kalit ("/api/prompts/cat:order" → "cat:order").
func key(r *http.Request) string { return strings.TrimSpace(r.PathValue("key")) }

// decode — JSON tanasini o'qiydi; xato bo'lsa 400 yozib false qaytaradi.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(dst); err != nil {
		http.Error(w, "json o'qib bo'lmadi: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// fail — xatoni mos status kodi bilan qaytaradi (matn o'zbekcha).
func fail(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), HTTPStatus(err))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
