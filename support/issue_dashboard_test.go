package support

import "testing"

// Trek yo'q — solishtirishga hech narsa yo'q.
func TestCompareDashboardNoTrack(t *testing.T) {
	if chk := compareDashboard(AdminkaOrder{OrderSN: "DG1", UserID: 7}, nil); chk != nil {
		t.Fatalf("trek bo'sh: nil kutilgan, keldi %+v", chk)
	}
}

// Trek bor, lekin yetkazmada chiqmadi — posilka hali Xitoyda.
func TestCompareDashboardNotFound(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"}, nil)
	if chk.Arrived() {
		t.Fatalf("yetkazmada yo'q: kelmagan bo'lishi kerak, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("xabar kutilmagan, keldi %q", got)
	}
}

// Trek yetkazmada chiqdi, mijoz hali olib ketmagan — bu MUAMMO EMAS,
// xabar ham ketmaydi. Adminka holati nima deb tursa ham.
func TestCompareDashboardArrivedIsNotAlert(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: " yt111 ", Status: StatusWaiting},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 7, BranchName: "SHOTA"}})
	if !chk.Arrived() || chk.Delivered() || chk.Mismatch {
		t.Fatalf("kelgan, olib ketilmagan, egasi bir xil kutilgan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("kelgan posilka uchun xabar kutilmagan, keldi %q", got)
	}
	if chk.Track != "YT111" {
		t.Fatalf("trek normallashtirilmadi: %q", chk.Track)
	}
}

// Mijoz olib ketgan — ham muammo emas, ham xabar emas.
func TestCompareDashboardDeliveredIsNotAlert(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 7, Delivered: true,
			DeliveredAt: "2026-09-01 10:00:00", BranchName: "SHOTA"}})
	if !chk.Delivered() {
		t.Fatalf("olib ketilgan deb aniqlanishi kerak, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("xabar kutilmagan, keldi %q", got)
	}
}

// Egasi mos kelmadi — yagona xabarga arzigulik holat.
func TestCompareDashboardOwnerMismatch(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 9}})
	if !chk.Mismatch {
		t.Fatalf("egasi mos kelmasligi kutilgan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != DashAlertOwner {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertOwner, got)
	}
	if chk.DashID != 9 || chk.OwnerID != 7 {
		t.Fatalf("egalar noto'g'ri olindi: %+v", chk)
	}
}

// Olib ketilgan bo'lsa ham egasi mos kelmasa xabar ketadi: posilkani
// boshqa odam olgan.
func TestCompareDashboardDeliveredToOtherOwnerAlerts(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111", UserID: 9, Delivered: true}})
	if got := chk.Alert(); got != DashAlertOwner {
		t.Fatalf("Alert: %q kutilgan, keldi %q", DashAlertOwner, got)
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
	if chk.Mismatch {
		t.Fatalf("egasi mos kelgan qator tanlanishi kerak, keldi %+v", chk)
	}
	if chk.Row.FullName != "o'zi" || chk.Rows != 2 {
		t.Fatalf("asosiy qator yoki soni noto'g'ri: %+v", chk)
	}
}

// O'xshash, lekin boshqa trek hisobga olinmaydi: qidiruv butun baza
// bo'yicha ketadi.
func TestCompareDashboardIgnoresOtherTracks(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT1119", UserID: 9}})
	if chk.Arrived() {
		t.Fatalf("boshqa trek hisobga olinmasligi kerak, keldi %+v", chk)
	}
}

// Bir tomonda user_id bo'sh bo'lsa xato deb hisoblanmaydi: ikkala API
// ham bu maydonni ba'zan bermaydi.
func TestCompareDashboardUnknownOwnerIsNotError(t *testing.T) {
	chk := compareDashboard(
		AdminkaOrder{OrderSN: "DG1", UserID: 7, ExpressNum: "YT111"},
		[]DeliveryOrder{{ExpressNum: "YT111"}})
	if chk.Mismatch {
		t.Fatalf("egasi noma'lum: xato kutilmagan, keldi %+v", chk)
	}
	if got := chk.Alert(); got != "" {
		t.Fatalf("xabar kutilmagan, keldi %q", got)
	}
}

// DropArrivedIssues - yetkazmada chiqmagan muammo ro'yxatda qoladi
// (bazaga tegilmaydi, DB bu testda nil).
func TestDropArrivedIssuesKeepsNotArrived(t *testing.T) {
	list := []*OrderIssue{{OrderSN: "DG1", ExpressNum: "YT111"}}
	kept, n := DropArrivedIssues(list, []DeliveryOrder{{ExpressNum: "YT999"}})
	if n != 0 || len(kept) != 1 {
		t.Fatalf("kelmagan muammo qolishi kerak: n=%d, kept=%d", n, len(kept))
	}
}

// Egasi mos kelmagan muammo tashlanmaydi: xato aynan shu yerda.
func TestDropArrivedIssuesKeepsMismatch(t *testing.T) {
	list := []*OrderIssue{{OrderSN: "DG1", ExpressNum: "YT111", OwnerUserID: 7}}
	kept, n := DropArrivedIssues(list, []DeliveryOrder{{ExpressNum: "YT111", UserID: 9}})
	if n != 0 || len(kept) != 1 {
		t.Fatalf("egasi mos kelmagan muammo qolishi kerak: n=%d, kept=%d", n, len(kept))
	}
}

// Yetkazma ro'yxati bo'sh bo'lsa ro'yxat o'zgarmaydi.
func TestDropArrivedIssuesEmptyDelivery(t *testing.T) {
	list := []*OrderIssue{{OrderSN: "DG1", ExpressNum: "YT111"}}
	kept, n := DropArrivedIssues(list, nil)
	if n != 0 || len(kept) != 1 {
		t.Fatalf("yetkazma bo'sh: ro'yxat o'zgarmasligi kerak: n=%d, kept=%d", n, len(kept))
	}
}

// needsArrivalCheck statusga QARAMAYDI — treki bor buyurtma yetarli.
func TestNeedsArrivalCheckIgnoresStatus(t *testing.T) {
	if needsArrivalCheck([]OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusPaid}},
	}) {
		t.Fatal("treksiz ro'yxat: false kutilgan")
	}
	if !needsArrivalCheck([]OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting, ExpressNum: "YT111"}},
	}) {
		t.Fatal("status 4, treki bor: true kutilgan")
	}
}
