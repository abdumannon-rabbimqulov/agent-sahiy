// Modelga ketadigan tizim ma'lumotini saralash.
//
// Xom javoblar katta (bitta buyurtma ~10 KB) va modelga hammasi kerak
// emas: har ortiqcha maydon token va chalkashlik. Shu yerda faqat
// javob yozish uchun zarur bo'lgan maydonlar qoladi.
package support

import (
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
}

// BriefOrders - buyurtmalarni ixchamlashtiradi. Sanalar odam o'qiydigan
// ko'rinishga o'tkaziladi (model xom "2026-08-21 16:43:54" dan foydali
// narsa yoza olmaydi, faqat token yeydi).
func BriefOrders(views []OrderView) []OrderBrief {
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
	// Umuman yozuv yo'q.
	Empty bool `json:"yozuv_yoq,omitempty"`
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
func BriefDelivery(orders []DeliveryOrder) DeliveryBrief {
	var out DeliveryBrief
	now := time.Now()

	for _, o := range orders {
		if !o.Delivered {
			out.Pending = append(out.Pending, PendingPickup{
				ExpressNum: o.ExpressNum,
				Branch:     firstNonEmpty(o.BranchName, o.LocationNumber, o.City),
				Address:    trimText(o.BranchAddress, 80),
				ArrivedAt:  sanaMatnISO(o.CreatedAt),
			})
			continue
		}

		t, ok := parseAnyTime(o.DeliveredAt)
		if !ok {
			// Sanasi o'qilmadi — "yetkazildi" deb ayta olmaymiz,
			// tekshiriladiganlar qatoriga tushadi.
			out.NeedCheck = append(out.NeedCheck, SentDelivery{
				ExpressNum: o.ExpressNum,
				Branch:     firstNonEmpty(o.BranchName, o.LocationNumber, o.City),
			})
			continue
		}

		row := SentDelivery{
			ExpressNum: o.ExpressNum,
			Branch:     firstNonEmpty(o.BranchName, o.LocationNumber, o.City),
			SentAt:     sanaMatnISO(o.DeliveredAt),
			Days:       int(now.Sub(t).Hours() / 24),
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

	out.Empty = len(out.Pending) == 0 && len(out.InDelivery) == 0 && len(out.NeedCheck) == 0
	return out
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
