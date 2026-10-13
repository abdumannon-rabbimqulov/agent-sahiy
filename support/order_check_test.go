package support

import (
	"strings"
	"testing"
	"time"
)

// shippedOrder - tekshiruv uchun buyurtma: to'langan, berilgan statusda,
// `paidDays` kun oldin to'langan va `shippedDays` kun oldin yo'lga
// chiqqan (shippedDays < 0 — jo'natish sanasi YO'Q).
func shippedOrder(status, shippedDays, paidDays int) AdminkaOrder {
	o := AdminkaOrder{
		PayStatus:  1,
		Status:     status,
		OrderSN:    "DG1",
		ExpressNum: "YT1",
		UserID:     7,
		PaidAt:     time.Now().AddDate(0, 0, -paidDays).Format(adminkaTimeLayout),
	}
	if shippedDays >= 0 {
		o.ShippedAt = time.Now().AddDate(0, 0, -shippedDays).Format(adminkaTimeLayout)
	}
	return o
}

// checked - buyurtmani yetkazma javobi bilan ketma-ketlikdan o'tkazadi.
func checked(o AdminkaOrder, rows []DeliveryOrder) OrderCheck {
	return CheckOrder(OrderCheckInput{
		Order: o, Rows: rows, HaveDelivery: true, DeliveryOK: true,
	})
}

// To'lanmagan buyurtma kutib turishi normal: hech narsa tekshirilmaydi.
func TestUnpaidOrderIsNeverChecked(t *testing.T) {
	o := shippedOrder(StatusWaiting, 50, 100)
	o.PayStatus = 0
	c := checked(o, nil)
	if c.Problem || c.Step != StepUnpaid {
		t.Fatalf("to'lanmagan buyurtma tekshirilmasligi kerak: %+v", c)
	}
}

// Treki yo'q buyurtma yetkazmada qidirilmaydi — qaror adminka
// statusiga qarab chiqadi.
func TestNoTrackFallsBackToStatusRule(t *testing.T) {
	o := shippedOrder(StatusPaid, -1, 10)
	o.ExpressNum = ""
	c := checked(o, nil)
	if c.Dash != DashUnknown {
		t.Fatalf("trek yo'q: yetkazma javobi noma'lum bo'lishi kerak, keldi %v", c.Dash)
	}
	if !c.Problem || c.Reason != ReasonStuckStatus {
		t.Fatalf("10 kun status 3 da turgan buyurtma muammo bo'lishi kerak: %+v", c)
	}
}

// Yetkazma so'rovi xato bergan bo'lsa "yetkazmada yo'q" deb xulosa
// chiqarilmaydi: aks holda API bir marta yotganda mijozning butun
// ro'yxati guruhga muammo bo'lib yog'iladi.
func TestDeliveryErrorIsNotAProblem(t *testing.T) {
	c := CheckOrder(OrderCheckInput{
		Order: shippedOrder(StatusFinished, 40, 60), HaveDelivery: true, DeliveryOK: false,
	})
	if c.Problem {
		t.Fatalf("so'rov xato bergan — muammo ochilmasligi kerak: %+v", c)
	}
	// Yetkazma javobi NOMA'LUM bo'lib qoladi: "topilmadi" emas.
	if c.Dash != DashUnknown {
		t.Fatalf("yetkazma javobi noma'lum bo'lishi kerak, keldi %v", c.Dash)
	}
	if c.Late != "" || c.StaffText != "" {
		t.Fatal("tekshira olmagan holat uchun ogohlantirish chiqmaydi")
	}
}

// Posilka yetkazmada bor — adminkadagi holat nima deb tursa ham
// muammo emas.
func TestDashboardFoundIsNeverProblem(t *testing.T) {
	c := checked(shippedOrder(StatusWaiting, 40, 200),
		[]DeliveryOrder{{ExpressNum: "YT1", UserID: 7}})
	if c.Problem || c.Step != StepDashFound {
		t.Fatalf("kelgan posilka muammo emas: %+v", c)
	}
	if c.Arrived != arrivedInDelivery {
		t.Fatalf("posilka kelgani yozilishi kerak: %q", c.Arrived)
	}
}

