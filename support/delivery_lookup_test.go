package support

import "testing"

// Status 4 va treki bor, lekin ro'yxatda yo'q — qo'shimcha so'rov
// yuborilishi kerak.
func TestWaitingTracksNotFound(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting, ExpressNum: "YT111"}},
	}
	got := waitingTracks(views, nil, nil)
	if len(got) != 1 || got[0] != "YT111" {
		t.Fatalf("YT111 kutilgan, keldi %v", got)
	}
}

// Ro'yxatda allaqachon bor — qayta so'ralmaydi.
func TestWaitingTracksAlreadyFound(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting, ExpressNum: "YT111"}},
	}
	if got := waitingTracks(views, []DeliveryOrder{{ExpressNum: "yt111"}}, nil); len(got) != 0 {
		t.Fatalf("qayta so'ralmasligi kerak, keldi %v", got)
	}
}

// Boshqa statuslar bu qo'shimcha so'rovga tushmaydi.
func TestWaitingTracksOnlyStatus4(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusPaid, ExpressNum: "YT111"}},
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG2", Status: StatusFinished, ExpressNum: "YT222"}},
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG3", Status: StatusWaiting, ExpressNum: "YT333"}},
	}
	got := waitingTracks(views, nil, nil)
	if len(got) != 1 || got[0] != "YT333" {
		t.Fatalf("faqat status 4 kutilgan, keldi %v", got)
	}
}

// Treksiz status 4 — so'rov yuborilmaydi (qidiradigan narsa yo'q).
func TestWaitingTracksNoTrack(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting}},
	}
	if got := waitingTracks(views, nil, nil); len(got) != 0 {
		t.Fatalf("treksiz buyurtma so'ralmasligi kerak, keldi %v", got)
	}
}

// Bir xil trekli ikkita status 4 buyurtma — bitta so'rov.
func TestWaitingTracksDedup(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting, ExpressNum: "YT111"}},
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG2", Status: StatusWaiting, ExpressNum: " yt111 "}},
	}
	if got := waitingTracks(views, nil, nil); len(got) != 1 {
		t.Fatalf("bitta so'rov kutilgan, keldi %v", got)
	}
}

func TestDedupTracks(t *testing.T) {
	got := dedupTracks([]string{"YT111", " yt111 ", "", "  ", "YT222"})
	if len(got) != 2 || got[0] != "YT111" || got[1] != "YT222" {
		t.Fatalf("ikkita noyob trek kutilgan, keldi %v", got)
	}
}

// Bitta posilka ham user_id, ham trek so'rovidan kelsa — bir marta.
func TestDedupDeliverySameRow(t *testing.T) {
	rows := []DeliveryOrder{
		{ExpressNum: "YT111", UserID: 7},
		{ExpressNum: "yt111", UserID: 7},
	}
	if got := dedupDelivery(rows); len(got) != 1 {
		t.Fatalf("bitta qator kutilgan, keldi %d", len(got))
	}
}

// Bir xil trek, LEKIN boshqa egalik — ikkalasi ham qoladi: aynan shu
// farq "posilka boshqa akkauntda" holatini ko'rsatadi.
func TestDedupDeliveryKeepsDifferentOwners(t *testing.T) {
	rows := []DeliveryOrder{
		{ExpressNum: "YT111", UserID: 7},
		{ExpressNum: "YT111", UserID: 9},
	}
	if got := dedupDelivery(rows); len(got) != 2 {
		t.Fatalf("ikkala qator qolishi kerak, keldi %d", len(got))
	}
}

// Treksiz yozuvlar bir-birini yutib yubormasligi kerak.
func TestDedupDeliveryKeepsTrackless(t *testing.T) {
	rows := []DeliveryOrder{{FullName: "a"}, {FullName: "b"}}
	if got := dedupDelivery(rows); len(got) != 2 {
		t.Fatalf("ikkala qator qolishi kerak, keldi %d", len(got))
	}
}

// Mijoznikiga to'g'ri kelmagan qator olib tashlanadi — mijozga
// ko'rsatilmaydi.
func TestOnlyOwnDeliveryDropsForeign(t *testing.T) {
	rows := []DeliveryOrder{
		{ExpressNum: "YT111", UserID: 7},
		{ExpressNum: "YT222", UserID: 9},
	}
	bad := onlyOwnDelivery(&rows, 7)
	if len(rows) != 1 || rows[0].ExpressNum != "YT111" {
		t.Fatalf("faqat mijozniki qolishi kerak: %+v", rows)
	}
	if len(bad) != 1 || bad[0] != "YT222" {
		t.Fatalf("begona trek qaytarilishi kerak, keldi %v", bad)
	}
}

// Allaqachon so'ralgan trek ikkinchi marta so'ralmaydi — natija
// bermagan bo'lsa ham.
func TestWaitingTracksSkipsAlreadyQueried(t *testing.T) {
	views := []OrderView{
		{AdminkaOrder: AdminkaOrder{OrderSN: "DG1", Status: StatusWaiting, ExpressNum: "YT111"}},
	}
	if got := waitingTracks(views, nil, []string{" yt111 "}); len(got) != 0 {
		t.Fatalf("qayta so'ralmasligi kerak, keldi %v", got)
	}
}
