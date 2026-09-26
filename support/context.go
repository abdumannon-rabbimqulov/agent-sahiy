// Modelga ketadigan tizim ma'lumotini saralash.
//
// Xom javoblar katta (bitta buyurtma ~10 KB) va modelga hammasi kerak
// emas: har ortiqcha maydon token va chalkashlik. Shu yerda faqat
// javob yozish uchun zarur bo'lgan maydonlar qoladi.
package support

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DeliveryDays - kuryerga berilgan buyurtma qancha muddatda yetib
// borishi kerak. Shu muddat ichida yetkazma "yetkazilmoqda"; undan
// oshsa holati NOANIQ — yetkazilgan bo'lishi ham, mijozning telefoni
// o'chiq bo'lgani uchun kuryer qaytargan bo'lishi ham mumkin.
const DeliveryDays = 3

// Mijoz turlari.
const (
	CustomerB2C     = "B2C (oddiy mijoz)"
	CustomerB2B     = "B2B (ulgurji mijoz)"
	CustomerUnknown = "noma'lum"
)

// CustomerType - buyurtmalardagi B2C_percentage bo'yicha mijoz turi.
// Noldan katta bo'lsa oddiy mijoz, aniq nol bo'lsa ulgurji; buyurtma
// topilmasa yoki maydon bo'sh bo'lsa "noma'lum".
func CustomerType(orders []AdminkaOrder) string {
	found := false
	for _, o := range orders {
		if o.B2CPercentage > 0 {
			return CustomerB2C
		}
		if o.PayStatus > 0 || o.OrderSN != "" {
			found = true
		}
	}
	if found {
		return CustomerB2B
	}
	return CustomerUnknown
}

// OrderBrief - modelga ketadigan buyurtma (faqat kerakli maydonlar).
type OrderBrief struct {
	OrderSN     string `json:"order_sn"`
	StatusLabel string `json:"status_label"`
	Paid        bool   `json:"paid"`
	PaidAt      string `json:"paid_at,omitempty"`
	Days        int    `json:"days_since_paid,omitempty"`
	Problem     bool   `json:"problem,omitempty"`
	InReview    bool   `json:"tekshiruvda,omitempty"`
	ExpressNum  string `json:"express_num,omitempty"`
	ShippedAt   string `json:"shipped_at,omitempty"`
	// ShippedDays - jo'natilganiga necha kun bo'lgani. Sana o'qilmasa 0.
	ShippedDays int    `json:"jonatilganiga_kun,omitempty"`
	PackageName string `json:"package_name,omitempty"`

	// StatusNote - status nimani bildiradi. Ayniqsa status 6 uchun
	// kerak: adminkadagi "yakunlangan" — Xitoy tomonidagi tranzaksiya
	// yopilgani, mijozga yetgani emas.
	StatusNote string `json:"status_izoh,omitempty"`
	// Arrived - posilka yetkazmada (dashboardda) chiqdimi. Faqat
	// yetkazma ma'lumoti ham olingan bo'lsa to'ldiriladi.
	Arrived string `json:"yetkazma,omitempty"`

	// Region - buyurtma manzili viloyati (adminkadagi `province`),
	// Kind - o'sha viloyatda yetkazish qanday ishlashi. Ikkalasi ham
	// tayyor matn: model uyga yetkazish bor-yo'qligini o'zi taxmin
	// qilmasin — Toshkent shahri va Toshkent viloyatidan boshqa hamma
	// joyda mijoz posilkani filialdan o'zi olib ketadi.
	Region string `json:"viloyat,omitempty"`
	Kind   string `json:"yetkazish,omitempty"`

	// Mismatch - posilka mijoz viloyatiga tushmagan (MarkMismatch).
	// To'ldirilgan bo'lsa `yetkazish` bo'sh qoladi: mijozga qayerdan
	// olishini aytib bo'lmaydi, avval xodim tuzatishi kerak.
	Mismatch string `json:"DIQQAT_xatolik,omitempty"`

	// OwnerUserID - buyurtma egasi, faqat u hozirgi mijoz BO'LMAGANDA
	// to'ldiriladi. Buyurtma raqami bo'yicha qidiruv adminkaning butun
	// bazasidan qidiradi: mijoz boshqa odamning DG raqamini yozsa ham
	// buyurtma topiladi. Model buni ko'rib tursin — begona buyurtma
	// tafsilotini mijozga aytib yubormasin.
	OwnerUserID int64 `json:"boshqa_mijozning_buyurtmasi,omitempty"`

	// AskReceived - posilka jo'natilganiga LongShipmentDays dan oshgan,
	// lekin yetkazmada hali chiqmagan. Bunda "yo'lda" deb javob berish
	// kam: mijoz posilkani allaqachon olgan, tizimda belgilanmay qolgan
	// bo'lishi mumkin. Tayyor ko'rsatma — model shuni bajarsin.
	AskReceived string `json:"DIQQAT_mijozdan_sora,omitempty"`
}

