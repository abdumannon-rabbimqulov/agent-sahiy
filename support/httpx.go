// Tashqi API'lar bilan ishlashda vaqtinchalik uzilishlarga chidamlilik.
package support

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultRetries - vaqtinchalik xatolarda nechta qo'shimcha urinish.
const DefaultRetries = 2

// retryDelay - urinishlar orasidagi tanaffus (har safar ikkilanadi).
// .env: HTTP_RETRY_DELAY_MS (default 2000).
func retryDelay() time.Duration {
	return time.Duration(envInt("HTTP_RETRY_DELAY_MS", 2000)) * time.Millisecond
}

// temporaryStatus - qayta urinishga arziydigan javoblar.
//
// 429 — tezlik chegarasi, 5xx — server tomoni. 522/524 Cloudflare'niki:
// origin javob bermayapti, odatda bir necha soniyadan keyin tiklanadi.
func temporaryStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// doWithRetry so'rovni yuboradi va vaqtinchalik xatoda qayta uriniladi.
// So'rov har urinishda qaytadan yasaladi (body qayta o'qilishi uchun).
//
// Javob tanasi to'liq o'qib qaytariladi — chaqiruvchi Body ni yopishi
// shart emas.
func doWithRetry(client *http.Client, newReq func() (*http.Request, error), attempts int) (int, []byte, error) {
	if attempts < 1 {
		attempts = 1
	}
	var (
		status int
		body   []byte
		err    error
	)
	delay := retryDelay()

	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(delay)
			delay *= 2
		}
		if delay > maxRetryDelay() {
			delay = maxRetryDelay()
		}

		var req *http.Request
		if req, err = newReq(); err != nil {
			return 0, nil, err
		}

		var resp *http.Response
		resp, err = client.Do(req)
		if err != nil {
			continue // tarmoq uzildi — qayta urinamiz
		}

		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		status = resp.StatusCode

		if !temporaryStatus(status) {
			return status, body, nil
		}
		err = nil // status orqali qaytariladi

		// Server o'zi "shuncha kutib turing" desa (Groq 429 da beradi),
		// o'sha vaqtga amal qilamiz — bizning taxminimizdan aniqroq.
		if d, ok := retryAfter(resp); ok && d > delay {
			delay = d
			if delay > maxRetryDelay() {
				delay = maxRetryDelay()
			}
		}
	}
	return status, body, err
}

// maxRetryDelay - kutishning yuqori chegarasi (.env: HTTP_RETRY_MAX_MS).
// Groq ba'zan "60 soniyadan keyin" deydi — bunday paytda kutgandan ko'ra
// murojaatni keyingi siklga qoldirgan ma'qul.
func maxRetryDelay() time.Duration {
	return time.Duration(envInt("HTTP_RETRY_MAX_MS", 20000)) * time.Millisecond
}

// retryAfter - javobdagi Retry-After sarlavhasi (soniyalarda yoki sana).
func retryAfter(resp *http.Response) (time.Duration, bool) {
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
	}
	return 0, false
}

// Retries - .env dagi HTTP_RETRIES (default 2).
func Retries() int { return envInt("HTTP_RETRIES", DefaultRetries) + 1 }
