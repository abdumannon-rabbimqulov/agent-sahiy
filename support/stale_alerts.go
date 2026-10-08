// Eski yetkazmalar bo'yicha ogohlantirishlarni xodimdan to'sish.
//
// Yetkazmaga kelganiga oylab bo'lgan posilkalar ham "muddati o'tgan"
// hisoblanadi va har safar xodimlar guruhiga chiqadi: bitta mijozda
// o'nlab eski yozuv bo'lsa, guruhga o'nlab qator yog'iladi va ular
// orasida HOZIR hal qilinishi kerak bo'lgan yangi holat ko'rinmay
// qoladi.
//
// Bunday yozuv bo'yicha qilinadigan ish ham yo'q: 255 kun oldin
// kuryerga berilgan posilka haqida hozir kuryerga qo'ng'iroq qilib
// bo'lmaydi, mijoz ham bu haqda so'ramayapti.
//
// Shuning uchun qoida: yetkazmaga kelganiga STALE_DELIVERY_DAYS dan
// oshgan posilka xodimga CHIQMAYDI — mijoz O'ZI o'sha buyurtma yoki
// trek raqamini yozmagan bo'lsa. Mijoz so'rasa — yoshidan qat'i nazar
// chiqadi: demak bu hali ham ochiq savol.
//
// Modelga ko'rsatiladigan ma'lumot (har bir yozuvning izohi) bu
// filtrdan ta'sirlanmaydi: mijoz so'rasa javob to'g'ri bo'lishi kerak.
// To'siladigan narsa faqat XODIMGA ketadigan ogohlantirish.
package support

import (
	"strconv"
	"strings"
)

// DefaultStaleDeliveryDays - yetkazmaga kelganiga shuncha kundan oshgan
// posilka xodimga chiqarilmaydi (mijoz o'zi so'ramasa).
const DefaultStaleDeliveryDays = 10

// StaleDeliveryDays - .env dagi STALE_DELIVERY_DAYS (standart 10).
// 0 bo'lsa filtr O'CHADI — hamma ogohlantirish chiqadi.
//
// envInt bu yerda ishlamaydi: u 0 ni ham xato deb hisoblab standart
// qiymatni qaytaradi, ya'ni filtrni o'chirib bo'lmas edi.
func StaleDeliveryDays() int {
	v := strings.TrimSpace(envStr("STALE_DELIVERY_DAYS", ""))
	if v == "" {
		return DefaultStaleDeliveryDays
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return DefaultStaleDeliveryDays
	}
	return n
}

// AskedTracks - mijoz O'ZI yozgan raqamlar va o'shalarga tegishli trek
// raqamlari to'plami.
//
// Mijoz DG raqamini yozishi mumkin, ogohlantirish esa trek raqami
// bo'yicha chiqadi — shuning uchun adminka buyurtmalaridan bog'lanish
// topiladi: so'ralgan DG (yoki trek) qaysi buyurtmaga tegishli bo'lsa,
// o'sha buyurtmaning treki ham "so'ralgan" hisoblanadi.
func AskedTracks(numbers []string, views []OrderView) map[string]bool {
	want := make(map[string]bool, len(numbers))
	for _, n := range numbers {
		if k := trackKey(n); k != "" {
			want[k] = true
		}
	}
	if len(want) == 0 {
		return want
	}
	for _, v := range views {
		if !want[trackKey(v.OrderSN)] && !want[trackKey(v.ExpressNum)] {
			continue
		}
		if k := trackKey(v.ExpressNum); k != "" {
			want[k] = true
		}
	}
	return want
}

// DropStaleAlerts - eskirgan ogohlantirishlarni olib tashlaydi va
// qolganlarini matn qilib qaytaradi. Ikkinchi qiymat — tashlanganlar
// soni (log uchun).
//
// `asked` — mijoz o'zi so'ragan treklar (AskedTracks). Shu to'plamdagi
// posilka yoshidan qat'i nazar qoladi.
func DropStaleAlerts(rows []DeliveryAlert, asked map[string]bool) ([]string, int) {
	limit := StaleDeliveryDays()
	out := make([]string, 0, len(rows))
	dropped := 0
	for _, a := range rows {
		if staleAlert(a.ExpressNum, a.Days, asked, limit) {
			dropped++
			continue
		}
		out = append(out, a.Text)
	}
	return out, dropped
}

// staleAlert - shu ogohlantirish eskirganmi (xodimga chiqmasinmi).
//
// Yoshi noma'lum (0) bo'lsa eskirgan deb HISOBLANMAYDI: sana o'qilmay
// qolgani ogohlantirishni yashirish uchun asos emas.
func staleAlert(track string, days int, asked map[string]bool, limit int) bool {
	if limit <= 0 || days <= limit {
		return false
	}
	return !asked[trackKey(strings.TrimSpace(track))]
}
