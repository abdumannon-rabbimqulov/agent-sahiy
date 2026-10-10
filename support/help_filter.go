// Guruhga chiqmaydigan "yordam kerak" xabarlari.
//
// Murojaatning eng ko'p uchraydigan turi: mijoz muammo haqida yozdi,
// lekin buyurtma raqamini yozmadi — AI undan raqamni so'radi va xulosa
// o'rniga "buyurtma raqami so'ralmoqda" deb yozdi. Bunday xabar guruhga
// chiqqanda xodimning qo'lidan hech narsa kelmaydi: buyurtma noma'lum,
// javob esa allaqachon mijozga ketgan. Guruh shunday bo'sh xabarlarga
// to'lib, orasidagi haqiqiy murojaatlar ko'zdan qochardi.
//
// Shuning uchun qoida: xulosada ANIQ narsa — buyurtma yoki trek raqami —
// bo'lsa xabar chiqadi; raqam yo'q va xulosa faqat "raqam so'raldi"
// degan ma'noni bildirsa, chiqmaydi.
//
// Kod topgan holatlar (in.Alerts) va ochilgan muammoli buyurtmalar bu
// filtrga tushmaydi: ularni model emas, tizim topgan va ularda doim
// aniq buyurtma bor.
package support

import "strings"

// askNumberPhrases - "mijozdan buyurtma raqami so'raldi" ma'nosidagi
// iboralar. Xulosani model yozadi, shuning uchun ibora bir xil emas.
var askNumberPhrases = []string{
	"raqami so'ral", "raqami soral", "raqamini so'ra", "raqamini sora",
	"raqam so'ral", "raqam soral", "raqami yo'q", "raqami yoq",
	"raqami keltirilmagan", "raqami ko'rsatilmagan", "raqami korsatilmagan",
	"raqami aniqlanmadi", "raqami topilmadi", "raqami berilmagan",
	"raqami noma'lum", "raqami nomalum",
	// Kirill va rus tilidagi variantlar.
	"рақами сўрал", "рақами йўқ", "номер заказа запрош",
	"номер заказа не указан", "запрашивается номер",
}

// helpOnlyAsksNumber - xulosa faqat "mijozdan buyurtma raqami so'raldi"
// degan ma'noni bildiradimi (ya'ni xodimga ish yo'q).
//
// Matnda buyurtma yoki trek raqami bo'lsa — FALSE: raqam bor ekan,
// xodim tekshira oladi, xabar guruhga chiqishi kerak.
func helpOnlyAsksNumber(help string) bool {
	t := strings.ToLower(strings.TrimSpace(help))
	if t == "" {
		return false
	}
	if hasAnyNumber(help) {
		return false
	}
	for _, p := range askNumberPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// hasAnyNumber - matnda buyurtma (DG…) yoki trek raqami bormi.
func hasAnyNumber(text string) bool {
	t := cleanForNumbers(cyrToLatNum(text))
	return orderSNRe.MatchString(t) ||
		letterTrackRe.MatchString(t) ||
		digitTrackRe.MatchString(t)
}
