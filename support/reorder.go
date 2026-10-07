// Qayta buyurtma: taqiqlangan tovar o'rniga boshqasini tanlash.
//
// Xodim guruhda qisqa yozadi ("boshqa tovar tanlang"), model esa mijozga
// butun tartibni tushuntiradi: shu summaga boshqa tovar tanlash, "To'lov
// qilish" ni bosib karta ma'lumotisiz qaytish ("to'lov kutilmoqda"
// holatiga o'tishi uchun), keyin bizga yozish.
//
// Lekin bu tartib HAR QANDAY buyurtmaga to'g'ri kelmaydi: pul o'sha
// buyurtmada turgan bo'lishi kerak. Faqat ikki holatda mumkin:
//
//	3  — sotib olingan, to'langan
//	10 — taqiqlangan tovar
//
// Boshqa holatda (posilka yo'lga chiqqan, yetkazilgan, to'lov o'tmagan...)
// mijozga bu ko'rsatmani berish xato: u "To'lov qilish" tugmasini topa
// olmaydi yoki ikkinchi marta pul to'lab yuboradi. Shuning uchun KOD
// tekshiradi: holat mos bo'lmasa javob mijozga ketmaydi, xodim guruhda
// aniq ogohlantirish oladi va o'zi qaror qiladi.
package support

import (
	"fmt"
	"log"
	"strings"
)

// reorderStatuses - qayta buyurtma mumkin bo'lgan holatlar.
var reorderStatuses = []int{StatusPaid, StatusBanned}

// ReorderAllowed - shu holatdagi buyurtmaga boshqa tovar tanlash mumkinmi.
func ReorderAllowed(status int) bool {
	for _, s := range reorderStatuses {
		if status == s {
			return true
		}
	}
	return false
}

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

// reorderBlockPrefix - xodimga ketadigan ogohlantirish boshi. Guruhdagi
// holat matni shu bo'yicha ajratiladi (telegram_updates.go).
const reorderBlockPrefix = "Qayta buyurtma holatga mos emas: "

// ReorderBlocked - xodim "boshqa tovar tanlang" deganda buyurtma holati
// shunga mos kelmasa, sababni qaytaradi (bo'sh satr — hammasi joyida).
//
// Holat ADMINKADAN JONLI olinadi: muammo yozuvidagi status xodim javob
// yozgan paytga kelib eskirgan bo'lishi mumkin.
//
// Tekshirib bo'lmasa (adminka javob bermadi, buyurtma topilmadi) javob
// TO'SILMAYDI: xodimning ishini aloqa uzilgani uchun ushlab qolish
// noto'g'ri bo'lardi — faqat logga yoziladi.
func ReorderBlocked(sns []string) string {
	if len(sns) == 0 {
		return ""
	}
	adm := AdminkaFromEnv()
	if adm.Token == "" {
		return ""
	}
	var bad []string
	for _, sn := range sns {
		sn = strings.TrimSpace(sn)
		if sn == "" {
			continue
		}
		o, err := findOrderBySN(adm, sn)
		if err != nil {
			log.Printf("qayta buyurtma: %s adminkadan olinmadi: %v", sn, err)
			continue
		}
		if o == nil {
			log.Printf("qayta buyurtma: %s adminkada topilmadi", sn)
			continue
		}
		if ReorderAllowed(o.Status) {
			continue
		}
		bad = append(bad, fmt.Sprintf("%s — %s (holat %d)", sn, StatusLabel(o.Status), o.Status))
	}
	if len(bad) == 0 {
		return ""
	}
	return reorderBlockPrefix + strings.Join(bad, "; ") + ". " +
		"Boshqa tovar tanlash faqat \"sotib olingan, to'langan\" (3) yoki " +
		"\"taqiqlangan tovar\" (10) holatida mumkin — javob mijozga YUBORILMADI."
}

// findOrderBySN - buyurtmani raqami bo'yicha adminkadan oladi.
func findOrderBySN(adm Adminka, sn string) (*AdminkaOrder, error) {
	rows, err := FetchOrders(adm, OrderFilter{OrderSN: sn, Size: 5})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if strings.EqualFold(strings.TrimSpace(rows[i].OrderSN), sn) {
			return &rows[i], nil
		}
	}
	return nil, nil
}