// BriefOrders - buyurtmalarni ixchamlashtiradi. Sanalar odam o'qiydigan
// ko'rinishga o'tkaziladi (model xom "2026-08-21 16:43:54" dan foydali
// narsa yoza olmaydi, faqat token yeydi).
func BriefOrders(views []OrderView, clientID int64) []OrderBrief {
	out := make([]OrderBrief, 0, len(views))
	for _, v := range views {
		b := OrderBrief{
			OrderSN:     v.OrderSN,
			StatusLabel: v.StatusLabel,
			Paid:        v.Paid,
			Days:        v.DaysSincePaid,
			Problem:     v.Problem,
			InReview:    v.InReview,
			ExpressNum:  v.ExpressNum,
			PackageName: trimText(v.PackageName, 60),
		}
		if v.Paid {
			b.PaidAt = sanaMatn(paidAtOr(v.AdminkaOrder))
		}
		if v.ShippedAt != "" {
			b.ShippedAt = sanaMatn(v.ShippedAt)
			b.ShippedDays = daysSinceText(v.ShippedAt)
		}
		if clientID > 0 && v.UserID > 0 && v.UserID != clientID {
			b.OwnerUserID = v.UserID
		}
		b.StatusNote = StatusMeaning(v.Status)
		if r := RegionOf(v.Province); r != "" {
			b.Region = r
			b.Kind = DeliveryKindText(r)
		}
		out = append(out, b)
	}
	return out
}

// Yetkazmada bor-yo'qligini bildiradigan tayyor matnlar.
const (
	arrivedInDelivery = "yetkazmada bor — posilka O'zbekistonga kelgan, " +
		"aniq holati \"yetkazma\" bo'limida"
	notInDelivery = "yetkazmada hali yo'q — posilka Xitoydan yo'lda, " +
		"O'zbekistonga kelmagan"
)

// MarkArrival - har bir buyurtmaga uning posilkasi yetkazmada (dashboardda)
// chiqqan-chiqmagani yoziladi.
//
// Adminkadagi status buni AYTMAYDI: u "yakunlangan" bo'lsa ham posilka
// hali yo'lda bo'lishi mumkin. Ikki manba trek raqami (`express_num`)
// orqali bog'lanadi.
func MarkArrival(briefs []OrderBrief, delivery []DeliveryOrder) {
	seen := make(map[string]bool, len(delivery))
	for _, d := range delivery {
		if t := strings.TrimSpace(d.ExpressNum); t != "" {
			seen[t] = true
		}
	}
	for i := range briefs {
		track := strings.TrimSpace(briefs[i].ExpressNum)
		if track == "" {
			// Trek hali berilmagan — posilka Xitoyda, yo'lga chiqmagan.
			continue
		}
		if seen[track] {
			briefs[i].Arrived = arrivedInDelivery
		} else {
			briefs[i].Arrived = notInDelivery
			// Jo'natilganiga juda ko'p bo'lgan bo'lsa, "yo'lda" deyish
			// yetarli emas — avval mijozdan olgan-olmagani so'raladi.
			if briefs[i].ShippedDays > LongShipmentDays() {
				briefs[i].AskReceived = askReceivedNote
			}
		}
	}
}

// DefaultLongShipmentDays - posilka jo'natilganidan keyin shu kundan
// ko'p yetkazmada chiqmasa, holat oddiy "yo'lda" emas: yo tizimda
// yozuv tushmay qolgan, yo mijoz allaqachon olgan.
const DefaultLongShipmentDays = 30

// LongShipmentDays - .env dagi LONG_SHIPMENT_DAYS (default 30).
func LongShipmentDays() int { return envInt("LONG_SHIPMENT_DAYS", DefaultLongShipmentDays) }

