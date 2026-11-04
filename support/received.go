// Mijoz allaqachon olgan tovar haqidagi murojaat.
//
// Mijoz buyurtma raqamini tashlab holatini so'raganda posilka qo'lida
// bo'lishi mumkin. Bu MUAMMO EMAS, lekin 2-promt "help" ni har doim
// to'ldiradi va xulosa "🆘 Yordam kerak" bo'lib guruhga tushardi —
// xodimning qo'lidan kelsa ham, qiladigan ishi yo'q edi.
//
// Endi qoida: tovar olingan bo'lsa model mijozdan "yaxshi keldimi, biror
// muammo bormi" deb so'raydi (pickedUpNote) va guruhga hech narsa
// chiqmaydi. Guruhga faqat mijoz tovar nosoz yoki noto'g'ri kelganini
// aytganda chiqadi.
package support

import (
	"fmt"
	"strings"
)

// defectPhrases - olingan tovardan shikoyat qilinayotganini bildiradigan
// bo'laklar. Tutuq belgisisiz yozilgan: matn ham normalizeApostrophes
// dan o'tadi ("yo'qolgan", "yoʻqolgan", "yoqolgan" — bir xil).
//
// "muammo" so'zi ataylab YO'Q: "muammo bor" nima buzilganini aytmaydi,
// xodimga ham aytadigan narsa yo'q — model nima muammo ekanini so'raydi.
var defectPhrases = []string{
	// O'zbekcha lotin.
	"singan", "siniq", "sinib", "nosoz", "buzilgan", "buzuq", "buzilib",
	"ishlamay", "yirtilgan", "yirtiq", "shikast", "yaroqsiz", "brak",
	"pachoq", "ezilgan", "sifatsiz", "sifati yomon", "yaxshi emas", "yaxshimas",
	"yetishmay", "kam keldi", "boshqa tovar", "notogri", "yoqolgan",
	"olmadim", "kelmadi",
	// O'zbekcha kirill.
	"синган", "синиқ", "носоз", "бузилган", "бузуқ", "ишламай", "йиртилган",
	"шикаст", "яроқсиз", "брак", "сифатсиз", "яхши эмас", "етишмай",
	"бошқа товар", "нотўғри", "йўқолган", "олмадим", "келмади",
	// Rus tili.
	"сломан", "разбит", "поврежд", "порван", "дефект", "не работает",
	"не тот товар", "не получил", "не пришл",
}

// receivedOKPhrases - "tovar yaxshi keldi" degan javob (bizning
// savolimizga). Yolg'iz "ha"/"да" so'z sifatida alohida tekshiriladi.
var receivedOKPhrases = []string{
	"yaxshi", "zor", "joyida", "muammo yoq", "muammosiz",
	"яхши", "зўр", "жойида", "муаммо йўқ",
	"хорошо", "нормально", "отлично", "нет проблем",
}

var receivedOKWords = []string{"ha", "xa", "ҳа", "ха", "да"}

// normalizeApostrophes - kichik harfga o'tkazadi va tutuq belgilarini
// olib tashlaydi: mijozlar ularni har xil yozadi yoki umuman yozmaydi.
func normalizeApostrophes(s string) string {
	return strings.NewReplacer("'", "", "ʻ", "", "ʼ", "", "’", "", "‘", "", "`", "").
		Replace(strings.ToLower(strings.TrimSpace(s)))
}

// MentionsDefect - matnda tovar nosoz, noto'g'ri yoki olinmagani haqida
// gap bormi.
func MentionsDefect(text string) bool {
	return containsAny(normalizeApostrophes(text), defectPhrases)
}

// ReportsDefect - javob berilmagan mijoz xabarlarida shikoyat bormi.
// Biz javob bergandan keyin eski shikoyat qayta ko'tarilmaydi
// (WantsCancel bilan bir mantiq).
func ReportsDefect(msgs []Message) bool {
	for _, m := range UnansweredClient(msgs) {
		if MentionsDefect(m.Message) {
			return true
		}
	}
	return false
}

// defectText - javob berilmagan xabarlardan shikoyat yozilganlari.
func defectText(msgs []Message) string {
	var parts []string
	for _, m := range UnansweredClient(msgs) {
		if MentionsDefect(m.Message) {
			parts = append(parts, strings.TrimSpace(m.Message))
		}
	}
	return trimText(strings.Join(parts, " "), 200)
}

// defectAlert - guruhga ketadigan qator: qaysi posilka, mijoz nima dedi.
// Model xulosa yozmagan bo'lsa ham xodim shu qatordan tushunadi.
func defectAlert(tracks []string, msgs []Message) string {
	return fmt.Sprintf("%s — mijoz tovarni olgan, lekin muammo aytdi: \"%s\"",
		strings.Join(tracks, ", "), defectText(msgs))
}

// receivedHelpMuted - tovar olingan buyurtma bo'yicha xulosa guruhga
// chiqmasligi kerakmi: mijoz shu xabarida raqam yozib holatni so'ragan
// (rasm orqali ham) yoki savolimizga "yaxshi keldi" deb javob bergan.
//
// Boshqa javoblar MUTE QILINMAYDI: "rangi men tanlagandek emas" kabi
// gap ro'yxatga sig'maydi va xodimga yetmay qolishdan ko'ra guruhga
// chiqqani yaxshi.
func receivedHelpMuted(msgs []Message, numberFromImage bool) bool {
	if numberFromImage {
		return true
	}
	for _, m := range UnansweredClient(msgs) {
		if hasAnyNumber(m.Message) || saysReceivedOK(m.Message) {
			return true
		}
	}
	return false
}

// saysReceivedOK - mijoz tovar yaxshi kelganini aytdimi.
func saysReceivedOK(text string) bool {
	t := normalizeApostrophes(text)
	if containsAny(t, receivedOKPhrases) {
		return true
	}
	for _, w := range splitWords(t) {
		if containsWord(receivedOKWords, w) {
			return true
		}
	}
	return false
}

func containsWord(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

// isReceived - yetkazma yozuvi "mijoz tovarni olgan" degan yagona holat:
// status 7 va delivered=true, jo'natma turidan qat'i nazar (olib_ketilgan
// yoki mijozga_yetkazilgan).
//
// Yolg'iz `delivered=true` bunga KIRMAYDI: u faqat kuryerga berilganini
// bildiradi, mijoz qo'liga tekkanini emas.
func isReceived(o DeliveryOrder) bool {
	return o.Delivered && o.Status == 7
}

// receivedTracks - so'ralgan HAMMA narsa mijoz qo'lida bo'lsa, o'sha
// treklar. Bittasi bo'lsa ham hali yetmagan, tekshirilishi kerak yoki
// xato bilan chiqqan bo'lsa nil: xulosa o'sha buyurtma haqida bo'lishi
// mumkin va guruhdan yashirilmasligi kerak.
func receivedTracks(brief DeliveryBrief, checks []OrderCheck) []string {
	done := append(append([]PickupDone{}, brief.PickedUp...), brief.Handed...)
	if len(done) == 0 || len(brief.Pending) > 0 ||
		len(brief.InDelivery) > 0 || len(brief.NeedCheck) > 0 {
		return nil
	}
	for _, c := range checks {
		if c.Step != StepReceived {
			return nil
		}
	}
	out := make([]string, 0, len(done))
	for _, p := range done {
		out = append(out, p.ExpressNum)
	}
	return out
}
