package support

import "time"

// Kun boshidagi salom — har alifbo uchun. Ikki so'z, ortiqchasi yo'q.
//
// Uchalasi ham modelga BIRGA beriladi: kod mijozning tilini o'zi
// aniqlamaydi (oxirgi xabar rasm bo'lsa xato tanlanardi), tilni promt
// tanlaydi. Ilgari faqat o'zbekcha lotin variant ketardi va ruscha
// yozgan mijozga ham o'zbekcha javob yozilib qolardi.
const (
	GreetingText  = "Assalomu alaykum" // o'zbekcha lotin
	GreetingUzCyr = "Ассалому алайкум" // o'zbekcha kirill
	GreetingRU    = "Здравствуйте"     // rus
)

// AskHelpText - oxirgi chora: suhbatning butun tarixidan ham mijoz nima
// so'rayotgani tushunilmasa beriladigan savol. Javob yo muammoni hal
// qiladi, yo shu savolni beradi — salom bilan cheklanib qolmaydi.
// Salom kabi, bu ham mijozning tilida beriladi.
const (
	AskHelpText  = "Sizga qanday yordam bera olaman?"
	AskHelpUzCyr = "Сизга қандай ёрдам бера оламан?"
	AskHelpRU    = "Чем я могу вам помочь?"
)

// AskHelpVariants - "tushunmadim" savolining hamma tildagi ko'rinishi
// (contract.go dagi zaxira aniqlash uchun).
func AskHelpVariants() []string {
	return []string{AskHelpText, AskHelpUzCyr, AskHelpRU}
}

// NeedsGreeting - shu suhbatda bugun biz tomondan (agent yoki xodim) hali
// hech narsa yozilmagan bo'lsa true.
//
// Salom kuniga bir marta beriladi: yangi kun boshlanganda mijozga
// salomlashib javob yoziladi, o'sha kun davomidagi keyingi javoblarda esa
// salom takrorlanmaydi — suhbat davom etyapti, qayta salomlashish g'alati
// ko'rinadi.
//
// Sanasi o'qib bo'lmaydigan xabar hisobga olinmaydi: salom ortiqcha
// bo'lgani, tushib qolganidan yaxshiroq emas — shuning uchun shubhali
// holatda salom beriladi.
func NeedsGreeting(msgs []Message) bool {
	today := time.Now().Format("2006-01-02")
	for _, m := range msgs {
		if m.FromClient() {
			continue
		}
		t, ok := parseAnyTime(m.CreatedAt)
		if !ok {
			continue
		}
		if t.Local().Format("2006-01-02") == today {
			return false
		}
	}
	return true
}