// askReceivedNote - jo'natilganiga 30 kundan oshgan, lekin yetkazmada
// chiqmagan buyurtma uchun modelga tayyor ko'rsatma.
//
// Bu holatda model ilgari shunchaki "Xitoydan yo'lda" deb yozardi va
// suhbat shu yerda tugardi — mijoz posilkani allaqachon olgan bo'lsa
// ham (tizimda belgilanmay qolgan) hech kim buni bilmasdi.
const askReceivedNote = "Buyurtma jo'natilganiga 30 kundan oshgan, lekin yetkazmada hali " +
	"chiqmagan. Javobni SAVOL bilan tugat: mijozdan shu buyurtmani allaqachon olgan-olmaganini " +
	"so'ra (tizimda belgilanmay qolgan bo'lishi mumkin). \"Yo'lda\" deb qat'iy aytma va muddat " +
	"va'da qilma; mijoz olmaganini aytsa — xodimlar tekshirishini ayt."

// daysSinceText - sana matnidan bugungacha o'tgan kun. Sana o'qilmasa
// yoki kelajakda bo'lsa 0 (modelga ma'nosiz son ketmasin).
func daysSinceText(s string) int {
	t, ok := parseAnyTime(s)
	if !ok {
		return 0
	}
	d := int(time.Since(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// PendingPickup - posilka O'zbekistonda, lekin hali mijozga
// topshirilmagan (kuryerga ham berilmagan).
//
// JSON kalitlari ataylab BETARAF: ilgari ro'yxat "olinmagan", joy esa
// "filial" deb atalardi va model shu ikki so'zga qarab, izohni o'qimay
// turib "borib olib keting" deb javob yozardi — posilka Markaziy omborda
// turgan va kuryer olib boradigan holatda ham. Endi kalitlar joyni
// bildiradi, xulosani emas: nima qilish kerakligi faqat `izoh`da.
type PendingPickup struct {
	ExpressNum string `json:"express_num"`
	// Branch - posilka HOZIR qayerda turgani. Bu mijoz boradigan manzil
	// degani emas: "Markaziy ombor" ichki taqsimlash nuqtasi.
	Branch    string `json:"hozir_qayerda"`
	Address   string `json:"shu_joyning_manzili,omitempty"`
	ArrivedAt string `json:"kelgan,omitempty"`
	// Region - mijozning viloyati (dashboarddagi `city`).
	Region string `json:"mijoz_viloyati,omitempty"`
	// Izoh - mijozga nima deyilishi kerakligi. Tayyor matn: model buni
	// qayta talqin qilmasin, shundayligicha aytsin — promt o'zgarmaydi.
	// Bu maydon boshqa hamma maydondan USTUN.
	Izoh string `json:"mijozga_nima_deyiladi,omitempty"`
}

// centralWarehouseBranch - "filial" markaziy ombor bo'lganda kelib
// tushadigan nom. Dashboarddan qaytadigan haqiqiy qiymat, kod ichida
// hardcode qilingan doim bir xil bo'lmasligi mumkin — shuning uchun
// solishtirish katta-kichik harf va bo'shliqqa sezgir emas.
const centralWarehouseBranch = "markaziy ombor"

// centralWarehouseNote - Markaziy omborda turgan (hali filialga
// jo'natilmagan) buyurtma uchun mijozga tayyor holatda beriladigan
// izoh. Toshkent shahri va viloyati uchun: u yerda kuryer manzilga
// olib boradi.
//
// Taqiq alohida yozilgan: Markaziy ombor mijoz boradigan punkt EMAS,
// lekin model "ombor" so'zini ko'rib "borib olib keting" deb yozib
// yuborardi — mijoz bekorga yo'lga chiqadi.
const centralWarehouseNote = "Buyurtma O'zbekistonga kelgan, hozir Markaziy omborda saralanmoqda. " +
	"Tez orada KURYER mijoz belgilagan manzilga olib boradi. " +
	"Markaziy ombor mijoz boradigan punkt EMAS — mijozga \"olib keting\", " +
	"\"omborga boring\" yoki \"filialdan oling\" DEMA."

// centralWarehouseRegionNote - xuddi shu holat, lekin mijoz Toshkentdan
// tashqarida: u yerda uyga yetkazish YO'Q, posilka mijoz viloyatidagi
// filialga jo'natiladi va mijoz o'sha yerdan oladi.
const centralWarehouseRegionNote = "Buyurtma O'zbekistonga kelgan, hozir Markaziy omborda " +
	"(Toshkentda) saralanmoqda. Tez orada mijoz viloyatidagi filialga jo'natiladi va mijoz " +
	"o'sha filialdan oladi. Posilka HALI mijoz viloyatiga yetmagan — mijozga hozir " +
	"\"borib olib keting\" DEMA, avval filialga yetib borishini kutish kerak."

// courierPendingNote - posilka mijoz viloyatidagi haqiqiy punktda, lekin
// bu viloyatda (Toshkent shahri/viloyati) yetkazishni kuryer bajaradi:
// mijoz o'zi borishi shart emas.
const courierPendingNote = "Posilka mijoz viloyatidagi punktda, kuryerga berilishi kutilmoqda. " +
	"Bu viloyatda KURYER manzilga olib boradi — mijozga \"o'zingiz borib oling\" DEMA."

// pickupNote - posilka mijozning o'z viloyatidagi filialda: olib
// ketishi kerak, uyiga olib borilmaydi.
const pickupNote = "Posilka mijoz viloyatidagi filialga yetib kelgan va tayyor — mijoz o'zi " +
	"borib olib ketadi (bu viloyatda uyga yetkazish yo'q). Filial nomi va manzilini ayt."

// pickupBranchPrefix - mijoz o'zi borib olib keta oladigan jismoniy
// punkt bo'lsa, location_number shu prefiks bilan boshlanadi (masalan
// "SHOTA-28" — branch_name "SHOTA"). Boshqa qiymat kelsa, bu hali
// mijozga ochiq punkt emas — Markaziy ombordagi kabi talqin qilinadi.
const pickupBranchPrefix = "SHOTA"

// isPickupBranch - location_number pickupBranchPrefix bilan
// boshlansa, bu haqiqiy o'zi-olib-ketish punkti.
func isPickupBranch(locationNumber string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(locationNumber)), pickupBranchPrefix)
}

// SentDelivery - kuryerga berilgan yetkazma (delivered = true).
//
// `Days` — berilganiga necha kun bo'lgani. Shu son ikki ro'yxatni
// ajratadi: DeliveryDays ichida — yo'lda, undan oshsa — noaniq.
type SentDelivery struct {
	ExpressNum string `json:"express_num,omitempty"`
	Branch     string `json:"filial,omitempty"`
	SentAt     string `json:"berilgan,omitempty"` // qachon kuryerga berilgan
	Days       int    `json:"kun"`                // berilganiga necha kun
	// Region - mijozning viloyati (dashboarddagi `city`).
	Region string `json:"mijoz_viloyati,omitempty"`
	// Izoh - kod tayyorlagan izoh (masalan filial viloyati mos emas).
	Izoh string `json:"izoh,omitempty"`
}

// DeliveryBrief - yetkazma bo'yicha modelga ketadigan xulosa.
type DeliveryBrief struct {
	// Pending — posilka O'zbekistonda, lekin hali mijozga topshirilmagan
	// va kuryerga ham berilmagan (delivered = false). Kalit "olinmagan"
	// emas: u "mijoz borib olmagan" degan ma'noni berib, modelni har
	// safar "olib keting" deyishga undardi.
	Pending []PendingPickup `json:"mijozga_topshirilmagan,omitempty"`
	// Kuryerga berilgan va muddati o'tmagan — hozir yo'lda.
	InDelivery []SentDelivery `json:"yetkazilmoqda,omitempty"`
	// Kuryerga berilganiga DeliveryDays dan oshgan — holati noaniq,
	// xodim tekshirishi kerak.
	NeedCheck []SentDelivery `json:"tekshirish_kerak,omitempty"`
	// O'zi-olib-ketish turida (express_line "Pickup"), mijoz
	// allaqachon filialdan olib ketgan (status=2, delivered=true).
	PickedUp []PickupDone `json:"olib_ketilgan,omitempty"`
	// Umuman yozuv yo'q.
	Empty bool `json:"yozuv_yoq,omitempty"`
	// Kind - mijozning viloyatida yetkazish qanday ishlaydi. Tayyor
	// matn: model o'zi taxmin qilmasin ("uyga olib boramiz" deb
	// noto'g'ri va'da bermasin).
	Kind string `json:"yetkazish_turi,omitempty"`
}

// BranchMismatch - posilka mijoz viloyatidagi filialda emas.
//
// Mijoz manzili (dashboarddagi `city`) va posilka turgan filial
// (`branch_name`) har xil viloyatga tushsa, posilka mijoz belgilagan
// manzilga yetmagan bo'ladi. Bu kod chiqaradigan xulosa — model buni
// o'zi topishi shart emas, xodimga yuboriladi.
type BranchMismatch struct {
	ExpressNum   string
	Region       string // mijoz viloyati
	Branch       string // posilka turgan filial
	BranchRegion string // o'sha filial qaysi viloyatda
}

// Text - xodimlar guruhiga va modelga ketadigan bir qatorlik izoh.
func (m BranchMismatch) Text() string {
	return fmt.Sprintf("%s — posilka mijoz viloyatiga (%s) emas, %s filialiga (%s) tushgan",
		m.ExpressNum, m.Region, m.Branch, m.BranchRegion)
}

// mismatchNote - shu holatda modelga beriladigan tayyor ko'rsatma.
// Mijozga va'da berilmaydi: xodim tekshiradi.
const mismatchNote = "Posilka mijoz viloyatidagi filialda emas — xodimga topshirildi. " +
	"Mijozga faqat \"tekshirilmoqda\" deb ayt, sabab yoki muddat aytma."

// alertKey - kod topgan holatlar JSON'da shu kalit ostida ketadi.
// Nomi ataylab baland: model uni e'tibordan chetda qoldirmasin.
const alertKey = "DIQQAT_muammo_topildi"

// alertGuidance - kod muammo topganda modelga beriladigan ko'rsatma.
//
// Bu ro'yxatdagi holat qolgan HAMMA maydondan ustun turadi. Sabab: xato
// holatda boshqa maydonlar (buyurtma viloyati, yetkazish turi) hamon
// "to'g'ri" ko'rinadi va model o'shalarga qarab mijozga ishonch bilan
// noto'g'ri ko'rsatma yozadi — masalan posilka Sirdaryoda turganda
// "Jizzax filialiga borib oling" deb. Mijoz bekorga yo'lga chiqadi.
const alertGuidance = "Tizim bu buyurtmada XATOLIK topdi va uni xodimga topshirdi. " +
	"Bu ro'yxat quyidagi hamma maydondan USTUN. " +
	"Mijozga posilka qayerdaligini, qaysi filialdan olishini yoki qachon yetishini AYTMA — " +
	"bu ma'lumotlar hozir ishonchsiz. " +
	"Mijozga shuni ayt: buyurtmasida xatolik aniqlandi, uzr so'raymiz, " +
	"xodimlarimiz tuzatish ustida ishlamoqda va hal bo'lishi bilan xabar beramiz. " +
	"Sabab, muddat yoki filial nomini aytma."

// MarkMismatch - posilka mijoz viloyatiga tushmagan buyurtmalarni
// belgilaydi va ularning "qayerdan olib ketish" ko'rsatmasini O'CHIRADI.
//
// Bu maydon (`yetkazish`) mijozning O'Z viloyatidan hisoblanadi, posilka
// qayerda turganidan emas. Posilka boshqa viloyatga tushganda u to'g'ri
// ko'rinib turadi-yu, aslida noto'g'ri bo'ladi — model shunga qarab
// mijozga "o'z viloyatingizdagi filialdan oling" deb yozib yuboradi.
// Ikkala manba trek raqami (`express_num`) orqali bog'lanadi.
func MarkMismatch(briefs []OrderBrief, bad []BranchMismatch) {
	if len(bad) == 0 {
		return
	}
	byTrack := make(map[string]BranchMismatch, len(bad))
	for _, m := range bad {
		if t := strings.TrimSpace(m.ExpressNum); t != "" {
			byTrack[t] = m
		}
	}
	for i := range briefs {
		m, ok := byTrack[strings.TrimSpace(briefs[i].ExpressNum)]
		if !ok {
			continue
		}
		briefs[i].Kind = "" // "o'z viloyatingizdan oling" — bu yerda noto'g'ri
		briefs[i].Mismatch = m.Text() + ". " + mismatchNote
	}
}

// PickupDone - o'zi-olib-ketish turidagi jo'natma, mijoz allaqachon
// filialdan olib ketgan (express_line "Pickup", status=2,
// delivered=true).
type PickupDone struct {
	ExpressNum string `json:"express_num,omitempty"`
	Branch     string `json:"filial,omitempty"`
	PickedAt   string `json:"olingan,omitempty"`
	// Region - mijozning viloyati (dashboarddagi `city`).
	Region string `json:"mijoz_viloyati,omitempty"`
	// Izoh - mijozga nima deyilishi kerakligi. PendingPickup dagi kabi
	// tayyor matn va HECH QACHON bo'sh qolmaydi: bo'sh bo'lganda model
	// qolgan maydonlarga qarab o'zi xulosa chiqarardi va mijozga
	// "kuryer qaytargan bo'lishi mumkin" deb yozardi — kuryer bu
	// jo'natmaga umuman tegishli emas.
	Izoh string `json:"mijozga_nima_deyiladi,omitempty"`
}

// pickedUpNote - posilka filialdan olib ketilgan holat uchun tayyor
// ko'rsatma.
//
// Bu jo'natma KURYERGA berilmagan: u o'zi-olib-ketish turida, filialda
// qo'lma-qo'l topshirilgan. Shuning uchun "kuryer qaytargan",
// "telefoningiz o'chiq bo'lgan" kabi izohlar bu yerda noto'g'ri —
// model ularni boshqa holatlardan ko'chirib yozardi.
const pickedUpNote = "Yetkazma ma'lumotida bu posilka FILIALDAN OLIB KETILGAN deb turadi " +
	"(o'zi-olib-ketish turi, olingan sanasi bor). Mijozga qaysi filialdan va qachon " +
	"olib ketilgani aytiladi. Bu jo'natma KURYERGA BERILMAGAN — \"kuryer\", " +
	"\"kuryer qaytargan\", \"telefoningiz o'chiq bo'lgan\" deb yozma. " +
	"Mijoz \"olmadim\" desa: buyurtmani O'ZI yoki yaqinlaridan biri olib ketgan " +
	"bo'lishi mumkinligini xushmuomala so'ra; baribir olmagan bo'lsa xodimlar " +
	"tekshirishini ayt — muddat va'da qilma."

// pickedUpRegionNote - xuddi shu holat, lekin mijoz Toshkentdan
// tashqarida: u yerda uyga yetkazishning O'ZI yo'q, shuning uchun
// kuryer haqidagi har qanday gap yanada noto'g'ri.
const pickedUpRegionNote = pickedUpNote + " Mijoz viloyatida uyga yetkazish UMUMAN YO'Q " +
	"(kuryer faqat Toshkent shahri va Toshkent viloyatida) — mijozga kuryer haqida hech narsa aytma."

// noCourierRegionNote - mijoz viloyatida kuryer yetkazish yo'q: posilka
// filialdan beriladi, mijoz o'zi borib oladi.
//
// "Kuryerga berilgan" ko'rinishidagi yozuvlarga (yetkazilmoqda /
// tekshirish_kerak) shu izoh qo'shiladi: model ularni Toshkentdagi
// kuryer holati deb o'qib, mijozga "kuryer qaytargan bo'lishi mumkin"
// deb yozardi.
const noCourierRegionNote = "Mijoz viloyatida KURYER yetkazish YO'Q (uyga yetkazish faqat " +
	"Toshkent shahri va Toshkent viloyatida) — posilka filialdan beriladi, mijoz o'zi borib oladi. " +
	"Javobingda \"kuryer\", \"uyga olib boramiz\", \"kuryer qaytargan\", \"telefoningiz o'chiq edi\" " +
	"kabi gaplarni ISHLATMA. Mijozdan so'ra: buyurtmani O'ZINGIZ yoki yaqiningiz filialdan " +
	"olib ketgan bo'lishi mumkinmi? Olmagan bo'lsa xodimlar tekshiradi — muddat va'da qilma."

// expressLineKind - express_line matnidan jo'natma turini aniqlaydi:
// "pickup" (mijoz o'zi olib ketadi) yoki "delivery" (kuryer olib
// boradi). Noma'lum bo'lsa "" qaytaradi.
func expressLineKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "pickup"):
		return "pickup"
	case strings.Contains(lower, "delivery"):
		return "delivery"
	default:
		return ""
	}
}

