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

import (
	"strings"
	"unicode"
)

// cancelPhrases - bekor qilish / pul qaytarish so'rovini bildiradigan
// iboralar. Qisqa o'zak olingan: "bekor qil" — qilaman, qilishni,
// qilinsin, qildiring... hammasini qamraydi.
var cancelPhrases = []string{
	// O'zbekcha lotin.
	"bekor qil", "bekor kil", "bekorga chiqar", "bekor qilin",
	"pulni qaytar", "pulimni qaytar", "pulni qaytr", "puli qaytar",
	"pulimni qaytr", "qaytarib ber", "qaytarib bering", "qaytarib berin",
	"vozvrat", "otmena",
	// O'zbekcha kirill.
	"бекор қил", "бекор кил", "пулни қайтар", "пулимни қайтар",
	"қайтариб бер",
	// Rus tili.
	"отмен", "верните деньги", "вернуть деньги", "возврат", "отказ",
	// Ingliz tili (kamdan-kam, lekin uchraydi).
	"cancel", "refund",
}

// cancelWordPrefixes - "otkaz" turkumi. Mijozlar uni turlicha yozadi
// ("atkaz qivorila", "narsani atkas qlaman", "otkazdim"), lekin bu
// bo'laklar qisqa — boshqa so'zning O'RTASIDA tasodifan uchrab qolishi
// mumkin. MentionsCancel esa modelning O'Z javobiga ham qo'llanadi
// (agent.go), ya'ni tasodifiy moslik ham murojaatni xodimga chiqarib,
// ham avto-javobni to'xtatib qo'yardi.
//
// Shuning uchun ular butun matndan emas, SO'Z BOSHIDAN izlanadi:
// "atkaz", "otkazdim" — ha; o'rtasida shu bo'lagi bor begona so'z — yo'q.
var cancelWordPrefixes = []string{"otkaz", "otkas", "atkaz", "atkas"}

// cancelOrderWords / cancelNotNeeded - "buyurtma kerak emas" turkumi.
// Ikki bo'lak matnda YONMA-YON turishi shart emas: "zakazim kerakmas",
// "zakazni kerak emas", "buyurtmani kerakmas" — hammasi shu ma'noda.
// Ilgari bu yerda "zakaz kerakmas" kabi ko'p so'zli aniq iboralar
// turardi va qo'shimchali shakllar umuman ushlanmasdi.
//
// Yolg'iz "kerakmas" yetarli emas: u bizning boshqa savolimizga javob
// bo'lishi mumkin ("rasm yuboraymi?" — "kerakmas"). Shuning uchun
// buyurtma so'zi ham bo'lishi shart.
var cancelOrderWords = []string{"zakaz", "buyurtma", "order", "заказ"}

var cancelNotNeeded = []string{
	"kerakmas", "kerak emas", "keremas", "kermas", "kerakmi emas",
	"не нужен", "не нужна", "не надо",
}

// WantsCancel - suhbat tarixidagi MIJOZ xabarlarida bekor qilish yoki
// pul qaytarish so'rovi bormi.
//
// Faqat mijoz yozgan matn ko'riladi: bizning javobimizda bu so'z
// uchrasa (masalan xodim tushuntirgan bo'lsa), qoida qayta ishga
// tushmasin.
//
// Oxirgi xabargina emas, JAVOBIMIZDAN KEYINGI hamma mijoz xabari
// ko'riladi: mijoz bekor qilishni so'rab, ketma-ket yana ikki gap yozsa
// ham so'rov e'tibordan qolmasin. Lekin biz javob bergandan keyin eski
// so'rov qayta ko'tarilmaydi — aks holda xodimlar guruhiga bitta so'rov
// uchun o'nlab ogohlantirish ketardi.
func WantsCancel(msgs []Message) bool {
	// Faqat JAVOB BERILMAGAN xabarlar: butun tarix ko'rilganda bir marta
	// aytilgan "pulimni qaytaring" abadiy qaytaverardi — mijoz keyin
	// "rahmat" yoki "?" deb yozganda ham xodimlar guruhiga o'sha
	// ogohlantirish yana ketardi (read.go: UnansweredClient).
	for _, m := range UnansweredClient(msgs) {
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
	// "otkaz" turkumi — faqat so'z boshida.
	for _, w := range splitWords(t) {
		for _, p := range cancelWordPrefixes {
			if strings.HasPrefix(w, p) {
				return true
			}
		}
	}
	// "buyurtma ... kerak emas" — ikki bo'lak yonma-yon bo'lishi shart emas.
	if containsAny(t, cancelOrderWords) && containsAny(t, cancelNotNeeded) {
		return true
	}
	return false
}

// splitWords - matnni so'zlarga ajratadi: harf va raqamdan boshqa
// hamma narsa ajratuvchi hisoblanadi ("atkaz, qivorila!" → "atkaz",
// "qivorila").
func splitWords(t string) []string {
	return strings.FieldsFunc(t, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// containsAny - ro'yxatdagi bo'laklardan birortasi matnda bormi.
func containsAny(t string, parts []string) bool {
	for _, p := range parts {
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
