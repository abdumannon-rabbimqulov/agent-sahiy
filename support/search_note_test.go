package support

import (
	"strings"
	"testing"
)

// Mijoz raqamni yozgan, lekin bazada topilmadi: modelga "qidirildi,
// chiqmadi" deb aytiladi va raqamni QAYTA so'rash taqiqlanadi. Busiz
// model bo'sh ro'yxatga qarab "buyurtma raqamingizni yuboring" deb
// yozardi va suhbat shu savolda aylanib qolardi.
func TestSearchNoteNotFound(t *testing.T) {
	q, alert := searchNote([]string{"DG60633349"}, false, false, false)

	if q["natija"] != "topilmadi" {
		t.Errorf("natija = %v, kerak topilmadi", q["natija"])
	}
	note, _ := q["korsatma"].(string)
	if !strings.Contains(note, "QAYTA") {
		t.Errorf("korsatma raqamni qayta so'rashni taqiqlamaydi: %q", note)
	}
	if !strings.Contains(alert, "DG60633349") {
		t.Errorf("xodimga ketadigan qatorda raqam yo'q: %q", alert)
	}
}

// Topildi: ko'rsatma ham, xodimga qator ham kerak emas — javobni model
// odatdagidek buyurtma ma'lumotidan yozadi.
func TestSearchNoteFound(t *testing.T) {
	q, alert := searchNote([]string{"DG60633349"}, true, false, false)

	if q["natija"] != "topildi" {
		t.Errorf("natija = %v, kerak topildi", q["natija"])
	}
	if _, ok := q["korsatma"]; ok {
		t.Errorf("topilgan buyurtma uchun korsatma qo'yilmasligi kerak: %v", q["korsatma"])
	}
	if alert != "" {
		t.Errorf("xodimga bekordan qator ketdi: %q", alert)
	}
}

// Qidiruvning O'ZI ishlamadi: "topilmadi" deyish yolg'on bo'ladi, lekin
// raqam ham qayta so'ralmaydi — murojaat xodimga topshiriladi.
func TestSearchNoteLookupError(t *testing.T) {
	q, alert := searchNote([]string{"DG60633349"}, false, true, false)

	if q["natija"] != "aniqlanmadi" {
		t.Errorf("natija = %v, kerak aniqlanmadi", q["natija"])
	}
	if q["korsatma"] != lookupErrNote {
		t.Errorf("korsatma = %v, kerak lookupErrNote", q["korsatma"])
	}
	if alert == "" {
		t.Error("texnik xatolik xodimga chiqmadi")
	}
}

// Raqam begona buyurtmaniki: ko'rsatma allaqachon `begona_buyurtma`
// ichida (foreignOrderNote) — ikkinchisi unga zid bo'lib qolardi.
func TestSearchNoteForeign(t *testing.T) {
	q, alert := searchNote([]string{"DG60633349"}, false, false, true)

	if q["natija"] != "topilmadi" {
		t.Errorf("natija = %v, kerak topilmadi", q["natija"])
	}
	if _, ok := q["korsatma"]; ok {
		t.Errorf("begona buyurtma uchun ikkinchi korsatma qo'yildi: %v", q["korsatma"])
	}
	if alert != "" {
		t.Errorf("begona buyurtma uchun qator ketdi: %q", alert)
	}
}

// Mijoz yozgan raqamlar javob yozadigan bosqichlarga bayroq bo'lib
// boradi: raqam qo'lda bo'lsa uni qayta so'rash mantiqsiz.
func TestFlagsBlockAskedNums(t *testing.T) {
	got := flagsBlock(map[string]any{
		FlagAskedNums: []string{"DG60633349"},
		FlagPending:   true,
	})
	want := flagsHeader + `{"mijoz_yozgan_raqamlar":["DG60633349"],"olinmagan_buyurtma_bor":true}`
	if got != want {
		t.Errorf("flagsBlock =\n%s\nkerak\n%s", got, want)
	}
}