// MaxDeliveryRows - har bir ro'yxatdan modelga ketadigan eng ko'p yozuv.
// Mijozda o'nlab eski yetkazma bo'lishi mumkin — hammasi javobga kerak
// emas, faqat token yeydi.
const MaxDeliveryRows = 5

// BriefDelivery - yetkazma yozuvlarini UCHGA ajratadi:
//
//   - olinmagan (delivered = false): filialda kutmoqda;
//   - yetkazilmoqda (delivered = true, DeliveryDays ichida): kuryerga
//     berilgan, hozir yo'lda;
//   - tekshirish_kerak (delivered = true, DeliveryDays dan oshgan):
//     holati NOANIQ — mijoz olgan bo'lishi ham, telefoni o'chiq bo'lgani
//     uchun kuryer qaytargan bo'lishi ham mumkin.
//
// `delivered = true` "mijoz qo'liga tegdi" degani EMAS: u faqat
// yetkazmaga berilganini bildiradi. Shuning uchun muddati o'tganini
// "yetkazildi" deb aytib bo'lmaydi — tekshirish kerak.
//
// Ikkala ro'yxat ham yangisidan eskisiga saralanadi va MaxDeliveryRows
// tadan oshmaydi.
func BriefDelivery(orders []DeliveryOrder) (DeliveryBrief, []BranchMismatch) {
	var out DeliveryBrief
	var bad []BranchMismatch
	now := time.Now()

	for _, o := range orders {
		// Mijoz viloyati dashboarddagi `city` da keladi ("Toshkent
		// shahri", "Qoraqalpog'iston"), posilka turgan joy esa
		// `branch_name` da ("SHOTA", "Nukus"). Ikkalasi bir viloyatga
		// tushmasa — posilka mijoz belgilagan manzilga ketmagan.
		region := RegionOf(o.City)
		branchRegion := RegionOf(o.BranchName)
		mismatch := region != "" && branchRegion != "" && region != branchRegion
		if mismatch {
			bad = append(bad, BranchMismatch{
				ExpressNum:   o.ExpressNum,
				Region:       region,
				Branch:       firstNonEmpty(o.BranchName, o.LocationNumber),
				BranchRegion: branchRegion,
			})
		}
		if out.Kind == "" {
			out.Kind = DeliveryKindText(region)
		}

		// O'zi-olib-ketish turi + status=7 + delivered=true — mijoz
		// buyurtmani ALLAQACHON o'zi olib ketgan. Boshqa bucketlarga
		// (ayniqsa "tekshirish_kerak"ga) tushmasin — yakunlangan holat.
		if o.Delivered && o.Status == 7 && expressLineKind(o.ExpressLine) == "pickup" {
			izoh := pickedUpNote
			if region != "" && !HomeDeliveryRegion(region) {
				izoh = pickedUpRegionNote
			}
			out.PickedUp = append(out.PickedUp, PickupDone{
				ExpressNum: o.ExpressNum,
				Branch:     firstNonEmpty(o.BranchName, o.LocationNumber),
				PickedAt:   sanaMatnISO(o.DeliveredAt),
				Region:     region,
				Izoh:       izoh,
			})
			continue
		}

		// Kuryerga yetkazish turida `delivered` maydoni har doim ham
		// o'z vaqtida yangilanmaydi — status=8 kuryer buyurtmani
		// olganini (yo'lda ekanini) alohida bildiradi.
		givenToCourier := o.Delivered ||
			(o.Status == 8 && expressLineKind(o.ExpressLine) == "delivery")

		if !givenToCourier {
			branch := firstNonEmpty(o.BranchName, o.LocationNumber)
			row := PendingPickup{
				ExpressNum: o.ExpressNum,
				Branch:     branch,
				Address:    trimText(plainVal(o.BranchAddress), 80),
				ArrivedAt:  sanaMatnISO(o.CreatedAt),
				Region:     region,
			}
			// Izoh tanlash tartibi: avval xato holat, keyin "hali yo'lda",
			// oxirida oddiy "kelib bo'ldi, olib keting". Izoh HECH QACHON
			// bo'sh qolmasligi kerak: bo'sh bo'lsa model qolgan maydonlarga
			// qarab o'zi xulosa chiqaradi va noto'g'ri ko'rsatma beradi.
			central := strings.EqualFold(strings.TrimSpace(branch), centralWarehouseBranch)
			// Filial tanilmasa va SHOTA punkti ham bo'lmasa — bu hali
			// mijozga ochiq punkt emas, Markaziy ombordagidek talqin
			// qilinadi.
			notOpenYet := branchRegion == "" && !isPickupBranch(o.LocationNumber)
			switch {
			case mismatch:
				// Viloyat mos emas — bu hamma izohdan muhimroq.
				row.Izoh = mismatchNote
			case central || notOpenYet:
				if HomeDeliveryRegion(region) || region == "" {
					row.Izoh = centralWarehouseNote
				} else {
					row.Izoh = centralWarehouseRegionNote
				}
			case region != "" && !HomeDeliveryRegion(region):
				row.Izoh = pickupNote
			default:
				// Toshkent shahri/viloyati + haqiqiy punkt: posilka joyida,
				// lekin bu viloyatda yetkazishni kuryer bajaradi.
				row.Izoh = courierPendingNote
			}
			out.Pending = append(out.Pending, row)
			continue
		}

		t, ok := parseAnyTime(o.DeliveredAt)
		if !ok {
			// Sanasi o'qilmadi — "yetkazildi" deb ayta olmaymiz,
			// tekshiriladiganlar qatoriga tushadi.
			out.NeedCheck = append(out.NeedCheck, SentDelivery{
				ExpressNum: o.ExpressNum,
				Branch:     firstNonEmpty(o.BranchName, o.LocationNumber),
				Region:     region,
				Izoh:       mismatchIzoh(mismatch, region),
			})
			continue
		}

		row := SentDelivery{
			ExpressNum: o.ExpressNum,
			Branch:     firstNonEmpty(o.BranchName, o.LocationNumber),
			SentAt:     sanaMatnISO(o.DeliveredAt),
			Days:       int(now.Sub(t).Hours() / 24),
			Region:     region,
			Izoh:       mismatchIzoh(mismatch, region),
		}
		if row.Days < 0 {
			row.Days = 0 // sana kelajakda — 0 kun deb hisoblaymiz
		}
		if row.Days <= DeliveryDays {
			out.InDelivery = append(out.InDelivery, row)
		} else {
			out.NeedCheck = append(out.NeedCheck, row)
		}
	}

	// Yangisi birinchi: mijoz odatda oxirgi buyurtmasini so'raydi.
	// Sanasi o'qilmagan yozuv (SentAt bo'sh) eng oxirida turadi — uning
	// "0 kun" i haqiqiy emas.
	sort.SliceStable(out.InDelivery, func(i, j int) bool { return out.InDelivery[i].Days < out.InDelivery[j].Days })
	sort.SliceStable(out.NeedCheck, func(i, j int) bool {
		a, b := out.NeedCheck[i], out.NeedCheck[j]
		if (a.SentAt == "") != (b.SentAt == "") {
			return b.SentAt == ""
		}
		return a.Days < b.Days
	})
	out.InDelivery = capRows(out.InDelivery)
	out.NeedCheck = capRows(out.NeedCheck)

	out.Empty = len(out.Pending) == 0 && len(out.InDelivery) == 0 &&
		len(out.NeedCheck) == 0 && len(out.PickedUp) == 0

	// Hamma yozuv xato filialga tushgan bo'lsa, "bu viloyatda yetkazish
	// qanday ishlaydi" degan umumiy matn ham olib tashlanadi. O'zi to'g'ri
	// bo'lsa-da, shu holatda u modelni "o'z viloyatingizdagi filialdan
	// oling" deyishga undaydi — posilka esa boshqa viloyatda.
	if len(bad) > 0 && len(bad) == len(orders) {
		out.Kind = ""
	}
	return out, bad
}

