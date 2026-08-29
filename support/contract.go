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

	// Promt - keyingi promt id'si. null yoki 0 => zanjir tugadi.
	Promt *uint `json:"promt"`

	// Raw - modelning asl javobi (panel va log uchun).
	Raw string `json:"-"`
}

// NextPromt - keyingi promt id'si va zanjir davom etadimi.
func (a AgentJSON) NextPromt() (uint, bool) {
	if a.Promt == nil || *a.Promt == 0 {
		return 0, false
	}
	return *a.Promt, true
}

// NeedsData - kodning tashqi API'ga borishi kerakmi.
func (a AgentJSON) NeedsData() bool { return a.Dashboard || a.Adminka }

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
