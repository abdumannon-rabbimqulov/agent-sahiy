// MUAMMO xabarlarining KUNLIK tartib raqami.
//
// Xodimlar guruhda kuniga o'nlab xabar oladi va ular orasida
// "qaysinisi haqida gapiryapmiz" degan savol tez-tez chiqadi. Muammo
// xabari boshiga "#12" qo'yiladi: raqam har kuni 1 dan boshlanadi
// (27-sentabrda #1…#40 bo'lsa, 28-sentabrda yana #1 dan).
//
// Raqam FAQAT xodim hal qilishi kerak bo'lgan muammoga beriladi —
// yangi muammo (⚠️) va AI "xodim kerak" degani (🆘). Tasdiq ("✅ hal
// qilindi deb belgilandi"), ogohlantirish va rasm xabarlari
// raqamlanmaydi: ilgari ular ham sanalgani uchun raqamlar orasi
// uzilib ketardi va "#8" muammo emas, tasdiq bo'lib chiqardi.
package support

import (
	"fmt"
	"log"
	"time"
)

// TelegramCounter - bir kunlik hisoblagich. Kalit — kun ("2026-09-27"),
// shuning uchun yangi kun avtomatik 1 dan boshlanadi va eski kunlar
// yozuvi tarix bo'lib qoladi.
type TelegramCounter struct {
	Day       string    `gorm:"primaryKey;size:10" json:"day"`
	Count     int       `gorm:"not null;default:0" json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// counterDayLayout - hisoblagich kaliti (mahalliy vaqt bo'yicha kun).
const counterDayLayout = "2006-01-02"

// nextGroupNumber - shu kunning navbatdagi raqami. Baza bilan
// ishlamasa 0 qaytadi va xabar raqamsiz ketadi: raqam xabarning
// yetkazilishidan muhimroq emas.
//
// Raqam yuborishdan OLDIN olinadi. Xabar ketmay qolsa raqam ishlatilmay
// qoladi (ro'yxatda uzilish bo'ladi) — bu takroriy raqamdan afzal:
// ikkita xabar bir xil "#12" bo'lsa, reply kimga tegishligi chalkashadi.
func nextGroupNumber() int {
	if DB == nil {
		return 0
	}
	day := time.Now().Format(counterDayLayout)

	// Natija structga o'qiladi: gorm uchun eng ishonchli yo'l.
	var row struct{ Count int }
	err := DB.Raw(`
		INSERT INTO telegram_counters (day, count, updated_at)
		VALUES (?, 1, now())
		ON CONFLICT (day) DO UPDATE
		   SET count = telegram_counters.count + 1, updated_at = now()
		RETURNING count`, day).Scan(&row).Error
	if err != nil {
		// Raqamsiz bo'lsa ham xabar ketaversin.
		log.Printf("telegram: kunlik raqam olinmadi: %v", err)
		return 0
	}
	return row.Count
}

// SendTelegramIssue - MUAMMO xabarini guruhga yuboradi: matn boshiga
// kunlik tartib raqami qo'yiladi. Muammo bo'lmagan xabarlar to'g'ridan
// to'g'ri SendTelegramMessage orqali ketadi va raqam olmaydi.
//
// Xabar KETMASA raqam qaytarib beriladi (releaseGroupNumber). Ilgari
// bunday emas edi va hisoblagich YUBORILGAN emas, URINILGAN xabarlarni
// sanardi: eslatma to'lqinida Telegram "429 Too Many Requests" qaytaradi,
// o'nlab xabar guruhga umuman chiqmaydi, lekin raqamlari allaqachon
// sarflangan bo'lardi. Shuning uchun kunlik son haqiqiy xabarlar sonidan
// bir necha barobar katta chiqardi (misol: bir kunda 758).
func SendTelegramIssue(text string) (int64, error) {
	n := nextGroupNumber()
	msgID, err := SendTelegramMessage(withGroupNumber(text, n), 0)
	if err != nil {
		releaseGroupNumber(n)
		return 0, err
	}
	return msgID, nil
}

// withGroupNumber - matn boshiga "#N " qo'yadi. Raqam olinmagan bo'lsa
// (n <= 0) matn o'zgarmaydi.
func withGroupNumber(text string, n int) string {
	if n <= 0 {
		return text
	}
	if text == "" {
		return fmt.Sprintf("#%d", n)
	}
	return fmt.Sprintf("#%d %s", n, text)
}

// releaseGroupNumber - ishlatilmay qolgan raqamni qaytaradi.
//
// Shart `count = ?` ataylab: raqam faqat HALI ENG OXIRGI bo'lsa
// qaytariladi. Oradan boshqa xabar o'tib ulgurgan bo'lsa hisoblagich
// tegilmaydi — aks holda keyingi xabar allaqachon berilgan raqamni
// ikkinchi marta olardi va guruhda ikkita "#12" paydo bo'lardi, reply
// esa qaysi biriga tegishli ekani chalkashardi. Ya'ni eng yomoni —
// ro'yxatda bitta raqam tushib qolishi.
func releaseGroupNumber(n int) {
	if DB == nil || n <= 0 {
		return
	}
	day := time.Now().Format(counterDayLayout)
	err := DB.Exec(`
		UPDATE telegram_counters
		   SET count = count - 1, updated_at = now()
		 WHERE day = ? AND count = ?`, day, n).Error
	if err != nil {
		log.Printf("telegram: kunlik raqam %d qaytarilmadi: %v", n, err)
	}
}
