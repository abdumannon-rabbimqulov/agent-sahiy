// Dastur VAQT MINTAQASI.
//
// Konteynerda `TZ` o'rnatilmagan (Dockerfile'da faqat `tzdata` paketi
// bor) — shuning uchun Go uchun `time.Local` UTC bo'lib qoladi, bazaga
// esa DSN orqali `TimeZone=Asia/Tashkent` beriladi (support/db.go).
// Ikki xil vaqt bir dasturda ishlaganda sana noto'g'ri chiqadi:
//
//   - telegram_number.go dagi KUNLIK hisoblagich kaliti Toshkent yarim
//     tunida emas, ertalab 05:00 da almashardi: kechasi ketgan xabarlar
//     kechagi raqamni davom ettirar (#162 gacha o'sib ketar), keyin
//     ertalab to'satdan #1 ga tushardi;
//   - issue.go dagi `time.ParseInLocation(..., time.Local)` adminkadan
//     kelgan Toshkent vaqtini UTC deb o'qib, kun hisobini 5 soatga
//     surardi.
//
// Shuning uchun mintaqa KODDA o'rnatiladi: deploy muhitiga bog'liq
// bo'lmaydi, `docker run` da TZ unutilsa ham sana to'g'ri qoladi.
package support

import (
	"log"
	"os"
	"time"
)

// DefaultTimezone - .env da ko'rsatilmasa ishlatiladigan mintaqa.
// Baza DSN'idagi default bilan bir xil (support/db.go) — ikkalasi bir
// joydan boshqarilsin.
const DefaultTimezone = "Asia/Tashkent"

// InitTimezone - `time.Local` ni APP_TIMEZONE (bo'lmasa DB_TIMEZONE,
// u ham bo'lmasa DefaultTimezone) bo'yicha o'rnatadi. loadEnv dan
// KEYIN, boshqa hamma ishdan OLDIN chaqiriladi: undan keyin
// `time.Now()`, `time.ParseInLocation(..., time.Local)` va kunlik
// hisoblagich kaliti bir xil mintaqada ishlaydi.
//
// Mintaqa topilmasa (konteynerda tzdata yo'q bo'lsa) dastur
// to'xtamaydi — ogohlantirish yoziladi va eski xatti-harakat qoladi.
func InitTimezone() {
	name := firstEnv("APP_TIMEZONE", "DB_TIMEZONE")
	if name == "" {
		name = DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("ogohlantirish: vaqt mintaqasi %q yuklanmadi (%v) — %s ishlatiladi",
			name, err, time.Local)
		return
	}
	time.Local = loc
	log.Printf("vaqt mintaqasi: %s", name)
}

// firstEnv - ro'yxatdagi birinchi bo'sh bo'lmagan muhit o'zgaruvchisi.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