// Yetkazmadagi egasi adminkadagiga mos kelmasa — muammo va xodimga
// qator chiqadi (ilgari xabar umuman chiqmasdi, muammo shunchaki
// ochiq qolardi).
func TestOwnerMismatchStaysProblemAndAlerts(t *testing.T) {
	c := checked(shippedOrder(StatusWaiting, 5, 50),
		[]DeliveryOrder{{ExpressNum: "YT1", UserID: 9}})
	if !c.Problem || c.Reason != ReasonOwner {
		t.Fatalf("egasi mos kelmadi — muammo bo'lishi kerak: %+v", c)
	}
	if !strings.Contains(c.StaffText, "DG1") {
		t.Fatalf("xodimga qator chiqishi kerak: %q", c.StaffText)
	}
	if c.OnTheWay != "" || c.ETA != "" {
		t.Fatal("xato topilgan buyurtmada muddat aytilmaydi")
	}
}

// Trek begona akkauntda chiqqan bo'lsa (qator ro'yxatdan olib
// tashlangan) bu EGASI xatosi, "yo'l muddati o'tdi" emas.
func TestForeignTrackIsOwnerMismatchNotLate(t *testing.T) {
	c := CheckOrder(OrderCheckInput{
		Order: shippedOrder(StatusWaiting, 25, 60), HaveDelivery: true,
		DeliveryOK: true, ForeignTrack: true,
	})
	if c.Reason != ReasonOwner {
		t.Fatalf("sabab %q bo'lishi kerak, keldi %q", ReasonOwner, c.Reason)
	}
}

// Adminkadagi viloyat bilan posilka tushgan joy mos kelmasa — muammo.
func TestProvinceMismatchBecomesProblem(t *testing.T) {
	o := shippedOrder(StatusWaiting, 5, 50)
	o.Province = "Jizzax"
	c := checked(o, []DeliveryOrder{{ExpressNum: "YT1", UserID: 7, City: "Farg'ona"}})
	if !c.Problem || c.Reason != ReasonRegion {
		t.Fatalf("viloyat mos kelmadi — muammo bo'lishi kerak: %+v", c)
	}
	if !strings.Contains(c.Mismatch, "tekshirilmoqda") {
		t.Fatalf("mijozga faqat \"tekshirilmoqda\" deyilishi kerak: %q", c.Mismatch)
	}
}

// Ikki tomondan biri bo'sh yoki tanilmagan bo'lsa xato deb
// hisoblanmaydi: `province` ham, `city` ham ba'zan bo'sh keladi.
func TestEmptyProvinceIsNotAMismatch(t *testing.T) {
	o := shippedOrder(StatusWaiting, 5, 50) // province bo'sh
	c := checked(o, []DeliveryOrder{{ExpressNum: "YT1", UserID: 7, City: "Farg'ona"}})
	if c.Problem {
		t.Fatalf("viloyat noma'lum — xato deb hisoblanmasligi kerak: %+v", c)
	}
}

// Qolgan kun oralig'i: [qolgan-5 … qolgan].
func TestOnTheWayETARange(t *testing.T) {
	cases := []struct {
		shipped  int
		from, to int
	}{
		{12, 3, 8},  // qolgan 8
		{18, 1, 2},  // qolgan 2
		{1, 14, 19}, // qolgan 19
		{0, 15, 20}, // bugun chiqdi
		{15, 1, 5},  // qolgan 5
	}
	for _, c := range cases {
		from, to := OrderETA(c.shipped)
		if from != c.from || to != c.to {
			t.Fatalf("%d kun o'tgan: %d-%d kutilgan, %d-%d keldi",
				c.shipped, c.from, c.to, from, to)
		}
	}
}

// Oraliq 1 kundan tushmaydi: "0 kun ichida keladi" mijozga "bugun"
// deb o'qiladi va bajarilmaydigan va'da bo'ladi.
func TestETANeverGoesBelowOneDay(t *testing.T) {
	for _, d := range []int{19, 20} {
		from, to := OrderETA(d)
		if from < 1 || to < 1 {
			t.Fatalf("%d kun: oraliq 1 dan tushmasligi kerak, %d-%d keldi", d, from, to)
		}
	}
	// Sana yo'q — oraliq umuman yo'q.
	if from, to := OrderETA(-1); from != 0 || to != 0 {
		t.Fatalf("sana yo'q: oraliq bo'lmasligi kerak, %d-%d keldi", from, to)
	}
}

