package support

import (
	"strings"
	"testing"
	"time"
)

func fail(key string) SystemCheck {
	return SystemCheck{Key: key, Name: "Adminka", Status: HealthFail,
		Message: "token rad etildi (401)", Affects: "buyurtma holati", Manual: true}
}

// Birinchi marta yiqilsa — xabar ketadi va nima ishlamayotgani yoziladi.
func TestHealthAlertFirstFailure(t *testing.T) {
	text, ok := healthAlertText(fail("adminka"), "", time.Time{})
	if !ok {
		t.Fatal("yiqilganda xabar yuborilmadi")
	}
	for _, want := range []string{"Adminka", "401", "buyurtma holati", "qo'lda"} {
		if !strings.Contains(text, want) {
			t.Errorf("xabarda %q yo'q:\n%s", want, text)
		}
	}
}

// Holat o'zgarmasa va eslatma vaqti kelmasa — xabar takrorlanmaydi.
func TestHealthAlertNoRepeatBeforeInterval(t *testing.T) {
	t.Setenv("HEALTH_REPEAT_HOURS", "12")
	if _, ok := healthAlertText(fail("adminka"), HealthFail, time.Now().Add(-time.Hour)); ok {
		t.Error("1 soatdan keyin takror xabar ketdi — 12 soat kutilishi kerak")
	}
}

// Eslatma vaqti kelganda — xabar "tuzalmagan" izohi bilan ketadi.
func TestHealthAlertRepeatAfterInterval(t *testing.T) {
	t.Setenv("HEALTH_REPEAT_HOURS", "12")
	text, ok := healthAlertText(fail("adminka"), HealthFail, time.Now().Add(-13*time.Hour))
	if !ok {
		t.Fatal("13 soatdan keyin eslatma ketmadi")
	}
	if !strings.Contains(text, "tuzalmagan") {
		t.Errorf("eslatma matni kutilganidek emas:\n%s", text)
	}
}

// HEALTH_REPEAT_HOURS=0 — eslatma butunlay o'chadi.
func TestHealthAlertRepeatDisabled(t *testing.T) {
	t.Setenv("HEALTH_REPEAT_HOURS", "0")
	if _, ok := healthAlertText(fail("adminka"), HealthFail, time.Now().Add(-100*time.Hour)); ok {
		t.Error("eslatma o'chirilgan bo'lsa ham xabar ketdi")
	}
}

// warn → fail: holat yomonlashdi, darhol xabar.
func TestHealthAlertWorsened(t *testing.T) {
	if _, ok := healthAlertText(fail("adminka"), HealthWarn, time.Now()); !ok {
		t.Error("warn → fail o'zgarishi haqida xabar ketmadi")
	}
}

// Tuzalganda bir marta "tuzaldi", keyin jim.
func TestHealthAlertRecovered(t *testing.T) {
	ok1 := SystemCheck{Key: "adminka", Name: "Adminka", Status: HealthOK, Message: "ishlayapti"}

	text, ok := healthAlertText(ok1, HealthFail, time.Now())
	if !ok || !strings.Contains(text, "tuzaldi") {
		t.Fatalf("tuzalganda xabar ketmadi: %q, %v", text, ok)
	}
	if _, ok := healthAlertText(ok1, HealthOK, time.Now()); ok {
		t.Error("holat yaxshi bo'lib turganda ham xabar ketdi")
	}
	// Hech qachon xabar ketmagan va holat yaxshi — jim turishi kerak.
	if _, ok := healthAlertText(ok1, "", time.Time{}); ok {
		t.Error("ishga tushganda sababsiz \"tuzaldi\" xabari ketdi")
	}
}

// lastHealthAlert saqlangan satrni o'qiydi; buzuq qiymatda yiqilmaydi.
func TestLastHealthAlertParse(t *testing.T) {
	if s, at := lastHealthAlert("yoq-bunday-kalit"); s != "" || !at.IsZero() {
		t.Errorf("bo'sh kalit = %q, %v; kutilgan \"\", nol vaqt", s, at)
	}
}
