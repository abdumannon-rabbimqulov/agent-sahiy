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

// PickupDays - "oxirgi kunlarda olingan" deb hisoblanadigan oraliq.
const PickupDays = 3

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

// PickedUpDay - bir kunda olib ketilgan buyurtmalar.
type PickedUpDay struct {
	Date   string `json:"sana"`
	Count  int    `json:"soni"`
	Branch string `json:"filial,omitempty"`
}

// DeliveryBrief - yetkazma bo'yicha modelga ketadigan xulosa.
type DeliveryBrief struct {
	// Olinmagan — filialda kutmoqda.
	Pending []PendingPickup `json:"olinmagan,omitempty"`
	// Oxirgi kunlarda olib ketilganlar (kun bo'yicha guruhlangan).
	RecentPickups []PickedUpDay `json:"oxirgi_kunlarda_olingan,omitempty"`
	// Umuman yozuv yo'q.
	Empty bool `json:"yozuv_yoq,omitempty"`
}

// BriefDelivery - yetkazma yozuvlarini ikkiga ajratadi:
//   - olinmagan: qaysi filialda turgani (mijozga shuni aytamiz);
//   - oxirgi PickupDays kun ichida olinganlar: sana va o'sha kuni nechta
//     buyurtma kelgani (mijozdan "oldingizmi?" deb so'rash uchun).
//
// Eski olingan buyurtmalar modelga umuman yuborilmaydi — ular javobga
// ta'sir qilmaydi, faqat token yeydi.
func BriefDelivery(orders []DeliveryOrder) DeliveryBrief {
	var out DeliveryBrief
	byDay := map[string]*PickedUpDay{}
	cutoff := time.Now().AddDate(0, 0, -PickupDays)

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
		if !ok || t.Before(cutoff) {
			continue // eski — modelga kerak emas
		}
		key := t.Format("2006-01-02")
		d, ok := byDay[key]
		if !ok {
			d = &PickedUpDay{Date: sanaMatnISO(o.DeliveredAt),
				Branch: firstNonEmpty(o.BranchName, o.LocationNumber, o.City)}
			byDay[key] = d
		}
		d.Count++
	}

	keys := make([]string, 0, len(byDay))
	for k := range byDay {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	for _, k := range keys {
		out.RecentPickups = append(out.RecentPickups, *byDay[k])
	}

	out.Empty = len(out.Pending) == 0 && len(out.RecentPickups) == 0
	return out
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
