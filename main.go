// Dastur kirish nuqtasi: .env o'qiladi, baza ochiladi, HTTP marshrutlar
// ulanadi va fon sikllari ishga tushadi.
//
// Shu fayldagi handlerlar — tashqi API'lar uchun proksi: suhbatlar,
// xabarlar, adminka buyurtmalari, yetkazma va muammoli buyurtmalar.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sahiy/support"
)

// loadEnv .env faylini o'qib muhit o'zgaruvchilariga qo'yadi.
// Allaqachon o'rnatilgan qiymat ustidan yozilmaydi.
func loadEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		os.Setenv(k, strings.Trim(strings.TrimSpace(v), `"'`))
	}
}

// writeRaw tashqi API'dan kelgan tayyor JSON'ni o'zgartirmasdan uzatadi.
func writeRaw(w http.ResponseWriter, out []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// requireMethod ruxsat etilmagan metodda 405 qaytaradi.
func requireMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, m := range allowed {
		if r.Method == m {
			return true
		}
	}
	writeErr(w, http.StatusMethodNotAllowed, "faqat "+strings.Join(allowed, " yoki "))
	return false
}

// withSupportToken support tizimiga so'rov yuboradi. Token eskirgan bo'lsa
// (ErrUnauthorized) bir marta yangilab qayta uriniladi.
func withSupportToken(fn func(baseURL, token string) ([]byte, error)) ([]byte, error) {
	creds := support.CredentialsFromEnv()
	token, err := support.Token(creds, support.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	out, err := fn(creds.BaseURL, token)
	if errors.Is(err, support.ErrUnauthorized) {
		if token, err = support.Refresh(creds, support.TokenFile); err == nil {
			out, err = fn(creds.BaseURL, token)
		}
	}
	return out, err
}

// withServiceToken yetkazma (service) API'siga so'rov yuboradi — token
// eskirsa bir marta yangilab qayta uriniladi.
//
// Adminka 401 i bu yerga tushmaydi: u support.ErrAdminkaUnauthorized bo'lib
// keladi (.env qo'lda yangilanadi), ya'ni behuda takrorlanmaydi.
func withServiceToken(fn func(svc support.Service, token string) ([]byte, error)) ([]byte, error) {
	svc := support.ServiceFromEnv()
	token, err := support.ServiceToken(svc, support.ServiceTokenFile)
	if err != nil {
		return nil, fmt.Errorf("service login: %w", err)
	}
	out, err := fn(svc, token)
	if errors.Is(err, support.ErrUnauthorized) {
		if token, err = support.ServiceRefresh(svc, support.ServiceTokenFile); err == nil {
			out, err = fn(svc, token)
		}
	}
	return out, err
}

// decodeBody bo'sh bo'lmagan body'ni JSON deb o'qiydi.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("body JSON emas: %v", err))
		return false
	}
	return true
}