// mismatchIzoh - viloyat mos kelmagan qatorga qo'yiladigan izoh.
func mismatchIzoh(mismatch bool, region string) string {
	if mismatch {
		// Viloyat mos emasligi hamma izohdan muhimroq.
		return mismatchNote
	}
	if region != "" && !HomeDeliveryRegion(region) {
		return noCourierRegionNote
	}
	return ""
}

// capRows - ro'yxatni MaxDeliveryRows tagacha qisqartiradi.
func capRows(rows []SentDelivery) []SentDelivery {
	if len(rows) > MaxDeliveryRows {
		return rows[:MaxDeliveryRows]
	}
	return rows
}

// parseAnyTime - adminka ("2026-08-21 10:00:00") va ISO ko'rinishlarini
// ikkalasini ham o'qiydi.
func parseAnyTime(s string) (time.Time, bool) {
	if t, ok := parseAdminkaTime(s); ok {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// sanaMatnISO - har qanday ko'rinishdagi sanani "21-avgust" qiladi.
func sanaMatnISO(s string) string {
	t, ok := parseAnyTime(s)
	if !ok {
		return ""
	}
	return sanaMatn(t.Format(adminkaTimeLayout))
}

// firstNonEmpty - birinchi haqiqiy (bo'sh ham, "null" ham bo'lmagan) qiymat.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = plainVal(v); v != "" {
			return v
		}
	}
	return ""
}

// boshQiymatlar - tashqi API bo'sh maydonni matn ko'rinishida qaytarganda
// keladigan qiymatlar. Ular modelga YETIB BORMASLIGI kerak: model
// `"manzil": "null"` ni ko'rib, mijozga "null" manzilini yozib yuborishi
// yoki yo'q ma'lumotni bor deb o'ylashi mumkin.
//
// Bu faqat TASHQI API matn maydonlariga tegishli — model qaytargan JSON
// bu yerdan o'tmaydi (masalan javobdagi `"promt": null` — zanjir tugagani,
// u o'z joyida to'g'ri o'qiladi).
var boshQiymatlar = map[string]bool{
	"null": true, "nil": true, "<nil>": true, "undefined": true,
	"n/a": true, "na": true, "-": true, "—": true,
}

// plainVal - tashqi API qiymatini tozalaydi: bo'shliqlar olib tashlanadi,
// "null" kabi soxta qiymatlar esa bo'sh satrga aylantiriladi (JSON'da
// `omitempty` bilan butunlay tushib qoladi).
func plainVal(s string) string {
	s = strings.TrimSpace(s)
	if boshQiymatlar[strings.ToLower(s)] {
		return ""
	}
	return s
}
