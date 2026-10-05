package support

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Birinchi N so'rovga 429, keyin 200 qaytaradigan soxta Bot API.
func TestTelegramRetriesOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":777}}`)
	}))
	defer srv.Close()

	os.Setenv("TELEGRAM_API_URL", srv.URL)
	os.Setenv("TELEGRAM_BOT_TOKEN", "x")
	os.Setenv("TELEGRAM_GROUP_ID", "-100")
	os.Setenv("TELEGRAM_MIN_GAP_MS", "10") // testni tezlashtirish uchun

	start := time.Now()
	id, err := SendTelegramMessage("salom", 0)
	if err != nil {
		t.Fatalf("429 dan keyin ham ketmadi: %v", err)
	}
	if id != 777 {
		t.Fatalf("message_id 777 kutilgandi, keldi: %d", id)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("3 urinish kutilgandi, bo'ldi: %d", got)
	}
	// Ikki marta 1 soniyadan kutilishi kerak edi.
	if d := time.Since(start); d < 2*time.Second {
		t.Fatalf("retry_after kutilmadi (%s)", d)
	}
	t.Logf("3 urinish, %s da ketdi", time.Since(start).Round(time.Millisecond))
}

// Tanaffus saqlanayotganini tekshiradi.
func TestTelegramKeepsGap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer srv.Close()

	os.Setenv("TELEGRAM_API_URL", srv.URL)
	os.Setenv("TELEGRAM_BOT_TOKEN", "x")
	os.Setenv("TELEGRAM_GROUP_ID", "-100")
	os.Setenv("TELEGRAM_MIN_GAP_MS", "300")

	SendTelegramMessage("1", 0) // navbatni boshlab olamiz
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := SendTelegramMessage("x", 0); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 900*time.Millisecond {
		t.Fatalf("3 xabar uchun kamida 900ms kutilgandi, bo'ldi: %s", d)
	}
	t.Logf("3 xabar %s da ketdi", time.Since(start).Round(time.Millisecond))
}
