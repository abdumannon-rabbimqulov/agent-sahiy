// Suhbat matnidan buyurtma va trek raqamlarini ajratish.
//
// Raqam topishni faqat modelga ishonib bo'lmaydi: u ba'zan mijoz yozgan
// raqamni tashlab ketadi va tizim "topilmadi" deb javob beradi. Shuning
// uchun raqamlar KODDA ham ajratiladi va model qaytargani bilan
// birlashtiriladi.
package support

import (
	"regexp"
	"strings"
)

var (
	// DG bilan boshlanadigan buyurtma raqami (kirillcha ДГ ham).
	// \b ishlatilmaydi: Go'da u ASCII bo'yicha ishlaydi va "дг" dan
	// oldin chegara topilmaydi — shuning uchun oldingi belgi o'zi
	// tekshiriladi.
	orderSNRe = regexp.MustCompile(`(?i)(?:^|[^0-9A-Za-zА-Яа-я])(?:DG|ДГ)\s?(\d{6,})`)
	// Harf bilan boshlanadigan trek: JT…, YT…, P…, SF… va h.k.
	letterTrackRe = regexp.MustCompile(`(?i)\b([A-Z]{1,2}\d{9,})\b`)
	// Faqat raqamli uzun trek (masalan 78975877791396).
	digitTrackRe = regexp.MustCompile(`\b(\d{11,})\b`)
)

// ExtractNumbers - MIJOZ yozgan xabarlardan buyurtma va trek raqamlari.
// Bizning javoblarimizdagi raqamlar olinmaydi: ular baribir mijozning
// so'rovidan kelib chiqqan.
func ExtractNumbers(msgs []Message) (orderSN, express []string) {
	seenSN := map[string]bool{}
	seenEx := map[string]bool{}

	for _, m := range msgs {
		if !m.FromClient() {
			continue
		}
		text := m.Message

		for _, g := range orderSNRe.FindAllStringSubmatch(text, -1) {
			sn := "DG" + g[1]
			if !seenSN[sn] {
				seenSN[sn] = true
				orderSN = append(orderSN, sn)
			}
		}

		// DG raqamlari trek deb qayta olinmasin.
		clean := orderSNRe.ReplaceAllString(text, " ")

		for _, g := range letterTrackRe.FindAllStringSubmatch(clean, -1) {
			t := strings.ToUpper(g[1])
			if !seenEx[t] {
				seenEx[t] = true
				express = append(express, t)
			}
		}
		for _, g := range digitTrackRe.FindAllStringSubmatch(clean, -1) {
			t := g[1]
			if !seenEx[t] {
				seenEx[t] = true
				express = append(express, t)
			}
		}
	}
	return orderSN, express
}

// mergeNumbers - ikkita ro'yxatni birlashtiradi (takrorlanmaydi,
// tartib saqlanadi, `max` tadan oshmaydi).
func mergeNumbers(a, b []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			key := strings.ToUpper(v)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, v)
			if len(out) >= max {
				return out
			}
		}
	}
	return out
}
