// Suhbatni yakunlash: mijozning oxirgi so'zi "rahmat" / "hop" bo'lsa,
// chiroyli xayrlashib javob beriladi.
//
// Nega alohida: bunday xabarda savol yo'q — na buyurtma qidirish, na
// tizimdan ma'lumot olish kerak. Modelga bersak, u baribir "yordam kerak
// bo'lsa yozing" deb yozadi, lekin bu bir necha ming token va bir necha
// soniya turadi. Shuning uchun javob KODDA tayyor: zanjir umuman
// yurmaydi (agent.go), token sarflanmaydi.
//
// Xavfsizlik qoidasi: xabar FAQAT minnatdorchilik/rozilik so'zlaridan
// iborat bo'lsagina shunday hisoblanadi. "Rahmat, lekin qachon keladi?"
// — bu savol, u odatdagi zanjirga ketadi.
package support

import (
	"strings"
	"unicode"
)

// Xayrlashish matnlari — mijozning tili va alifbosi bo'yicha.
// Mijoz aynan qaysi so'z bilan yakunlaganidan aniqlanadi (closingWords),
// shuning uchun bu yerda tilni taxmin qilish xavfi yo'q.
const (
	FarewellUzLat = "Sizga ham rahmat! Yana savolingiz bo'lsa, bemalol yozing — doim yordam beramiz. Xayrli kun!"
	FarewellUzCyr = "Сизга ҳам раҳмат! Яна саволингиз бўлса, бемалол ёзинг — доим ёрдам берамиз. Хайрли кун!"
	FarewellRU    = "Спасибо вам! Если появятся вопросы — пишите, всегда рады помочь. Хорошего дня!"
)

// closingLang - yakunlovchi so'zning tili.
type closingLang int

const (
	langUnknown closingLang = iota // "ok", "👍" — til bilinmaydi
	langUzLat                      // rahmat, hop, mayli
	langUzCyr                      // раҳмат, хоп, майли
	langRU                         // спасибо, хорошо, понял
)

// closingWords - suhbatni yakunlaydigan so'zlar va ularning tili.
// Mijoz shulardan tashqari BIRON so'z yozsa, xabar yakunlovchi
// hisoblanmaydi.
var closingWords = map[string]closingLang{
	// O'zbekcha lotin.
	"rahmat": langUzLat, "raxmat": langUzLat, "rahmatt": langUzLat,
	"tashakkur": langUzLat, "minnatdorman": langUzLat,
	"hop": langUzLat, "xop": langUzLat, "xo'p": langUzLat, "хop": langUzLat,
	"mayli": langUzLat, "yaxshi": langUzLat, "yahshi": langUzLat,
	"tushunarli": langUzLat, "tushundim": langUzLat,
	"bo'ladi": langUzLat, "boladi": langUzLat, "buladi": langUzLat,
	"kutaman": langUzLat, "albatta": langUzLat, "zo'r": langUzLat, "zor": langUzLat,

	// O'zbekcha kirill.
	"раҳмат": langUzCyr, "рахмат": langUzCyr, "ташаккур": langUzCyr,
	"хоп": langUzCyr, "майли": langUzCyr, "яхши": langUzCyr,
	"тушунарли": langUzCyr, "тушундим": langUzCyr,
	"бўлади": langUzCyr, "булади": langUzCyr, "кутаман": langUzCyr,

	// Rus tili.
	"спасибо": langRU, "спс": langRU, "благодарю": langRU,
	"хорошо": langRU, "ладно": langRU, "понятно": langRU,
	"понял": langRU, "поняла": langRU, "буду": langRU, "ждать": langRU,
	"отлично": langRU, "супер": langRU,

	// Tili bilinmaydigan, lekin ma'nosi bir xil.
	"ok": langUnknown, "okey": langUnknown, "okay": langUnknown,
	"ок": langUnknown, "окей": langUnknown, "👍": langUnknown,
	"🙏": langUnknown, "😊": langUnknown, "❤": langUnknown, "🌹": langUnknown,
}

// closingFillers - yakunlovchi so'z yonida kelishi mumkin bo'lgan, o'zi
// hech narsa so'ramaydigan so'zlar ("katta rahmat", "спасибо большое").
var closingFillers = map[string]bool{
	"katta": true, "kop": true, "ko'p": true, "sizga": true, "size": true,
	"ham": true, "juda": true, "aka": true, "opa": true, "uka": true,
	"katta_": true, "катта": true, "кўп": true, "куп": true, "сизга": true,
	"ҳам": true, "хам": true, "жуда": true, "ака": true, "опа": true,
	"большое": true, "вам": true, "тебе": true, "огромное": true, "и": true,
	"тоже": true, "за": true, "помощь": true, "va": true,
}

