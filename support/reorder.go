// Qayta buyurtma: taqiqlangan tovar o'rniga boshqasini tanlash.
//
// Xodim guruhda qisqa yozadi ("boshqa tovar tanlang"), model esa mijozga
// butun tartibni tushuntiradi: shu summaga boshqa tovar tanlash, "To'lov
// qilish" ni bosib karta ma'lumotisiz qaytish ("to'lov kutilmoqda"
// holatiga o'tishi uchun), keyin bizga yozish.
//
// Mijoz tovar tanlab bergandan keyingi ish — tovarni ko'rib, sotib olish —
// xodimniki; shu fayl o'sha tanlovni topib, guruhga chiqarishga yordam
// beradi.
package support

import (
	"strings"
)

// reorderPhrases - xodim javobida "boshqa tovar tanlasin" ma'nosini
// bildiradigan iboralar. O'zak qisqa olingan: "boshqa tovar" — tovarni,
// tovarni tanlang, tovar tanlab bersin... hammasini qamraydi.
var reorderPhrases = []string{
	// O'zbekcha lotin.
	"boshqa tovar", "boshqa mahsulot", "boshqa narsa", "boshqasini tanla",
	"boshqa buyurtma qil", "boshqa zakaz", "almashtir",
	"taqiqlangan", "taqiqlangin", "taqiq",
	// O'zbekcha kirill.
	"бошқа товар", "бошқа маҳсулот", "бошқа нарса", "бошқасини танла",
	"алмаштир", "тақиқланган", "тақиқ",
	// Rus tili.
	"другой товар", "другого товара", "выбрать другой", "выберет другой",
	"запрещен", "запрещённ", "запрещенн", "замен",
}

// MentionsReorder - matnda "shu buyurtma o'rniga boshqa tovar tanlansin"
// ma'nosi bormi (xodim javobi yoki model matni).
func MentionsReorder(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, p := range reorderPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// ---- Mijoz almashtirish uchun tovar tanlab berdi ----
//
// Nega alohida: biz mijozga "shu summaga boshqa tovar tanlang" deganimizdan
// keyin u havola yoki skrinshot tashlaydi. Bundan keyingi ish — tovarni
// ko'rib, sotib olish — faqat XODIM qila oladigan ish, modelning aytadigan
// gapi yo'q. Avval bu holat 1-promtda "qayta buyurtma" yo'nalishiga
// ketardi va model mijozga yana o'sha 3 qadamni qaytarib yozardi: mijoz
// tovarni allaqachon tanlagan, javob esa uni boshidan boshlashga undardi.
//
// Endi: murojaat xodimlar guruhiga chiqadi (tanlangan havola bilan),
// mijozga esa faqat "qabul qildik, ko'rib chiqilmoqda" deyiladi.

// pickShopHosts - Xitoy savdo saytlari. Mijoz tovarni shu saytlardan
// biriga havola tashlab tanlaydi.
var pickShopHosts = []string{
	"aliexpress", "1688.com", "taobao", "tmall", "pinduoduo", "yangkeduo",
	"jd.com", "poizon", "dewu", "weidian", "xiaohongshu", "temu", "alibaba",
	"m.intl.taobao", "detail.tmall", "sahiy",
}

// pickPhrases - "tovarni tanladim" ma'nosini bildiradigan iboralar.
// Havolasiz javoblar uchun: mijoz skrinshot tashlab "mana shuni" deb
// yozishi mumkin.
var pickPhrases = []string{
	// O'zbekcha lotin.
	"tanladim", "tanladik", "tanlab oldim", "mana shu", "mana bu",
	"shu tovar", "bu tovar", "shuni oling", "shuni olasiz", "shuni ol",
	"buni oling", "buni olasiz", "shu mahsulot", "bu mahsulot",
	// O'zbekcha kirill.
	"танладим", "танладик", "мана шу", "мана бу", "шу товар", "бу товар",
	"шуни ол", "буни ол",
	// Rus tili.
	"выбрал", "выбрала", "выбрали", "вот этот", "вот это", "этот товар",
	"это товар", "возьмите этот", "беру этот",
}

// MentionsPick - matnda tanlangan tovar bormi: savdo sayti havolasi
// yoki "mana shuni tanladim" ma'nosidagi gap.
func MentionsPick(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, h := range pickShopHosts {
		if strings.Contains(t, h) {
			return true
		}
	}
	for _, p := range pickPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// MaxPickedToStaff - guruhga ko'pi bilan nechta tanlov matni chiqadi.
const MaxPickedToStaff = 3

// PickedReplacement - biz "boshqa tovar tanlang" deganimizdan KEYIN mijoz
// tovar tanlab bergan bo'lsa, uning tanlovini (havola yoki matn) qaytaradi.
// Bo'sh ro'yxat — bunday holat yo'q.
//
// Uch shart birga bajarilishi kerak:
//   - tarixda BIZNING "boshqa tovar tanlang" xabarimiz bor;
//   - o'sha xabardan KEYIN mijoz havola, rasm yoki "tanladim" deb yozgan;
//   - oxirgi xabar mijozdan (ya'ni hozir uning tanloviga javob yozamiz).
//
// Tartib muhim: bizning savolimizdan OLDIN mijoz havola tashlagan bo'lsa
// (masalan oddiy buyurtma so'rab), bu tanlov emas.
func PickedReplacement(msgs []Message) []string {
	if len(msgs) == 0 || !msgs[len(msgs)-1].FromClient() {
		return nil
	}

	askedAt := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].FromClient() && MentionsReorder(msgs[i].Message) {
			askedAt = i
			break
		}
	}
	if askedAt < 0 {
		return nil
	}

	var out []string
	for i := askedAt + 1; i < len(msgs); i++ {
		m := msgs[i]
		if !m.FromClient() {
			continue
		}
		txt := strings.TrimSpace(m.Message)
		if txt == "" {
			continue
		}
		if !MentionsPick(txt) && !isImageLink(txt) {
			continue
		}
		out = append(out, txt)
	}
	if len(out) > MaxPickedToStaff {
		out = out[len(out)-MaxPickedToStaff:]
	}
	return out
}

// pickedAlert - xodimlar guruhiga ketadigan holat. Tanlangan havola shu
// yerda ko'rinadi: xodim suhbatni ochmasdan ham ishga kirisha oladi.
func pickedAlert(picked []string) string {
	return "Mijoz almashtirish uchun TOVAR TANLADI — xodim ko'rib, sotib olishi kerak. " +
		"Tanlovi: " + strings.Join(picked, " | ") +
		". Mijozga faqat \"qabul qilindi, ko'rib chiqilmoqda\" deyildi."
}
