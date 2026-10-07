package support

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHasGreeting(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Assalomu alaykum! Buyurtmangiz yo'lda.", true},
		{"DG60607041 — Assalomu alaykum, buyurtmangiz yo'lda.", true},
		{"DG60607041, DG60607042 — Ассалому алайкум! Буюртмангиз йўлда.", true},
		{"Здравствуйте! Ваш заказ в пути.", true},
		{"Buyurtmangiz ertaga jo'natiladi.", false},
		{"DG60607041 — buyurtmangiz ertaga jo'natiladi.", false},
		{"", false},
		// Salom matnning OXIRIDA bo'lsa — bu salom emas (xayrlashuv emas,
		// shunchaki so'z): bosh qismdan tashqarida qolishi kerak.
		{strings.Repeat("Buyurtmangiz ertaga jo'natiladi. ", 3) + "salom", false},
	}
	for _, c := range cases {
		if got := hasGreeting(c.text); got != c.want {
			t.Errorf("hasGreeting(%q) = %v, kerak %v", c.text, got, c.want)
		}
	}
}

func TestWithGreetingIdempotent(t *testing.T) {
	text := "DG60607041 — buyurtmangiz ertaga jo'natiladi."
	once := WithGreeting(text)
	if !strings.HasPrefix(once, GreetUzLat) {
		t.Fatalf("salom qo'shilmadi: %q", once)
	}
	if !strings.Contains(once, "DG60607041") {
		t.Fatalf("buyurtma raqami yo'qoldi: %q", once)
	}
	if twice := WithGreeting(once); twice != once {
		t.Errorf("ikkinchi chaqiruv matnni o'zgartirdi:\n%q\n%q", once, twice)
	}
}

func TestWithGreetingLang(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Buyurtmangiz ertaga jo'natiladi.", GreetUzLat},
		{"Буюртмангиз эртага жўнатилади.", GreetUzCyr},
		{"Буюртмангиз келди, олиб кетишингиз мумкин.", GreetUzCyr},
		{"Ваш заказ отправлен, ожидайте доставку.", GreetRU},
	}
	for _, c := range cases {
		got := WithGreeting(c.text)
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("WithGreeting(%q) = %q, kerak %q bilan boshlanishi", c.text, got, c.want)
		}
	}
}

func TestWithoutGreeting(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{
			"birinchi qator salom",
			"Assalomu alaykum!\n\nBuyurtmangiz ertaga jo'natiladi.",
			"Buyurtmangiz ertaga jo'natiladi.",
		},
		{
			"kirill salom",
			"Ассалому алайкум!\n\nБуюртмангиз эртага жўнатилади.",
			"Буюртмангиз эртага жўнатилади.",
		},
		{
			"rus salom",
			"Здравствуйте!\n\nВаш заказ отправлен.",
			"Ваш заказ отправлен.",
		},
		{
			// Salom gap ichida — javobning bir qismi, tegilmaydi.
			"gap ichidagi salom",
			"Assalomu alaykum, buyurtmangiz ertaga jo'natiladi.\nSavolingiz bo'lsa yozing.",
			"Assalomu alaykum, buyurtmangiz ertaga jo'natiladi.\nSavolingiz bo'lsa yozing.",
		},
		{
			"salom yo'q",
			"Buyurtmangiz ertaga jo'natiladi.\nSavolingiz bo'lsa yozing.",
			"Buyurtmangiz ertaga jo'natiladi.\nSavolingiz bo'lsa yozing.",
		},
		{
			"bitta qator",
			"Assalomu alaykum! Buyurtmangiz ertaga jo'natiladi.",
			"Assalomu alaykum! Buyurtmangiz ertaga jo'natiladi.",
		},
	}
	for _, c := range cases {
		if got := WithoutGreeting(c.text); got != c.want {
			t.Errorf("%s: WithoutGreeting = %q, kerak %q", c.name, got, c.want)
		}
	}
}

