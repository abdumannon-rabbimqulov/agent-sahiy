package support

import (
	"strings"
	"testing"
)

func TestNumbersFromStaffText(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		wantSN []string
		wantEx []string
	}{
		{
			"DG lotinda",
			"DG60732205 bu buyurtmangiz tez orada sotuvchi chiqaradi",
			[]string{"DG60732205"}, nil,
		},
		{
			"DG kirillda — lotinga keltiriladi",
			"ДГ60732205 buyurtma jo'natildi",
			[]string{"DG60732205"}, nil,
		},
		{
			"DG probel bilan",
			"DG 60732205 tayyor",
			[]string{"DG60732205"}, nil,
		},
		{
			"trek raqami",
			"JT7899123456789 bilan ketdi",
			nil, []string{"JT7899123456789"},
		},
		{
			"uzun raqamli trek",
			"78975877791396 raqami bo'yicha kuzating",
			nil, []string{"78975877791396"},
		},
		{
			"telefon raqami — raqam emas",
			"mijoz +998901370006 raqamiga qo'ng'iroq qiling",
			nil, nil,
		},
		{
			"raqamsiz javob",
			"ertaga jo'natamiz",
			nil, nil,
		},
	}
	for _, c := range cases {
		sn, ex := numbersFromText(c.text)
		if !sameNums(sn, c.wantSN) {
			t.Errorf("%s: order_sn = %v, kerak %v", c.name, sn, c.wantSN)
		}
		if !sameNums(ex, c.wantEx) {
			t.Errorf("%s: trek = %v, kerak %v", c.name, ex, c.wantEx)
		}
	}
}

func TestContainsNum(t *testing.T) {
	yes := []string{
		"DG60732205 buyurtmangiz yo'lda",
		"DG 60732205 buyurtmangiz yo'lda",
		"dg-60732205 buyurtmangiz yo'lda",
		"ДГ60732205 буюртмангиз йўлда",
		"Buyurtmangiz (DG60732205) yo'lda",
	}
	for _, text := range yes {
		if !containsNum(text, "DG60732205") {
			t.Errorf("containsNum(%q) = false, kerak true", text)
		}
	}
	no := []string{
		"",
		"Buyurtmangiz yo'lda",
		"DG60732206 buyurtmangiz yo'lda",
	}
	for _, text := range no {
		if containsNum(text, "DG60732205") {
			t.Errorf("containsNum(%q) = true, kerak false", text)
		}
	}
}

func TestCardLike(t *testing.T) {
	if !cardLike("8600123412341234") {
		t.Error("8600… karta raqami deb tanilmadi")
	}
	if cardLike("78975877791396") {
		t.Error("14 xonali trek karta deb hisoblandi")
	}
	if cardLike("JT7899123456789") {
		t.Error("harfli trek karta deb hisoblandi")
	}
}

func TestEffectiveNumbers(t *testing.T) {
	group := []string{"DG60111111", "DG60222222", "DG60333333", "DG60444444"}

	t.Run("xodim bitta raqam yozgan — faqat o'sha", func(t *testing.T) {
		sn, ex := effectiveNumbers("DG60732205 tez orada chiqadi", group)
		if !sameNums(sn, []string{"DG60732205"}) {
			t.Errorf("order_sn = %v", sn)
		}
		if len(ex) != 0 {
			t.Errorf("trek = %v, bo'sh bo'lishi kerak", ex)
		}
	})

	t.Run("xodim raqam yozmagan — guruh xabaridagilar", func(t *testing.T) {
		sn, ex := effectiveNumbers("ertaga jo'natamiz", group)
		if !sameNums(sn, group) {
			t.Errorf("order_sn = %v, kerak %v", sn, group)
		}
		if len(ex) != 0 {
			t.Errorf("trek = %v", ex)
		}
	})

	t.Run("faqat trek yozgan", func(t *testing.T) {
		sn, ex := effectiveNumbers("JT7899123456789 bilan ketdi", group)
		if len(sn) != 0 {
			t.Errorf("order_sn = %v, bo'sh bo'lishi kerak", sn)
		}
		if !sameNums(ex, []string{"JT7899123456789"}) {
			t.Errorf("trek = %v", ex)
		}
	})

	t.Run("karta raqami trek bo'lib ketmaydi", func(t *testing.T) {
		sn, ex := effectiveNumbers("pulni 8600123412341234 kartaga qaytaramiz", group)
		if len(ex) != 0 {
			t.Errorf("trek = %v, karta raqami chiqib ketdi", ex)
		}
		// Xodim raqam yozmagan hisoblanadi — guruh raqamlari qoladi.
		if !sameNums(sn, group) {
			t.Errorf("order_sn = %v, kerak %v", sn, group)
		}
	})

	t.Run("guruh ham, xodim ham bo'sh", func(t *testing.T) {
		sn, ex := effectiveNumbers("ok", nil)
		if len(sn) != 0 || len(ex) != 0 {
			t.Errorf("sn = %v, ex = %v — ikkalasi bo'sh bo'lishi kerak", sn, ex)
		}
	})
}

func TestWithOrderSNNoDuplicate(t *testing.T) {
	cases := []string{
		"DG60732205 buyurtmangiz tez orada jo'natiladi.",
		"DG 60732205 buyurtmangiz tez orada jo'natiladi.",
		"ДГ60732205 буюртмангиз жўнатилади.",
	}
	for _, text := range cases {
		if got := WithOrderSN(text, []string{"DG60732205"}); got != text {
			t.Errorf("raqam takror qo'shildi:\n%q\n%q", text, got)
		}
	}
}

func TestWithOrderSNAfterGreeting(t *testing.T) {
	text := "Assalomu alaykum!\n\nBuyurtmangiz tez orada jo'natiladi."
	got := WithOrderSN(text, []string{"DG60732205"})
	if !strings.HasPrefix(got, "Assalomu alaykum!") {
		t.Fatalf("salom birinchi qatorda qolmadi: %q", got)
	}
	if !strings.Contains(got, "DG60732205 — ") {
		t.Fatalf("raqam qo'shilmadi: %q", got)
	}
	// Eng muhimi: yuborish paytidagi salom tozalash raqamni yutib
	// yubormasligi kerak (greeting.go: WithoutGreeting).
	if out := WithoutGreeting(got); !strings.Contains(out, "DG60732205") {
		t.Errorf("salom olib tashlanganda raqam yo'qoldi: %q", out)
	}
}

func TestWithOrderSNTrackSurvivesGreetingStrip(t *testing.T) {
	text := "Assalomu alaykum!\nPosilkangiz yo'lga chiqdi."
	got := WithOrderSN(text, []string{"78975877791396"})
	if out := WithoutGreeting(got); !strings.Contains(out, "78975877791396") {
		t.Errorf("trek raqami salom bilan birga o'chib ketdi: %q", out)
	}
}

func TestForeignNumberNote(t *testing.T) {
	group := []string{"DG60111111", "DG60222222"}

	if note := foreignNumberNote([]string{"DG60111111"}, group); note != "" {
		t.Errorf("xabardagi raqam uchun eslatma chiqdi: %q", note)
	}
	note := foreignNumberNote([]string{"DG60999999"}, group)
	if !strings.Contains(note, "DG60999999") {
		t.Errorf("begona raqam uchun eslatma yo'q: %q", note)
	}
	if n := foreignNumberNote([]string{"DG60999999"}, nil); n != "" {
		t.Errorf("taqqoslashga asos yo'q, eslatma bo'lmasligi kerak: %q", n)
	}
}

// sameNums - ikkita raqam ro'yxati bir xilmi (tartibi bilan).
func sameNums(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
