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
	// \s* (bitta emas): OCR "DG" bilan raqam orasiga qator ko'chirish yoki
	// bir nechta probel tashlab yuborishi mumkin (masalan "DG\n60679679").
	orderSNRe = regexp.MustCompile(`(?i)(?:^|[^0-9A-Za-zА-Яа-я])(?:DG|ДГ)\s*(\d{6,})`)
	// Harf bilan boshlanadigan trek: JT…, YT…, P…, SF… va h.k.
	letterTrackRe = regexp.MustCompile(`(?i)\b([A-Z]{1,2}\d{9,})\b`)
	// Faqat raqamli uzun trek (masalan 78975877791396).
	digitTrackRe = regexp.MustCompile(`\b(\d{11,})\b`)

	// urlRe - matndagi havola. Mijoz rasm yuborsa, xabar matni —
	// havolaning o'zi bo'ladi va uning ichida fayl nomi sifatida uzun
	// raqam turadi ("…/1788967261401804602-image_picker_….png"). U
	// trek raqami EMAS: havola butunlay olib tashlanadi.
	urlRe = regexp.MustCompile(`(?i)https?://\S+`)

	// phoneRe - O'zbekiston telefon raqami (+998901370006, 998901370006).
	// 12 xonali bo'lgani uchun digitTrackRe uni trek deb olib qo'yardi:
	// mijoz telefonini yozsa, tizim o'sha raqam bo'yicha buyurtma
	// qidirib "topilmadi" deb javob berardi.
	phoneRe = regexp.MustCompile(`(^|\D)(\+?998\d{9})(\D|$)`)

	// plusPhoneRe - boshqa davlat raqami ham "+" bilan yoziladi; trek
	// raqami hech qachon "+" bilan boshlanmaydi.
	plusPhoneRe = regexp.MustCompile(`\+\d{9,}`)
)

// cleanForNumbers - matnni raqam qidirishga tayyorlaydi: havolalar va
// telefon raqamlari olib tashlanadi (uzunligi saqlanadi — qolgan
// raqamlarning chegaralari buzilmasin).
func cleanForNumbers(text string) string {
	if isImageLink(strings.TrimSpace(text)) {
		return "" // butun xabar — rasm havolasi
	}
	text = urlRe.ReplaceAllStringFunc(text, blankOut)
	text = plusPhoneRe.ReplaceAllStringFunc(text, blankOut)
	for {
		loc := phoneRe.FindStringSubmatchIndex(text)
		if loc == nil {
			return text
		}
		// 2-guruh — telefonning o'zi; atrofidagi belgilar tegilmaydi.
		text = text[:loc[4]] + blankOut(text[loc[4]:loc[5]]) + text[loc[5]:]
	}
}

// blankOut - matnni bir xil uzunlikdagi probelga almashtiradi.
func blankOut(s string) string { return strings.Repeat(" ", len(s)) }

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
		text := cleanForNumbers(m.Message)
		if text == "" {
			continue
		}

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

// KeepMentioned - model qaytargan raqamlardan faqat SUHBATDA haqiqatan
// uchraganlarini qoldiradi.
//
// Nega kerak: model ba'zan raqamni o'zidan to'qiydi (promtdagi misolni
// ko'chiradi yoki raqamni "tuzatib" yuboradi). Qidiruv esa raqam
// bo'yicha BUTUN adminka bazasidan ketadi — to'qilgan raqam boshqa
// odamning buyurtmasiga tushib qoladi. Natijada hech kim so'ramagan
// buyurtma bo'yicha muammo ochilib, xodimlar guruhiga eslatma yog'ilardi
// ("Mijoz: X (so'ragan: Y)").
//
// Suhbatning HAMMA xabari (mijozniki ham, bizniki ham) tekshiriladi:
// xodim javobida aytilgan raqam haqida mijoz "o'shani ayting" deb
// yozishi mumkin. `extra` — koddan kelgan ishonchli raqamlar (masalan
// mijoz yuborgan rasmdan OCR o'qiganlari).
func KeepMentioned(nums []string, msgs []Message, extra []string) []string {
	if len(nums) == 0 {
		return nil
	}
	var hay strings.Builder
	for _, m := range msgs {
		hay.WriteString(normalizeNum(m.Message))
		hay.WriteByte('\n')
	}
	for _, e := range extra {
		hay.WriteString(normalizeNum(e))
		hay.WriteByte('\n')
	}
	text := hay.String()

	out := make([]string, 0, len(nums))
	for _, n := range nums {
		key := normalizeNum(n)
		if key == "" {
			continue
		}
		if strings.Contains(text, key) {
			out = append(out, n)
		}
	}
	return out
}

// normalizeNum - solishtirish uchun ko'rinish: faqat harf va raqam,
// katta harfda. "DG 60679679", "dg-60679679" va "DG60679679" bir xil
// bo'lib qoladi.
func normalizeNum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// containsNum - matnda shu raqam (DG… yoki trek) bormi.
//
// `strings.Contains` yetarli emas: model raqamni "DG 60732205",
// "DG-60732205" yoki kirillcha "ДГ60732205" deb yozishi mumkin — uchala
// holatda ham raqam MATNDA BOR, lekin oddiy taqqoslash "yo'q" deydi va
// kod uni ikkinchi marta qo'shib yuboradi.
//
// Shuning uchun ikkala tomon ham bir ko'rinishga keltiriladi: ajratuvchilar
// tashlanadi (normalizeNum) va kirillcha "ДГ" lotincha "DG" ga qaytariladi
// (orderSNRe ham aynan shunday qaraydi).
func containsNum(text, num string) bool {
	n := normalizeNum(cyrToLatNum(num))
	if n == "" {
		return false
	}
	return strings.Contains(normalizeNum(cyrToLatNum(text)), n)
}

// cyrToLatNum - raqam yonidagi kirill harflarini lotinchaga qaytaradi
// ("ДГ60732205" → "DG60732205"). Faqat buyurtma/trek raqamlarida
// uchraydigan harflar.
var cyrToLatNumRepl = strings.NewReplacer(
	"Д", "D", "д", "D",
	"Г", "G", "г", "G",
	"Ж", "J", "ж", "J",
	"Т", "T", "т", "T",
	"С", "S", "с", "S",
	"Р", "P", "р", "P",
	"У", "Y", "у", "Y",
	"В", "V", "в", "V",
	"Е", "E", "е", "E",
	"А", "A", "а", "A",
	"К", "K", "к", "K",
	"О", "O", "о", "O",
	"М", "M", "м", "M",
	"Н", "H", "н", "H",
	"Х", "X", "х", "X",
)

func cyrToLatNum(s string) string { return cyrToLatNumRepl.Replace(s) }

// cardLike - raqam bank kartasiga o'xshaydimi (16 xonali yoki O'zbekiston
// kartalari prefiksi bilan boshlanadi).
//
// Nega kerak: trek raqami qoidasi (11+ xonali) kartani ham tutadi. Mijoz
// yozgan matnda bu xavfsiz edi (raqam faqat qidiruvga ketardi), lekin
// XODIM matnidagi raqam endi mijozga ko'rsatiladigan javobga tushadi —
// karta raqami "trek raqamingiz" bo'lib ketmasligi kerak.
func cardLike(num string) bool {
	n := normalizeNum(num)
	if len(n) != 16 {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
