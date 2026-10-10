// Buyurtmani bekor qilish / pulni qaytarish so'rovi.
//
// Nega alohida: bu pul bilan bog'liq QAROR, uni faqat xodim qabul
// qiladi. Model esa mijozning gapini takrorlab, "so'rovingizni qabul
// qildik" deb IJOBIY javob yozib yuborardi — mijoz buni "bekor
// qilindi" deb tushunadi va keyin xodim boshqa gap aytishga majbur
// bo'ladi.
//
// Shuning uchun bunday murojaat KOD darajasida ushlanadi: modelga
// qat'iy taqiq beriladi, murojaat xodimlar guruhiga chiqadi va model
// baribir taqiqni buzsa, javob mijozga avtomatik ketmaydi.
package support

import "strings"

// cancelPhrases - bekor qilish / pul qaytarish so'rovini bildiradigan
// iboralar. Qisqa o'zak olingan: "bekor qil" — qilaman, qilishni,
// qilinsin, qildiring... hammasini qamraydi.
var cancelPhrases = []string{
	// O'zbekcha lotin.
	"bekor qil", "bekor kil", "bekorga chiqar", "bekor qilin",
	"pulni qaytar", "pulimni qaytar", "pulni qaytr", "puli qaytar",
	"pulimni qaytr", "qaytarib ber", "qaytarib bering", "qaytarib berin",
	"vozvrat", "otmena", "otkaz",
	// O'zbekcha kirill.
	"бекор қил", "бекор кил", "пулни қайтар", "пулимни қайтар",
	"қайтариб бер",
	// Rus tili.
	"отмен", "верните деньги", "вернуть деньги", "возврат", "отказ",
	// Ingliz tili (kamdan-kam, lekin uchraydi).
	"cancel", "refund",
}

// WantsCancel - suhbat tarixidagi MIJOZ xabarlarida bekor qilish yoki
// pul qaytarish so'rovi bormi.
//
// Faqat mijoz yozgan matn ko'riladi: bizning javobimizda bu so'z
// uchrasa (masalan xodim tushuntirgan bo'lsa), qoida qayta ishga
// tushmasin.
//
// Tarixning hammasi (oxirgi HISTORY_LIMIT ta xabar) ko'riladi, faqat
// oxirgi xabar emas: mijoz bekor qilishni so'rab, keyin boshqa gap
// yozsa ham mavzu ochiqligicha qoladi.
func WantsCancel(msgs []Message) bool {
	for _, m := range msgs {
		if !m.FromClient() {
			continue
		}
		if MentionsCancel(m.Message) {
			return true
		}
	}
	return false
}

// MentionsCancel - matnda bekor qilish / pul qaytarish haqida gap bormi.
// Mijoz xabarini ham, model yozgan javobni ham shu funksiya tekshiradi.
func MentionsCancel(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, p := range cancelPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// cancelAlert - xodimlar guruhiga ketadigan holat. Sozlamadan qat'i
// nazar yuboriladi (Alerts bo'lsa help doim guruhga chiqadi).
const cancelAlert = "Mijoz BUYURTMANI BEKOR QILISH / PUL QAYTARISH so'radi — " +
	"qarorni xodim qabul qiladi. Mijozga faqat \"tekshirilmoqda\" deyildi."

// cancelReplyAlert - model taqiqni buzib, javobida baribir bekor qilish
// haqida yozgan holat. Javob mijozga avtomatik ketmaydi: admin panelda
// tasdiqlashni kutadi.
const cancelReplyAlert = "AI javobida bekor qilish / pul qaytarish haqida gap bor — " +
	"javob mijozga YUBORILMADI, admin tasdig'ini kutmoqda."
