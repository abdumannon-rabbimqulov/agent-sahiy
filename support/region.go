// Viloyat (province) va filial (branch_name) nomlarini solishtirish.
//
// Ikki manba ikki xil yozadi: adminkada mijoz ko'rsatgan manzil viloyati
// ("Qoraqalpog'iston"), dashboardda esa posilka turgan filial nomi
// ("Nukus", "SAHIY JIZZAX", "SHOTA"). Ikkalasi bir viloyatga tushishi
// kerak. Tushmasa — posilka mijoz belgilagan manzilga emas, boshqa
// yo'nalishga ketgan.
package support

import "strings"

// O'zbekiston viloyatlari — yagona yozilishi.
const (
	RegionTashkentCity = "Toshkent shahri"
	RegionTashkentProv = "Toshkent viloyati"
	RegionKarakalpak   = "Qoraqalpog'iston"
	RegionAndijan      = "Andijon"
	RegionBukhara      = "Buxoro"
	RegionFergana      = "Farg'ona"
	RegionJizzakh      = "Jizzax"
	RegionNamangan     = "Namangan"
	RegionNavoi        = "Navoiy"
	RegionKashkadarya  = "Qashqadaryo"
	RegionSamarkand    = "Samarqand"
	RegionSirdarya     = "Sirdaryo"
	RegionSurkhandarya = "Surxondaryo"
	RegionKhorezm      = "Xorazm"
)

// regionKeys - matnda uchraydigan kalit so'z → viloyat. Kalitlar
// normRegion() dan o'tgan ko'rinishda (kichik harf, apostrof va
// bo'shliqsiz) yoziladi.
//
// Ro'yxatga filiallarning haqiqiy nomlari ham kiradi ("SHOTA" —
// Toshkentdagi postamat, "SAHIY GULISTION" — Guliston, Sirdaryo),
// chunki filial nomi har doim ham viloyat nomi bilan bir xil emas.
// Uzunroq kalit avval tekshiriladi (masalan "toshkentviloyati"
// "toshkent" dan oldin).
var regionKeys = []struct {
	key    string
	region string
}{
	// Toshkent viloyati — shahar bilan chalkashmasin uchun birinchi.
	{"toshkentviloyati", RegionTashkentProv},
	{"toshkentvil", RegionTashkentProv},
	{"chirchiq", RegionTashkentProv},
	{"olmaliq", RegionTashkentProv},
	{"angren", RegionTashkentProv},
	{"yangiyol", RegionTashkentProv},
	{"bekobod", RegionTashkentProv},
	{"nurafshon", RegionTashkentProv},
	{"ohangaron", RegionTashkentProv},
	{"parkent", RegionTashkentProv},
	{"qibray", RegionTashkentProv},
	{"zangiota", RegionTashkentProv},

	// Toshkent shahri.
	{"toshkentshahri", RegionTashkentCity},
	{"tashkentcity", RegionTashkentCity},
	{"toshkentsh", RegionTashkentCity},
	{"shota", RegionTashkentCity},
	{"toshkent", RegionTashkentCity},
	{"tashkent", RegionTashkentCity},

	{"qoraqalpog", RegionKarakalpak},
	{"qoraqalpok", RegionKarakalpak},
	{"karakalpak", RegionKarakalpak},
	{"nukus", RegionKarakalpak},

	{"andijon", RegionAndijan},
	{"andijan", RegionAndijan},
	{"asaka", RegionAndijan},
	{"xonobod", RegionAndijan},

	{"buxoro", RegionBukhara},
	{"buhoro", RegionBukhara},
	{"gijduvon", RegionBukhara},
	{"kogon", RegionBukhara},

	{"fargona", RegionFergana},
	{"fergana", RegionFergana},
	{"qoqon", RegionFergana},
	{"kokand", RegionFergana},
	{"margilon", RegionFergana},
	{"quvasoy", RegionFergana},

	{"jizzax", RegionJizzakh},
	{"jizzak", RegionJizzakh},

	{"namangan", RegionNamangan},
	{"chortoq", RegionNamangan},
	{"chust", RegionNamangan},

	{"navoiy", RegionNavoi},
	{"navoi", RegionNavoi},
	{"zarafshon", RegionNavoi},

	{"qashqadaryo", RegionKashkadarya},
	{"kashkadar", RegionKashkadarya},
	{"qarshi", RegionKashkadarya},
	{"shahrisabz", RegionKashkadarya},

	{"samarqand", RegionSamarkand},
	{"samarkand", RegionSamarkand},
	{"urgut", RegionSamarkand},
	{"kattaqorgon", RegionSamarkand},

	{"sirdaryo", RegionSirdarya},
	{"sirdarya", RegionSirdarya},
	{"gulistion", RegionSirdarya},
	{"guliston", RegionSirdarya},
	{"yangiyer", RegionSirdarya},

	{"surxondaryo", RegionSurkhandarya},
	{"surxandaryo", RegionSurkhandarya},
	{"termiz", RegionSurkhandarya},
	{"denov", RegionSurkhandarya},

	{"xorazm", RegionKhorezm},
	{"horazm", RegionKhorezm},
	{"urganch", RegionKhorezm},
	{"xiva", RegionKhorezm},
}

// normRegion - nomni solishtirishga tayyorlaydi: kichik harf, apostrof,
// bo'shliq va tinish belgilari tashlanadi, "o'"/"g'" oddiy "o"/"g" ga
// tushadi. "SAHIY GULISTION", "Guliston" va "guliston shahri" — bitta
// ko'rinishga keladi.
func normRegion(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case 'ʻ', 'ʼ', '‘', '’', '`', '\'', '´', ' ', '-', '.', ',', '"':
			continue // apostrof va ajratgichlar tashlanadi
		case 'ў':
			b.WriteRune('o')
		case 'ғ':
			b.WriteRune('g')
		case 'қ':
			b.WriteRune('q')
		case 'ҳ':
			b.WriteRune('h')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RegionOf - matndan (viloyat nomi yoki filial nomi) viloyatni
// aniqlaydi. Tanilmasa bo'sh satr qaytadi — bunday holatda hech qanday
// xulosa chiqarilmaydi (noma'lum nomni "mos emas" deb hisoblamaymiz).
func RegionOf(s string) string {
	n := normRegion(s)
	if n == "" {
		return ""
	}
	for _, k := range regionKeys {
		if strings.Contains(n, k.key) {
			return k.region
		}
	}
	return ""
}

// HomeDeliveryRegion - shu viloyatda kuryer mijozning uyiga olib
// boradimi. Faqat Toshkent shahri va Toshkent viloyati; qolgan hamma
// joyda mijoz posilkani filialdan (punktdan) o'zi olib ketadi.
func HomeDeliveryRegion(region string) bool {
	return region == RegionTashkentCity || region == RegionTashkentProv
}

// DeliveryKindText - mijozning viloyatiga qarab yetkazish turi.
// Model shu tayyor matnga tayanadi, o'zi taxmin qilmasin.
func DeliveryKindText(region string) string {
	switch {
	case region == "":
		return ""
	case HomeDeliveryRegion(region):
		return "kuryer mijozning manziliga olib boradi (" + region + ")"
	default:
		return "mijoz filialdan (punktdan) o'zi olib ketadi — " + region +
			" viloyatida uyga yetkazish yo'q"
	}
}
