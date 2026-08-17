package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Usage — bitta (yoki bir nechta) AI so'roviga sarflangan tokenlar.
// Raqamlar provayder javobidan olinadi — taxmin emas, bilinadigan aniq son.
type Usage struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`     // kirish (system + suhbat)
	CachedTokens     int    `json:"cached_tokens"`     // kirishning kesh'dan olingan, arzonroq qismi
	CompletionTokens int    `json:"completion_tokens"` // chiqish (model yozgan javob)
	Calls            int    `json:"calls"`             // nechta so'rov
	DurationMS       int64  `json:"duration_ms"`       // so'rovlarga ketgan vaqt
}

// Total — jami token soni.
func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }

// Add ikki hisobni qo'shadi (model nomi birinchi bo'sh bo'lmagandan olinadi).
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

// Speed — chiqish tezligi (token/sekund). Vaqt o'lchanmagan bo'lsa 0.
func (u Usage) Speed() float64 {
	if u.DurationMS <= 0 || u.CompletionTokens == 0 {
		return 0
	}
	return float64(u.CompletionTokens) / (float64(u.DurationMS) / 1000)
}

// String — loglar uchun qisqacha ko'rinish.
func (u Usage) String() string {
	s := fmt.Sprintf("%d token (kirish %d · kesh %d · chiqish %d, %d so'rov)",
		u.Total(), u.PromptTokens, u.CachedTokens, u.CompletionTokens, u.Calls)
	if u.DurationMS > 0 {
		s += fmt.Sprintf(", %.1fs", float64(u.DurationMS)/1000)
		if sp := u.Speed(); sp > 0 {
			s += fmt.Sprintf(" · %.0f tok/s", sp)
		}
	}
	return s
}

// GenOptions — bitta so'rovning generatsiya sozlamalari. Nol qiymat —
// provayderning o'z default'i.
type GenOptions struct {
	// MaxTokens — javobning eng ko'p tokeni (router uchun juda kichik).
	MaxTokens int
	// Temperature — 0 bo'lsa TempZero bilan aniq nol so'ralishi mumkin.
	Temperature float64
	// TempZero — true bo'lsa temperature=0 yuboriladi (deterministik javob).
	TempZero bool
	// JSON — provayderdan qat'iy JSON javob so'raladi.
	JSON bool
	// Schema — javobning JSON Schema'si. Berilsa provayder modelni AYNAN
	// shu shaklga majburlaydi (Ollama `format` maydoni sxemani qabul
	// qiladi). Shunda "faqat JSON yoz" kabi ko'rsatmalar prompt matnida
	// kerak emas — shakl kod tomonidan kafolatlanadi.
	Schema json.RawMessage
}

// Temp — yuboriladigan temperature va uni umuman yuborish kerakligini
// qaytaradi.
func (o GenOptions) Temp() (float64, bool) {
	if o.TempZero {
		return 0, true
	}
	if o.Temperature > 0 {
		return o.Temperature, true
	}
	return 0, false
}

// Meter — bitta suhbatga ketgan tokenlarni yig'ib boradigan hisoblagich.
// Context orqali uzatiladi, shuning uchun Ask/Classify/Summarize imzolari
// o'zgarmaydi. Mutex — bir ctx bir nechta goroutinada ishlatilsa ham xavfsiz.
type Meter struct {
	mu    sync.Mutex
	u     Usage
	warns []string
}

// Add hisobga qo'shadi (nil Meter'ga yozish xavfsiz — e'tiborsiz qoldiriladi).
func (m *Meter) Add(u Usage) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.u = m.u.Add(u)
}

// Warn ogohlantirish qo'shadi (masalan lokal modelda kontekst to'lib qolgani).
// Javob baribir olingan bo'ladi — bu xato emas, e'tibor talab qiladigan holat.
func (m *Meter) Warn(msg string) {
	if m == nil || msg == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warns = append(m.warns, msg)
}

// Warnings — yig'ilgan ogohlantirishlar.
func (m *Meter) Warnings() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.warns...)
}

// Usage — hozirgi yig'indi.
func (m *Meter) Usage() Usage {
	if m == nil {
		return Usage{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.u
}

// meterKey — context kaliti (tashqi paketlar bilan to'qnashmasligi uchun
// xususiy tur).
type meterKey struct{}

// WithMeter ctx'ga hisoblagich bog'laydi. Shundan keyin shu ctx bilan
// qilingan har bir AI so'rovi hisobga tushadi.
func WithMeter(ctx context.Context, m *Meter) context.Context {
	return context.WithValue(ctx, meterKey{}, m)
}

// meterFrom ctx'dagi hisoblagichni qaytaradi (bo'lmasa nil — Add e'tiborsiz).
func meterFrom(ctx context.Context) *Meter {
	m, _ := ctx.Value(meterKey{}).(*Meter)
	return m
}
