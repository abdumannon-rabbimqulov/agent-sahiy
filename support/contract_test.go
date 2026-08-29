package support

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestParseAgentJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want AgentJSON
	}{
		{"oddiy", `{"dashboard":true,"chat":"Salom","promt":4}`, AgentJSON{Dashboard: true, Chat: "Salom"}},
		{"ramka", "```json\n{\"adminka\":true,\"promt\":null}\n```", AgentJSON{Adminka: true}},
		{"matn ichida", `Mana javob: {"chat":"Xo'p {a}","promt":2} tamom`, AgentJSON{Chat: "Xo'p {a}"}},
	}
	for _, c := range cases {
		got, err := ParseAgentJSON(c.raw)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Dashboard != c.want.Dashboard || got.Adminka != c.want.Adminka || got.Chat != c.want.Chat {
			t.Errorf("%s: %+v", c.name, got)
		}
	}

	// promt qiymatlari
	a, _ := ParseAgentJSON(`{"promt":4}`)
	if id, ok := a.NextPromt(); !ok || id != 4 {
		t.Errorf("promt 4 kutilgan: %v %v", id, ok)
	}
	a, _ = ParseAgentJSON(`{"promt":null}`)
	if _, ok := a.NextPromt(); ok {
		t.Error("null da zanjir tugashi kerak")
	}

	if _, err := ParseAgentJSON("umuman JSON emas"); err == nil {
		t.Error("xato kutilgan edi")
	}
}

func TestUnansweredClientIDs(t *testing.T) {
	c := func(id int64) Message { return Message{ID: id, SenderType: "client"} }
	a := func(id int64) Message { return Message{ID: id, SenderType: "agent"} }

	cases := []struct {
		name string
		msgs []Message
		want string
	}{
		{"oxirgi xodim javobidan keyingilar", []Message{c(1), a(2), c(3), c(4)}, "3,4"},
		{"xodim javobi yo'q", []Message{c(1), c(2)}, "1,2"},
		{"oxirgi so'z xodimniki", []Message{c(1), a(2)}, ""},
		{"bo'sh", nil, ""},
	}
	for _, tc := range cases {
		if got := JoinIDs(UnansweredClientIDs(tc.msgs)); got != tc.want {
			t.Errorf("%s: %q kutilgan, %q keldi", tc.name, tc.want, got)
		}
	}

	// SplitIDs — JoinIDs ning teskarisi
	if got := JoinIDs(SplitIDs(" 5, 6 ,,7 ")); got != "5,6,7" {
		t.Errorf("SplitIDs: %q", got)
	}
}

func TestFormatTranscript(t *testing.T) {
	// 14 ta xabar — modelga faqat oxirgi 10 tasi ketishi kerak.
	var msgs []Message
	for i := 1; i <= 14; i++ {
		typ := "client"
		if i%2 == 0 {
			typ = "agent"
		}
		msgs = append(msgs, Message{ID: int64(i), SenderType: typ,
			Message: fmt.Sprintf("xabar %d", i), CreatedAt: "2026-08-29T10:00:00Z"})
	}
	// Bo'sh matnli xabar tashlanishi kerak.
	msgs = append(msgs, Message{ID: 15, SenderType: "client", Message: "   "})

	var got []TranscriptMessage
	if err := json.Unmarshal([]byte(formatTranscript(msgs)), &got); err != nil {
		t.Fatal(err)
	}
	// Oxirgi 10 tasi — id 6..15; 15-si bo'sh, ya'ni tashlanadi → 9 ta.
	if len(got) != 9 {
		t.Fatalf("9 ta kutilgan, %d keldi", len(got))
	}
	if got[0].Message != "xabar 6" || got[8].Message != "xabar 14" {
		t.Errorf("oxirgi 10 tasi emas: %s … %s", got[0].Message, got[8].Message)
	}
	if got[0].Type != "agent" || got[1].Type != "client" {
		t.Errorf("type noto'g'ri: %s / %s", got[0].Type, got[1].Type)
	}
}
