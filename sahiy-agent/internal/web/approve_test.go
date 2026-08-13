package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sahiy-agent/internal/models"
)

// draftGate — tasdiqlash shartlarini tekshiradi (bazasiz, faqat qoidalar).
func TestDraftHolatShartlari(t *testing.T) {
	cases := []struct {
		name string
		in   models.Interaction
		ok   bool
	}{
		{"tekshirishni kutayotgan javob", models.Interaction{Status: models.StatusAIDraft, AIReply: "salom"}, true},
		{"allaqachon yuborilgan", models.Interaction{Status: models.StatusAIDraft, AIReply: "salom", Sent: true}, false},
		{"AI o'zi yuborgan", models.Interaction{Status: models.StatusAISent, AIReply: "salom"}, false},
		{"eskalatsiya", models.Interaction{Status: models.StatusPending, AIReply: "salom"}, false},
		{"rad etilgan", models.Interaction{Status: models.StatusRejected, AIReply: "salom"}, false},
		{"bo'sh matn", models.Interaction{Status: models.StatusAIDraft, AIReply: "   "}, false},
	}
	for _, c := range cases {
		got := c.in.Status == models.StatusAIDraft && !c.in.Sent && strings.TrimSpace(c.in.AIReply) != ""
		if got != c.ok {
			t.Errorf("%s: tasdiqlash mumkinmi = %v, kutilgan %v", c.name, got, c.ok)
		}
	}
}

// Sozlamalar endpointi noma'lum kalitni qabul qilmasligi kerak.
func TestSettingsNomalumKalit(t *testing.T) {
	body := strings.NewReader(`{"delete_everything": true}`)
	r := httptest.NewRequest("PUT", "/api/settings", body)
	var payload map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for k := range payload {
		if k == "ai_enabled" || k == "auto_reply" {
			t.Errorf("test noto'g'ri: %q ruxsat etilgan kalit", k)
		}
	}
}

func TestPathID(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/interactions/42/send", nil)
	r.SetPathValue("id", "42")
	id, err := pathID(r)
	if err != nil || id != 42 {
		t.Errorf("pathID = %d, err=%v", id, err)
	}
	for _, bad := range []string{"0", "abc", ""} {
		r := httptest.NewRequest("POST", "/x", nil)
		r.SetPathValue("id", bad)
		if _, err := pathID(r); err == nil {
			t.Errorf("%q uchun xato kutilgan", bad)
		}
	}
	_ = http.StatusOK
}
