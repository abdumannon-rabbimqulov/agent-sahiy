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
	PackageName string `json:"package_name,omitempty"`

	// Region - buyurtma manzili viloyati (adminkadagi `province`),
	// Kind - o'sha viloyatda yetkazish qanday ishlashi. Ikkalasi ham
	// tayyor matn: model uyga yetkazish bor-yo'qligini o'zi taxmin
	// qilmasin — Toshkent shahri va Toshkent viloyatidan boshqa hamma
	// joyda mijoz posilkani filialdan o'zi olib ketadi.
	Region string `json:"viloyat,omitempty"`
	Kind   string `json:"yetkazish,omitempty"`

	// OwnerUserID - buyurtma egasi, faqat u hozirgi mijoz BO'LMAGANDA
	// to'ldiriladi. Buyurtma raqami bo'yicha qidiruv adminkaning butun
	// bazasidan qidiradi: mijoz boshqa odamning DG raqamini yozsa ham
	// buyurtma topiladi. Model buni ko'rib tursin — begona buyurtma
	// tafsilotini mijozga aytib yubormasin.
	OwnerUserID int64 `json:"boshqa_mijozning_buyurtmasi,omitempty"`
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
		}
		if clientID > 0 && v.UserID > 0 && v.UserID != clientID {
			b.OwnerUserID = v.UserID
		}
		if r := RegionOf(v.Province); r != "" {
			b.Region = r
			b.Kind = DeliveryKindText(r)
		}
		out = append(out, b)
	}
	return out
}

// PendingPickup - mijoz hali olib ketmagan yetkazma.
type PendingPickup struct {
	ExpressNum string `json:"express_num"`
	Branch     string `json:"filial"`
	Address    string `json:"manzil,omitempty"`
	ArrivedAt  string `json:"kelgan,omitempty"`
	// Region - mijozning viloyati (dashboarddagi `city`).
	Region string `json:"mijoz_viloyati,omitempty"`
	// Izoh - filial "Markaziy ombor" bo'lganda tayyor holatda
	// to'ldiriladi: bu filial mijoz o'zi borib oladigan nuqta emas,
	// buyurtma hali taqsimlash bosqichida turibdi va tez orada mijoz
	// belgilagan manzilga jo'natiladi. Model buni qayta talqin
	// qilmasin deb tayyor matn shu yerda beriladi — promt o'zgarmaydi.
	Izoh string `json:"izoh,omitempty"`
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
const centralWarehouseNote = "Buyurtma hozircha Markaziy omborda — tez orada mijoz belgilagan manzilga yetkaziladi."

// centralWarehouseRegionNote - xuddi shu holat, lekin mijoz Toshkentdan
// tashqarida: u yerda uyga yetkazish YO'Q, posilka mijoz viloyatidagi
// filialga jo'natiladi va mijoz o'sha yerdan oladi.
const centralWarehouseRegionNote = "Buyurtma hozircha Markaziy omborda — tez orada mijoz " +
	"viloyatidagi filialga jo'natiladi, mijoz o'sha filialdan olib ketadi."

// pickupNote - posilka mijozning o'z viloyatidagi filialda: olib
// ketishi kerak, uyiga olib borilmaydi.
const pickupNote = "Posilka mijoz viloyatidagi filialda turibdi — mijoz o'zi borib olib ketadi " +
	"(bu viloyatda uyga yetkazish yo'q)."

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
	// Olinmagan — filialda kutmoqda (delivered = false).
	Pending []PendingPickup `json:"olinmagan,omitempty"`
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

// PickupDone - o'zi-olib-ketish turidagi jo'natma, mijoz allaqachon
// filialdan olib ketgan (express_line "Pickup", status=2,
// delivered=true).
type PickupDone struct {
	ExpressNum string `json:"express_num,omitempty"`
	Branch     string `json:"filial,omitempty"`
	PickedAt   string `json:"olingan,omitempty"`
	// Region - mijozning viloyati (dashboarddagi `city`).
	Region string `json:"mijoz_viloyati,omitempty"`
}

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
			out.PickedUp = append(out.PickedUp, PickupDone{
				ExpressNum: o.ExpressNum,
				Branch:     firstNonEmpty(o.BranchName, o.LocationNumber),
				PickedAt:   sanaMatnISO(o.DeliveredAt),
				Region:     region,
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
				Address:    trimText(o.BranchAddress, 80),
				ArrivedAt:  sanaMatnISO(o.CreatedAt),
				Region:     region,
			}
			// Izoh tanlash tartibi: avval xato holat, keyin "hali yo'lda",
			// oxirida oddiy "kelib bo'ldi, olib keting".
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
				Izoh:       mismatchIzoh(mismatch),
			})
			continue
		}

		row := SentDelivery{
			ExpressNum: o.ExpressNum,
			Branch:     firstNonEmpty(o.BranchName, o.LocationNumber),
			SentAt:     sanaMatnISO(o.DeliveredAt),
			Days:       int(now.Sub(t).Hours() / 24),
			Region:     region,
			Izoh:       mismatchIzoh(mismatch),
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
	return out, bad
}

// mismatchIzoh - viloyat mos kelmagan qatorga qo'yiladigan izoh.
func mismatchIzoh(mismatch bool) string {
	if mismatch {
		return mismatchNote
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

// firstNonEmpty - birinchi bo'sh bo'lmagan qiymat.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
