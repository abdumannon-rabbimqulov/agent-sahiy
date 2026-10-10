// Kunning birinchi javobida salomlashish.
//
// Mijoz bilan suhbat kun bo'yi davom etadi: har javobda "Assalomu alaykum"
// deyish g'alati, umuman salomlashmaslik esa sovuq ko'rinadi. Shuning uchun
// qoida: bir mijozga KUNDA BIR MARTA — o'sha kunning birinchi javobi salom
// bilan boshlanadi.
//
// Nega qaror YUBORISH paytida qabul qilinadi (deliverChat, agent.go):
// `auto_reply` default o'chiq, ya'ni javoblar qoralama bo'lib admin
// tasdig'ini kutadi. Kechqurun yozilgani ertalab tasdiqlanishi mumkin, bir
// mijozning ikki suhbatiga ikki qoralama tayyor turishi ham mumkin.
// Generatsiya paytida qo'yilgan salom bunday holatda ikki marta ketardi
// yoki eskirgan bo'lardi. Mijozga ketadigan yagona yo'l — deliverChat,
// shuning uchun hakam ham shu yerda.
//
// Model ham `salom` belgisini oladi (agent.go, staff_reply.go) — javobni
// o'zi jonli jumla bilan boshlashi uchun. Kod esa faqat natijani
// tekshiradi: salom yetishmasa qo'shadi, ortiqcha bo'lsa olib tashlaydi.
package support

import (
	"log"
	"strings"
	"time"
	"unicode"
)

// Salom matnlari — mijozning tili va alifbosi bo'yicha.
const (
	GreetUzLat = "Assalomu alaykum!"
	GreetUzCyr = "Ассалому алайкум!"
	GreetRU    = "Здравствуйте!"
)

// greetText - til bo'yicha salom matni.
func greetText(lang closingLang) string {
	switch lang {
	case langUzCyr:
		return GreetUzCyr
	case langRU:
		return GreetRU
	default:
		return GreetUzLat
	}
}

// greetWords - matnda salom allaqachon borligini bildiradigan so'zlar
// (kichik harfga keltirilgan holda qidiriladi).
var greetWords = []string{
	"assalom", "salom", "ассалом", "салом",
	"здравствуй", "здрасьте", "добрый день", "доброе утро", "добрый вечер",
	"hello", "hi ",
}

// uzCyrWords - o'zbek kirillini rus tilidan ajratadigan belgilar.
// Faqat harf (ў, қ, ғ, ҳ) yetarli emas: qisqa matnda ular uchramasligi
// mumkin ("Буюртмангиз келди" → rus deb xato qilinardi).
var uzCyrWords = []string{
	"сиз", "учун", "билан", "бўлади", "булади", "буюртма", "ҳурмат", "хурмат",
	"йўқ", "йук", "кун", "керак", "бор", "етиб", "жўнат", "жунат", "омбор",
}

// uzCyrLetters - o'zbek kirilliga xos harflar.
const uzCyrLetters = "ўқғҳЎҚҒҲ"

// greetLangOf - salom qaysi tilda yozilishi kerak. Til JAVOB MATNIDAN
// aniqlanadi (mijoz xabaridan emas): salom tananing o'zidan boshqa
// alifboda bo'lib qolmasligi kerak.
func greetLangOf(text string) closingLang {
	if !hasCyrillic(text) {
		return langUzLat
	}
	if strings.ContainsAny(text, uzCyrLetters) {
		return langUzCyr
	}
	low := strings.ToLower(text)
	for _, w := range uzCyrWords {
		if strings.Contains(low, w) {
			return langUzCyr
		}
	}
	return langRU
}

// greetScanLen - matn boshidan nechta runa salom uchun tekshiriladi.
const greetScanLen = 60

// hasGreeting - matnda allaqachon salom bormi.
//
// Faqat prefiksni tekshirish yetarli emas: WithOrderSN matn oldiga
// buyurtma raqamini qo'yadi ("DG123 — Assalomu alaykum, ..."), shuning
// uchun matnning boshidagi bir necha o'n belgi ko'riladi.
func hasGreeting(text string) bool {
	head := strings.ToLower(trimLeadingNumbers(text))
	if r := []rune(head); len(r) > greetScanLen {
		head = string(r[:greetScanLen])
	}
	for _, w := range greetWords {
		if strings.Contains(head, w) {
			return true
		}
	}
	return false
}

// trimLeadingNumbers - matn boshidagi "DG123, DG456 — " prefiksini olib
// tashlaydi (WithOrderSN qo'shgan bo'lsa).
func trimLeadingNumbers(text string) string {
	if i := strings.Index(text, " — "); i > 0 && i < 120 {
		head := text[:i]
		if strings.IndexFunc(head, func(r rune) bool { return r == '.' || r == '!' || r == '?' }) < 0 {
			return strings.TrimSpace(text[i+len(" — "):])
		}
	}
	return text
}

// WithGreeting - javob matnini salom bilan boshlaydi.
//
// Matnda salom allaqachon bo'lsa (model o'zi yozgan yoki javob ikkinchi
// marta yuborilayotgan bo'lsa) hech narsa qo'shilmaydi — funksiya
// idempotent.
func WithGreeting(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || hasGreeting(text) {
		return text
	}
	return greetText(greetLangOf(text)) + "\n\n" + text
}

