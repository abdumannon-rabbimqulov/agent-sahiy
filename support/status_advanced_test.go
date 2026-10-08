package support

import "testing"

// 3 → 4: buyurtma Xitoyda oldinga siljidi — muammo hal bo'lgan.
func TestStatusAdvancedPaidToWaiting(t *testing.T) {
	if !StatusAdvanced(StatusPaid, StatusWaiting) {
		t.Fatal("3 → 4 oldinga siljish deb hisoblanishi kerak")
	}
}

// 4 → 7: posilka yo'lga chiqdi — hal bo'lgan va mijozga xabar beriladi.
func TestStatusAdvancedWaitingToShipped(t *testing.T) {
	if !StatusAdvanced(StatusWaiting, StatusShipped) {
		t.Fatal("4 → 7 oldinga siljish deb hisoblanishi kerak")
	}
}

// 3 → 7: bitta bosqich sakrab o'tilgan, lekin siljish o'zgarmaydi.
func TestStatusAdvancedPaidToShipped(t *testing.T) {
	if !StatusAdvanced(StatusPaid, StatusShipped) {
		t.Fatal("3 → 7 oldinga siljish deb hisoblanishi kerak")
	}
}

// Yo'lga chiqqan buyurtma muammo emas.
func TestShippedIsNotAProblem(t *testing.T) {
	o := AdminkaOrder{PayStatus: 1, Status: StatusShipped, PaidAt: "2026-01-01 00:00:00"}
	if IsProblem(o) {
		t.Fatal("status 7 muammo bo'lmasligi kerak")
	}
}

// Status 7 ning nomi va ma'nosi bo'sh qolmasligi kerak: bo'sh bo'lsa
// model qolgan maydonlarga qarab o'zi xulosa chiqaradi.
func TestShippedHasLabelAndMeaning(t *testing.T) {
	if StatusLabel(StatusShipped) != "yo'lga chiqqan" {
		t.Fatalf("nomi noto'g'ri: %q", StatusLabel(StatusShipped))
	}
	if StatusMeaning(StatusShipped) == "" {
		t.Fatal("status 7 uchun ma'no bo'sh qolmasligi kerak")
	}
}

// Holat o'zgarmagan — hal bo'lgani yo'q.
func TestStatusAdvancedSameStatus(t *testing.T) {
	if StatusAdvanced(StatusPaid, StatusPaid) {
		t.Fatal("3 → 3 siljish emas")
	}
	if StatusAdvanced(StatusWaiting, StatusWaiting) {
		t.Fatal("4 → 4 siljish emas")
	}
	if StatusAdvanced(StatusShipped, StatusShipped) {
		t.Fatal("7 → 7 siljish emas")
	}
}

// Orqaga qaytish ham, yomonlashish ham hal bo'lish emas.
func TestStatusAdvancedNotBackwardsOrWorse(t *testing.T) {
	if StatusAdvanced(StatusWaiting, StatusPaid) {
		t.Fatal("4 → 3 siljish emas")
	}
	if StatusAdvanced(StatusShipped, StatusWaiting) {
		t.Fatal("7 → 4 siljish emas")
	}
	if StatusAdvanced(StatusShipped, StatusBanned) {
		t.Fatal("7 → 10 (taqiqlangan tovar) hal bo'lish emas")
	}
	// Eng muhimi: taqiqlangan tovar ham o'zgarish, lekin hal bo'lish EMAS.
	if StatusAdvanced(StatusWaiting, StatusBanned) {
		t.Fatal("4 → 10 (taqiqlangan tovar) hal bo'lish emas")
	}
	if StatusAdvanced(StatusPaid, StatusBanned) {
		t.Fatal("3 → 10 (taqiqlangan tovar) hal bo'lish emas")
	}
}

// 4 → 6 ("tranzaksiya yopilgan") bu funksiyaga kirmaydi: u IsProblem
// orqali o'z-o'zidan yopiladi.
func TestStatusAdvancedFinishedHandledElsewhere(t *testing.T) {
	if StatusAdvanced(StatusWaiting, StatusFinished) {
		t.Fatal("4 → 6 bu yerda emas, IsProblem orqali yopiladi")
	}
	if IsProblem(AdminkaOrder{PayStatus: 1, Status: StatusFinished, PaidAt: "2026-01-01 00:00:00"}) {
		t.Fatal("status 6 muammo bo'lmasligi kerak")
	}
}

// Status 4 ning O'ZI muammo bo'lishdan to'xtamaydi: o'sha holatda uzoq
// turib qolgan buyurtma baribir muammo.
func TestWaitingStatusStillAProblem(t *testing.T) {
	o := AdminkaOrder{PayStatus: 1, Status: StatusWaiting, PaidAt: "2026-01-01 00:00:00"}
	if !IsProblem(o) {
		t.Fatal("uzoq turgan status 4 muammo bo'lishi kerak")
	}
}
