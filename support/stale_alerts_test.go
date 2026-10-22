package support

import (
	"os"
	"testing"
	"time"
)

func withStaleDays(t *testing.T, v string) {
	t.Helper()
	old, had := os.LookupEnv("STALE_DELIVERY_DAYS")
	os.Setenv("STALE_DELIVERY_DAYS", v)
	t.Cleanup(func() {
		if had {
			os.Setenv("STALE_DELIVERY_DAYS", old)
			return
		}
		os.Unsetenv("STALE_DELIVERY_DAYS")
	})
}

// Yangi yozuv (chegaradan kam) xodimga chiqadi.
func TestDropStaleAlertsKeepsFresh(t *testing.T) {
	withStaleDays(t, "10")
	rows := []DeliveryAlert{{ExpressNum: "YT111", Days: 4, Text: "yangi"}}
	got, n := DropStaleAlerts(rows, nil)
	if n != 0 || len(got) != 1 || got[0] != "yangi" {
		t.Fatalf("yangi ogohlantirish qolishi kerak: n=%d, got=%v", n, got)
	}
}

// Chegaraning o'zi (aynan 10 kun) hali eskirgan emas.
func TestDropStaleAlertsBoundary(t *testing.T) {
	withStaleDays(t, "10")
	got, n := DropStaleAlerts([]DeliveryAlert{{ExpressNum: "YT111", Days: 10, Text: "x"}}, nil)
	if n != 0 || len(got) != 1 {
		t.Fatalf("aynan 10 kun qolishi kerak: n=%d, got=%v", n, got)
	}
	got, n = DropStaleAlerts([]DeliveryAlert{{ExpressNum: "YT111", Days: 11, Text: "x"}}, nil)
	if n != 1 || len(got) != 0 {
		t.Fatalf("11 kun tashlanishi kerak: n=%d, got=%v", n, got)
	}
}

// Mijoz O'ZI so'ragan bo'lsa — yoshidan qat'i nazar chiqadi.
func TestDropStaleAlertsKeepsAsked(t *testing.T) {
	withStaleDays(t, "10")
	rows := []DeliveryAlert{
		{ExpressNum: "YT111", Days: 255, Text: "so'ralgan"},
		{ExpressNum: "YT222", Days: 255, Text: "so'ralmagan"},
	}
	got, n := DropStaleAlerts(rows, map[string]bool{"YT111": true})
	if n != 1 || len(got) != 1 || got[0] != "so'ralgan" {
		t.Fatalf("faqat so'ralgani qolishi kerak: n=%d, got=%v", n, got)
	}
}

// Yoshi noma'lum (0) bo'lsa yashirilmaydi: sana o'qilmagani
// ogohlantirishni to'sish uchun asos emas.
func TestDropStaleAlertsUnknownAgeKept(t *testing.T) {
	withStaleDays(t, "10")
	got, n := DropStaleAlerts([]DeliveryAlert{{ExpressNum: "YT111", Days: 0, Text: "x"}}, nil)
	if n != 0 || len(got) != 1 {
		t.Fatalf("yoshi noma'lum yozuv qolishi kerak: n=%d, got=%v", n, got)
	}
}

// 0 — filtr o'chirilgan, hamma ogohlantirish chiqadi.
func TestDropStaleAlertsDisabled(t *testing.T) {
	withStaleDays(t, "0")
	got, n := DropStaleAlerts([]DeliveryAlert{{ExpressNum: "YT111", Days: 999, Text: "x"}}, nil)
	if n != 0 || len(got) != 1 {
		t.Fatalf("filtr o'chirilgan: hammasi qolishi kerak: n=%d, got=%v", n, got)
	}
}

// Mijoz DG raqamini yozsa, o'sha buyurtmaning TREKI ham "so'ralgan".
func TestAskedTracksResolvesOrderSN(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", ExpressNum: "YT111"}},
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG2", ExpressNum: "YT222"}},
	}
	got := AskedTracks([]string{"dg1"}, views)
	if !got["YT111"] {
		t.Fatalf("DG1 ning treki so'ralgan bo'lishi kerak: %v", got)
	}
	if got["YT222"] {
		t.Fatalf("so'ralmagan buyurtma treki kirmasligi kerak: %v", got)
	}
}

// Mijoz trek raqamining o'zini yozsa ham ishlaydi.
func TestAskedTracksAcceptsTrack(t *testing.T) {
	got := AskedTracks([]string{" yt111 "}, nil)
	if !got["YT111"] {
		t.Fatalf("trek normallashtirilib qo'shilishi kerak: %v", got)
	}
}

// Mijoz hech narsa yozmagan — to'plam bo'sh, demak eski yozuvlar
// to'siladi.
func TestAskedTracksEmpty(t *testing.T) {
	views := []OrderView{{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", ExpressNum: "YT111"}}}
	if got := AskedTracks(nil, views); len(got) != 0 {
		t.Fatalf("bo'sh to'plam kutilgan, keldi %v", got)
	}
}

// Eski, mijoz so'ramagan yetkazma yozuvi modelga berilmaydi; so'ralgani,
// yangisi va sanasi noma'lumi qoladi.
func TestDropOldDelivery(t *testing.T) {
	t.Setenv("OLD_ORDER_DAYS", "")
	ago := func(days int) string {
		return time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	}
	rows := []DeliveryOrder{
		{ExpressNum: "78994753793731", CreatedAt: ago(176)},
		{ExpressNum: "435021736284451", CreatedAt: ago(231)},
		{ExpressNum: "JT3178117220928", CreatedAt: ago(1)},
		{ExpressNum: "YT000"},
	}
	asked := map[string]bool{trackKey("435021736284451"): true}
	got, n := dropOldDelivery(rows, asked)
	if n != 1 {
		t.Fatalf("1 ta yozuv tashlanishi kerak, %d ta", n)
	}
	for _, r := range got {
		if r.ExpressNum == "78994753793731" {
			t.Fatalf("176 kunlik so'ralmagan yozuv qolib ketdi")
		}
	}
	if !askedFound(got, asked) {
		t.Fatalf("so'ralgan trek topilishi kerak")
	}
	if askedFound(got, map[string]bool{trackKey("DG607328814"): true}) {
		t.Fatalf("so'ralmagan yozuvlar \"topildi\" deb hisoblanmasin")
	}
}
