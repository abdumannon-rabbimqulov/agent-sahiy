// Shartnoma: AI agent qaytaradigan JSON va uni o'qish.
//
// Promt matnini siz yozasiz — kod faqat javobning SHAKLINI biladi:
//
//	{"dashboard":true,"adminka":false,"order_sn":["DG60597226"],
//	 "express_num":[],"chat":"Salom...","help":"...","promt":4}
package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrBadJSON - model JSON qaytarmadi.
var ErrBadJSON = errors.New("model JSON qaytarmadi")

// AgentJSON - AI qaytaradigan qaror. Noma'lum kalitlar e'tiborsiz qoladi.
type AgentJSON struct {
	// Dashboard - yetkazma (delivery) ma'lumotini tekshirish kerakmi.
	Dashboard bool `json:"dashboard"`
	// Adminka - daigou (Xitoy tomoni) buyurtmalarini tekshirish kerakmi.
	Adminka bool `json:"adminka"`

	// Qidiruv filtri (bo'sh bo'lsa mijoz client_id bo'yicha qidiriladi).
	OrderSN    []string `json:"order_sn"`
	ExpressNum []string `json:"express_num"`

	// Chat - mijozga yuboriladigan javob.
	Chat string `json:"chat"`
	// Help - Telegram guruhga (xodimlarga) yuboriladigan matn.
	Help string `json:"help"`

	// NotUnderstood - model mijozning muammosini tushunmadi
	// ("tushunmadim": true). Kod bunda savol berishdan oldin adminka va
	// dashboarddan mijozning kelmagan buyurtmalarini o'zi tekshiradi.
	NotUnderstood bool `json:"tushunmadim"`

	// Promt - keyingi promt id'si. null, false yoki 0 => zanjir tugadi.
	// Model son o'rniga satr ("2") yoki bool qaytarsa ham o'qiladi.
	Promt PromtRef `json:"promt"`

	// Raw - modelning asl javobi (panel va log uchun).
	Raw string `json:"-"`
}

// PromtRef - "promt" kalitining qiymati. Model turli shaklda qaytarishi
// mumkin: 2, "2", null, false — hammasi bir xil tushuniladi.
type PromtRef struct {
	ID uint
}

// UnmarshalJSON son, sonli satr, null va false ni qabul qiladi.
func (p *PromtRef) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" || s == "false" || s == `""` {
		p.ID = 0
		return nil
	}
	s = strings.Trim(s, `"`)
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// Tushunarsiz qiymat — zanjir tugadi deb hisoblanadi
		// (xato qaytarsak butun javob yo'qoladi).
		p.ID = 0
		return nil
	}
	p.ID = uint(n)
	return nil
}

// MarshalJSON - id 0 bo'lsa null.
func (p PromtRef) MarshalJSON() ([]byte, error) {
	if p.ID == 0 {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatUint(uint64(p.ID), 10)), nil
}

// NextPromt - keyingi promt id'si va zanjir davom etadimi.
func (a AgentJSON) NextPromt() (uint, bool) {
	if a.Promt.ID == 0 {
		return 0, false
	}
	return a.Promt.ID, true
}

// NeedsData - kodning tashqi API'ga borishi kerakmi.
func (a AgentJSON) NeedsData() bool { return a.Dashboard || a.Adminka }

// IsUnclear - model mijoz nima so'rayotganini tushunmadimi.
//
// Ikki yo'l bilan aniqlanadi: `"tushunmadim": true` kaliti (promt shuni
// yozishga o'rgatadi) yoki javobning o'zi "sizga qanday yordam bera
// olaman" savoli bo'lsa (mijozning har qaysi tilidagi ko'rinishida) —
// promt eski bo'lsa ham fallback ishlasin.
func (a AgentJSON) IsUnclear() bool {
	if a.NotUnderstood {
		return true
	}
	chat := strings.ToLower(a.Chat)
	for _, v := range AskHelpVariants() {
		if strings.Contains(chat, strings.ToLower(v)) {
			return true
		}
	}
	return false
}

// Numbers - modeldan kelgan buyurtma/trek raqamlari (bo'shlari tashlanadi).
func (a AgentJSON) Numbers() []string {
	var out []string
	for _, s := range append(append([]string{}, a.OrderSN...), a.ExpressNum...) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ParseAgentJSON model javobini o'qiydi. Model matn ichiga JSON qo'yib
// yuborsa ham (```json ... ``` yoki tushuntirish bilan) ajratib olinadi.
func ParseAgentJSON(raw string) (AgentJSON, error) {
	body := extractJSON(raw)
	if body == "" {
		return AgentJSON{Raw: raw}, fmt.Errorf("%w: %s", ErrBadJSON, snippet([]byte(raw)))
	}
	var a AgentJSON
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		return AgentJSON{Raw: raw}, fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	a.Raw = raw
	a.Chat = strings.TrimSpace(a.Chat)
	a.Help = strings.TrimSpace(a.Help)
	return a, nil
}

// extractJSON matndan birinchi to'liq JSON obyektini ajratadi.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// ```json ... ``` ramkasi.
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// satr ichidagi qavslar hisobga olinmaydi
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
