// Tashqi tizimlar holati: har bir manba (adminka, support, yetkazma,
// Telegram, model) ishlayaptimi va tokeni qachon tugaydi.
//
// Nega kerak: adminka tokeni .env dan QO'LDA olinadi va avtomatik
// yangilanmaydi. U tugasa agent ishlashda davom etadi, lekin buyurtma
// ma'lumotini ko'rmay qoladi: javoblarda "adminka_error: token rad etildi
// (401)" chiqadi, mijoz turi "noma'lum" bo'lib qoladi va muammoli
// buyurtmalar topilmaydi. Bu jimgina sodir bo'ladi — shuning uchun holat
// panelga chiqariladi (GET /api/health).
package support

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Holat darajalari.
const (
	HealthOK   = "ok"   // ishlayapti
	HealthWarn = "warn" // ishlayapti, lekin e'tibor kerak (token tugash arafasida)
	HealthFail = "fail" // ishlamayapti
)

// TokenWarnDays - token tugashiga shundan kam kun qolsa "warn".
const TokenWarnDays = 7

// healthTTL - natija shu muddat keshlanadi: panel har ochilganda
// tashqi API'larga 5 ta so'rov ketmasin.
const healthTTL = 60 * time.Second

// SystemCheck - bitta tizimning holati.
type SystemCheck struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	// Message - odam o'qiydigan izoh: nima bo'lgani va nima qilish kerak.
	Message string `json:"message"`
	// Affects - bu tizim ishlamasa nima ishlamaydi (panelda ko'rsatiladi).
	Affects string `json:"affects,omitempty"`
	// ExpiresAt / DaysLeft - token muddati (faqat qo'lda yangilanadigan
	// tokenlar uchun to'ldiriladi).
	ExpiresAt string   `json:"expires_at,omitempty"`
	DaysLeft  *float64 `json:"days_left,omitempty"`
	// Manual - token qo'lda yangilanadi (avtomatik login yo'q).
	Manual bool `json:"manual,omitempty"`
}

// HealthReport - barcha tizimlar + umumiy holat.
type HealthReport struct {
	// Status - eng yomon holat: bittasi fail bo'lsa "fail".
	Status    string        `json:"status"`
	CheckedAt time.Time     `json:"checked_at"`
	Systems   []SystemCheck `json:"systems"`
}

var healthCache = struct {
	sync.Mutex
	val HealthReport
	at  time.Time
}{}

// Health tizimlar holatini qaytaradi. Natija healthTTL davomida
// keshlanadi; force=true bo'lsa kesh chetlab o'tiladi.
func Health(force bool) HealthReport {
	healthCache.Lock()
	defer healthCache.Unlock()

	if !force && !healthCache.at.IsZero() && time.Since(healthCache.at) < healthTTL {
		return healthCache.val
	}

	checks := []func() SystemCheck{
		checkAdminka, checkSupportAPI, checkDelivery, checkTelegram, checkModel,
	}
	out := make([]SystemCheck, len(checks))
	var wg sync.WaitGroup
	for i, fn := range checks {
		wg.Add(1)
		go func(i int, fn func() SystemCheck) {
			defer wg.Done()
			out[i] = fn()
		}(i, fn)
	}
	wg.Wait()

	rep := HealthReport{Status: HealthOK, CheckedAt: time.Now(), Systems: out}
	for _, c := range out {
		if c.Status == HealthFail {
			rep.Status = HealthFail
			break
		}
		if c.Status == HealthWarn {
			rep.Status = HealthWarn
		}
	}

	healthCache.val = rep
	healthCache.at = time.Now()
	return rep
}

// checkAdminka - eng muhim tekshiruv: token .env dan qo'lda olinadi,
// o'zi yangilanmaydi. Ikki narsa ko'riladi — JWT ichidagi muddat va
// haqiqiy so'rov (token muddati tugamasdan ham bekor qilingan bo'lishi
// mumkin).
func checkAdminka() SystemCheck {
	c := SystemCheck{
		Key: "adminka", Name: "Adminka (Xitoy buyurtmalari)", Manual: true,
		Affects: "buyurtma holati, mijoz turi, muammoli buyurtmalar",
	}

	a := AdminkaFromEnv()
	if a.Token == "" {
		c.Status = HealthFail
		c.Message = ".env dagi ADMINKA_TOKEN_BEARER bo'sh"
		return c
	}

	// Muddat tokenning o'zidan o'qiladi — so'rov yubormasdan ham
	// "necha kun qoldi" ma'lum bo'ladi.
	if exp, ok := jwtExpiry(a.Token); ok {
		days := time.Until(exp).Hours() / 24
		c.ExpiresAt = exp.Format(time.RFC3339)
		c.DaysLeft = &days
		if days <= 0 {
			c.Status = HealthFail
			c.Message = fmt.Sprintf("token muddati tugagan (%s) — .env dagi ADMINKA_TOKEN_BEARER ni yangilang",
				exp.Format("2006-01-02 15:04"))
			return c
		}
	}

	// Tirik so'rov: muddati bor token ham bekor qilingan bo'lishi mumkin.
	if _, err := FetchOrders(a, OrderFilter{Page: 1, Size: 1}); err != nil {
		c.Status = HealthFail
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrAdminkaUnauthorized) {
			c.Message = "token rad etildi (401) — .env dagi ADMINKA_TOKEN_BEARER ni yangilab, api konteynerini qayta ishga tushiring"
		} else {
			c.Message = err.Error()
		}
		return c
	}

	c.Status = HealthOK
	c.Message = "ishlayapti"
	if c.DaysLeft != nil {
		days := *c.DaysLeft
		c.Message = fmt.Sprintf("token %.0f kundan keyin tugaydi", days)
		if days < TokenWarnDays {
			c.Status = HealthWarn
			c.Message += " — yangilashni rejalashtiring"
		}
	}
	return c
}