// Yo'l muddati o'tgan posilka status 6 ("Xitoyda tranzaksiya
// yopilgan") da ham muammo: bu status kuzatiladigan ro'yxatda yo'q,
// ya'ni DetectIssues uni ko'rmaydi.
func TestLateShipmentIsProblemEvenForFinishedStatus(t *testing.T) {
	c := checked(shippedOrder(StatusFinished, 25, 60), nil)
	if !c.Problem || c.Reason != ReasonLateShipment {
		t.Fatalf("25 kun yo'lda, yetkazmada yo'q — muammo bo'lishi kerak: %+v", c)
	}
	if c.AskReceived != "" {
		t.Fatal("20-30 kun orasida mijozdan hech narsa so'ralmaydi")
	}
	if !strings.Contains(c.Late, "tekshirilmoqda") {
		t.Fatalf("mijozga \"tekshirilmoqda\" deyilishi kerak: %q", c.Late)
	}
}

// 30 kundan oshsa mijozdan posilkani olgan-olmagani so'raladi.
func TestVeryLateShipmentAsksCustomer(t *testing.T) {
	c := checked(shippedOrder(StatusShipped, 35, 60), nil)
	if !c.Problem || c.Reason != ReasonVeryLate {
		t.Fatalf("35 kun yo'lda — mijozdan so'rash kerak: %+v", c)
	}
	if c.AskReceived == "" {
		t.Fatal("mijozdan so'rash ko'rsatmasi berilishi kerak")
	}
}

// Jo'natish sanasi ham yo'q, yetkazmada ham yo'q — muammo.
func TestNoShipDateAndNotInDashboardIsProblem(t *testing.T) {
	c := checked(shippedOrder(StatusWaiting, -1, 30), nil)
	if !c.Problem || c.Reason != ReasonNoShipDate {
		t.Fatalf("sana ham, yetkazma ham yo'q — muammo bo'lishi kerak: %+v", c)
	}
}

// Sanasi yo'q, lekin hali kun chegarasiga yetmagan buyurtma muammo
// emas: bir soat oldin to'langan buyurtma xodimga chiqmasligi kerak.
func TestNoShipDateUnderThresholdIsNotAProblem(t *testing.T) {
	c := checked(shippedOrder(StatusWaiting, -1, 2), nil)
	if c.Problem {
		t.Fatalf("status 4 uchun chegara 7 kun — 2 kun muammo emas: %+v", c)
	}
}

// Sana o'qilmasa muddat VA'DA QILINMAYDI.
func TestUnparsableShipDateDoesNotPromiseETA(t *testing.T) {
	o := shippedOrder(StatusWaiting, -1, 30)
	o.ShippedAt = "tez orada"
	c := checked(o, nil)
	if c.ETA != "" || c.OnTheWay != "" {
		t.Fatalf("o'qilmagan sanadan oraliq chiqmasligi kerak: %q / %q", c.ETA, c.OnTheWay)
	}
}

// Status 12 (abnormality) ham kuzatiladi va 3 kundan keyin muammo
// bo'ladi.
func TestStatus12IsWatchedLikeThreeAndFour(t *testing.T) {
	if !watchedStatus(StatusAbnormal) {
		t.Fatal("status 12 kuzatiladigan ro'yxatda bo'lishi kerak")
	}
	if c := checked(shippedOrder(StatusAbnormal, -1, 4), nil); !c.Problem {
		t.Fatalf("status 12, 4 kun — muammo bo'lishi kerak: %+v", c)
	}
	if c := checked(shippedOrder(StatusAbnormal, -1, 2), nil); c.Problem {
		t.Fatalf("status 12, 2 kun — hali muammo emas: %+v", c)
	}
	if StatusMeaning(StatusAbnormal) == "" {
		t.Fatal("status 12 uchun modelga ma'no berilishi kerak")
	}
}

