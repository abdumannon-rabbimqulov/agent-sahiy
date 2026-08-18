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

func main() {
	loadEnv(".env")

	addr := os.Getenv("WEB_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	http.HandleFunc("/api/chats", chatsHandler)
	http.HandleFunc("/api/messages", messagesHandler)
	log.Println("tinglanmoqda:", addr, "— POST /api/chats, GET /api/messages?conversation_id=..")
	log.Fatal(http.ListenAndServe(addr, nil))
}