// chatsHandler: bizga POST kelsa, orqada support serveriga POST yuborib
// suhbatlarni qaytaradi. Body: {"client_id":8198749,"page":1,"limit":10}
// client_id berilsa faqat o'sha mijozning suhbatlari keladi.
func chatsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var f support.ChatFilter
	if !decodeBody(w, r, &f) {
		return
	}

	out, err := withSupportToken(func(baseURL, token string) ([]byte, error) {
		return support.ChatsJSON(baseURL, token, f)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// messagesHandler: GET /api/messages?conversation_id=54030&limit=10
// Suhbatning oxirgi xabarlarini (agent ham, client ham) qaytaradi.
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
	if err != nil || convID <= 0 {
		writeErr(w, http.StatusBadRequest, "conversation_id berilmagan")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = support.DefaultMessageLimit
	}

	out, err := withSupportToken(func(baseURL, token string) ([]byte, error) {
		return support.MessagesJSON(baseURL, token, convID, limit)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// ordersHandler: GET /api/orders?user_id=7988331&order_sn=DG..&express_num=..&page=1&size=10
// Adminka (daigou) buyurtmalarini qaytaradi — har buyurtmada 17 ta maydon.
func ordersHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	q := r.URL.Query()
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)
	f := support.OrderFilter{
		UserID:     userID,
		OrderSN:    q.Get("order_sn"),
		ExpressNum: q.Get("express_num"),
		Status:     q.Get("status"),
		Keyword:    q.Get("keyword"),
	}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Size, _ = strconv.Atoi(q.Get("size"))

	if f.UserID == 0 && f.OrderSN == "" && f.ExpressNum == "" && f.Keyword == "" {
		writeErr(w, http.StatusBadRequest, "user_id, order_sn yoki express_num berilmagan")
		return
	}

	out, err := support.OrdersJSON(support.AdminkaFromEnv(), f)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// dashboardHandler: GET /api/dashboard?user_id=8231476  yoki  ?track=YT75...
// Yetkazma buyurtmalarini qaytaradi — har buyurtmada 8 ta maydon.
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	q := r.URL.Query()
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)
	track := q.Get("track")
	if track == "" {
		track = q.Get("track_number")
	}
	if userID == 0 && track == "" {
		writeErr(w, http.StatusBadRequest, "user_id yoki track berilmagan")
		return
	}
	f := support.DeliveryFilter{TrackNumber: track, UserID: userID}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Size, _ = strconv.Atoi(q.Get("size"))

	out, err := withServiceToken(func(svc support.Service, token string) ([]byte, error) {
		return support.DeliveryJSON(svc, token, f)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// problemHandler: GET yoki POST /api/problem
// Adminkadagi (to'langan) buyurtmalarni dashboarddagi (kelgan) yetkazmalar
// bilan trek raqami orqali solishtirib, kelmaganlarini qaytaradi.
// GET  /api/problem?user_id=7903808  |  ?order_sn=DG..  |  ?express_num=..
// POST /api/problem  {"user_id":7903808,"size":50}
func problemHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}

	var f support.ProblemFilter
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		f.UserID, _ = strconv.ParseInt(q.Get("user_id"), 10, 64)
		f.OrderSN = q.Get("order_sn")
		// dashboard endpointidagi kabi track/track_number ham qabul qilinadi.
		f.ExpressNum = firstNonEmpty(q.Get("express_num"), q.Get("track"), q.Get("track_number"))
		f.Size, _ = strconv.Atoi(q.Get("size"))
		f.MaxPages, _ = strconv.Atoi(q.Get("max_pages"))
	} else if !decodeBody(w, r, &f) {
		return
	}

	if f.UserID == 0 && f.OrderSN == "" && f.ExpressNum == "" {
		writeErr(w, http.StatusBadRequest, "user_id, order_sn yoki express_num berilmagan")
		return
	}

	adm := support.AdminkaFromEnv()
	out, err := withServiceToken(func(svc support.Service, token string) ([]byte, error) {
		return support.ProblemJSON(adm, svc, token, f)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// orderHandler: GET yoki POST /api/order
// Bitta buyurtmani DG raqami yoki trek raqami bo'yicha topadi va adminka +
// dashboard ma'lumotini birga qaytaradi. Trek bo'lmasa `dashboard: false`.
// GET  /api/order?q=DG60597226  |  ?order_sn=DG..  |  ?express_num=..
// POST /api/order  {"q":"DG60597226"}
func orderHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}

	var query string
	// dashboard endpointidagi kabi track/track_number ham qabul qilinadi.
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		query = firstNonEmpty(q.Get("q"), q.Get("order_sn"), q.Get("express_num"),
			q.Get("track"), q.Get("track_number"))
	} else {
		var body struct {
			Query       string `json:"q"`
			OrderSN     string `json:"order_sn"`
			ExpressNum  string `json:"express_num"`
			Track       string `json:"track"`
			TrackNumber string `json:"track_number"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		query = firstNonEmpty(body.Query, body.OrderSN, body.ExpressNum,
			body.Track, body.TrackNumber)
	}

	if query == "" {
		writeErr(w, http.StatusBadRequest, "order_sn yoki express_num berilmagan")
		return
	}

	adm := support.AdminkaFromEnv()
	out, err := withServiceToken(func(svc support.Service, token string) ([]byte, error) {
		return support.OrderCardJSON(adm, svc, token, query)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRaw(w, out)
}

// firstNonEmpty - bir necha nomdan kelgan qiymatlardan birinchi bo'sh
// bo'lmaganini qaytaradi.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	loadEnv(".env")

	addr := os.Getenv("WEB_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Baza + admin user (login: 991134543 / parol: 991134543) + sozlamalar.
	if _, err := support.InitDB(); err != nil {
		log.Fatal("baza: ", err)
	}

	mux := http.NewServeMux()

	// Tashqi API proksisi (mavjud endpointlar).
	mux.HandleFunc("/api/chats", chatsHandler)
	mux.HandleFunc("/api/messages", messagesHandler)
	mux.HandleFunc("/api/orders", ordersHandler)
	mux.HandleFunc("/api/dashboard", dashboardHandler)
	mux.HandleFunc("/api/problem", problemHandler)
	mux.HandleFunc("/api/order", orderHandler)

	// Autentifikatsiya
	mux.HandleFunc("POST /api/auth/login", loginHandler)
	mux.HandleFunc("GET /api/auth/me", meHandler)

	// Promt CRUD
	mux.HandleFunc("GET /api/promts", support.RequireAuth(promtListHandler))
	mux.HandleFunc("POST /api/promts", support.RequireAuth(promtCreateHandler))
	mux.HandleFunc("GET /api/promts/{id}", support.RequireAuth(promtGetHandler))
	mux.HandleFunc("PUT /api/promts/{id}", support.RequireAuth(promtUpdateHandler))
	mux.HandleFunc("DELETE /api/promts/{id}", support.RequireAuth(promtDeleteHandler))

	// Statistika
	mux.HandleFunc("GET /api/stats", support.RequireAuth(statsHandler))
	mux.HandleFunc("GET /api/stats/daily", support.RequireAuth(dailyStatsHandler))
	mux.HandleFunc("GET /api/stats/clients", support.RequireAuth(clientStatsHandler))

	// AI javoblari va tasdiqlash navbati
	mux.HandleFunc("GET /api/interactions", support.RequireAuth(interactionsHandler))
	mux.HandleFunc("GET /api/interactions/{id}", support.RequireAuth(interactionGetHandler))
	mux.HandleFunc("PATCH /api/interactions/{id}", support.RequireAuth(interactionPatchHandler))
	mux.HandleFunc("POST /api/interactions/{id}/approve", support.RequireAuth(approveHandler))
	mux.HandleFunc("POST /api/interactions/{id}/reject", support.RequireAuth(rejectHandler))

	// Muammoli buyurtmalar
	mux.HandleFunc("GET /api/issues", support.RequireAuth(issuesHandler))
	mux.HandleFunc("GET /api/issues/{id}", support.RequireAuth(issueGetHandler))
	mux.HandleFunc("POST /api/issues/{id}/resolve", support.RequireAuth(issueResolveHandler))
	mux.HandleFunc("POST /api/issues/review", support.RequireAuth(issuesReviewHandler))
	mux.HandleFunc("GET /api/stats/issues/daily", support.RequireAuth(issuesDailyHandler))

	// Sozlamalar (avto-javob tugmasi) va qo'lda ishga tushirish
	mux.HandleFunc("GET /api/settings", support.RequireAuth(settingsHandler))
	mux.HandleFunc("PUT /api/settings", support.RequireAuth(settingsUpdateHandler))
	mux.HandleFunc("POST /api/agent/run", support.RequireAuth(agentRunHandler))
	mux.HandleFunc("POST /api/agent/scan", support.RequireAuth(agentScanHandler))

	// Avtomatik hujjat (FastAPI'dagi /docs kabi).
	mux.HandleFunc("/openapi.json", openapiHandler)
	mux.HandleFunc("/redoc", redocHandler)
	mux.HandleFunc("/docs", docsHandler)
	mux.HandleFunc("/", docsHandler) // "/" → hujjat, qolgani → 404

	// Fon sikli: yangi mijoz xabarlarini kuzatadi (panel orqali o'chiriladi).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	support.StartPoller(ctx)

	// Telegram guruhidagi javoblar (muammo yechimlari) — agent
	// o'chirilgan bo'lsa ham o'qiladi.
	support.StartTelegramPoller(ctx)

	srv := &http.Server{Addr: addr, Handler: withCORS(mux)}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	log.Println("tinglanmoqda:", addr,
		"— hujjat: http://localhost"+addr+"/docs |",
		"POST /api/auth/login, CRUD /api/promts,",
		"GET /api/stats, /api/interactions, PUT /api/settings,",
		"POST /api/agent/run")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Println("to'xtadi")
}