// Har bir status uchun o'z chegarasi: 3 → 3 kun, 4 → 7 kun, 12 → 3 kun.
func TestPerStatusDaysThresholds(t *testing.T) {
	if d := StatusProblemDays(StatusPaid); d != 3 {
		t.Fatalf("status 3 uchun 3 kun kutilgan, %d keldi", d)
	}
	if d := StatusProblemDays(StatusWaiting); d != 7 {
		t.Fatalf("status 4 uchun 7 kun kutilgan, %d keldi", d)
	}
	if d := StatusProblemDays(StatusAbnormal); d != 3 {
		t.Fatalf("status 12 uchun 3 kun kutilgan, %d keldi", d)
	}
	// Status 4: 5 kun — hali muammo emas, 8 kun — muammo.
	if c := checked(shippedOrder(StatusWaiting, -1, 5), nil); c.Problem {
		t.Fatalf("status 4, 5 kun — muammo emas: %+v", c)
	}
	if c := checked(shippedOrder(StatusWaiting, -1, 8), nil); !c.Problem {
		t.Fatalf("status 4, 8 kun — muammo: %+v", c)
	}
}

// .env dagi PROBLEM_STATUS_DAYS ustun turadi, ro'yxatda yo'q status
// uchun esa eski PROBLEM_DAYS ishlatiladi.
func TestPerStatusDaysFromEnv(t *testing.T) {
	t.Setenv("PROBLEM_STATUS_DAYS", "4:1,xato,12:")
	t.Setenv("PROBLEM_DAYS", "9")
	if d := StatusProblemDays(StatusWaiting); d != 1 {
		t.Fatalf("sozlamadagi 1 kun kutilgan, %d keldi", d)
	}
	// Noto'g'ri juftliklar e'tiborsiz qoldiriladi, butun sozlama
	// tashlanmaydi: ro'yxatda yo'q status PROBLEM_DAYS ga tushadi.
	if d := StatusProblemDays(StatusPaid); d != 9 {
		t.Fatalf("zaxira chegara 9 kutilgan, %d keldi", d)
	}
}

// Yaqinda yo'lga chiqqan buyurtma, yetkazma javobi noma'lum bo'lsa ham
// muammo emas: posilka yo'lda.
func TestFreshShipmentSurvivesUnknownDelivery(t *testing.T) {
	c := CheckOrder(OrderCheckInput{Order: shippedOrder(StatusWaiting, 10, 90)})
	if c.Problem {
		t.Fatalf("10 kun oldin yo'lga chiqqan buyurtma muammo emas: %+v", c)
	}
}

// Xodimga qator faqat mijoz O'ZI so'ragan buyurtma uchun chiqadi:
// yo'ldagi buyurtma ham, mijoz so'ramagan eski muammo ham chiqmaydi.
func TestCheckAlertsOnlyAskedProblems(t *testing.T) {
	onTheWay := checked(shippedOrder(StatusWaiting, 3, 90), nil)
	late := checked(shippedOrder(StatusFinished, 25, 90), nil)
	checks := []OrderCheck{onTheWay, late}

	asked := map[string]bool{"YT1": true}
	if got := CheckAlerts(checks, asked, nil); len(got) != 1 {
		t.Fatalf("1 qator kutilgan, %d keldi: %v", len(got), got)
	}
	// Mijoz so'ramagan: guruhga chiqmaydi (muammo yozuvi bazada
	// ochiq turadi).
	if got := CheckAlerts(checks, nil, nil); len(got) != 0 {
		t.Fatalf("so'ralmagan muammo chiqmasligi kerak: %v", got)
	}
}

// Hozir ochilgan muammo mijoz so'ramasa ham chiqadi: xodim uni
// birinchi marta ko'radi.
func TestCheckAlertsIncludesFreshlyOpened(t *testing.T) {
	late := checked(shippedOrder(StatusFinished, 25, 90), nil)
	opened := []*OrderIssue{{OrderSN: late.OrderSN}}
	if got := CheckAlerts([]OrderCheck{late}, nil, opened); len(got) != 1 {
		t.Fatalf("yangi ochilgan muammo chiqishi kerak: %v", got)
	}
}