// checkSupportAPI - suhbatlar manbasi. Token avtomatik yangilanadi
// (login .env dagi LOGIN/PASSWORD bilan), shuning uchun muddat emas,
// faqat ishlashi tekshiriladi.
func checkSupportAPI() SystemCheck {
	c := SystemCheck{
		Key: "support", Name: "Support API (suhbatlar)",
		Affects: "suhbatlarni o'qish va javob yuborish",
	}
	_, err := withToken(func(baseURL, token string) ([]Chat, error) {
		return FetchChats(baseURL, token, ChatFilter{Page: 1, Limit: 1})
	})
	if err != nil {
		c.Status = HealthFail
		c.Message = err.Error()
		return c
	}
	c.Status = HealthOK
	c.Message = "ishlayapti (token avtomatik yangilanadi)"
	return c
}

// checkDelivery - yetkazma (service) API'si. Bu ham o'zi login qiladi.
func checkDelivery() SystemCheck {
	c := SystemCheck{
		Key: "delivery", Name: "Yetkazma API (posilkalar)",
		Affects: "posilka qayerda, filialga keldimi",
	}
	svc := ServiceFromEnv()
	if _, err := ServiceToken(svc, ServiceTokenFile); err != nil {
		c.Status = HealthFail
		c.Message = err.Error()
		return c
	}
	c.Status = HealthOK
	c.Message = "ishlayapti (token avtomatik yangilanadi)"
	return c
}

// checkTelegram - xodimlar guruhiga xabar ketadimi. getMe hech narsa
// yubormaydi, faqat bot tokenini tekshiradi.
func checkTelegram() SystemCheck {
	c := SystemCheck{
		Key: "telegram", Name: "Telegram bot (xodimlar guruhi)", Manual: true,
		Affects: "muammolarni guruhga yuborish va xodim javoblarini o'qish",
	}
	if os.Getenv("TELEGRAM_BOT_TOKEN") == "" || os.Getenv("TELEGRAM_GROUP_ID") == "" {
		c.Status = HealthFail
		c.Message = "TELEGRAM_BOT_TOKEN yoki TELEGRAM_GROUP_ID berilmagan"
		return c
	}
	if _, err := telegramPost("getMe", map[string]any{}); err != nil {
		c.Status = HealthFail
		c.Message = err.Error()
		return c
	}
	c.Status = HealthOK
	c.Message = "ishlayapti"
	return c
}

// checkModel - AI provayderi kaliti bormi. Tirik so'rov yuborilmaydi:
// u pul turadi va token sarflaydi.
func checkModel() SystemCheck {
	c := SystemCheck{Key: "model", Affects: "javob tayyorlash"}
	if AIProvider() == ProviderDeepSeek {
		c.Name = "Model (DeepSeek)"
		if os.Getenv("DEEPSEEK_API_KEY") == "" {
			c.Status, c.Message = HealthFail, "DEEPSEEK_API_KEY berilmagan"
			return c
		}
	} else {
		c.Name = "Model (Groq)"
		if os.Getenv("GROQ_API_KEY") == "" {
			c.Status, c.Message = HealthFail, "GROQ_API_KEY berilmagan"
			return c
		}
	}
	c.Status = HealthOK
	c.Message = "kalit o'rnatilgan"
	return c
}

// jwtExpiry JWT ichidagi "exp" ni vaqt sifatida qaytaradi.
// Token JWT bo'lmasa yoki exp yo'q bo'lsa ok=false.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(strings.TrimPrefix(token, "Bearer "), ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// HTTPStatus - holatga mos HTTP kodi: tizim ishlamayotganda monitoring
// (uptime-kuzatuv) ham sezsin.
func (h HealthReport) HTTPStatus() int {
	if h.Status == HealthFail {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}
