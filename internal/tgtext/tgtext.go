// Package tgtext — Telegram xabarini "bosib nusxalanadigan" (monospace)
// bo'laklar bilan tayyorlaydi.
//
// Telegram'da `code` entity bo'lgan matnni bosgan zahoti nusxalanadi —
// shuning uchun ID va buyurtma raqamlari shu ko'rinishda yuboriladi.
// Matn ichida bo'lak ` (backtick) bilan belgilanadi, Build esa
// backtick'larni olib tashlab, entity'lar uchun o'rinlarni hisoblaydi.
package tgtext

import (
	"regexp"
	"strings"
)

// Span — xabardagi bitta monospace bo'lak.
// Offset va Length Telegram talab qilgani kabi UTF-16 birliklarida
// (emoji kabi belgilar 2 birlik sanaladi).
type Span struct {
	Offset int
	Length int
}

// numRe — nusxalashga arziydigan raqamlar: №/# bilan yozilgan yoki
// 4 va undan ko'p xonali sonlar (buyurtma raqami, ID, telefon...).
var numRe = regexp.MustCompile(`(?:№|#)\s?\d{2,}|\d{4,}`)

// MarkNumbers matndagi buyurtma/ID raqamlarini backtick ichiga oladi,
// ya'ni Build ularni nusxalanadigan qilib belgilaydi.
func MarkNumbers(s string) string {
	// Matnda tasodifan uchragan backtick belgilashni buzmasligi uchun.
	s = strings.ReplaceAll(s, "`", "'")
	return numRe.ReplaceAllString(s, "`$0`")
}

// Build backtick bilan belgilangan bo'laklarni ajratadi va toza matn
// hamda ularning UTF-16 o'rinlarini qaytaradi.
// Yopilmagan backtick e'tiborsiz qoldiriladi (xabar baribir ketadi).
func Build(raw string) (string, []Span) {
	var (
		b     strings.Builder
		spans []Span
		open  = -1
		pos   = 0 // joriy UTF-16 o'rni
	)
	for _, r := range raw {
		if r == '`' {
			if open < 0 {
				open = pos
			} else {
				if pos > open {
					spans = append(spans, Span{Offset: open, Length: pos - open})
				}
				open = -1
			}
			continue
		}
		b.WriteRune(r)
		if r > 0xFFFF {
			pos += 2 // surrogate juftlik (emoji va h.k.)
		} else {
			pos++
		}
	}
	return b.String(), spans
}
