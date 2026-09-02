package support

import "time"

// GreetingText - kun boshidagi salom. Ikki so'z, ortiqchasi yo'q.
const GreetingText = "Assalomu alaykum"

// AskHelpText - oxirgi chora: suhbatning butun tarixidan ham mijoz nima
// so'rayotgani tushunilmasa beriladigan savol. Javob yo muammoni hal
// qiladi, yo shu savolni beradi — salom bilan cheklanib qolmaydi.
const AskHelpText = "Sizga qanday yordam bera olaman?"

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