// IsClosingMessage - xabar faqat minnatdorchilik/rozilik so'zlaridan
// iboratmi. Savol belgisi, raqam yoki notanish so'z bo'lsa — yo'q.
//
// Ikkinchi qiymat — so'z qaysi tilda yozilgani (xayrlashish shu tilda
// yoziladi).
func IsClosingMessage(text string) (closingLang, bool) {
	words, ok := closingTokens(text)
	if !ok {
		return langUnknown, false
	}

	lang := langUnknown
	found := false // hech bo'lmasa bitta HAQIQIY yakunlovchi so'z bo'lsin
	for _, w := range words {
		l, isClosing := closingWords[w]
		if !isClosing {
			if closingFillers[w] {
				continue // "katta", "большое" — o'zi yakunlamaydi
			}
			return langUnknown, false // notanish so'z — bu savol bo'lishi mumkin
		}
		found = true
		// Birinchi tili aniq so'z butun xabarning tilini belgilaydi.
		if lang == langUnknown && l != langUnknown {
			lang = l
		}
	}
	if !found {
		return langUnknown, false
	}
	if lang == langUnknown && hasCyrillic(text) {
		// "ок" — kirillcha yozgan, lekin qaysi til noma'lum. Rus tili
		// ehtimoli yuqori: o'zbek kirillda odatda "хоп"/"раҳмат" yoziladi.
		lang = langRU
	}
	return lang, true
}

// closingTokens - xabarni so'zlarga ajratadi. Xabar juda uzun bo'lsa,
// savol belgisi yoki raqam bo'lsa — umuman ko'rilmaydi.
func closingTokens(text string) ([]string, bool) {
	t := strings.TrimSpace(strings.ToLower(text))
	if t == "" || len([]rune(t)) > 60 {
		return nil, false
	}
	// Savol yoki buyurtma raqami bor xabar — bu yakunlash emas.
	if strings.ContainsAny(t, "?？") {
		return nil, false
	}
	for _, r := range t {
		if unicode.IsDigit(r) {
			return nil, false
		}
	}

	fields := strings.FieldsFunc(t, func(r rune) bool {
		if r == '\'' || r == '’' {
			return false // "xo'p" bitta so'z bo'lib qolsin
		}
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})

	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "'’")
		f = strings.TrimFunc(f, func(r rune) bool {
			// Emoji variant belgilari ("❤️") tushib qolsin.
			return r == '️' || r == '︎'
		})
		if f == "" {
			continue
		}
		out = append(out, normalizeApostrophe(f))
	}
	if len(out) == 0 || len(out) > 5 {
		return nil, false
	}
	return out, true
}

// normalizeApostrophe - "xo’p" va "xo'p" bir xil yozilsin.
func normalizeApostrophe(s string) string {
	return strings.ReplaceAll(s, "’", "'")
}

// hasCyrillic - matnda kirill harfi bormi.
func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// FarewellText - berilgan til uchun xayrlashish matni.
func FarewellText(lang closingLang) string {
	switch lang {
	case langUzCyr:
		return FarewellUzCyr
	case langRU:
		return FarewellRU
	default:
		return FarewellUzLat
	}
}

// Farewell - suhbat mijozning "rahmat"/"hop" so'zi bilan yakunlanganmi.
// Shunday bo'lsa, mijozning tilidagi xayrlashish matni qaytadi.
//
// Faqat OXIRGI xabar ko'riladi: "rahmat" dan keyin mijoz yana savol
// yozgan bo'lsa, suhbat davom etyapti.
//
// Xabar rasm bo'lsa — yakunlash emas: rasmni ko'rib javob berish kerak
// (image_numbers.go).
func Farewell(msgs []Message) (string, bool) {
	if len(msgs) == 0 {
		return "", false
	}
	last := msgs[len(msgs)-1]
	if !last.FromClient() {
		return "", false
	}
	if isImageLink(strings.TrimSpace(last.Message)) {
		return "", false
	}
	lang, ok := IsClosingMessage(last.Message)
	if !ok {
		return "", false
	}
	return FarewellText(lang), true
}
