package support

import (
	"strings"
	"testing"
	"time"
)

// shippedDaysAgo - N kun oldin yo'lga chiqqan, adminkada hamon 4 da
// turgan va to'lovdan beri oylar o'tgan buyurtma.
func shippedDaysAgo(n int) AdminkaOrder {
	return AdminkaOrder{
		PayStatus:  1,
		Status:     StatusWaiting,
		OrderSN:    "DG1",
		ExpressNum: "YT1",
		PaidAt:     time.Now().AddDate(0, 0, -90).Format(adminkaTimeLayout),
		ShippedAt:  time.Now().AddDate(0, 0, -n).Format(adminkaTimeLayout),
	}
}

// Yaqinda yo'lga chiqqan buyurtma guruhga muammo bo'lib chiqmaydi:
// to'lovdan beri 90 kun o'tgan bo'lsa ham, posilka yo'lda.
func TestFreshShipmentIsNotAProblem(t *testing.T) {
	for _, d := range []int{0, 5, 20} {
		if IsProblem(shippedDaysAgo(d)) {
			t.Fatalf("%d kun oldin yo'lga chiqqan buyurtma muammo bo'lmasligi kerak", d)
		}
	}
}

// Yo'l muddati o'tib ketgan bo'lsa himoya ishlamaydi — buyurtma
// odatdagicha muammo bo'lib xodimga chiqadi.
func TestOldShipmentStaysAProblem(t *testing.T) {
	if !IsProblem(shippedDaysAgo(FreshShipmentDays() + 1)) {
		t.Fatal("yo'l muddati o'tgan buyurtma muammo bo'lib qolishi kerak")
	}
}

// Sana bo'sh bo'lsa (Xitoyda hali jo'natilmagan) hech narsa
// o'zgarmaydi: eski himoya kuchda.
func TestNoShippedDateStaysAProblem(t *testing.T) {
	o := shippedDaysAgo(1)
	o.ShippedAt = ""
	if ShippedDaysOf(o) != -1 {
		t.Fatalf("sana yo'q: -1 kutilgan, %d keldi", ShippedDaysOf(o))
	}
	if !IsProblem(o) {
		t.Fatal("jo'natilmagan, 90 kun turgan buyurtma muammo bo'lishi kerak")
	}
}

// briefWithChecks - buyurtmani ketma-ketlikdan o'tkazib, modelga
// ketadigan ko'rinishini qaytaradi.
func briefWithChecks(o AdminkaOrder, rows []DeliveryOrder) OrderBrief {
	views := []OrderView{NewOrderView(o)}
	briefs := BriefOrders(views, 0)
	ApplyChecks(briefs, CheckOrders(views, rows, true, true, nil))
	return briefs[0]
}

// Modelga yo'lga chiqqan SANA va qolgan kun oralig'i beriladi.
func TestOnTheWayNoteHasShippedDate(t *testing.T) {
	// Yetkazmada hali yo'q: ro'yxat bo'sh.
	o := shippedDaysAgo(3)
	b := briefWithChecks(o, nil)
	if b.OnTheWay == "" {
		t.Fatal("yaqinda yo'lga chiqqan buyurtma uchun tayyor javob bo'lishi kerak")
	}
	// `shipped_at` yetib kelmagan buyurtmadan olib tashlanadi
	// (notArrivedBrief) — sana izoh matnining o'zida turadi.
	if sana := sanaMatn(o.ShippedAt); sana == "" || !strings.Contains(b.OnTheWay, sana) {
		t.Fatalf("javobda yo'lga chiqqan sana turishi kerak: %q", b.OnTheWay)
	}
	if b.AskReceived != "" {
		t.Fatal("mijozdan so'rash ko'rsatmasi bu holatda berilmaydi")
	}
	if b.Problem {
		t.Fatal("yo'ldagi buyurtma `problem` deb belgilanmasligi kerak")
	}
	// Oraliq alohida maydonda ham, izoh matnida ham turishi kerak.
	if b.ETA == "" || !strings.Contains(b.OnTheWay, b.ETA) {
		t.Fatalf("qolgan kun oralig'i yo'q: ETA=%q izoh=%q", b.ETA, b.OnTheWay)
	}
}

// Posilka yetkazmada chiqqan bo'lsa "tez orada keladi" deyilmaydi: u
// allaqachon O'zbekistonda.
func TestArrivedOrderHasNoOnTheWayNote(t *testing.T) {
	b := briefWithChecks(shippedDaysAgo(3), []DeliveryOrder{{ExpressNum: "YT1"}})
	if b.OnTheWay != "" {
		t.Fatalf("kelgan posilka uchun \"yo'lda\" javobi berilmasligi kerak: %q", b.OnTheWay)
	}
	if b.Arrived != arrivedInDelivery {
		t.Fatalf("posilka kelgani yozilishi kerak: %q", b.Arrived)
	}
}

// Yetib kelmagan buyurtmada modelga faqat holat beriladi: viloyat,
// "filialdan o'zi olib ketadi" matni, trek va tovar nomi bo'lmaydi.
// Aks holda model Xitoydagi buyurtmani "filialda kutmoqda" deb yozardi.
func TestNotArrivedBriefIsMinimal(t *testing.T) {
	o := shippedDaysAgo(3)
	o.Province = "Buxoro viloyati"
	o.PackageName = "kiyim"
	b := briefWithChecks(o, nil)
	if b.Arrived != "belgilangan manzilga yetib kelmagan" {
		t.Fatalf("yetkazma: %q", b.Arrived)
	}
	if b.Region != "" || b.Kind != "" || b.StatusNote != "" ||
		b.ExpressNum != "" || b.PackageName != "" || b.PaidAt != "" {
		t.Fatalf("ortiqcha maydon qolgan: %+v", b)
	}
	if b.OrderSN == "" || b.StatusLabel == "" {
		t.Fatalf("holat maydonlari yo'qolgan: %+v", b)
	}

	// Treki yo'q (Xitoydan jo'natilmagan) buyurtma ham shunday.
	o.ExpressNum, o.ShippedAt = "", ""
	b = briefWithChecks(o, nil)
	if b.Kind != "" || b.Region != "" || b.Arrived == "" {
		t.Fatalf("treksiz buyurtma: %+v", b)
	}
}
