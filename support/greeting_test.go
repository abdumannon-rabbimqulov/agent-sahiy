package support

import (
	"testing"
	"time"
)

func TestNeedsGreeting(t *testing.T) {
	today := time.Now().Format("2006-01-02 15:04:05")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")

	cases := []struct {
		name string
		msgs []Message
		want bool
	}{
		{"bugun agent yozmagan", []Message{
			{SenderType: "client", CreatedAt: today},
		}, true},
		{"agent kecha yozgan", []Message{
			{SenderType: "agent", CreatedAt: yesterday},
			{SenderType: "client", CreatedAt: today},
		}, true},
		{"agent bugun yozgan", []Message{
			{SenderType: "agent", CreatedAt: today},
			{SenderType: "client", CreatedAt: today},
		}, false},
		{"sana o'qilmadi", []Message{
			{SenderType: "agent", CreatedAt: "shalag'-shulug'"},
		}, true},
		{"bo'sh suhbat", nil, true},
	}

	for _, c := range cases {
		if got := NeedsGreeting(c.msgs); got != c.want {
			t.Errorf("%s: NeedsGreeting = %v, kutilgan %v", c.name, got, c.want)
		}
	}
}

func TestBuildUserMessageGreeting(t *testing.T) {
	withGreet := buildUserMessage("[]", nil, true)
	if !contains(withGreet, GreetingText) {
		t.Errorf("salom kerak edi, ko'rsatma yo'q: %s", withGreet)
	}
	// Salom javobning boshi bo'lishi kerak, o'zi emas.
	if !contains(withGreet, "javobning boshi") {
		t.Errorf("salom bilan cheklanmaslik ko'rsatmasi yo'q: %s", withGreet)
	}

	noGreet := buildUserMessage("[]", nil, false)
	if contains(noGreet, GreetingText) {
		t.Errorf("salom kerak emas edi, ko'rsatma bor: %s", noGreet)
	}

	// Tushunmasa nima so'rashi — ikkala holatda ham aytiladi.
	for _, s := range []string{withGreet, noGreet} {
		if !contains(s, AskHelpText) {
			t.Errorf("\"%s\" ko'rsatmasi yo'q: %s", AskHelpText, s)
		}
	}
}
