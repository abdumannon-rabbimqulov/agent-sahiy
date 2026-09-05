// Modelga sarflangan tokenlar hisobi va ularning USD'dagi narxi
// (.env: GROQ_PRICE_IN / GROQ_PRICE_OUT).
package support

import (
	"fmt"
	"os"
	"strconv"
)

// Usage - AI so'rov(lar)iga sarflangan tokenlar. Raqamlar provayder
// javobidan olinadi, taxmin qilinmaydi.
type Usage struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`     // kirish
	CachedTokens     int    `json:"cached_tokens"`     // kirishning kesh'dan olingan qismi
	CompletionTokens int    `json:"completion_tokens"` // chiqish
	Calls            int    `json:"calls"`
	DurationMS       int64  `json:"duration_ms"`
}

// Total - jami token.
func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }

// Add ikki hisobni qo'shadi (model nomi birinchi bo'sh bo'lmaganidan olinadi).
func (u Usage) Add(o Usage) Usage {
	if u.Model == "" {
		u.Model = o.Model
	}
	u.PromptTokens += o.PromptTokens
	u.CachedTokens += o.CachedTokens
	u.CompletionTokens += o.CompletionTokens
	u.Calls += o.Calls
	u.DurationMS += o.DurationMS
	return u
}

// String - log uchun qisqacha.
func (u Usage) String() string {
	return fmt.Sprintf("%d token (kirish %d · kesh %d · chiqish %d, %d so'rov, %.1fs)",
		u.Total(), u.PromptTokens, u.CachedTokens, u.CompletionTokens, u.Calls,
		float64(u.DurationMS)/1000)
}

// envFloat .env dagi son (bo'sh yoki xato bo'lsa 0).
func envFloat(key string) float64 {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// Cost - sarflangan pul (USD). Narxlar 1 mln token uchun:
// GROQ_PRICE_IN / GROQ_PRICE_OUT. Berilmagan bo'lsa 0 qaytadi —
// panelda faqat token soni ko'rinadi.
func (u Usage) Cost() float64 {
	in, out := envFloat("GROQ_PRICE_IN"), envFloat("GROQ_PRICE_OUT")
	if in == 0 && out == 0 {
		return 0
	}
	return (float64(u.PromptTokens)*in + float64(u.CompletionTokens)*out) / 1_000_000
}
