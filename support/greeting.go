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

// cutOrderPrefix - matn boshidagi "DG123, DG456 — " prefiksini ajratadi
// (WithOrderSN qo'shgan bo'lsa). Prefiks qaytariladi, chunki salomni
// olib tashlaganda u matnda QOLISHI kerak.
func cutOrderPrefix(text string) (prefix, rest string) {
	if i := strings.Index(text, " — "); i > 0 && i < 120 {
		head := text[:i]
		if strings.IndexFunc(head, func(r rune) bool { return r == '.' || r == '!' || r == '?' }) < 0 {
			cut := i + len(" — ")
			return text[:cut], strings.TrimSpace(text[cut:])
		}
	}
	return "", text
}

// trimLeadingNumbers - o'sha prefiksni olib tashlaydi.
func trimLeadingNumbers(text string) string {
	_, rest := cutOrderPrefix(text)
	return rest
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
// Salom alohida qatorda turishi SHART EMAS: model uni ko'pincha birinchi
// gapning ichiga qo'shib yozadi ("Здравствуйте! Сегодня поступили новые
// грузы...") — ilgari aynan shunday matndan salom olinmay qolar va mijoz
// ketma-ket ikki marta salom olardi. Shuning uchun matnning BOSHIDAGI
// salom bo'lagi (birinchi qator yoki birinchi qisqa gap) olinadi.
//
// Matn ichidagi "salom" so'ziga tegilmaydi: faqat bosh bo'lak va faqat
// butunlay salomdan iborat bo'lsa.
func WithoutGreeting(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	// Buyurtma raqami prefiksi joyida qoladi, salom esa uning ortidan
	// qidiriladi ("DG123 — Assalomu alaykum, ...").
	prefix, body := cutOrderPrefix(text)
	changed := false
	// Bir necha salom ketma-ket kelishi mumkin ("Assalomu alaykum!
	// Hurmatli mijoz, ..."), lekin cheksiz emas: ikki qadam yetarli.
	for i := 0; i < 2; i++ {
		rest, ok := dropGreetingHead(body)
		if !ok {
			break
		}
		body, changed = rest, true
	}
	if !changed {
		return text
	}
	return strings.TrimSpace(prefix + body)
}

// greetHeadMax - salom bo'lagi eng ko'pi bilan shuncha runa bo'lishi
// mumkin. Uzunroq bosh bo'lak — bu allaqachon javobning mazmuni.
const greetHeadMax = 40

// dropGreetingHead - matn boshidagi salom bo'lagini olib tashlaydi.
//
// Bo'lak chegarasi: qator oxiri yoki birinchi tinish belgisi
// (". ! ? , ; :"). Bo'lak butunlay salomdan iborat bo'lsagina olinadi,
// va ortidan matn qolishi shart — butun javob salomdan iborat bo'lsa
// hech narsa o'zgarmaydi (bo'sh xabar yuborib bo'lmaydi).
func dropGreetingHead(body string) (string, bool) {
	cut := greetHeadEnd(body)
	if cut <= 0 {
		return body, false
	}
	head := body[:cut]
	rest := strings.TrimSpace(body[cut:])
	if rest == "" || !isGreetingOnly(head) {
		return body, false
	}
	return upperFirst(rest), true
}

// greetHeadEnd - bosh bo'lak tugaydigan joy (tinish belgisi bilan birga).
// Topilmasa yoki juda uzoq bo'lsa -1.
func greetHeadEnd(body string) int {
	n := 0
	for i, r := range body {
		if r == '\n' {
			return i
		}
		if strings.ContainsRune(".!?,;:", r) {
			return i + len(string(r))
		}
		if n++; n > greetHeadMax {
			return -1
		}
	}
	return -1
}

// upperFirst - birinchi harfni bosh harfga aylantiradi: salom vergul
// bilan ulangan bo'lsa ("Assalomu alaykum, buyurtmangiz...") qolgan gap
// kichik harfdan boshlanib qolmasin.
func upperFirst(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
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
//  1. baza: bugun shu mijozga (yoki shu suhbatga) MATNLI javob
//     yuborilganmi (`chat_reply` bo'sh bo'lmagan va `sent_at` bugungi).
//     Faqat `help` guruhga ketgan murojaatga ham `sent_at` yoziladi,
//     lekin mijoz hech narsa olmagan — shuning uchun `chat_reply <> ”`
//     sharti bor;
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

	// Suhbat ham hisobga olinadi: javob yozuvida mijoz id'si
	// aniqlanmay qolgan bo'lishi mumkin (xodim javobi guruhdan kelganda
	// `client_id` 0 bo'lib qoladi) — bunday yozuv faqat `client_id`
	// bo'yicha qidirilganda ko'rinmaydi va mijoz ikkinchi salomni oladi.
	q := DB.Model(&Interaction{}).Where("chat_reply <> '' AND sent_at >= ?", start)
	if conversationID > 0 {
		q = q.Where("client_id = ? OR conversation_id = ?", clientID, conversationID)
	} else {
		q = q.Where("client_id = ?", clientID)
	}

	var n int64
	err := q.Limit(1).Count(&n).Error
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
