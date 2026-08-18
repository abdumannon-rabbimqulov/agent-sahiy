package prompts

import (
	"context"
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
	try TryFunc
}

// TryFunc — promptni SAQLAMASDAN haqiqiy model orqali sinab ko'radi.
// main.go beradi (prompts paketi ai/ollama haqida hech narsa bilmaydi);
// nil bo'lsa /try endpointi 503 qaytaradi.
type TryFunc func(ctx context.Context, req TryRequest) (any, error)

// TryRequest — sinov so'rovi. Content — hali saqlanmagan tahrir.
type TryRequest struct {
	Key        string `json:"-"`
	Content    string `json:"content"`
	Transcript string `json:"transcript"`
	// OrderInfo — "tizimdan olingan buyurtma" o'rniga qo'yiladigan matn
	// (ixtiyoriy, block:order yo'lini tekshirish uchun).
	OrderInfo string `json:"order_info"`
}

// NewHandler yangi transport.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SetTry — "Sinab ko'rish" funksiyasini ulaydi.
func (h *Handler) SetTry(f TryFunc) { h.try = f }

// Register marshrutlarni ro'yxatdan o'tkazadi.
//
// Kalit ichida ":" bo'ladi ("cat:order"), shuning uchun {key...} wildcard.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/prompts", h.list)
	mux.HandleFunc("POST /api/prompts", h.create)
	// meta — {key...} dan OLDIN turishi shart emas (ServeMux aniqrog'ini
	// tanlaydi), lekin o'qishga qulay bo'lsin uchun shu yerda.
	mux.HandleFunc("GET /api/prompts-meta", h.meta)
	mux.HandleFunc("GET /api/prompt-backups/{key...}", h.backups)
	mux.HandleFunc("POST /api/prompt-restore/{key...}", h.restore)
	mux.HandleFunc("POST /api/prompt-try/{key...}", h.tryPrompt)
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
	writeJSON(w, http.StatusCreated, saved{Prompt: p, Warnings: h.svc.Warn(p.Key, p.Content)})
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
	writeJSON(w, http.StatusOK, saved{Prompt: p, Warnings: h.svc.Warn(p.Key, p.Content)})
}

// saved — saqlangan prompt + saqlashni to'xtatmagan ogohlantirishlar
// (masalan kutilgan placeholder yozilmagan).
type saved struct {
	models.Prompt
	Warnings []string `json:"warnings,omitempty"`
}

// meta — dashboard uchun kalitlar va placeholder'lar ro'yxati.
func (h *Handler) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Meta())
}

// backups — kalitning saqlangan nusxalari.
func (h *Handler) backups(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Backups(key(r))
	if err != nil {
		fail(w, err)
		return
	}
	if items == nil {
		items = []models.PromptBackup{}
	}
	writeJSON(w, http.StatusOK, items)
}

// restore — nusxadagi matnni qaytaradi ({"id": 12}).
func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID uint `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.Restore(key(r), body.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved{Prompt: p, Warnings: h.svc.Warn(p.Key, p.Content)})
}

// tryPrompt — saqlanmagan matnni haqiqiy model orqali sinaydi.
func (h *Handler) tryPrompt(w http.ResponseWriter, r *http.Request) {
	if h.try == nil {
		http.Error(w, "sinab ko'rish ulanmagan", http.StatusServiceUnavailable)
		return
	}
	var body TryRequest
	if !decode(w, r, &body) {
		return
	}
	body.Key = key(r)
	if strings.TrimSpace(body.Content) == "" || strings.TrimSpace(body.Transcript) == "" {
		http.Error(w, "prompt matni va sinov murojaati bo'sh bo'lmasligi kerak",
			http.StatusBadRequest)
		return
	}
	out, err := h.try(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
