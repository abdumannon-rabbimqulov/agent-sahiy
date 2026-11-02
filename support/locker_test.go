package support

import (
	"testing"
	"time"
)

// Status 5 — posilka postamatda: mijozga postamat nomi va manzili
// aytiladi, Toshkentda bo'lsa ham kuryer va'da qilinmaydi.
func TestBriefDeliveryLocker(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")
	for _, delivered := range []bool{false, true} {
		b, bad := BriefDelivery([]DeliveryOrder{{
			ExpressNum:    "79038787407551",
			City:          "Toshkent viloyati",
			BranchName:    "SHOTA",
			Status:        5,
			Delivered:     delivered,
			CreatedAt:     arrived,
			LockerName:    "Tashkent",
			LockerAddress: "Olma Market-M11",
		}})
		if len(b.Pending) != 1 {
			t.Fatalf("delivered=%v: mijozga_topshirilmagan %d ta, 1 kutilgan: %+v", delivered, len(b.Pending), b)
		}
		row := b.Pending[0]
		if row.Branch != "Tashkent" || row.Address != "Olma Market-M11" {
			t.Errorf("postamat nomi/manzili noto'g'ri: %+v", row)
		}
		if row.Izoh != lockerNote {
			t.Errorf("izoh postamat matni emas: %s", row.Izoh)
		}
		if b.Kind != lockerKind {
			t.Errorf("yetkazish_turi postamat emas: %s", b.Kind)
		}
		if len(bad) != 0 || len(b.Alerts) != 0 {
			t.Errorf("xodimga ogohlantirish kutilmagan: bad=%+v alerts=%+v", bad, b.Alerts)
		}
	}
}

// Manzil kelmasa model uni to'qimasin, xodim aniqlab beradi.
func TestBriefDeliveryLockerNoAddress(t *testing.T) {
	b, _ := BriefDelivery([]DeliveryOrder{{ExpressNum: "1", Status: 5}})
	if len(b.Pending) != 1 || b.Pending[0].Izoh != lockerNoAddressNote {
		t.Fatalf("manzilsiz postamat izohi noto'g'ri: %+v", b.Pending)
	}
	if b.Pending[0].Branch != "Postamat" {
		t.Errorf("hozir_qayerda: %s", b.Pending[0].Branch)
	}
	if len(b.Alerts) != 1 {
		t.Errorf("1 ta ogohlantirish kutilgan, %d ta", len(b.Alerts))
	}
}

// Postamat va kuryer posilkasi aralash bo'lsa, umumiy matn olib tashlanadi.
func TestBriefDeliveryLockerMixedClearsKind(t *testing.T) {
	b, _ := BriefDelivery([]DeliveryOrder{
		{ExpressNum: "1", Status: 5, LockerName: "Tashkent", LockerAddress: "Olma Market-M11"},
		{ExpressNum: "2", City: "Toshkent shahri", BranchName: "SHOTA", LocationNumber: "SHOTA-28"},
	})
	if b.Kind != "" {
		t.Errorf("aralash turda yetkazish_turi bo'sh bo'lishi kerak: %s", b.Kind)
	}
}

// Buyurtmadagi "kuryer olib boradi" postamat izohiga zid kelmasin;
// boshqa buyurtmaniki o'zgarmaydi.
func TestMarkInLocker(t *testing.T) {
	briefs := []OrderBrief{
		{OrderSN: "DG1", ExpressNum: "YT1", Kind: "kuryer"},
		{OrderSN: "DG2", ExpressNum: "YT2", Kind: "kuryer"},
	}
	MarkInLocker(briefs, []DeliveryOrder{
		{ExpressNum: "yt1", Status: 5},
		{ExpressNum: "YT2", Status: 2},
	})
	if briefs[0].Kind != "" || briefs[1].Kind != "kuryer" {
		t.Errorf("yetkazish noto'g'ri: %+v", briefs)
	}
}

// Dashboarddagi `locker` obyekti maydonlarga to'g'ri o'qiladi.
func TestLockerFieldsFromDashboard(t *testing.T) {
	m := map[string]any{"locker": map[string]any{"name": "Tashkent", "address": "Olma Market-M11"}}
	if got := str(get(m, "locker.name")); got != "Tashkent" {
		t.Errorf("locker.name: %q", got)
	}
	if got := str(get(m, "locker.address")); got != "Olma Market-M11" {
		t.Errorf("locker.address: %q", got)
	}
}