// WithoutGreeting - salom KERAK BO'LMAGANDA model qo'ygan salomni olib
// tashlaydi (masalan qoralama kechqurun tayyorlangan, ertalab tasdiqlangan
// va shu kunda mijoz allaqachon javob olgan).
//
// Faqat birinchi QATOR butunlay salomdan iborat bo'lsa olinadi: gap
// o'rtasidagi yoki javob mazmuniga kirgan "salom" so'ziga tegilmaydi.
func WithoutGreeting(text string) string {
	text = strings.TrimSpace(text)
	line, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return text
	}
	if !isGreetingOnly(line) {
		return text
	}
	return strings.TrimSpace(rest)
}

// isGreetingOnly - qator faqat salomdan iboratmi ("Assalomu alaykum!",
// "Здравствуйте!"). Harf-raqamdan boshqa belgilar e'tiborga olinmaydi.
func isGreetingOnly(line string) bool {
	words := strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if len(words) == 0 || len(words) > 3 {
		return false
	}
	if !hasGreeting(line) {
		return false
	}
	for _, w := range words {
		if !greetFiller(w) {
			return false
		}
	}
	return true
}

// greetFiller - salom qatorida kelishi mumkin bo'lgan so'zlar.
func greetFiller(w string) bool {
	switch w {
	case "assalomu", "assalom", "alaykum", "salom", "ассалому", "ассалом",
		"алайкум", "салом", "здравствуйте", "здравствуй", "добрый", "доброе",
		"день", "утро", "вечер", "hello", "hi", "hurmatli", "ҳурматли":
		return true
	}
	return false
}

// startOfToday - bugungi kunning boshi (mahalliy mintaqada: time.Local
// kodda Asia/Tashkent qilib o'rnatiladi — timezone.go).
func startOfToday() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

// needGreeting - shu javob mijozga BUGUNGI birinchi javobimizmi.
//
// Ikki manba ham "yo'q" desagina salom beriladi:
//
//  1. baza: bugun shu mijozga MATNLI javob yuborilganmi (`chat_reply` bo'sh
//     bo'lmagan va `sent_at` bugungi). Faqat `help` guruhga ketgan
//     murojaatga ham `sent_at` yoziladi, lekin mijoz hech narsa olmagan —
//     shuning uchun `chat_reply <> ”` sharti bor;
//  2. suhbat tarixi: bugun bizdan ketgan xabar bormi. Xodim support
//     panelida mijozga O'ZI javob yozsa, bazada hech qanday yozuv
//     qolmaydi — buni faqat tarix ko'rsatadi.
//
// Tarix qisqa (HistoryLimit) bo'lgani uchun u yolg'iz yetarli emas, baza
// esa qo'lda yozilgan javobni ko'rmaydi — ikkisi bir-birini to'ldiradi.
//
// Shubha bo'lsa (baza yo'q, so'rov xatosi) — salom BERILMAYDI: ikki marta
// salomlashgandan ko'ra bir kun salomsiz o'tishi yaxshi.
func needGreeting(clientID, conversationID int64, msgs []Message) bool {
	if !GreetingEnabled() {
		return false
	}
	if clientID <= 0 {
		log.Printf("agent: mijoz id yo'q — salom qo'shilmadi")
		return false
	}
	if DB == nil {
		return false
	}
	start := startOfToday()

	var n int64
	err := DB.Model(&Interaction{}).
		Where("client_id = ? AND chat_reply <> '' AND sent_at >= ?", clientID, start).
		Limit(1).Count(&n).Error
	if err != nil {
		log.Printf("agent: mijoz %d — bugungi javoblar tekshirilmadi: %v", clientID, err)
		return false
	}
	if n > 0 {
		return false
	}
	// Xodim support panelida qo'lda javob bergan bo'lishi mumkin — buni
	// faqat tarix ko'rsatadi. Tarix hali olinmagan bo'lsa (avto-javob
	// yo'li) shu yerda olinadi: baza tekshiruvi o'tgandan KEYIN, ya'ni
	// ko'pi bilan kunda bir marta bir mijoz uchun.
	if msgs == nil && conversationID > 0 {
		h, err := fetchHistory(conversationID)
		if err != nil {
			log.Printf("agent: suhbat %d — salom uchun tarix olinmadi: %v", conversationID, err)
			return false
		}
		msgs = h
	}
	return !ourMessageToday(msgs, start)
}

// ourMessageToday - tarixda bugun BIZDAN (agent yoki xodim) ketgan xabar
// bormi. Sanasi o'qilmagan xabar e'tiborga olinmaydi.
func ourMessageToday(msgs []Message, start time.Time) bool {
	for _, m := range msgs {
		if m.FromClient() {
			continue
		}
		if t, ok := parseAnyTime(m.CreatedAt); ok && !t.Before(start) {
			return true
		}
	}
	return false
}

// applyGreeting - javob matnini yuborishga tayyorlaydi: kunning birinchi
// javobi bo'lsa salom qo'shiladi, aks holda (model ortiqcha yozgan bo'lsa)
// salom olib tashlanadi.
//
// `msgs` - yuborishdan oldingi tekshiruvda allaqachon olingan tarix
// (bo'lmasa kerak bo'lganda o'zi oladi).
func applyGreeting(in *Interaction, msgs []Message) string {
	if in == nil || in.ChatReply == "" {
		return ""
	}
	// Sozlama o'chirilgan bo'lsa matnga umuman tegilmaydi: model ham
	// salom haqida ko'rsatma olmagan.
	if !GreetingEnabled() {
		return in.ChatReply
	}
	if needGreeting(in.ClientID, in.ConversationID, msgs) {
		return WithGreeting(in.ChatReply)
	}
	return WithoutGreeting(in.ChatReply)
}
