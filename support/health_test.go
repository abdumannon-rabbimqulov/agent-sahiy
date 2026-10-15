package support

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// jwt - test uchun faqat payload'i to'g'ri bo'lgan token yasaydi
// (imzo tekshirilmaydi: jwtExpiry faqat "exp" ni o'qiydi).
func jwt(t *testing.T, exp time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return "x." + base64.RawURLEncoding.EncodeToString(payload) + ".y"
}

func TestJWTExpiry(t *testing.T) {
	want := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	got, ok := jwtExpiry(jwt(t, want))
	if !ok || !got.Equal(want) {
		t.Fatalf("jwtExpiry = %v, %v; kutilgan %v, true", got, ok, want)
	}

	// "Bearer " old qo'shimchasi bilan ham ishlashi kerak: .env da
	// token shu ko'rinishda yozilishi mumkin.
	if _, ok := jwtExpiry("Bearer " + jwt(t, want)); !ok {
		t.Error("\"Bearer \" prefiksi bilan muddat o'qilmadi")
	}

	for _, bad := range []string{"", "abc", "a.b", "a.!!!.c"} {
		if _, ok := jwtExpiry(bad); ok {
			t.Errorf("jwtExpiry(%q) ok qaytardi — JWT emas", bad)
		}
	}
}

// Token bo'sh bo'lsa tashqi so'rov yubormasdan "fail" qaytishi kerak.
func TestCheckAdminkaNoToken(t *testing.T) {
	t.Setenv("ADMINKA_TOKEN_BEARER", "")
	c := checkAdminka()
	if c.Status != HealthFail {
		t.Fatalf("status = %q; kutilgan %q", c.Status, HealthFail)
	}
	if c.Affects == "" {
		t.Error("Affects bo'sh — panelda nima ishlamayotgani ko'rinmaydi")
	}
}

// Muddati tugagan token: so'rov yuborilmaydi, darhol "fail".
func TestCheckAdminkaExpired(t *testing.T) {
	t.Setenv("ADMINKA_TOKEN_BEARER", jwt(t, time.Now().Add(-time.Hour)))
	c := checkAdminka()
	if c.Status != HealthFail {
		t.Fatalf("status = %q; kutilgan %q", c.Status, HealthFail)
	}
	if c.DaysLeft == nil || *c.DaysLeft > 0 {
		t.Errorf("days_left = %v; kutilgan manfiy", c.DaysLeft)
	}
}

// Umumiy holat eng yomon tizim bo'yicha hisoblanadi.
func TestHealthReportHTTPStatus(t *testing.T) {
	if got := (HealthReport{Status: HealthFail}).HTTPStatus(); got != 503 {
		t.Errorf("fail → %d; kutilgan 503", got)
	}
	for _, s := range []string{HealthOK, HealthWarn} {
		if got := (HealthReport{Status: s}).HTTPStatus(); got != 200 {
			t.Errorf("%s → %d; kutilgan 200", s, got)
		}
	}
}
