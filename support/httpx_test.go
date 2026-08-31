package support

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryOnTemporary - 522/429 dan keyin qayta urinib, muvaffaqiyatga
// erishishini tekshiradi (Sahiy Cloudflare 522, Groq 429 holatlari).
func TestRetryOnTemporary(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "2")
	t.Setenv("HTTP_RETRY_DELAY_MS", "1")

	for _, code := range []int{429, 500, 522, 524} {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) < 3 {
				w.WriteHeader(code)
				fmt.Fprint(w, "vaqtinchalik")
				return
			}
			w.WriteHeader(200)
			fmt.Fprint(w, `{"ok":true}`)
		}))

		newReq := func() (*http.Request, error) { return http.NewRequest(http.MethodGet, srv.URL, nil) }
		status, body, err := doWithRetry(srv.Client(), newReq, Retries())
		srv.Close()

		if err != nil || status != 200 {
			t.Errorf("status %d: %d keldi (%v)", code, status, err)
		}
		if string(body) != `{"ok":true}` {
			t.Errorf("status %d: body %q", code, body)
		}
		if calls != 3 {
			t.Errorf("status %d: 3 urinish kutilgan, %d bo'ldi", code, calls)
		}
	}
}

// TestNoRetryOnClientError - 4xx (401 kabi) qayta urinilmaydi: token
// yaroqsiz bo'lsa takrorlash bekorga vaqt.
func TestNoRetryOnClientError(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "2")
	t.Setenv("HTTP_RETRY_DELAY_MS", "1")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(401)
	}))
	defer srv.Close()

	newReq := func() (*http.Request, error) { return http.NewRequest(http.MethodGet, srv.URL, nil) }
	status, _, _ := doWithRetry(srv.Client(), newReq, Retries())
	if status != 401 {
		t.Errorf("401 kutilgan: %d", status)
	}
	if calls != 1 {
		t.Errorf("bitta urinish kutilgan, %d bo'ldi", calls)
	}
}

// TestGroqRetriesRateLimit - Groq klienti 429 dan keyin javob olishini
// va tokenlarni to'g'ri o'qishini tekshiradi.
func TestGroqRetriesRateLimit(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "1")
	t.Setenv("HTTP_RETRY_DELAY_MS", "1")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"message":"Rate limit reached"}}`)
			return
		}
		fmt.Fprint(w, `{"model":"test","choices":[{"message":{"content":"{\"chat\":\"salom\"}"}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer srv.Close()

	g := Groq{BaseURL: srv.URL, APIKey: "test", Model: "test", MaxTokens: 100}
	out, u, err := g.Generate(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if calls != 2 {
		t.Errorf("2 urinish kutilgan, %d bo'ldi", calls)
	}
	if u.PromptTokens != 10 || u.CompletionTokens != 5 {
		t.Errorf("tokenlar: %+v", u)
	}
	a, err := ParseAgentJSON(out)
	if err != nil || a.Chat != "salom" {
		t.Errorf("javob o'qilmadi: %q (%v)", out, err)
	}
}

// TestAdminkaRetriesCloudflare - adminka 522 qaytarsa qayta uriniladi.
func TestAdminkaRetriesCloudflare(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "1")
	t.Setenv("HTTP_RETRY_DELAY_MS", "1")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(522)
			fmt.Fprint(w, `{"error_code":522,"title":"Error 522: Connection timed out"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"data":[{"order_sn":"DG1","status":3,"created_at":"2026-08-01 10:00:00"}]}}`)
	}))
	defer srv.Close()

	rows, err := FetchOrders(Adminka{BaseURL: srv.URL, Token: "test"}, OrderFilter{Size: 5})
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if calls != 2 {
		t.Errorf("2 urinish kutilgan, %d bo'ldi", calls)
	}
	if len(rows) != 1 || rows[0].OrderSN != "DG1" {
		t.Errorf("buyurtma o'qilmadi: %+v", rows)
	}
}

// TestPickOrderPayFields - adminka javobidan pay_status va paid_at
// o'qilishini tekshiradi (maydon ustki darajada ham, ichma-ich ham
// kelishi mumkin).
func TestPickOrderPayFields(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "0")

	body := `{"data":{"data":[
	  {"order_sn":"DG1","status":3,"pay_status":1,"paid_at":"2026-08-21 10:00:00",
	   "created_at":"2026-08-20 09:00:00"},
	  {"order_sn":"DG2","status":4,"pay_status":0,"created_at":"2026-08-25 09:00:00"},
	  {"order_sn":"DG3","status":3,"order":{"pay_status":1,"paid_at":"2026-08-22 11:30:00"},
	   "created_at":"2026-08-22 09:00:00"}
	]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	rows, err := FetchOrders(Adminka{BaseURL: srv.URL, Token: "test"}, OrderFilter{Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("3 ta buyurtma kutilgan: %d", len(rows))
	}

	if rows[0].PayStatus != 1 || rows[0].PaidAt != "2026-08-21 10:00:00" {
		t.Errorf("DG1: pay_status=%d paid_at=%q", rows[0].PayStatus, rows[0].PaidAt)
	}
	if rows[1].PayStatus != 0 || rows[1].PaidAt != "" {
		t.Errorf("DG2 to'lanmagan bo'lishi kerak: pay_status=%d paid_at=%q", rows[1].PayStatus, rows[1].PaidAt)
	}
	// Ichma-ich yo'l ham o'qiladi.
	if rows[2].PayStatus != 1 || rows[2].PaidAt != "2026-08-22 11:30:00" {
		t.Errorf("DG3 (order.*): pay_status=%d paid_at=%q", rows[2].PayStatus, rows[2].PaidAt)
	}

	// To'lanmagan buyurtma muammoli emas.
	if IsProblem(rows[1]) {
		t.Error("to'lanmagan buyurtma muammoli deb belgilandi")
	}
}

// TestRetryAfterHeader - server "shuncha kutib turing" desa, o'sha
// vaqtga amal qilinadi; juda uzun bo'lsa chegaraga kesiladi.
func TestRetryAfterHeader(t *testing.T) {
	t.Setenv("HTTP_RETRIES", "1")
	t.Setenv("HTTP_RETRY_DELAY_MS", "1")
	t.Setenv("HTTP_RETRY_MAX_MS", "150") // chegara: uzun kutish kesiladi

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "60") // Groq shunday qaytaradi
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	start := time.Now()
	newReq := func() (*http.Request, error) { return http.NewRequest(http.MethodGet, srv.URL, nil) }
	status, body, err := doWithRetry(srv.Client(), newReq, Retries())
	elapsed := time.Since(start)

	if err != nil || status != 200 || string(body) != "ok" {
		t.Fatalf("status %d, body %q, err %v", status, body, err)
	}
	// 60 soniya emas, chegaradagi 150 ms atrofida kutilishi kerak.
	if elapsed > 2*time.Second {
		t.Errorf("juda uzoq kutdi: %s", elapsed)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("Retry-After e'tiborga olinmadi: %s", elapsed)
	}
}