// TestWithGreetingThenWithout - ikki funksiya bir-birining teskarisi.
func TestWithGreetingThenWithout(t *testing.T) {
	text := "DG60607041 — buyurtmangiz ertaga jo'natiladi."
	if got := WithoutGreeting(WithGreeting(text)); got != text {
		t.Errorf("borish-kelish matnni buzdi: %q", got)
	}
}

// TestOurMessageToday - needGreeting ning tarix yarmi: bugun bizdan ketgan
// xabar bo'lsa salom berilmaydi.
func TestOurMessageToday(t *testing.T) {
	start := startOfToday()
	yesterday := start.Add(-2 * time.Hour)   // kecha 22:00
	today := start.Add(9 * time.Hour)        // bugun 09:00
	midnight := start.Add(1 * time.Minute)   // bugun 00:01
	lateNight := start.Add(-1 * time.Minute) // kecha 23:59

	cases := []struct {
		name string
		msgs []Message
		want bool // bugun bizdan xabar ketganmi
	}{
		{"xabar yo'q", nil, false},
		{
			"kecha javob berilgan",
			[]Message{
				{SenderType: "agent", CreatedAt: yesterday.Format(time.RFC3339)},
				{SenderType: "client", CreatedAt: today.Format(time.RFC3339)},
			},
			false,
		},
		{
			"bugun javob berilgan",
			[]Message{
				{SenderType: "client", CreatedAt: yesterday.Format(time.RFC3339)},
				{SenderType: "agent", CreatedAt: today.Format(time.RFC3339)},
			},
			true,
		},
		{
			"kecha 23:59 — bugun hisoblanmaydi",
			[]Message{{SenderType: "agent", CreatedAt: lateNight.Format(time.RFC3339)}},
			false,
		},
		{
			"bugun 00:01 — bugun hisoblanadi",
			[]Message{{SenderType: "agent", CreatedAt: midnight.Format(time.RFC3339)}},
			true,
		},
		{
			// Adminka formati: "2026-10-07 09:00:00".
			"adminka formatidagi sana",
			[]Message{{SenderType: "agent", CreatedAt: today.Format("2006-01-02 15:04:05")}},
			true,
		},
		{
			"faqat mijoz xabarlari",
			[]Message{
				{SenderType: "client", CreatedAt: today.Format(time.RFC3339)},
				{SenderType: "client", CreatedAt: today.Format(time.RFC3339)},
			},
			false,
		},
		{
			"o'qilmagan sana — e'tiborga olinmaydi",
			[]Message{{SenderType: "agent", CreatedAt: "nomalum"}},
			false,
		},
	}
	for _, c := range cases {
		if got := ourMessageToday(c.msgs, start); got != c.want {
			t.Errorf("%s: ourMessageToday = %v, kerak %v", c.name, got, c.want)
		}
	}
}

// TestGreetLangOf - salom tili javob matnidan to'g'ri aniqlanadi.
func TestGreetLangOf(t *testing.T) {
	cases := []struct {
		text string
		want closingLang
	}{
		{"Buyurtmangiz yo'lda", langUzLat},
		{"Буюртмангиз йўлда", langUzCyr},
		{"Буюртмангиз келди", langUzCyr}, // qisqa, o'zbekcha harflarsiz
		{"Ваш заказ уже в пути", langRU},
		{"", langUzLat},
	}
	for _, c := range cases {
		if got := greetLangOf(c.text); got != c.want {
			t.Errorf("greetLangOf(%q) = %v, kerak %v", c.text, got, c.want)
		}
	}
}

// Tekshiruvlar matnlari chindan ham farqli bo'lishiga ishonch.
func TestGreetTextsDiffer(t *testing.T) {
	seen := map[string]bool{}
	for _, lang := range []closingLang{langUzLat, langUzCyr, langRU, langUnknown} {
		txt := greetText(lang)
		if txt == "" {
			t.Fatalf("%v uchun salom matni bo'sh", lang)
		}
		seen[txt] = true
	}
	if len(seen) != 3 {
		t.Errorf("3 xil salom matni kerak, bor: %d (%v)", len(seen), fmt.Sprint(seen))
	}
}
