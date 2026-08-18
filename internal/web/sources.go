// Bu fayl — tashqi manbalar ustidagi O'ZIMIZNING GET endpointlarimiz.
//
// Agent ilgari delivery / daigou / support API'lariga faqat sikl ichida
// borardi va saralangan natijani ko'rishning yagona yo'li logga qarash edi.
// Endi har bir manbaning saralangan ko'rinishi HTTP orqali ham ochiq:
//
//	GET /api/source/delivery?track=...|client_id=...
//	GET /api/source/daigou?order_sn=...|express_num=...|client_id=...
//	GET /api/source/support?conversation_id=...|client_id=...&limit=...
//	GET /api/source/all?...            — uchalasi bitta so'rovda
//
// Barchasi mavjud Basic Auth ostida (Handler() shu muxga yozadi) va faqat
// o'qiydi. web paketi tashqi API mijozlarini bilmaydi: mantiq
// internal/sources da, bu yerga Options.Lookup orqali beriladi.
package web

import (
	"net/http"
	"strconv"
	"strings"

	"sahiy-agent/internal/sources"
)

// registerSources — manba endpointlarini muxga qo'shadi.
func (s *Server) registerSources(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/source/delivery", s.handleSourceDelivery)
	mux.HandleFunc("GET /api/source/daigou", s.handleSourceDaigou)
	mux.HandleFunc("GET /api/source/support", s.handleSourceSupport)
	mux.HandleFunc("GET /api/source/all", s.handleSourceAll)
}

// srcQuery — so'rov satridan Query yig'adi.
//
// express_num — track'ning taxallusi: daigou tomonida maydon shunday
// ataladi, yetkazmada esa track_number, lekin qiymat bitta.
func srcQuery(r *http.Request) sources.Query {
	q := r.URL.Query()
	track := strings.TrimSpace(q.Get("track"))
	if track == "" {
		track = strings.TrimSpace(q.Get("express_num"))
	}
	return sources.Query{
		ClientID:       atoi64(q.Get("client_id")),
		Track:          track,
		OrderSN:        strings.TrimSpace(q.Get("order_sn")),
		ConversationID: atoi64(q.Get("conversation_id")),
		Limit:          int(atoi64(q.Get("limit"))),
	}
}

func atoi64(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// src — Lookup o'rnatilganmi va so'rovda kamida bitta shart bormi.
// Bo'lmasa javob yozib, false qaytaradi.
func (s *Server) src(w http.ResponseWriter, q sources.Query) (*sources.Sources, bool) {
	if s.opt.Lookup == nil {
		http.Error(w, "manba qidiruvi ulanmagan", http.StatusServiceUnavailable)
		return nil, false
	}
	if q.Empty() {
		http.Error(w, "kamida bitta parametr kerak: client_id, track (express_num), order_sn yoki conversation_id",
			http.StatusBadRequest)
		return nil, false
	}
	return s.opt.Lookup, true
}

func (s *Server) handleSourceDelivery(w http.ResponseWriter, r *http.Request) {
	q := srcQuery(r)
	src, ok := s.src(w, q)
	if !ok {
		return
	}
	writeJSON(w, src.Delivery(q))
}

func (s *Server) handleSourceDaigou(w http.ResponseWriter, r *http.Request) {
	q := srcQuery(r)
	src, ok := s.src(w, q)
	if !ok {
		return
	}
	writeJSON(w, src.Daigou(q))
}

func (s *Server) handleSourceSupport(w http.ResponseWriter, r *http.Request) {
	q := srcQuery(r)
	src, ok := s.src(w, q)
	if !ok {
		return
	}
	writeJSON(w, src.Support(q))
}

// handleSourceAll — uchala manba bitta javobda. Bitta manba xato bersa ham
// javob 200 bo'ladi: xato o'sha blokning `error` maydonida keladi, qolgan
// ma'lumot yo'qolmaydi.
func (s *Server) handleSourceAll(w http.ResponseWriter, r *http.Request) {
	q := srcQuery(r)
	src, ok := s.src(w, q)
	if !ok {
		return
	}
	writeJSON(w, src.All(q))
}
