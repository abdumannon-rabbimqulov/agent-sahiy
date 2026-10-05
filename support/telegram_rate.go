// Telegramga yuborishni TEZLIGINI cheklash va 429 da qayta urinish.
//
// Telegram bitta guruhga sekundiga ~1 (daqiqasiga ~20) xabardan ko'pini
// qabul qilmaydi. Eslatma to'lqinida (support/issue_detect.go) bot
// o'nlab xabarni ketma-ket otardi va Telegram ularni "429 Too Many
// Requests" bilan RAD ETARDI — ya'ni xodimlar o'sha eslatmalarni
// umuman ko'rmasdi, logda esa faqat "eslatmasi ketmadi" qolardi.
//
// Shuning uchun hamma yuborish shu yerdan, BITTA navbat orqali o'tadi:
// xabarlar orasida eng kam tanaffus saqlanadi, 429 kelsa Telegram
// aytgan `retry_after` kutiladi va xabar qayta yuboriladi.
package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// DefaultTelegramGapMS - ketma-ket ikki xabar orasidagi eng kam
	// tanaffus. 3000ms ≈ daqiqasiga 20 xabar — Telegram guruh limiti.
	DefaultTelegramGapMS = 3000
	// DefaultTelegramRetries - 429 dan keyin necha marta qayta urinish.
	DefaultTelegramRetries = 3
	// maxRetryAfter - Telegram juda uzoq kutishni aytsa ham shundan
	// ortiq kutilmaydi: sikl butunlay qotib qolmasin.
	maxRetryAfter = 60 * time.Second
)

// TelegramGap - .env dagi TELEGRAM_MIN_GAP_MS.
func TelegramGap() time.Duration {
	return time.Duration(envInt("TELEGRAM_MIN_GAP_MS", DefaultTelegramGapMS)) * time.Millisecond
}

// TelegramRetries - .env dagi TELEGRAM_MAX_RETRY.
func TelegramRetries() int { return envInt("TELEGRAM_MAX_RETRY", DefaultTelegramRetries) }

// tgQueue - yuborish navbati. Butun jarayonda bitta: hamma goroutine
// shu qulf orqali o'tadi, shuning uchun Telegramga bir vaqtda ikkita
// so'rov ketmaydi.
var tgQueue struct {
	mu   sync.Mutex
	last time.Time // oxirgi so'rov qachon ketgani
}

// telegramPost - Bot API ga bitta so'rov: navbat, tanaffus va 429 da
// qayta urinish shu yerda. Javob tanasi (xom JSON) qaytariladi.
//
// Qulf butun urinishlar davomida ushlab turiladi — 429 "butun guruhga"
// tegishli, shuning uchun kutish paytida boshqa goroutine xabar
// yuborsa limit yana buzilardi.
func telegramPost(method string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/bot%s/%s", TelegramAPI(), os.Getenv("TELEGRAM_BOT_TOKEN"), method)

	tgQueue.mu.Lock()
	defer tgQueue.mu.Unlock()

	retries := TelegramRetries()
	for attempt := 0; ; attempt++ {
		// Oldingi so'rovdan keyin yetarli vaqt o'tmagan bo'lsa kutamiz.
		if gap := TelegramGap(); gap > 0 {
			if wait := gap - time.Since(tgQueue.last); wait > 0 {
				time.Sleep(wait)
			}
		}

		resp, err := (&http.Client{Timeout: 20 * time.Second}).
			Post(url, "application/json", bytes.NewReader(body))
		tgQueue.last = time.Now()
		if err != nil {
			return nil, fmt.Errorf("telegram: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return raw, nil
		}
		// 429 — limit. Telegram qancha kutishni o'zi aytadi.
		if resp.StatusCode == http.StatusTooManyRequests && attempt < retries {
			wait := tgRetryAfter(raw)
			log.Printf("telegram: limit (429) — %s kutilib qayta urinamiz (%d/%d)",
				wait, attempt+1, retries)
			time.Sleep(wait)
			continue
		}
		return nil, fmt.Errorf("telegram (status %d): %s", resp.StatusCode, snippet(raw))
	}
}

// tgRetryAfter - 429 javobidagi `parameters.retry_after` (soniya).
// O'qilmasa yoki juda katta bo'lsa oqilona qiymatga keltiriladi.
func tgRetryAfter(raw []byte) time.Duration {
	var out struct {
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(raw, &out)
	d := time.Duration(out.Parameters.RetryAfter) * time.Second
	if d <= 0 {
		d = TelegramGap()
	}
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}
