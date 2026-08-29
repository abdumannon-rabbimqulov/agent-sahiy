package support

import (
	"encoding/json"
	"fmt"
	"strings"
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

	// promt qiymatlari — model turli shaklda qaytarishi mumkin
	for _, raw := range []string{`{"promt":4}`, `{"promt":"4"}`} {
		a, err := ParseAgentJSON(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if id, ok := a.NextPromt(); !ok || id != 4 {
			t.Errorf("%s: promt 4 kutilgan, %v %v keldi", raw, id, ok)
		}
	}
	for _, raw := range []string{`{"promt":null}`, `{"promt":false}`, `{"promt":""}`, `{"promt":0}`, `{"chat":"x"}`} {
		a, err := ParseAgentJSON(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if _, ok := a.NextPromt(); ok {
			t.Errorf("%s: zanjir tugashi kerak edi", raw)
		}
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

func TestScriptHint(t *testing.T) {
	cl := func(text string) []Message { return []Message{{SenderType: "client", Message: text}} }

	cases := []struct {
		name string
		msgs []Message
		want string // hint ichida bo'lishi kerak bo'lgan so'z
	}{
		{"rus", cl("здравствуйте, мой заказ DG60648223 куплен но не отправлен"), "RUS tilida"},
		{"rus qisqa", cl("DG60607041 что с этим заказом."), "RUS tilida"},
		{"o'zbek kirill", cl("DG60645244 буниси качондан бери КУПЛЕНО да турибди"), "O'ZBEK tilida"},
		{"o'zbek lotin", cl("DG60623437 raqamli zakazim otmen qilinibti"), "LOTIN alifboda"},
	}
	for _, c := range cases {
		if got := scriptHint(c.msgs); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q kutilgan, keldi: %q", c.name, c.want, got)
		}
	}

	// Oxirgi MIJOZ xabari hisobga olinadi, xodimniki emas.
	mix := []Message{
		{SenderType: "client", Message: "здравствуйте, когда придёт заказ"},
		{SenderType: "agent", Message: "tekshiryapmiz"},
	}
	if h := scriptHint(mix); !strings.Contains(h, "RUS tilida") {
		t.Errorf("oxirgi mijoz xabari ruscha edi: %q", h)
	}
	if h := scriptHint(nil); h != "" {
		t.Errorf("bo'sh kutilgan: %q", h)
	}
}
