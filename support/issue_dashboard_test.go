package support

import "testing"

// Trek yo'q — solishtirishga hech narsa yo'q.
func TestCompareDashboardNoTrack(t *testing.T) {
	if chk := compareDashboard(AdminkaOrder{OrderSN: "DG1", UserID: 7}, nil); chk != nil {
		t.Fatalf("trek bo'sh: nil kutilgan, keldi %+v", chk)
	}
}

// Trek bor, lekin yetkazmada chiqmadi — posilka hali Xitoyda, xabar yo'q.
func TestCompareDashboardNotFound(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"}, nil)
	if chk == nil || chk.Found {
		t.Fatalf("yetkazmada yo'q: found=false kutilgan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("xabar kutilmagan, keldi %q", got)
	}
}

// Trek yetkazmada chiqdi va egasi bir xil — "posilka kelgan" xabari.
func TestCompareDashboardArrived(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: " yt111 "},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 7, BranchName: "Chilonzor"}})
	if chk == nil || !chk.Found || chk.Mismatch {
		t.Fatalf("kelgan, egasi bir xil kutilgan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != DashAlertArrived {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertArrived, got)
	}
	if chk.Track != "YT111" {
		t.Fatalf("trek normallashtirilmadi: %q", chk.Track)
	}
}

// Egasi mos kelmadi — xatolik xabari.
func TestCompareDashboardOwnerMismatch(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 9}})
	if chk == nil || !chk.Mismatch {
		t.Fatalf("egasi mos kelmasligi kutilgan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != DashAlertOwner {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertOwner, got)
	}
	if chk.DashID != 9 || chk.OwnerID != 7 {
		t.Fatalf("egalar noto'g'ri olindi: %+v", chk)
	}
}

// Bitta trekka bir nechta qator: egasi MOS KELGANI asosiy bo'lishi
// kerak, aks holda tasodifiy birinchi qatorga qarab "xatolik" deb
// xabar ketadi.
func TestCompareDashboardPrefersMatchingOwner(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{
			{ExpressNum: "YT111", UserID: 9, FullName: "begona"},
			{ExpressNum: "YT111", UserID: 7, FullName: "o'zi"},
		})
	if chk == nil || chk.Mismatch {
		t.Fatalf("egasi mos kelgan qator tanlanishi kerak, keldi %+v", chk)
	}
	if chk.Row.FullName != "o'zi" || chk.Rows != 2 {
		t.Fatalf("asosiy qator yoki soni noto'g'ri: %+v", chk)
	}
}

// O'xshash, lekin boshqa trek yetkazmadan kelsa hisobga olinmaydi:
// qidiruv butun baza bo'yicha ketadi.
func TestCompareDashboardIgnoresOtherTracks(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT1119", UserID: 9}})
	if chk == nil || chk.Found {
		t.Fatalf("boshqa trek hisobga olinmasligi kerak, keldi %+v", chk)
	}
}

// Bir tomonda user_id bo'sh bo'lsa xato deb hisoblanmaydi: ikkala API
// ham bu maydonni ba'zan bermaydi.
func TestCompareDashboardUnknownOwnerIsNotError(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111"}})
	if chk == nil || chk.Mismatch {
		t.Fatalf("egasi noma'lum: xato kutilmagan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != DashAlertArrived {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertArrived, got)
	}
}

// Mijoz olib ketgan posilka guruhga CHIQMAYDI.
func TestCompareDashboardDeliveredNoAlert(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 7, Delivered: true,
			DeliveredAt: "2026-09-01 10:00:00", BranchName: "SHOTA"}})
	if chk == nil || !chk.Delivered() {
		t.Fatalf("olib ketilgan deb aniqlanishi kerak, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("olib ketilgan posilka uchun xabar kutilmagan, keldi %q", got)
	}
}

// Olib ketilgan bo'lsa ham egasi mos kelmasa — bu xato, xabar ketadi:
// posilkani boshqa odam olib ketgan.
func TestCompareDashboardDeliveredToOtherOwnerAlerts(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 9, Delivered: true}})
	if got := chk.Alert(); got != DashAlertOwner {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertOwner, got)
	}
}

// Yetkazmada bor, lekin hali olib ketilmagan — 📦 xabari ketadi.
func TestCompareDashboardArrivedNotTakenAlerts(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 7, BranchName: "SHOTA"}})
	if chk.Delivered() {
		t.Fatalf("olib ketilmagan bo'lishi kerak: %+v", chk)
	}
	if got := chk.Alert(); got != DashAlertArrived {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertArrived, got)
	}
}

// issuesHaveTrack - treksiz ro'yxat uchun yetkazma so'ralmaydi.
func TestIssuesHaveTrack(t *testing.T) {
	if issuesHaveTrack(nil) {
		t.Fatal("bo'sh ro'yxat: false kutilgan")
	}
	if issuesHaveTrack([]*OrderIssue{{OrderSN: "DG1"}, {OrderSN: "DG2", ExpressNum: "  "}}) {
		t.Fatal("treksiz ro'yxat: false kutilgan")
	}
	if !issuesHaveTrack([]*OrderIssue{{OrderSN: "DG1"}, {OrderSN: "DG2", ExpressNum: "YT111"}}) {
		t.Fatal("treki bor ro'yxat: true kutilgan")
	}
}

// DropDeliveredIssues - olib ketilmaganlar ro'yxatda qoladi, bazaga
// tegilmaydi (DB bu testda nil).
func TestDropDeliveredIssuesKeepsNotTaken(t *testing.T) {
	list := []*OrderIssue{{OrderSN: "DG1", ExpressNum: "YT111"}}
	kept, n := DropDeliveredIssues(list, []DeliveryOrder{{ExpressNum: "YT111"}})
	if n != 0 || len(kept) != 1 {
		t.Fatalf("olib ketilmagan muammo qolishi kerak: n=%d, kept=%d", n, len(kept))
	}
}

// Yetkazma ro'yxati bo'sh bo'lsa ham ro'yxat o'zgarmaydi.
func TestDropDeliveredIssuesEmptyDelivery(t *testing.T) {
	list := []*OrderIssue{{OrderSN: "DG1", ExpressNum: "YT111"}}
	kept, n := DropDeliveredIssues(list, nil)
	if n != 0 || len(kept) != 1 {
		t.Fatalf("yetkazma bo'sh: ro'yxat o'zgarmasligi kerak: n=%d, kept=%d", n, len(kept))
	}
}
