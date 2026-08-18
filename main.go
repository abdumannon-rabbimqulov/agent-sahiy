package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

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

// chatsHandler: bizga POST kelsa, orqada support serveriga POST yuborib
// suhbatlarni qaytaradi. Body: {"client_id":8198749,"page":1,"limit":10}
// client_id berilsa faqat o'sha mijozning suhbatlari keladi.
func chatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"faqat POST"}`, http.StatusMethodNotAllowed)
		return
	}

	var f support.ChatFilter
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"body JSON emas: %v"}`, err), http.StatusBadRequest)
			return
		}
	}

	creds := support.CredentialsFromEnv()
	token, err := support.Token(creds, support.TokenFile)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"login: %v"}`, err), http.StatusBadGateway)
		return
	}

	out, err := support.ChatsJSON(creds.BaseURL, token, f)
	if errors.Is(err, support.ErrUnauthorized) {
		// Token eskirgan — yangilab bir marta qayta urinamiz.
		if token, err = support.Refresh(creds, support.TokenFile); err == nil {
			out, err = support.ChatsJSON(creds.BaseURL, token, f)
		}
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// messagesHandler: GET /api/messages?conversation_id=54030&limit=10
// Suhbatning oxirgi xabarlarini (agent ham, client ham) qaytaradi.
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"faqat GET"}`, http.StatusMethodNotAllowed)
		return
	}

	convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
	if err != nil || convID <= 0 {
		http.Error(w, `{"error":"conversation_id berilmagan"}`, http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = support.DefaultMessageLimit
	}

	creds := support.CredentialsFromEnv()
	token, err := support.Token(creds, support.TokenFile)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"login: %v"}`, err), http.StatusBadGateway)
		return
	}

	out, err := support.MessagesJSON(creds.BaseURL, token, convID, limit)
	if errors.Is(err, support.ErrUnauthorized) {
		// Token eskirgan — yangilab bir marta qayta urinamiz.
		if token, err = support.Refresh(creds, support.TokenFile); err == nil {
			out, err = support.MessagesJSON(creds.BaseURL, token, convID, limit)
		}
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// ordersHandler: GET /api/orders?user_id=7988331&order_sn=DG..&express_num=..&page=1&size=10
// Adminka (daigou) buyurtmalarini qaytaradi — har buyurtmada 17 ta maydon.
func ordersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"faqat GET"}`, http.StatusMethodNotAllowed)
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
		http.Error(w, `{"error":"user_id, order_sn yoki express_num berilmagan"}`, http.StatusBadRequest)
		return
	}

	out, err := support.OrdersJSON(support.AdminkaFromEnv(), f)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// dashboardHandler: GET /api/dashboard?user_id=8231476  yoki  ?track=YT75...
// Yetkazma buyurtmalarini qaytaradi — har buyurtmada 8 ta maydon.
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"faqat GET"}`, http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)
	track := q.Get("track")
	if track == "" {
		track = q.Get("track_number")
	}
	if userID == 0 && track == "" {
		http.Error(w, `{"error":"user_id yoki track berilmagan"}`, http.StatusBadRequest)
		return
	}
	f := support.DeliveryFilter{TrackNumber: track, UserID: userID}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Size, _ = strconv.Atoi(q.Get("size"))

	svc := support.ServiceFromEnv()
	token, err := support.ServiceToken(svc, support.ServiceTokenFile)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"service login: %v"}`, err), http.StatusBadGateway)
		return
	}

	out, err := support.DeliveryJSON(svc, token, f)
	if errors.Is(err, support.ErrUnauthorized) {
		// Token eskirgan — yangilab bir marta qayta urinamiz.
		if token, err = support.ServiceRefresh(svc, support.ServiceTokenFile); err == nil {
			out, err = support.DeliveryJSON(svc, token, f)
		}
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func main() {
	loadEnv(".env")

	addr := os.Getenv("WEB_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	http.HandleFunc("/api/chats", chatsHandler)
	http.HandleFunc("/api/messages", messagesHandler)
	http.HandleFunc("/api/orders", ordersHandler)
	http.HandleFunc("/api/dashboard", dashboardHandler)
	log.Println("tinglanmoqda:", addr,
		"— POST /api/chats, GET /api/messages?conversation_id=..,",
		"GET /api/orders?user_id=.., GET /api/dashboard?user_id=..")
	log.Fatal(http.ListenAndServe(addr, nil))
}
