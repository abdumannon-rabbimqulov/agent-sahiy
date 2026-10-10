package support

import (
	"strings"
	"testing"
	"time"
)

// Mijozning shahri (`city`) bo'sh, posilka esa Farg'onadagi filialda:
// u yerda kuryer yetkazish yo'q, mijoz o'zi olib ketadi. Ilgari kod
// `city` bo'sh bo'lsa Toshkent deb hisoblab, xodimlar guruhiga
// "kuryer hali olib bormagan" degan noto'g'ri ogohlantirish yuborardi.
func TestBriefDeliveryBranchRegionFallback(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -10).Format("2006-01-02 15:04:05")
	rows := []DeliveryOrder{{
		ExpressNum:    "79031670041637",
		BranchName:    "SAHIY FARGONA",
		BranchAddress: "Farg'ona viloyati, Farg'ona shahri",
		City:          "", // mijozning shahri noma'lum
		CreatedAt:     arrived,
	}}

	brief, _ := BriefDelivery(rows)

	if len(brief.Alerts) != 0 {
		t.Fatalf("kuryer ogohlantirishi ketdi: %q", brief.Alerts[0].Text)
	}
	if len(brief.Pending) != 1 {
		t.Fatalf("olinmagan qatorlar soni = %d", len(brief.Pending))
	}
	row := brief.Pending[0]
	if row.Izoh != pickupNote {
		t.Errorf("izoh = %q, kerak pickupNote", row.Izoh)
	}
	if row.Region != RegionFergana {
		t.Errorf("viloyat = %q, kerak %q", row.Region, RegionFergana)
	}
	if !strings.Contains(brief.Kind, "o'zi olib ketadi") {
		t.Errorf("yetkazish turi = %q", brief.Kind)
	}
}

// Toshkentda esa aksincha: kuryer yetkazadi va muddat o'tgan bo'lsa
// xodim tekshirishi kerak — ogohlantirish qolishi shart.
func TestBriefDeliveryTashkentCourierAlert(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -10).Format("2006-01-02 15:04:05")
	rows := []DeliveryOrder{{
		ExpressNum:     "79031670041638",
		BranchName:     "SHOTA",
		LocationNumber: "SHOTA-28",
		City:           "Toshkent shahri",
		CreatedAt:      arrived,
	}}

	brief, _ := BriefDelivery(rows)

	if len(brief.Alerts) != 1 {
		t.Fatalf("ogohlantirish soni = %d, kerak 1", len(brief.Alerts))
	}
	if !strings.Contains(brief.Alerts[0].Text, "kuryer hali olib bormagan") {
		t.Errorf("ogohlantirish matni = %q", brief.Alerts[0].Text)
	}
}
